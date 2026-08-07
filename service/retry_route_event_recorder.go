package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const ginKeyCurrentRetryRouteEventID = "current_retry_route_event_id"

func BuildRetryRouteEvent(c *gin.Context, decision operation_setting.RetryPolicyDecision, err *types.NewAPIError) *model.RetryRouteEvent {
	if c == nil {
		return &model.RetryRouteEvent{}
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	event := &model.RetryRouteEvent{
		RequestId:         c.GetString(common.RequestIdKey),
		UpstreamRequestId: c.GetString(common.UpstreamRequestIdKey),
		UserId:            common.GetContextKeyInt(c, constant.ContextKeyUserId),
		Username:          common.GetContextKeyString(c, constant.ContextKeyUserName),
		TokenId:           tokenID,
		TokenName:         c.GetString("token_name"),
		OriginalModel:     common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
		UpstreamModel:     c.GetString("upstream_model_name"),
		IsStream:          common.GetContextKeyBool(c, constant.ContextKeyIsStream),
		RuleSource:        decision.Source,
		RuleName:          decision.RuleName,
		Action:            decision.Action,
		Matched:           decision.Matched,
		SourceChannelId:   common.GetContextKeyInt(c, constant.ContextKeyChannelId),
		SourceChannelName: common.GetContextKeyString(c, constant.ContextKeyChannelName),
		SourceChannelType: common.GetContextKeyInt(c, constant.ContextKeyChannelType),
		SourceGroup:       common.GetContextKeyString(c, constant.ContextKeyUserGroup),
	}
	if event.OriginalModel == "" {
		event.OriginalModel = c.GetString("original_model")
	}
	if event.SourceGroup == "" {
		event.SourceGroup = c.GetString("group")
	}
	if c.Request != nil && c.Request.URL != nil {
		event.RequestPath = c.Request.URL.Path
	}
	if retryIndex, ok := c.Get("relay_retry_index"); ok {
		if value, ok := retryIndex.(int); ok {
			event.AttemptIndex = value
		}
	}
	if err != nil {
		event.StatusCode = err.StatusCode
		event.ErrorType = string(err.GetErrorType())
		event.ErrorCode = string(err.GetErrorCode())
		event.ErrorMessage = err.MaskSensitiveErrorWithStatusCode()
	}
	if start := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime); !start.IsZero() {
		event.UseTimeMs = int(time.Since(start).Milliseconds())
	}
	event.Extra = retryRouteDecisionExtra(decision)
	fillRetryRouteEventWorkspace(event, tokenID)
	return event
}

func retryRouteDecisionExtra(decision operation_setting.RetryPolicyDecision) string {
	extra := make(map[string]interface{})
	if decision.SampleRate > 0 {
		extra["sample_rate"] = decision.SampleRate
	}
	if len(extra) == 0 {
		return ""
	}
	data, err := common.Marshal(extra)
	if err != nil {
		return ""
	}
	return string(data)
}

func RecordRetryRouteDecision(c *gin.Context, decision operation_setting.RetryPolicyDecision, err *types.NewAPIError) {
	if c == nil || !decision.Matched {
		return
	}
	event := BuildRetryRouteEvent(c, decision, err)
	if event.RequestId == "" {
		return
	}
	if recordErr := model.RecordRetryRouteEvent(event); recordErr != nil {
		common.SysLog("failed to record retry route event: " + recordErr.Error())
		return
	}
	SetCurrentRetryRouteEventID(c, event.Id)
}

func UpdateCurrentRetryRouteTarget(c *gin.Context, channel *model.Channel, targetGroup string) {
	id, ok := GetCurrentRetryRouteEventID(c)
	if !ok {
		return
	}
	if err := model.UpdateRetryRouteEventTarget(id, channel, targetGroup); err != nil {
		common.SysLog("failed to update retry route event target: " + err.Error())
	}
}

func MarkRetryRouteFinal(c *gin.Context, success bool, status string) {
	if c == nil {
		return
	}
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		return
	}
	if err := model.MarkRetryRouteEventsFinal(requestID, success, status); err != nil {
		common.SysLog("failed to mark retry route events final: " + err.Error())
	}
}

func SetCurrentRetryRouteEventID(c *gin.Context, id int) {
	if c == nil || id <= 0 {
		return
	}
	c.Set(ginKeyCurrentRetryRouteEventID, id)
}

func GetCurrentRetryRouteEventID(c *gin.Context) (int, bool) {
	if c == nil {
		return 0, false
	}
	raw, ok := c.Get(ginKeyCurrentRetryRouteEventID)
	if !ok {
		return 0, false
	}
	id, ok := raw.(int)
	return id, ok && id > 0
}

func fillRetryRouteEventWorkspace(event *model.RetryRouteEvent, tokenID int) {
	if event == nil || tokenID <= 0 || model.DB == nil {
		return
	}
	var token model.Token
	if err := model.DB.Select("id", "workspace_id").Where("id = ?", tokenID).First(&token).Error; err != nil {
		return
	}
	event.WorkspaceId = token.WorkspaceId
	if token.WorkspaceId <= 0 {
		return
	}
	var workspace model.Workspace
	if err := model.DB.Select("id", "name").Where("id = ?", token.WorkspaceId).First(&workspace).Error; err != nil {
		return
	}
	event.WorkspaceName = workspace.Name
}
