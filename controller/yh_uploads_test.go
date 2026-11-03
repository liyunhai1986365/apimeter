package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHUploadRejectsMissingOrUnownedGroup(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/assets/groups" {
			t.Errorf("unexpected upstream request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"success","data":{"group_id":42}}`))
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL)
	w := yhAction(r, "CreateAssetGroup", `{"Name":"owned"}`, 1)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	before := calls.Load()
	for _, mode := range []struct{ name, input string }{
		{"single", `"url":"https://example.com/a.png","upload_mode":"async"`},
		{"batch", `"urls":["https://example.com/a.png","https://example.com/b.png"]`},
	} {
		for _, tc := range []struct {
			name, group  string
			user, status int
		}{
			{"missing", "", 1, http.StatusBadRequest},
			{"empty", `"group_id":"",`, 1, http.StatusBadRequest},
			{"unknown", `"group_id":"987654321",`, 1, http.StatusNotFound},
			{"another_user", `"group_id":"42",`, 2, http.StatusNotFound},
		} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				var response *httptest.ResponseRecorder
				require.NotPanics(t, func() {
					response = seedanceCall(r, http.MethodPost, "/v1/yh/assets/uploads", fmt.Sprintf("{%s%s}", tc.group, mode.input), tc.user)
				})
				require.Equal(t, tc.status, response.Code, response.Body.String())
				if tc.status == http.StatusNotFound {
					require.Equal(t, "asset_not_found", gjson.GetBytes(response.Body.Bytes(), "error.code").String())
				} else {
					require.Contains(t, response.Body.String(), "group_id")
				}
				require.Equal(t, before, calls.Load(), "invalid uploads must not reach upstream")
			})
		}
	}
}

func TestYHUploadHandlesAndPartialFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		body, _ := io.ReadAll(r.Body)
		var response string
		switch r.URL.Path {
		case "/v1/assets/groups":
			response = `{"code":"success","data":{"group_id":42}}`
		case "/v1/assets":
			require.Equal(t, "42", gjson.GetBytes(body, "group_id").Raw)
			if gjson.GetBytes(body, "urls").Exists() {
				response = `{"code":"success","data":{"batch_id":"task-batch","items":[{"id":"task-batch","status":"Processing"},{"id":"task-batch","status":"Processing"}]}}`
			} else {
				response = `{"code":"success","data":{"id":"task-single","status":"Processing"}}`
			}
		case "/v1/assets/batch/get":
			require.JSONEq(t, `{"batch_id":"task-batch"}`, string(body))
			response = `{"code":"success","data":{"items":[{"id":"asset-ready","status":"Active","group_id":42},{"status":"Failed","error":"invalid URL"}]}}`
		case "/v1/assets/get":
			if gjson.GetBytes(body, "id").String() == "task-single" {
				response = `{"code":"success","data":{"id":"task-single","real_id":"asset-single","status":"Active","group_id":42}}`
			} else {
				response = `{"code":"success","data":{"id":"asset-ready","status":"Active","group_id":42}}`
			}
		case "/v1/assets/groups/delete":
			response = `{"code":"success"}`
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(500)
			response = `{}`
		}
		_, _ = w.Write([]byte(response))
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL)
	w := yhAction(r, "CreateAssetGroup", `{"Name":"batch"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	for _, body := range []string{`{"group_id":"42","urls":["https://example.com/a.png","https://example.com/b.png"]}`, `{"group_id":"42","url":"https://example.com/a.png","upload_mode":"async"}`} {
		w = seedanceCall(r, "POST", "/v1/yh/assets/uploads", body, 1)
		require.Equal(t, 200, w.Code, w.Body.String())
		id := gjson.GetBytes(w.Body.Bytes(), "Id").String()
		before := calls.Load()
		require.Equal(t, 404, yhAction(r, "GetAsset", `{"Id":"`+id+`"}`, 1).Code, "upload handle is not an asset")
		require.Equal(t, 404, seedanceCall(r, "GET", "/v1/yh/assets/uploads/"+id, "", 2).Code)
		require.Equal(t, before, calls.Load())
		w = seedanceCall(r, "GET", "/v1/yh/assets/uploads/"+id, "", 1)
		require.Equal(t, 200, w.Code, w.Body.String())
		if id == "task-batch" {
			require.Equal(t, "Failed", gjson.GetBytes(w.Body.Bytes(), "Status").String())
			require.Equal(t, "asset-ready", gjson.GetBytes(w.Body.Bytes(), "Items.0.Id").String())
			require.Equal(t, "invalid URL", gjson.GetBytes(w.Body.Bytes(), "Items.1.Error").String())
		} else {
			require.Equal(t, "asset-single", gjson.GetBytes(w.Body.Bytes(), "Items.0.Id").String())
		}
	}
	w = yhAction(r, "GetAsset", `{"Id":"asset-ready"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = yhAction(r, "DeleteAssetGroup", `{"Id":"42"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	before := calls.Load()
	require.Equal(t, 404, seedanceCall(r, "GET", "/v1/yh/assets/uploads/task-batch", "", 1).Code)
	require.Equal(t, before, calls.Load())
}
