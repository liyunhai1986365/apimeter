package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type tokenAutoGroupsInput struct {
	Set    bool
	Groups []string
}

func (input *tokenAutoGroupsInput) UnmarshalJSON(data []byte) error {
	input.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		input.Groups = nil
		return nil
	}
	return common.Unmarshal(data, &input.Groups)
}

type tokenRequest struct {
	model.Token
	AutoGroups tokenAutoGroupsInput `json:"auto_groups"`
}

type tokenResponse struct {
	*model.Token
	AutoGroups []string `json:"auto_groups"`
}

func buildMaskedTokenResponse(token *model.Token) *tokenResponse {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	autoGroups, err := token.GetAutoGroups()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to parse auto groups for token %d: %v", token.Id, err))
		autoGroups = nil
	}
	if len(autoGroups) == 0 {
		autoGroups = nil
	}
	return &tokenResponse{Token: &maskedToken, AutoGroups: autoGroups}
}

func buildMaskedTokenResponses(tokens []*model.Token) []*tokenResponse {
	maskedTokens := make([]*tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func getTokenRequestUserGroup(c *gin.Context) (string, error) {
	if userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup); userGroup != "" {
		return userGroup, nil
	}
	if userGroup := c.GetString("group"); userGroup != "" {
		return userGroup, nil
	}
	return model.GetUserGroup(c.GetInt("id"), false)
}

func setTokenAutoGroups(c *gin.Context, token *model.Token, groups []string) bool {
	if len(groups) == 0 {
		if err := token.SetAutoGroups(nil); err != nil {
			common.ApiError(c, err)
			return false
		}
		return true
	}

	maxCount := setting.GetMaxTokenAutoGroups()
	if len(groups) > maxCount {
		common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsTooMany, map[string]any{"Max": maxCount})
		return false
	}

	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if _, ok := seen[group]; ok {
			common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsDuplicate, map[string]any{"Group": group})
			return false
		}
		seen[group] = struct{}{}
		if !service.IsUserSelectableGroup(userGroup, group) {
			common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsInvalid, map[string]any{"Group": group})
			return false
		}
	}

	if err := token.SetAutoGroups(groups); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func tokenWorkspaceIDFromQuery(c *gin.Context) (int, error) {
	raw := strings.TrimSpace(c.Query("workspace_id"))
	if raw == "" {
		return 0, nil
	}
	workspaceId, err := strconv.Atoi(raw)
	if err != nil || workspaceId < 0 {
		return 0, fmt.Errorf("workspace_id 无效")
	}
	return workspaceId, nil
}

func prepareTokenWorkspaceFilter(c *gin.Context, scope *service.WorkspaceAccessScope) (int, error) {
	if !scope.IsSubaccount {
		if _, err := model.EnsureDefaultWorkspace(scope.OwnerUserId); err != nil {
			return 0, err
		}
	}
	workspaceId, err := tokenWorkspaceIDFromQuery(c)
	if err != nil {
		return 0, err
	}
	if workspaceId > 0 {
		if !scope.CanAccessWorkspace(workspaceId) {
			return 0, fmt.Errorf("workspace not found or unavailable")
		}
		if _, err := model.GetUserWorkspaceByID(scope.OwnerUserId, workspaceId); err != nil {
			return 0, err
		}
	}
	return workspaceId, nil
}

