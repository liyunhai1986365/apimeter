package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const currentAgentIDKey = "current_agent_id"

type agentRequest struct {
	OwnerUserId        int     `json:"owner_user_id"`
	Name               string  `json:"name"`
	Slug               string  `json:"slug"`
	Status             int     `json:"status"`
	PriceMode          string  `json:"price_mode"`
	DefaultMarkup      float64 `json:"default_markup"`
	MinWithdrawAmount  int     `json:"min_withdraw_amount"`
	WithdrawFeeRate    float64 `json:"withdraw_fee_rate"`
	SettlementCurrency string  `json:"settlement_currency"`
	Branding           string  `json:"branding"`
}

type agentDomainRequest struct {
	Domain string `json:"domain"`
	Status int    `json:"status"`
}

type agentGroupRatioRequest struct {
	GroupName       string   `json:"group_name"`
	SystemGroupName string   `json:"system_group_name"`
	Ratio           float64  `json:"ratio"`
	Visible         *bool    `json:"visible"`
	VisibleGroups   []string `json:"visible_groups"`
	RemoveGroups    []string `json:"remove_groups"`
}

type agentWithdrawalRequest struct {
	AmountQuota int     `json:"amount_quota"`
	AmountMoney float64 `json:"amount_money"`
	AccountInfo string  `json:"account_info"`
	Status      string  `json:"status"`
	AdminRemark string  `json:"admin_remark"`
}

type agentUserRequest struct {
	UserId    int    `json:"user_id"`
	Status    int    `json:"status"`
	Source    string `json:"source"`
	Delta     int    `json:"delta"`
	GroupName string `json:"group_name"`
}

