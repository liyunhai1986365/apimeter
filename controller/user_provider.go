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
	if err := service.ValidateUserProvider(req.Channel); err != nil {
		common.ApiError(c, err)
		return
	}

	userID := c.GetInt("id")
	channel := req.Channel
	channel.Id = 0
	channel.CreatedTime = common.GetTimestamp()
	channel.Scope = model.ChannelScopeUserOwned
	channel.OwnerUserId = userID
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

	if err := model.InsertUserOwnedProvider(channel); err != nil {
		common.ApiError(c, err)
		return
	}
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
	channel.Group = "" // The generated group is immutable; omit it from this partial update.
	updateKeys := channel.Key != "" && channel.Key != origin.Key
	if updateKeys {
		channel.ChannelInfo = origin.ChannelInfo
	} else {
		channel.Key = ""
	}
	// Struct updates omit zero values. Never write status from an owner edit,
	// including when an administrator disables the channel during this request.
	channel.Status = 0
	if err := validateChannel(channel, false); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	// Partial edits retain the stored network destination and type.
	check := *channel
	if check.Type == 0 {
		check.Type = origin.Type
	}
	if check.BaseURL == nil {
		check.BaseURL = origin.BaseURL
	}
	if check.Setting == nil {
		check.Setting = origin.Setting
	}
	if check.HeaderOverride == nil {
		check.HeaderOverride = origin.HeaderOverride
	}
	if check.ParamOverride == nil {
		check.ParamOverride = origin.ParamOverride
	}
	if err := service.ValidateUserProvider(&check); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := channel.UpdateWithSnapshot(origin, updateKeys); err != nil {
		common.ApiError(c, err)
		return
	}
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
	if err := model.DeleteUserOwnedProvider(userID, id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