func normalizeTokenGroupForRequest(c *gin.Context, token *model.Token) error {
	if token == nil {
		return nil
	}
	userId := c.GetInt("id")
	isSubaccount := false
	if scope, ok := c.Get("workspace_access_scope"); ok {
		if workspaceScope, valid := scope.(*service.WorkspaceAccessScope); valid {
			userId = workspaceScope.OwnerUserId
			isSubaccount = workspaceScope.IsSubaccount
		}
	}
	userGroup := ""
	if dbGroup, err := model.GetUserGroup(userId, false); err == nil {
		userGroup = strings.TrimSpace(dbGroup)
	}
	if userGroup == "" {
		userGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	if userGroup == "" {
		userGroup = c.GetString("group")
	}
	var agentCtx *types.AgentContext
	if ctx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && !isSubaccount {
		agentCtx = ctx
	}
	if agentCtx != nil {
		if agentUserGroup, err := agentservice.GetUserGroup(agentCtx, userId, userGroup); err == nil {
			userGroup = agentUserGroup
		} else {
			return err
		}
	}
	group, policy, err := service.NormalizeTokenGroupPolicyForUserWithAgent(token.GroupPolicy, userGroup, userId, agentCtx)
	if err != nil {
		return err
	}
	if group != "" || token.GroupPolicy != "" {
		token.Group = group
		token.GroupPolicy = policy
	}
	if service.HasMultipleOrderedTokenGroups(token.GroupPolicy) {
		token.CrossGroupRetry = true
	}
	if strings.TrimSpace(token.Group) == "" && strings.TrimSpace(token.GroupPolicy) == "" {
		token.Group = service.AutoGroupName
	}
	if token.GroupPolicy == "" {
		if err := service.ValidateExplicitTokenGroupForUserWithAgent(token.Group, userGroup, userId, agentCtx); err != nil {
			return err
		}
	}
	return nil
}

func tokenUsesUserOwnedProvider(token *model.Token) bool {
	if token == nil {
		return false
	}
	if model.IsUserOwnedProviderGroup(strings.TrimSpace(token.Group)) {
		return true
	}
	if strings.TrimSpace(token.GroupPolicy) == "" {
		return false
	}
	var policy service.TokenGroupPolicy
	if err := common.Unmarshal([]byte(token.GroupPolicy), &policy); err != nil {
		return false
	}
	for _, group := range append(policy.Groups, policy.ExcludedGroups...) {
		if model.IsUserOwnedProviderGroup(strings.TrimSpace(group)) {
			return true
		}
	}
	return false
}

func getTokenForWorkspaceScope(scope *service.WorkspaceAccessScope, id int) (*model.Token, error) {
	return model.GetTokenByIds(id, scope.OwnerUserId, scope.WorkspaceFilter())
}

func GetAllTokens(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo := common.GetPageQuery(c)
	workspaceId, err := prepareTokenWorkspaceFilter(c, scope)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokens, err := model.GetAllUserTokens(scope.OwnerUserId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), workspaceId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachWorkspaceNames(tokens); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachTokenTodayUsedQuota(tokens, time.Time{}); err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(scope.OwnerUserId, workspaceId, scope.WorkspaceFilter())
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetTokenFilterOptions(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	workspaces, err := model.ListUserWorkspaceFilterOptions(scope.OwnerUserId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokens, err := model.GetUserTokenFilterOptions(scope.OwnerUserId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"workspaces": workspaces,
		"tokens":     tokens,
	})
}

func SearchTokens(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	workspaceId, err := prepareTokenWorkspaceFilter(c, scope)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokens, total, err := model.SearchUserTokens(scope.OwnerUserId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), workspaceId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachWorkspaceNames(tokens); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachTokenTodayUsedQuota(tokens, time.Time{}); err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := getTokenForWorkspaceScope(scope, id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachWorkspaceNames([]*model.Token{token}); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachTokenTodayUsedQuota([]*model.Token{token}, time.Time{}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenAutoGroups(c *gin.Context) {
	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"groups":    service.GetUserAutoGroup(userGroup),
		"max_count": setting.GetMaxTokenAutoGroups(),
	})
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := getTokenForWorkspaceScope(scope, id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId, nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(common.NormalizeTokenKey(tokenKey), false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":               "token_usage",
			"name":                 token.Name,
			"total_granted":        token.RemainQuota + token.UsedQuota,
			"total_used":           token.UsedQuota,
			"total_available":      token.RemainQuota,
			"unlimited_quota":      token.UnlimitedQuota,
			"model_limits":         token.GetModelLimitsMap(),
			"model_limits_enabled": token.ModelLimitsEnabled,
			"expires_at":           expiredAt,
		},
	})
}

func AddToken(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	request := tokenRequest{}
	err = c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token := request.Token
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if scope.IsSubaccount && token.WorkspaceId <= 0 {
		common.ApiErrorMsg(c, "workspace is required")
		return
	}
	if scope.IsSubaccount && !scope.CanAccessWorkspace(token.WorkspaceId) {
		common.ApiErrorMsg(c, "workspace not found or unavailable")
		return
	}
	workspace, err := model.ResolveUserWorkspace(scope.OwnerUserId, token.WorkspaceId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := normalizeTokenGroupForRequest(c, &token); err != nil {
		common.ApiError(c, err)
		return
	}
	if scope.IsSubaccount && tokenUsesUserOwnedProvider(&token) {
		common.ApiErrorMsg(c, "workspace accounts cannot use user-owned providers")
		return
	}
	// 非无限额度时，检查额度值是否超出有效范围
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := common.QuotaFromFloat(1000000000 * common.QuotaPerUnit)
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	// The per-user token cap counts the owner's whole tenant, so it stays unscoped.
	count, err := model.CountUserTokens(scope.OwnerUserId, 0, nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	if token.Group == "auto" {
		if !setTokenAutoGroups(c, &token, request.AutoGroups.Groups) {
			return
		}
	} else {
		if strings.TrimSpace(token.GroupPolicy) == "" {
			token.CrossGroupRetry = false
		}
		_ = token.SetAutoGroups(nil)
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             scope.OwnerUserId,
		WorkspaceId:        workspace.Id,
		WorkspaceName:      workspace.Name,
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              token.Group,
		GroupPolicy:        token.GroupPolicy,
		CrossGroupRetry:    token.CrossGroupRetry,
		AutoGroups:         token.AutoGroups,
		ImageSettings:      token.ImageSettings.Normalized(),
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanToken,
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := getTokenForWorkspaceScope(scope, id)
	if err == nil {
		err = token.Delete()
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func ResetTokenQuota(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	if _, err := getTokenForWorkspaceScope(scope, id); err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.ResetUserTokenWorkspaceQuota(scope.OwnerUserId, id, time.Time{}, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.AttachTokenTodayUsedQuota([]*model.Token{token}, time.Time{}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func UpdateToken(c *gin.Context) {
	scope, err := workspaceAccessScope(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	statusOnly := c.Query("status_only")
	request := tokenRequest{}
	err = c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token := request.Token
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if statusOnly == "" {
		if err := normalizeTokenGroupForRequest(c, &token); err != nil {
			common.ApiError(c, err)
			return
		}
		if scope.IsSubaccount && tokenUsesUserOwnedProvider(&token) {
			common.ApiErrorMsg(c, "workspace accounts cannot use user-owned providers")
			return
		}
	}
	if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := common.QuotaFromFloat(1000000000 * common.QuotaPerUnit)
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	cleanToken, err := getTokenForWorkspaceScope(scope, token.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		if token.WorkspaceId > 0 {
			if !scope.CanAccessWorkspace(token.WorkspaceId) {
				common.ApiErrorMsg(c, "workspace not found or unavailable")
				return
			}
			workspace, err := model.GetUserWorkspaceByID(scope.OwnerUserId, token.WorkspaceId)
			if err != nil {
				common.ApiError(c, err)
				return
			}
			cleanToken.WorkspaceId = workspace.Id
			cleanToken.WorkspaceName = workspace.Name
		}
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		cleanToken.RemainQuota = token.RemainQuota
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		cleanToken.Group = token.Group
		cleanToken.GroupPolicy = token.GroupPolicy
		cleanToken.CrossGroupRetry = token.CrossGroupRetry
		if token.Group != service.AutoGroupName {
			if strings.TrimSpace(token.GroupPolicy) == "" {
				cleanToken.CrossGroupRetry = false
			}
			_ = cleanToken.SetAutoGroups(nil)
		} else if request.AutoGroups.Set {
			if !setTokenAutoGroups(c, cleanToken, request.AutoGroups.Groups) {
				return
			}
		}
		cleanToken.ImageSettings = token.ImageSettings.Normalized()
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if cleanToken.WorkspaceName == "" {
		if err := model.AttachWorkspaceNames([]*model.Token{cleanToken}); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	})
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, scope.OwnerUserId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	scope, scopeErr := workspaceAccessScope(c)
	if scopeErr != nil {
		common.ApiError(c, scopeErr)
		return
	}
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, scope.OwnerUserId, scope.WorkspaceFilter())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
	}
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
