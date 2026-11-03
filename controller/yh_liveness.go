package controller

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/inflight"
	"github.com/tidwall/gjson"
)

type yhGroupSnapshot struct {
	status  int
	header  http.Header
	body    []byte
	expires time.Time
}

var yhGroupReads inflight.Group[yhGroupSnapshot]
var yhGroupCache = struct {
	sync.Mutex
	entries map[string]yhGroupSnapshot
}{entries: make(map[string]yhGroupSnapshot)}

// The mutex protects only the small cache map. Network I/O and waits never
// retain it or a database transaction. Each caller still checks live ownership.
func readYHGroups(client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%p\x00%s\x00%s", client, req.URL.String(), req.Header.Get("Authorization")))))
	lookup := func() (yhGroupSnapshot, bool) {
		yhGroupCache.Lock()
		defer yhGroupCache.Unlock()
		v, ok := yhGroupCache.entries[key]
		return v, ok && time.Now().Before(v.expires)
	}
	v, ok := lookup()
	if !ok {
		var err error
		v, err = yhGroupReads.Do(req.Context(), key, func(ctx context.Context) (yhGroupSnapshot, error) {
			if v, ok := lookup(); ok {
				return v, nil
			}
			ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			resp, body, err := readYHResponse(client, req.Clone(ctx))
			if err != nil {
				return yhGroupSnapshot{}, err
			}
			v := yhGroupSnapshot{status: resp.StatusCode, header: resp.Header.Clone(), body: body, expires: time.Now().Add(time.Second)}
			if !yhFailure(resp, body) && len(body) <= 128<<10 {
				yhGroupCache.Lock()
				for k, entry := range yhGroupCache.entries {
					if time.Now().After(entry.expires) {
						delete(yhGroupCache.entries, k)
					}
				}
				if len(yhGroupCache.entries) < 64 {
					yhGroupCache.entries[key] = v
				}
				yhGroupCache.Unlock()
			}
			return v, nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return yhHTTPResponse(req, v.status, v.body, v.header), v.body, nil
}

func (s yhAssetRequest) completedSession(req *http.Request) (*http.Response, error) {
	groups, err := model.FindScopedAssetBindingsContext(req.Context(), s.binding.ChannelID, s.binding.UserID, s.binding.Scope, "group", s.session.SessionGroupID)
	if errors.Is(err, model.ErrAssetNotOwned) {
		return yhError(req, 404, "NotFound", "authenticated group is unavailable"), nil
	}
	if err != nil {
		return nil, err
	}
	if len(groups) != 1 {
		return yhError(req, 404, "NotFound", "authenticated group is unavailable"), nil
	}
	body, err := common.Marshal(map[string]string{"GroupId": s.session.SessionGroupID, "GroupType": "LivenessFace"})
	if err != nil {
		return nil, err
	}
	return yhHTTPResponse(req, http.StatusOK, body, nil), nil
}

func (s yhAssetRequest) completeSession(req *http.Request, data gjson.Result) (*http.Response, error) {
	items := data.Get("items")
	if !items.IsArray() {
		return nil, fmt.Errorf("invalid YH real-person group list")
	}
	var matches []gjson.Result
	for _, item := range items.Array() {
		if item.Get("artist_name").String() == s.session.SessionName {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		resp := yhError(req, http.StatusConflict, "ValidationPending", "authentication has not produced a visible group; query again later")
		resp.Header.Set("Retry-After", "2")
		return resp, nil
	}
	if len(matches) != 1 {
		return yhError(req, http.StatusConflict, "ValidationConflict", "authentication matched multiple groups"), nil
	}
	item := matches[0]
	at, err := time.Parse(time.RFC3339, item.Get("authorized_at").String())
	if err != nil || at.Unix() < s.session.SessionCreatedAt-60 || at.Unix() > s.session.SessionCreatedAt+180 {
		return yhError(req, http.StatusConflict, "ValidationConflict", "authentication group timestamp cannot be associated with this session"), nil
	}
	id, err := yhGroupID(item.Get("id"))
	if err != nil {
		return nil, err
	}
	group := s.binding
	group.Kind = "group"
	group.ID = id
	group.CanonicalID = id
	group.GroupID = ""
	group.GroupType = "LivenessFace"
	s.session.SessionGroupID = id
	bindings := []model.AssetBinding{s.session, group}
	if alias := item.Get("group_id").String(); alias != "" {
		if !validAssetHandle(alias) {
			return nil, fmt.Errorf("invalid YH real-person group alias")
		}
		group.ID = alias
		bindings = append(bindings, group)
	}
	// A single bounded ownership transaction atomically completes the session
	// and records both IDs. Concurrent completion cannot pick a different group.
	if err := model.SaveAssetBindingsContext(req.Context(), bindings); err != nil {
		return nil, err
	}
	return s.completedSession(req)
}