func GetAgentSelf(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	agent, err := model.GetAgentById(agentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	balance, err := agentservice.GetBalance(agent.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	agentCtx, _ := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	common.ApiSuccess(c, gin.H{
		"agent":   agent,
		"context": agentCtx,
		"balance": balance,
	})
}

func UpdateAgentSelfBranding(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	var req agentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	agent, err := agentservice.UpdateBranding(agentID, req.Branding)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, agent)
}

func AdminListAgents(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	agents, total, err := model.ListAgents(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(agents)
	common.ApiSuccess(c, pageInfo)
}

func AdminListAgentDomains(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	status := -1
	if statusQuery := strings.TrimSpace(c.Query("status")); statusQuery != "" {
		parsedStatus, err := strconv.Atoi(statusQuery)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		status = parsedStatus
	}
	domains, total, err := model.ListAgentDomainsByStatus(status, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, domain := range domains {
		agentservice.FillDomainCNAMETarget(&domain.AgentDomain)
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(domains)
	common.ApiSuccess(c, pageInfo)
}

func AdminCreateAgent(c *gin.Context) {
	var req agentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	agent := &model.Agent{
		OwnerUserId:        req.OwnerUserId,
		Name:               req.Name,
		Slug:               req.Slug,
		Status:             req.Status,
		PriceMode:          req.PriceMode,
		DefaultMarkup:      req.DefaultMarkup,
		MinWithdrawAmount:  req.MinWithdrawAmount,
		WithdrawFeeRate:    req.WithdrawFeeRate,
		SettlementCurrency: req.SettlementCurrency,
		Branding:           req.Branding,
	}
	normalizeAgentDefaults(agent)
	if err := model.DB.Create(agent).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, agent)
}

func AdminUpdateAgent(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	updates := map[string]interface{}{
		"owner_user_id":       req.OwnerUserId,
		"name":                req.Name,
		"slug":                req.Slug,
		"status":              req.Status,
		"price_mode":          req.PriceMode,
		"default_markup":      req.DefaultMarkup,
		"min_withdraw_amount": req.MinWithdrawAmount,
		"withdraw_fee_rate":   req.WithdrawFeeRate,
		"settlement_currency": req.SettlementCurrency,
		"branding":            req.Branding,
	}
	if updates["price_mode"] == "" {
		updates["price_mode"] = model.AgentPriceModeMultiplier
	}
	if updates["default_markup"].(float64) <= 0 {
		updates["default_markup"] = 1.0
	}
	if updates["settlement_currency"] == "" {
		updates["settlement_currency"] = "USD"
	}
	if err := model.DB.Model(&model.Agent{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	agent, err := model.GetAgentById(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, agent)
}

func AdminUpdateAgentStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Model(&model.Agent{}).Where("id = ?", id).Update("status", req.Status).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": req.Status})
}

func AdminListAgentWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	agentID, _ := strconv.Atoi(c.Query("agent_id"))
	withdrawals, total, err := model.ListAgentWithdrawals(agentID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	withdrawalViews, err := agentservice.BuildWithdrawalViews(withdrawals)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawalViews)
	common.ApiSuccess(c, pageInfo)
}

func AdminCompleteAgentWithdrawal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := agentservice.CompleteWithdrawal(id, req.Status, req.AdminRemark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": req.Status})
}

func AgentListDomains(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	domains, total, err := model.ListAgentDomains(agentID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for i, domain := range domains {
		if domain.Status == model.AgentDomainStatusActive {
			continue
		}
		verified, verifyErr := agentservice.TryAutoVerifyDomainCNAME(c.Request.Context(), agentID, domain.Id)
		if verifyErr != nil {
			common.ApiError(c, verifyErr)
			return
		}
		if verified != nil {
			domains[i] = verified
		}
	}
	agentservice.FillDomainCNAMETargets(domains)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(domains)
	common.ApiSuccess(c, pageInfo)
}

func AgentCreateDomain(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	var req agentDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	domain, err := agentservice.CreateDomain(agentID, req.Domain)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	verified, verifyErr := agentservice.TryAutoVerifyDomainCNAME(c.Request.Context(), agentID, domain.Id)
	if verifyErr != nil {
		common.ApiError(c, verifyErr)
		return
	}
	if verified != nil {
		domain = verified
	}
	common.ApiSuccess(c, domain)
}

func AgentVerifyDomainCNAME(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	domainParam := c.Param("domain_id")
	if domainParam == "" {
		domainParam = c.Param("id")
	}
	id, err := strconv.Atoi(domainParam)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	domain, err := agentservice.VerifyDomainCNAME(c.Request.Context(), agentID, id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, domain)
}

func AgentDomainTLSAsk(c *gin.Context) {
	if !agentservice.AuthorizeTLSAskSecret(c.GetHeader("X-Agent-TLS-Ask-Secret"), c.Query("secret")) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "tls ask secret is invalid"})
		return
	}

	domain := c.Query("domain")
	if domain == "" {
		domain = c.Query("host")
	}
	allowed, err := agentservice.CanIssueTLSCertificateForDomain(domain)
	if err != nil || !allowed {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "message": "domain is not allowed for tls issuance"})
		return
	}
	common.ApiSuccess(c, gin.H{"domain": agentservice.NormalizeHost(domain)})
}

func AgentUpdateDomainStatus(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	domainParam := c.Param("domain_id")
	if domainParam == "" {
		domainParam = c.Param("id")
	}
	id, err := strconv.Atoi(domainParam)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Status == model.AgentDomainStatusActive {
		err = agentservice.ActivateDomain(agentID, id)
	} else {
		err = agentservice.UpdateDomainStatus(agentID, id, req.Status)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": req.Status})
}

func AgentListGroupRatios(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	ratios, err := agentservice.ListGroupRatios(agentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ratios)
}

func AgentUpsertGroupRatio(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	var req agentGroupRatioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	visible := true
	if req.Visible != nil {
		visible = *req.Visible
	}
	ratio, err := agentservice.UpsertGroupRatio(agentID, req.GroupName, req.SystemGroupName, req.Ratio, visible, req.VisibleGroups, req.RemoveGroups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	views, err := agentservice.ListGroupRatios(agentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for _, view := range views {
		if view.GroupName == ratio.GroupName {
			common.ApiSuccess(c, view)
			return
		}
	}
	common.ApiSuccess(c, agentservice.GroupRatioView{
		GroupName:       ratio.GroupName,
		SystemGroupName: ratio.SystemGroupName,
		ConfiguredRatio: ratio.Ratio,
		EffectiveRatio:  ratio.Ratio,
		Configured:      true,
		Visible:         ratio.Visible,
		Available:       true,
		VisibleGroups:   req.VisibleGroups,
		RemoveGroups:    req.RemoveGroups,
	})
}

func AgentListUsers(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	users, total, err := model.ListAgentUsers(agentID, c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
}

func AgentBindUser(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	var req agentUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	source := req.Source
	if source == "" {
		source = model.AgentUserSourceAdminBind
	}
	if err := model.BindUserToAgent(agentID, req.UserId, source); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"user_id": req.UserId})
}

func AgentUpdateUserStatus(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := agentservice.UpdateUserGlobalStatus(agentID, userID, req.Status); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"user_id": userID, "status": req.Status})
}

