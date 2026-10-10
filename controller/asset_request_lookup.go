package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const assetRequestLookupContextKey = "asset_request_lookup"

type assetScopeCacheKey struct {
	channelID   int
	endpointKey string
}

// Keep routing configuration consistent within one request. Ownership and
// revocation are deliberately not cached here and remain live database reads.
type assetRequestLookup struct {
	channels map[int]*model.Channel
	scopes   map[assetScopeCacheKey]string
}

func assetLookupsForRequest(c *gin.Context) *assetRequestLookup {
	if value, ok := c.Get(assetRequestLookupContextKey); ok {
		return value.(*assetRequestLookup)
	}
	lookup := &assetRequestLookup{
		channels: make(map[int]*model.Channel),
		scopes:   make(map[assetScopeCacheKey]string),
	}
	c.Set(assetRequestLookupContextKey, lookup)
	return lookup
}

func assetChannelForRequest(c *gin.Context, id int) (*model.Channel, error) {
	if c.Request.Context().Err() != nil {
		return nil, errAssetStateUnavailable
	}
	lookup := assetLookupsForRequest(c)
	if channel := lookup.channels[id]; channel != nil {
		return channel, nil
	}
	channel, err := model.GetChannelByIdContext(c.Request.Context(), id, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, model.ErrAssetNotOwned
		}
		common.SysError("read asset channel: " + err.Error())
		return nil, errAssetStateUnavailable
	}
	lookup.channels[id] = channel
	return channel, nil
}

func assetAccountScopeForRequest(c *gin.Context, ch *model.Channel, profile *configurable.Profile) (string, error) {
	return resolveAssetAccountScope(c.Request.Context(), ch, profile, assetLookupsForRequest(c).scopes)
}
