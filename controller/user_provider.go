package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type UserOwnedProviderItem struct {
	model.Channel
	Stats model.UserOwnedProviderStats `json:"stats"`
}

func ListUserOwnedProviders(c *gin.Context) {
	userID := c.GetInt("id")
	channels, err := model.ListUserOwnedProviderChannels(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		channelIDs = append(channelIDs, channel.Id)
	}
	statsByChannel, err := model.GetUserOwnedProviderStats(userID, channelIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]UserOwnedProviderItem, 0, len(channels))
	for _, channel := range channels {
		stats := statsByChannel[channel.Id]
		stats.ChannelId = channel.Id
		items = append(items, UserOwnedProviderItem{
			Channel: channel,
			Stats:   stats,
		})
	}
	common.ApiSuccess(c, items)
}

func CreateUserOwnedProvider(c *gin.Context) {
	req := AddChannelRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Channel == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel is required"})
		return
	}
	if req.Mode == "" {
		req.Mode = "single"
	}
	if req.Mode != "single" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "自有供应商暂仅支持单渠道创建"})
		return
	}
	if err := validateChannel(req.Channel, true); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}

	userID := c.GetInt("id")
	channel := req.Channel
	channel.Id = 0
	channel.CreatedTime = common.GetTimestamp()
	channel.Scope = model.ChannelScopeUserOwned
	channel.OwnerUserId = userID
	channel.Group = "pending_user_owned_provider"
	if channel.Status == 0 {
		channel.Status = common.ChannelStatusEnabled
	}
	if channel.RetryEnabled == nil {
		channel.RetryEnabled = common.GetPointer(true)
	}
	if channel.AutoBan == nil {
		channel.AutoBan = common.GetPointer(1)
	}
	if channel.ChannelInfo.MultiKeyMode == "" {
		channel.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
	}

	if err := channel.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	channel.Group = model.BuildUserOwnedProviderGroup(userID, channel.Id)
	if err := model.DB.Model(channel).Select("group", "scope", "owner_user_id").Updates(channel).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	if err := channel.UpdateAbilities(nil); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	channel.Key = ""
	clearChannelInfo(channel)
	common.ApiSuccess(c, channel)
}

func UpdateUserOwnedProvider(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req := AddChannelRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Channel == nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "channel is required"})
		return
	}
	userID := c.GetInt("id")
	origin, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if origin.Scope != model.ChannelScopeUserOwned || origin.OwnerUserId != userID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问该自有供应商"})
		return
	}

	channel := req.Channel
	channel.Id = id
	channel.CreatedTime = origin.CreatedTime
	channel.Scope = model.ChannelScopeUserOwned
	channel.OwnerUserId = userID
	channel.Group = origin.Group
	channel.ChannelInfo = origin.ChannelInfo
	if channel.Key == "" {
		channel.Key = origin.Key
	}
	if err := validateChannel(channel, false); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := channel.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	channel.Key = ""
	clearChannelInfo(channel)
	common.ApiSuccess(c, channel)
}

func DeleteUserOwnedProvider(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userID := c.GetInt("id")
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Scope != model.ChannelScopeUserOwned || channel.OwnerUserId != userID {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "无权访问该自有供应商"})
		return
	}
	if err := channel.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	service.ResetProxyClientCache()
	common.ApiSuccess(c, nil)
}