func AgentUpdateUserGroup(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := agentservice.UpdateUserGroup(agentID, userID, req.GroupName); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"user_id": userID, "group": req.GroupName})
}

func AgentAdjustUserQuota(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req agentUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	inAgent, err := model.IsUserInAgent(agentID, userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !inAgent {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "用户不属于当前代理"})
		return
	}
	if err := model.DeltaUpdateUserQuota(userID, req.Delta); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RecordLog(c.GetInt("id"), model.LogTypeManage, "代理调整用户额度")
	common.ApiSuccess(c, gin.H{"user_id": userID, "delta": req.Delta})
}

func AgentListUserTokens(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	inAgent, err := model.IsUserInAgent(agentID, userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !inAgent {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "用户不属于当前代理"})
		return
	}
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userID)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func AgentListLedger(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	ledgers, total, err := model.ListAgentLedger(agentID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ledgerViews, err := agentservice.BuildLedgerViews(agentID, ledgers)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(ledgerViews)
	common.ApiSuccess(c, pageInfo)
}

func AgentListWithdrawals(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	withdrawals, total, err := model.ListAgentWithdrawals(agentID, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	withdrawalViews, err := agentservice.BuildWithdrawalViews(withdrawals)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawalViews)
	common.ApiSuccess(c, pageInfo)
}

func AgentSubmitWithdrawal(c *gin.Context) {
	agentID, ok := currentAgentID(c)
	if !ok {
		return
	}
	var req agentWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	var withdrawal *model.AgentWithdrawal
	var err error
	if req.AmountMoney > 0 {
		withdrawal, err = agentservice.SubmitWithdrawalAmount(agentID, req.AmountMoney, req.AccountInfo)
	} else {
		withdrawal, err = agentservice.SubmitWithdrawal(agentID, req.AmountQuota, req.AmountMoney, req.AccountInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	withdrawalViews, err := agentservice.BuildWithdrawalViews([]*model.AgentWithdrawal{withdrawal})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawalViews[0])
}

func currentAgentID(c *gin.Context) (int, bool) {
	if id := c.GetInt(currentAgentIDKey); id > 0 {
		return id, true
	}
	idParam := c.Param("agent_id")
	if idParam == "" {
		idParam = c.Param("id")
	}
	id, err := strconv.Atoi(idParam)
	if err == nil && id > 0 {
		return id, true
	}
	if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
		return agentCtx.AgentID, true
	}
	agent, err := ensureUserAgent(c)
	if err != nil {
		common.ApiError(c, err)
		return 0, false
	}
	if agent != nil {
		c.Set(currentAgentIDKey, agent.Id)
		return agent.Id, true
	}
	common.ApiErrorMsg(c, "当前域名未配置代理站点")
	return 0, false
}

func ensureUserAgent(c *gin.Context) (*model.Agent, error) {
	userID := c.GetInt("id")
	role := c.GetInt("role")
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	agent, err := model.GetAgentByConsoleUserId(userID)
	if err == nil {
		return agent, nil
	}
	if !errors.Is(err, model.ErrAgentNotFound) {
		return nil, err
	}
	if role < common.RoleAdminUser {
		return nil, errors.New("当前用户还没有代理站点")
	}
	return nil, errors.New("当前管理员还没有开通代理站点，请先在代理管理中创建")
}

func normalizeAgentDefaults(agent *model.Agent) {
	if agent.Status == 0 {
		agent.Status = model.AgentStatusEnabled
	}
	if agent.PriceMode == "" {
		agent.PriceMode = model.AgentPriceModeMultiplier
	}
	if agent.DefaultMarkup <= 0 {
		agent.DefaultMarkup = 1
	}
	if agent.SettlementCurrency == "" {
		agent.SettlementCurrency = "USD"
	}
}
