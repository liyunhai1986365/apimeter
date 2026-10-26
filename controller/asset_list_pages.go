package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/inflight"
	"github.com/tidwall/gjson"
)

type assetListRow struct {
	id, raw string
}

type parsedAssetListPage struct {
	assetListPage
	path  string
	items gjson.Result
	rows  []assetListRow
}

func parseAssetListPage(resp *http.Response, body []byte, kind string) (parsedAssetListPage, error) {
	p := parsedAssetListPage{assetListPage: assetListPage{resp, body}}
	if resp.StatusCode >= 400 || !assetResponseSuccessful(body) {
		return p, nil
	}
	var err error
	p.path, p.items, err = assetListItems(body)
	if err != nil {
		return p, err
	}
	keys := []string{"Id", "id", "asset_id", "AssetId"}
	if kind != "asset" {
		keys = append(keys, "GroupId", "group_id")
	}
	for _, row := range p.items.Array() {
		ids := assetResponseStrings([]byte(row.Raw), keys...)
		id := ""
		if len(ids) > 0 {
			id = ids[0]
		}
		p.rows = append(p.rows, assetListRow{id, row.Raw})
	}
	return p, nil
}

// Include the first page in scan identity without including per-request
// metadata (such as RequestId), which would defeat concurrent sharing.
func assetListFirstPageFingerprint(body []byte) (string, error) {
	_, items, err := assetListItems(body)
	if err != nil {
		return "", err
	}
	parts := []string{items.Raw}
	for _, path := range []string{"Result.NextToken", "NextToken", "Result.TotalCount", "data.total", "data.total_count", "total", "total_count", "TotalCount"} {
		parts = append(parts, gjson.GetBytes(body, path).Raw)
	}
	data, err := common.Marshal(parts)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

type assetListPageSet struct {
	key     string
	readers int
	bytes   int
	expires time.Time
	started bool
	pages   map[string]parsedAssetListPage
	reads   inflight.Group[parsedAssetListPage]
}

// Retain raw pages only while at least one matching list scan is running.
// Readers at different speeds can replay pages without refetching/parsing them.
// This is not an ownership cache. Every scan independently filters by its own
// bindings and revalidates its output. A global payload budget bounds retention;
// over-budget pages still work through ordinary in-flight coalescing.
// Freeze membership before the first subsequent page fetch. A later scan has
// already fetched its own fresh first page and must not replay older pages.
var assetListPageSets = struct {
	sync.Mutex
	sets  map[string]*assetListPageSet
	bytes int
}{sets: make(map[string]*assetListPageSet)}

const assetListPageBufferLimit = 64 << 20

func acquireAssetListPages(key string) *assetListPageSet {
	assetListPageSets.Lock()
	defer assetListPageSets.Unlock()
	s := assetListPageSets.sets[key]
	if s == nil || s.started || time.Now().After(s.expires) {
		s = &assetListPageSet{key: key, pages: make(map[string]parsedAssetListPage), expires: time.Now().Add(assetListScanTimeout)}
		assetListPageSets.sets[key] = s
	}
	s.readers++
	return s
}

func (s *assetListPageSet) release() {
	assetListPageSets.Lock()
	defer assetListPageSets.Unlock()
	s.readers--
	if s.readers == 0 {
		if assetListPageSets.sets[s.key] == s {
			delete(assetListPageSets.sets, s.key)
		}
		assetListPageSets.bytes -= s.bytes
		s.bytes, s.pages = 0, nil
	}
}

func (s *assetListPageSet) cached(key string) (parsedAssetListPage, bool) {
	assetListPageSets.Lock()
	defer assetListPageSets.Unlock()
	p, ok := s.pages[key]
	return p, ok
}

func (s *assetListPageSet) read(client *http.Client, req *http.Request, kind string) (parsedAssetListPage, error) {
	if s != nil {
		assetListPageSets.Lock()
		s.started = true
		assetListPageSets.Unlock()
	}
	read := func(ctx context.Context) (parsedAssetListPage, error) {
		// Separate generations even when a previous fetch is still in flight.
		resp, body, err := readAssetListPageInScope(client, req.Clone(ctx), fmt.Sprintf("%p", s))
		if err != nil {
			return parsedAssetListPage{}, err
		}
		return parseAssetListPage(resp, body, kind)
	}
	if s == nil || client.Jar != nil || (req.Body != nil && req.GetBody == nil) {
		return read(req.Context())
	}
	var requestBody []byte
	if req.GetBody != nil {
		reader, err := req.GetBody()
		if err != nil {
			return parsedAssetListPage{}, err
		}
		requestBody, err = io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return parsedAssetListPage{}, err
		}
	}
	data, err := common.Marshal([]any{fmt.Sprintf("%p", client), kind, req.Method, req.URL.String(), req.Host, req.Header, string(requestBody)})
	if err != nil {
		return parsedAssetListPage{}, err
	}
	key := fmt.Sprintf("%x", sha256.Sum256(data))
	if p, ok := s.cached(key); ok {
		return p, req.Context().Err()
	}
	return s.reads.Do(req.Context(), key, func(ctx context.Context) (parsedAssetListPage, error) {
		if p, ok := s.cached(key); ok {
			return p, nil
		}
		p, err := read(ctx)
		if err != nil || p.response.StatusCode >= 400 || !assetResponseSuccessful(p.body) {
			return p, err
		}
		// gjson rows reference the items string; count both it and the body,
		// plus row/header/key overhead, towards the retained payload budget.
		size := len(p.body) + len(p.items.Raw) + len(p.rows)*128 + 4096
		assetListPageSets.Lock()
		if s.readers > 0 && assetListPageSets.bytes+size <= assetListPageBufferLimit {
			s.pages[key] = p
			s.bytes += size
			assetListPageSets.bytes += size
		}
		assetListPageSets.Unlock()
		return p, nil
	})
}
