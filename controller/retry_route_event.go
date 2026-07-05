package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type retryRouteRuleTestRequest struct {
	Input retryRouteRuleTestInput             `json:"input"`
	Rules []operation_setting.RetryPolicyRule `json:"rules"`
}

type retryRouteRuleTestInput struct {
	ModelName    string          `json:"model_name"`
	Group        string          `json:"group"`
	RequestPath  string          `json:"request_path"`
	IsStream     bool            `json:"is_stream"`
	TokenID      int             `json:"token_id"`
	WorkspaceID  int             `json:"workspace_id"`
	ChannelID    int             `json:"channel_id"`
	ChannelType  int             `json:"channel_type"`
	ErrorType    types.ErrorType `json:"error_type"`
	ErrorCode    types.ErrorCode `json:"error_code"`
	StatusCode   int             `json:"status_code"`
	ErrorMessage string          `json:"error_message"`
}

type retryRouteRuleTestDecision struct {
	Matched          bool                                 `json:"matched"`
	ShouldRetry      bool                                 `json:"should_retry"`
	Action           string                               `json:"action"`
	Source           string                               `json:"source"`
	RuleName         string                               `json:"rule_name"`
	RetryGroups      []string                             `json:"retry_groups"`
	MaxRetries       int                                  `json:"max_retries"`
	Targets          operation_setting.RetryPolicyTargets `json:"targets"`
	TargetChannelIDs []int                                `json:"target_channel_ids"`
	TargetTags       []string                             `json:"target_tags"`
	TargetModel      string                               `json:"target_model"`
	ExcludeChannelID int                                  `json:"exclude_channel_id"`
}

type retryRouteRuleStatsResponse struct {
	Items []model.RetryRouteRuleStat `json:"items"`
}

func GetRetryRouteEvents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	events, total, err := model.GetRetryRouteEvents(parseRetryRouteEventQuery(c, pageInfo))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(events)
	common.ApiSuccess(c, pageInfo)
}

func GetRetryRouteEvent(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	event, err := model.GetRetryRouteEventByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, event)
}

func GetRetryRouteEventStats(c *gin.Context) {
	stats, err := model.GetRetryRouteEventStats(parseRetryRouteEventQuery(c, common.GetPageQuery(c)))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, stats)
}

func GetRetryRouteRuleStats(c *gin.Context) {
	stats, err := model.GetRetryRouteRuleStats(parseRetryRouteEventQuery(c, common.GetPageQuery(c)))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, retryRouteRuleStatsResponse{Items: stats})
}

func TestRetryRouteRules(c *gin.Context) {
	var req retryRouteRuleTestRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	decision, err := operation_setting.TestRetryPolicyRules(retryRouteRuleInputToPolicyInput(req.Input), req.Rules)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiSuccess(c, retryRouteDecisionResponse(decision))
}

func parseRetryRouteEventQuery(c *gin.Context, pageInfo *common.PageInfo) model.RetryRouteEventQuery {
	startTimestamp, _ := strconv.ParseInt(strings.TrimSpace(c.Query("start_timestamp")), 10, 64)
	endTimestamp, _ := strconv.ParseInt(strings.TrimSpace(c.Query("end_timestamp")), 10, 64)
	return model.RetryRouteEventQuery{
		RequestId:       c.Query("request_id"),
		Username:        c.Query("username"),
		UserId:          parseRetryRouteQueryInt(c.Query("user_id")),
		TokenName:       c.Query("token_name"),
		TokenId:         parseRetryRouteQueryInt(c.Query("token_id")),
		LogId:           parseRetryRouteQueryInt(c.Query("log_id")),
		RequestHash:     c.Query("request_hash"),
		ModelName:       c.Query("model_name"),
		RuleName:        c.Query("rule_name"),
		Action:          c.Query("action"),
		SourceChannelId: parseRetryRouteQueryInt(c.Query("source_channel_id")),
		TargetChannelId: parseRetryRouteQueryInt(c.Query("target_channel_id")),
		SourceGroup:     c.Query("source_group"),
		TargetGroup:     c.Query("target_group"),
		StatusCode:      parseRetryRouteQueryInt(c.Query("status_code")),
		ErrorCode:       c.Query("error_code"),
		ErrorType:       c.Query("error_type"),
		FinalStatus:     c.Query("final_status"),
		StartTimestamp:  startTimestamp,
		EndTimestamp:    endTimestamp,
		Page:            pageInfo.GetPage(),
		PageSize:        pageInfo.GetPageSize(),
	}
}

func parseRetryRouteQueryInt(value string) int {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func retryRouteRuleInputToPolicyInput(input retryRouteRuleTestInput) operation_setting.RetryPolicyInput {
	return operation_setting.RetryPolicyInput{
		ModelName:    input.ModelName,
		Group:        input.Group,
		RequestPath:  input.RequestPath,
		IsStream:     input.IsStream,
		TokenID:      input.TokenID,
		WorkspaceID:  input.WorkspaceID,
		ChannelID:    input.ChannelID,
		ChannelType:  input.ChannelType,
		ErrorType:    input.ErrorType,
		ErrorCode:    input.ErrorCode,
		StatusCode:   input.StatusCode,
		ErrorMessage: input.ErrorMessage,
	}
}

func retryRouteDecisionResponse(decision operation_setting.RetryPolicyDecision) retryRouteRuleTestDecision {
	return retryRouteRuleTestDecision{
		Matched:          decision.Matched,
		ShouldRetry:      decision.ShouldRetry,
		Action:           decision.Action,
		Source:           decision.Source,
		RuleName:         decision.RuleName,
		RetryGroups:      decision.RetryGroups,
		MaxRetries:       decision.MaxRetries,
		Targets:          decision.Targets,
		TargetChannelIDs: decision.TargetChannelIDs,
		TargetTags:       decision.TargetTags,
		TargetModel:      decision.TargetModel,
		ExcludeChannelID: decision.ExcludeChannelID,
	}
}
