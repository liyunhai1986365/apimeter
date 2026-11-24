package service

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestPersonalResponseLimit(t *testing.T) {
	block := make([]byte, 32<<10)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dst io.Writer = w
		n := maxUserUpstreamResponseBytes
		switch r.URL.Path {
		case "/declared":
			w.Header().Set("Content-Length", strconv.Itoa(n+1))
			return
		case "/small":
			_, _ = w.Write([]byte("ok"))
			return
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			defer zw.Close()
			dst = zw
			n++
		case "/chunked":
			n++
		}
		for n > 0 {
			k := min(n, len(block))
			if _, err := dst.Write(block[:k]); err != nil {
				return
			}
			n -= k
		}
	}))
	defer server.Close()
	local := server.Client().Transport.(*http.Transport).Clone()
	local.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	defer local.CloseIdleConnections()
	client := &http.Client{Transport: userEgressTransport{local}}
	for _, tc := range []struct {
		path    string
		count   int64
		exceeds bool
	}{{"small", 2, false}, {"exact", maxUserUpstreamResponseBytes, false}, {"declared", 0, true}, {"chunked", maxUserUpstreamResponseBytes, true}, {"gzip", maxUserUpstreamResponseBytes, true}} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := client.Get("https://example.com/" + tc.path)
			var n int64
			if tc.path != "declared" {
				require.NoError(t, err)
				defer resp.Body.Close()
				if tc.path == "gzip" {
					require.True(t, resp.Uncompressed)
				}
				n, err = io.Copy(io.Discard, resp.Body)
			} else {
				require.Nil(t, resp)
			}
			require.Equal(t, tc.count, n)
			if tc.exceeds {
				var limit *http.MaxBytesError
				require.True(t, errors.As(err, &limit), "unexpected error: %v", err)
				require.EqualValues(t, maxUserUpstreamResponseBytes, limit.Limit)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPersonalChannelNetworkBoundary(t *testing.T) {
	configureSSRFTestFetchSetting(t)
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	system_setting.GetFetchSetting().AllowPrivateIp = true
	owned := &model.Channel{Scope: model.ChannelScopeUserOwned, Type: constant.ChannelTypeOpenAI}
	client, err := GetHttpClientWithProxySettings("", owned.GetSetting())
	require.NoError(t, err)
	require.Same(t, userEgressClient, client)
	for _, raw := range []string{"http://127.0.0.1:888", "https://127.0.0.1", "https://[::1]", "https://169.254.169.254", "https://private.localhost", "https://api.example.org:9999"} {
		owned.BaseURL = &raw
		require.Error(t, ValidateUserProvider(owned), raw)
		req, err := http.NewRequest(http.MethodPost, raw, nil)
		require.NoError(t, err)
		_, err = client.Transport.RoundTrip(req)
		require.Error(t, err, raw)
	}
	_, err = GetHttpClientWithProxySettings("http://proxy.example.org", owned.GetSetting())
	require.Error(t, err)
	require.ErrorIs(t, client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	owned.BaseURL = common.GetPointer("https://api.example.org")
	require.NoError(t, ValidateUserProvider(owned))
	req, err := http.NewRequest(http.MethodPost, *owned.BaseURL, nil)
	require.NoError(t, err)
	req.Host = "internal.example"
	_, err = client.Transport.RoundTrip(req)
	require.ErrorContains(t, err, "host override")
	owned.Setting = common.GetPointer(`{"thinking_to_content":true,"proxy":""}`)
	require.NoError(t, ValidateUserProvider(owned))
	owned.Setting = common.GetPointer(`{"proxy":"http://proxy.example"}`)
	require.Error(t, ValidateUserProvider(owned))
	owned.Setting = nil
	platform := *owned
	platform.Scope = model.ChannelScopePlatform
	trusted, err := GetHttpClientWithProxySettings("", platform.GetSetting())
	require.NoError(t, err)
	require.NotSame(t, client, trusted)
}

func TestHistoricalPersonalChannelPollingPreservesTasks(t *testing.T) {
	truncate(t)
	const channel = 934
	seedChannel(t, channel)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel).Update("scope", model.ChannelScopeUserOwned).Error)
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })
	task := makeTask(1, channel, 100, 1, BillingSourceWallet, 0)
	require.NoError(t, model.DB.Create(task).Error)
	ids, tasks := []string{"pending"}, map[string]*model.Task{"pending": task}
	require.ErrorIs(t, updateSunoTasks(t.Context(), channel, ids, tasks), model.ErrPlatformChannelRequired)
	require.ErrorIs(t, updateVideoTasks(t.Context(), "video", channel, ids, tasks), model.ErrPlatformChannelRequired)
	require.ErrorIs(t, updateImageTasks(t.Context(), channel, ids, tasks), model.ErrPlatformChannelRequired)
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Equal(t, task.Status, saved.Status)
	require.Equal(t, task.Quota, saved.Quota)
}

func TestPersonalEgressPinsPublicIP(t *testing.T) {
	d := userDialer
	addresses := staticSSRFResolver{"upstream.example": {{IP: net.ParseIP("8.8.8.8")}}}
	d.resolver = addresses
	var calls []string
	d.dialContext = func(_ context.Context, _, addr string) (net.Conn, error) {
		calls = append(calls, addr)
		return testConn(t), nil
	}
	_, err := d.DialContext(context.Background(), "tcp", "upstream.example:443")
	require.NoError(t, err)
	require.Equal(t, []string{"8.8.8.8:443"}, calls)
	addresses["upstream.example"] = append(addresses["upstream.example"], net.IPAddr{IP: net.ParseIP("10.0.0.1")})
	_, err = d.DialContext(context.Background(), "tcp", "upstream.example:443")
	require.Error(t, err)
	require.Len(t, calls, 1)
}
