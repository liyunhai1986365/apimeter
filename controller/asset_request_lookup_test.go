package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func assetLookupTestContext(method, path, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	if method == http.MethodGet {
		c.Params = gin.Params{{Key: "id", Value: "owned"}}
	}
	return c
}

func TestAssetRequestLookupReusesRoutingReadsAndKeepsOwnershipLive(t *testing.T) {
	assetSecurityRouter(t, "http://127.0.0.1:1", "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
	type contextKey struct{}
	c := assetLookupTestContext(http.MethodGet, "/api/assets/owned", "")
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), contextKey{}, "request"))
	channelReads, scopeReads := 0, 0
	requestContexts := true
	db := model.DB
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:asset_routing_reads", func(tx *gorm.DB) {
		isScope := false
		for _, arg := range tx.Statement.Vars {
			if arg == "asset-library-scope-v1" {
				isScope = true
			}
		}
		if tx.Statement.Table == "channels" {
			channelReads++
		} else if isScope {
			scopeReads++
		} else {
			return
		}
		requestContexts = requestContexts && tx.Statement.Context.Value(contextKey{}) == "request"
	}))
	t.Cleanup(func() { db.Callback().Query().Remove("test:asset_routing_reads") })
	a, err := resolveAssetAccessRoute(c, "", "")
	require.NoError(t, err)
	require.NotNil(t, a)
	require.NoError(t, prepareAssetAccess(c, a.channel, a.profile, a.resource))
	require.Equal(t, 1, channelReads)
	require.Equal(t, 1, scopeReads)
	require.True(t, requestContexts, "both routing reads must use the request context")

	// Request-local routing reuse must never become an ownership cache.
	require.NoError(t, model.InvalidateAssetBindings(a.bindings[0], false))
	_, err = resolveAssetAccessRoute(c, "", "")
	require.ErrorIs(t, err, model.ErrAssetNotOwned)
	require.Equal(t, 1, channelReads)
	require.Equal(t, 1, scopeReads)
}

func TestAssetRequestLookupDoesNotMaskChannelFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"database unavailable", errors.New("test database unavailable"), errAssetStateUnavailable},
		{"channel missing", gorm.ErrRecordNotFound, model.ErrAssetNotOwned},
	} {
		for _, video := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/asset", true: "/video"}[video], func(t *testing.T) {
				assetSecurityRouter(t, "http://127.0.0.1:1", "tgxmaas", "seedance-tgxmaas")
				seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
				db := model.DB
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:asset_channel_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "channels" {
						tx.AddError(tc.err)
					}
				}))
				t.Cleanup(func() { db.Callback().Query().Remove("test:asset_channel_failure") })
				var err error
				if video {
					c := assetLookupTestContext(http.MethodPost, "/v1/videos", `{"image_url":"asset://owned"}`)
					err = lockAssetVideoChannel(c, &relaycommon.RelayInfo{UserId: 1, OriginModelName: tgxRegressionModel})
				} else {
					c := assetLookupTestContext(http.MethodGet, "/api/assets/owned", "")
					_, err = resolveAssetAccessRoute(c, "", "")
				}
				require.ErrorIs(t, err, tc.want)
			})
		}
	}
}

func TestAssetRequestScopeCachePreservesEndpointIsolationAndCredentialRotation(t *testing.T) {
	assetSecurityRouter(t, "http://127.0.0.1:1", "youniyouju", "")
	c := assetLookupTestContext(http.MethodGet, "/api/assets/owned", "")
	ch, err := assetChannelForRequest(c, 20)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	originalScope, err := assetAccountScopeForRequest(c, ch, profile)
	require.NoError(t, err)
	ch.Key = "rotated-same-endpoint"
	rotatedScope, err := assetAccountScopeForRequest(c, ch, profile)
	require.NoError(t, err)
	require.Equal(t, originalScope, rotatedScope)

	settings := ch.GetSetting()
	settings.Protocol.AssetLibrary.BaseURL += "/other-endpoint"
	ch.SetSetting(settings)
	otherScope, err := assetAccountScopeForRequest(c, ch, profile)
	require.NoError(t, err)
	require.NotEqual(t, originalScope, otherScope, "a channel ID alone cannot key the scope cache")

	// A new request must read current channel configuration from the database.
	require.NoError(t, model.DB.Save(ch).Error)
	next := assetLookupTestContext(http.MethodGet, "/api/assets/owned", "")
	reloaded, err := assetChannelForRequest(next, 20)
	require.NoError(t, err)
	require.NotSame(t, ch, reloaded)
	newScope, err := assetAccountScopeForRequest(next, reloaded, profile)
	require.NoError(t, err)
	require.Equal(t, otherScope, newScope)
}

func TestAssetRequestLookupHonorsCanceledContext(t *testing.T) {
	assetSecurityRouter(t, "http://127.0.0.1:1", "youniyouju", "")
	c := assetLookupTestContext(http.MethodGet, "/api/assets/owned", "")
	ch, err := assetChannelForRequest(c, 20)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	_, err = assetAccountScopeForRequest(c, ch, profile)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(ctx)
	_, err = assetChannelForRequest(c, 20)
	require.ErrorIs(t, err, errAssetStateUnavailable)
	_, err = assetAccountScopeForRequest(c, ch, profile)
	require.ErrorIs(t, err, errAssetStateUnavailable)
	_, err = model.GetChannelByIdContext(ctx, 20, true)
	require.ErrorIs(t, err, context.Canceled)
}
