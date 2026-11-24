package service

import (
	"context"
	"crypto/hmac"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func isSiteImageID(name string) bool {
	ext := path.Ext(name)
	id, err := uuid.Parse(strings.TrimSuffix(name, ext))
	return err == nil && id.Version() == 4 && name == id.String()+relayTempImageExt(name)
}

func openSiteImage(name string) (*os.File, os.FileInfo, error) {
	if !isSiteImageID(name) {
		return nil, nil, os.ErrNotExist
	}
	root, err := os.OpenRoot(relayTempImageDir())
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, nil, os.ErrNotExist
	}
	return f, st, nil
}

// Match the same URL the storage writer emits. Never pass a caller's host,
// path or extra query parameters to an operator-trusted HTTP client.
func downloadSiteImage(ctx context.Context, raw string) (*http.Response, bool, error) {
	u, err := url.Parse(raw)
	relative := strings.HasPrefix(raw, relayTempImageRoutePrefix)
	if err != nil || (!relative && ((u.Scheme != "http" && u.Scheme != "https") || u.Host == "")) || u.User != nil || u.Fragment != "" || u.RawPath != "" {
		return nil, false, nil
	}
	id := path.Base(u.Path)
	if !isSiteImageID(id) {
		return nil, false, nil
	}
	configuredBase := strings.TrimRight(strings.TrimSpace(os.Getenv("API_TEMP_IMAGE_PUBLIC_BASE_URL")), "/")
	localBase := configuredBase
	// Without a configured base, the writer uses the incoming request's host.
	// Recognize an existing public image by ID; never dial that host to read it.
	if localBase == "" && !relative {
		localBase = u.Scheme + "://" + u.Host
	}
	if raw == localBase+relayTempImageRoutePrefix+id {
		if err := ctx.Err(); err != nil {
			return nil, true, err
		}
		f, st, err := openSiteImage(id)
		if err != nil {
			if !relative && configuredBase == "" && os.IsNotExist(err) {
				// Another gateway may use the same route. Its URL still
				// goes through the ordinary public-only transport.
				return nil, false, nil
			}
			return nil, true, err
		}
		h := make(http.Header)
		h.Set("Content-Type", tempImageContentTypeForFilename(id))
		h.Set("Content-Length", strconv.FormatInt(st.Size(), 10))
		return &http.Response{StatusCode: http.StatusOK, Header: h, Body: f, ContentLength: st.Size()}, true, nil
	}

	date := path.Base(path.Dir(u.Path))
	if _, err := time.Parse("20060102", date); err != nil {
		return nil, false, nil
	}
	cfg, err := tempImageStorageConfigFromEnv(nil) // operator configuration only
	if err != nil || (cfg.Storage != "oss" && cfg.Storage != "r2") {
		return nil, false, nil
	}
	object := cfg.OSSObjectPrefix + "/" + date + "/" + id
	var target string
	if cfg.OSSPublicBaseURL != "" {
		target = cfg.OSSPublicBaseURL + "/" + escapeOSSObjectName(object)
	} else if cfg.Storage == "oss" {
		endpoint, resource, err := tempImageOSSObjectURL(cfg, object)
		if err != nil {
			return nil, false, nil
		}
		expText := u.Query().Get("Expires")
		exp, err := strconv.ParseInt(expText, 10, 64)
		if err != nil || exp <= time.Now().Unix() {
			return nil, false, nil
		}
		q := url.Values{"Expires": {expText}, "OSSAccessKeyId": {cfg.OSSAccessKeyID}, "Signature": {ossSignature(cfg.OSSAccessKeySecret, "GET\n\n\n"+expText+"\n"+resource)}}
		target = endpoint + "?" + q.Encode()
	} else {
		endpoint, err := tempImageR2ObjectURL(cfg, object)
		if err != nil {
			return nil, false, nil
		}
		signed, err := time.Parse("20060102T150405Z", u.Query().Get("X-Amz-Date"))
		if err != nil {
			return nil, false, nil
		}
		ttl, err := strconv.ParseInt(u.Query().Get("X-Amz-Expires"), 10, 32)
		now := time.Now()
		if err != nil || ttl <= 0 || signed.After(now) || !signed.Add(time.Duration(ttl)*time.Second).After(now) {
			return nil, false, nil
		}
		r, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, false, nil
		}
		target = presignR2RequestAt(r, cfg, time.Duration(ttl)*time.Second, signed)
	}
	if !hmac.Equal([]byte(target), []byte(raw)) {
		return nil, false, nil
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, true, err
	}
	client := WithoutRedirects(cfg.httpClient())
	if client.Timeout == 0 {
		client.Timeout = 2 * time.Minute
	}
	resp, err := client.Do(r)
	if err != nil {
		if ctx.Err() != nil {
			return nil, true, ctx.Err()
		}
		return nil, true, errors.New("site image storage unavailable")
	}
	return resp, true, nil
}
