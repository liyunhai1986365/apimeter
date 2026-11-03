package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func lockYHDraftVideoChannel(c *gin.Context, info *relaycommon.RelayInfo, body []byte) error {
	if info == nil {
		return nil
	}
	content := gjson.GetBytes(body, "content")
	if !content.Exists() {
		metadata := gjson.GetBytes(body, "metadata")
		if metadata.Type == gjson.String {
			metadata = gjson.Parse(metadata.String())
		}
		content = metadata.Get("content")
	}
	var id string
	for _, item := range content.Array() {
		if item.Get("type").String() != "draft_task" {
			continue
		}
		value := item.Get("draft_task.id")
		if value.Type != gjson.String || !validAssetHandle(value.String()) || id != "" {
			return fmt.Errorf("one valid draft task ID is required")
		}
		id = value.String()
	}
	if id == "" {
		return nil
	}
	// ChannelMeta is initialized later by RelayTaskSubmit. The distributor's
	// context already identifies its candidate, so resolve it only for drafts.
	selectedYH := false
	if channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId); channelID > 0 {
		selected, err := assetChannelForRequest(c, channelID)
		if err != nil {
			return err
		}
		p := selected.GetSetting().Protocol
		selectedYH = p != nil && p.ProfileID == "seedance-yh"
	}
	task, exists, err := model.GetByTaskIDOrUpstreamID(info.UserId, id)
	if err != nil {
		return err
	}
	if !exists {
		if !selectedYH {
			return nil
		}
		return model.ErrAssetNotOwned
	}
	ch, err := assetChannelForRequest(c, task.ChannelId)
	if err != nil {
		return err
	}
	originalYH := ch.GetSetting().Protocol != nil && ch.GetSetting().Protocol.ProfileID == "seedance-yh"
	if !selectedYH && !originalYH {
		return nil
	}
	if task.Status != model.TaskStatusSuccess || !task.PrivateData.SeedanceDraft {
		return model.ErrAssetNotOwned
	}
	if ch.Status != common.ChannelStatusEnabled || ch.GetSetting().Protocol == nil || ch.GetSetting().Protocol.ProfileID != "seedance-yh" || ch.ChannelInfo.IsMultiKey || task.PrivateData.Key == "" || task.PrivateData.Key != ch.Key {
		return model.ErrAssetNotOwned
	}
	if raw, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
		selected, err := parseSpecificChannelID(raw)
		if err != nil || selected != ch.Id {
			return model.ErrAssetNotOwned
		}
	}
	filter := service.BuildProtocolChannelFilter(&service.RetryParam{Ctx: c, ModelName: info.OriginModelName})
	if filter != nil && !filter(ch) {
		return model.ErrAssetNotOwned
	}
	// Pin the draft's channel without changing the group authorized by the
	// distributor. AutoGroup contains the concrete selection for auto policies.
	group := common.GetContextKeyString(c, constant.ContextKeyAutoGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if group == "" || group == service.AutoGroupName || !configurableResourceChannelAbilityEnabled(ch, group, info.OriginModelName) {
		return model.ErrAssetNotOwned
	}
	if locked, ok := info.LockedChannel.(*model.Channel); ok && locked != nil && locked.Id != ch.Id {
		return model.ErrAssetNotOwned
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
	if err := middleware.SetupContextForSelectedChannel(c, ch, info.OriginModelName); err != nil {
		return err.Err
	}
	info.InitChannelMeta(c)
	info.UsingGroup = group
	info.LockedChannel = ch
	return nil
}
