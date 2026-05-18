package agent

import (
	"math"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(
		&model.Agent{},
		&model.AgentDomain{},
		&model.AgentUser{},
		&model.AgentPricingRule{},
		&model.AgentGroupRatio{},
		&model.AgentLedger{},
		&model.AgentWithdrawal{},
	))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{name: "lowercases host", host: "Agent.Example.COM", want: "agent.example.com"},
		{name: "strips port", host: "agent.example.com:8443", want: "agent.example.com"},
		{name: "strips trailing dot", host: "agent.example.com.", want: "agent.example.com"},
		{name: "keeps invalid split fallback", host: "agent.example.com:bad", want: "agent.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, NormalizeHost(tt.host))
		})
	}
}

func TestResolveByHostOnlyReturnsActiveAgentDomain(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1.2,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{
		AgentId: 1,
		Domain:  "agent.example.com",
		Status:  model.AgentDomainStatusActive,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{
		AgentId: 1,
		Domain:  "disabled.example.com",
		Status:  model.AgentDomainStatusDisabled,
	}).Error)

	ctx, err := ResolveByHost("Agent.Example.com:443")
	require.NoError(t, err)
	require.NotNil(t, ctx)
	require.Equal(t, 1, ctx.AgentID)
	require.Equal(t, "agent.example.com", ctx.Domain)
	require.Equal(t, 10, ctx.OwnerUserID)
	require.Equal(t, 1.2, ctx.DefaultMarkup)

	ctx, err = ResolveByHost("disabled.example.com")
	require.NoError(t, err)
	require.Nil(t, ctx)
}

func TestRequireUserInAgent(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.AgentUser{
		AgentId: 1,
		UserId:  100,
		Status:  model.AgentUserStatusEnabled,
		Source:  model.AgentUserSourceDomain,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{
		AgentId: 1,
		UserId:  101,
		Status:  model.AgentUserStatusDisabled,
		Source:  model.AgentUserSourceDomain,
	}).Error)

	require.NoError(t, RequireUserInAgent(&Context{AgentID: 1}, 100))
	require.Error(t, RequireUserInAgent(&Context{AgentID: 1}, 101))
	require.Error(t, RequireUserInAgent(&Context{AgentID: 2}, 100))
	require.NoError(t, RequireUserInAgent(nil, 100))
}

func TestUpsertGroupRatioRejectsBelowSystemRatioAndIsolatesAgents(t *testing.T) {
	setupAgentTestDB(t)
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))

	_, err := UpsertGroupRatio(1, "vip", 1.2)
	require.ErrorIs(t, err, ErrAgentGroupRatioBelowSystem)

	ratio, err := UpsertGroupRatio(1, "vip", 1.5)
	require.NoError(t, err)
	require.Equal(t, "vip", ratio.GroupName)
	require.Equal(t, 1.5, ratio.Ratio)

	require.NoError(t, err)
	agentOneRatios, err := EffectiveGroupRatioMap(1)
	require.NoError(t, err)
	agentTwoRatios, err := EffectiveGroupRatioMap(2)
	require.NoError(t, err)

	require.Equal(t, 1.5, agentOneRatios["vip"])
	require.Equal(t, 1.25, agentTwoRatios["vip"])
}

func TestBaseQuotaFromChargedScalesByAgentGroupRatio(t *testing.T) {
	snapshot := &BillingSnapshot{
		AgentID:           1,
		Domain:            "agent.example.com",
		Group:             "vip",
		BaseGroupRatio:    1.25,
		ChargedGroupRatio: 1.5,
	}

	require.Equal(t, 1000, BaseQuotaFromCharged(snapshot, 1200))
	require.Equal(t, 1200, ApplySnapshot(snapshot, 1200))

	require.Equal(t, 1000, ApplySnapshot(nil, 1000))
}

func TestSettleConsumeWritesProfitLedger(t *testing.T) {
	setupAgentTestDB(t)

	err := SettleConsume(&BillingSnapshot{AgentID: 1, Domain: "agent.example.com"}, 100, 200, 1000, 1200)
	require.NoError(t, err)

	var ledger model.AgentLedger
	require.NoError(t, model.DB.First(&ledger).Error)
	require.Equal(t, 1, ledger.AgentId)
	require.Equal(t, 100, ledger.UserId)
	require.Equal(t, 200, ledger.LogId)
	require.Equal(t, model.AgentLedgerTypeConsumeProfit, ledger.Type)
	require.Equal(t, 1000, ledger.BaseQuota)
	require.Equal(t, 1200, ledger.ChargedQuota)
	require.Equal(t, 200, ledger.ProfitQuota)
	require.Equal(t, 200, ledger.BalanceAfter)
}

func TestWithdrawalLifecycle(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, SettleConsume(&BillingSnapshot{AgentID: 1, Domain: "agent.example.com"}, 100, 200, 1000, 1300))

	balance, err := GetBalance(1)
	require.NoError(t, err)
	require.Equal(t, 300, balance.ProfitQuota)
	require.Equal(t, 0, balance.PendingWithdrawalQuota)
	require.Equal(t, 300, balance.AvailableQuota)

	withdrawal, err := SubmitWithdrawal(1, 200, 2.0, "bank account")
	require.NoError(t, err)
	require.Equal(t, model.AgentWithdrawalStatusPending, withdrawal.Status)

	balance, err = GetBalance(1)
	require.NoError(t, err)
	require.Equal(t, 100, balance.AvailableQuota)
	require.Equal(t, 200, balance.PendingWithdrawalQuota)

	require.Error(t, CompleteWithdrawal(withdrawal.Id, model.AgentWithdrawalStatusPaid, ""))
	require.NoError(t, CompleteWithdrawal(withdrawal.Id, model.AgentWithdrawalStatusApproved, "ok"))
	require.NoError(t, CompleteWithdrawal(withdrawal.Id, model.AgentWithdrawalStatusPaid, "paid"))

	balance, err = GetBalance(1)
	require.NoError(t, err)
	require.Equal(t, 100, balance.ProfitQuota)
	require.Equal(t, 0, balance.PendingWithdrawalQuota)
	require.Equal(t, 100, balance.AvailableQuota)

	var ledger model.AgentLedger
	require.NoError(t, model.DB.Where("type = ?", model.AgentLedgerTypeWithdraw).First(&ledger).Error)
	require.Equal(t, -200, ledger.ProfitQuota)
	require.Equal(t, 100, ledger.BalanceAfter)
}

func TestGetBalanceReturnsSettlementAmounts(t *testing.T) {
	setupAgentTestDB(t)
	oldExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:                 1,
		OwnerUserId:        10,
		Name:               "RMB Agent",
		Slug:               "rmb-agent",
		Status:             model.AgentStatusEnabled,
		SettlementCurrency: "RMB",
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentLedger{
		AgentId:      1,
		Type:         model.AgentLedgerTypeConsumeProfit,
		ProfitQuota:  int(common.QuotaPerUnit),
		BalanceAfter: int(common.QuotaPerUnit),
	}).Error)

	balance, err := GetBalance(1)
	require.NoError(t, err)
	require.Equal(t, "RMB", balance.Currency)
	require.Equal(t, int(common.QuotaPerUnit), balance.ProfitQuota)
	require.InDelta(t, 7, balance.ProfitAmount, 0.000001)
	require.InDelta(t, 7, balance.AvailableAmount, 0.000001)
}

func TestBuildLedgerViewsReturnsSettlementAmounts(t *testing.T) {
	setupAgentTestDB(t)
	oldExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:                 1,
		OwnerUserId:        10,
		Name:               "RMB Agent",
		Slug:               "rmb-agent",
		Status:             model.AgentStatusEnabled,
		SettlementCurrency: "RMB",
	}).Error)

	views, err := BuildLedgerViews(1, []*model.AgentLedger{{
		AgentId:      1,
		Type:         model.AgentLedgerTypeConsumeProfit,
		ProfitQuota:  int(common.QuotaPerUnit),
		BalanceAfter: int(common.QuotaPerUnit * 2),
	}})
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "RMB", views[0].Currency)
	require.InDelta(t, 7, views[0].ProfitAmount, 0.000001)
	require.InDelta(t, 14, views[0].BalanceAfterAmount, 0.000001)
}

func TestSubmitWithdrawalAmountUsesSettlementCurrency(t *testing.T) {
	setupAgentTestDB(t)
	oldExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = oldExchangeRate
	})

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:                 1,
		OwnerUserId:        10,
		Name:               "RMB Agent",
		Slug:               "rmb-agent",
		Status:             model.AgentStatusEnabled,
		SettlementCurrency: "RMB",
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentLedger{
		AgentId:      1,
		Type:         model.AgentLedgerTypeConsumeProfit,
		ProfitQuota:  int(common.QuotaPerUnit),
		BalanceAfter: int(common.QuotaPerUnit),
	}).Error)

	withdrawal, err := SubmitWithdrawalAmount(1, 3.5, "bank account")
	require.NoError(t, err)
	require.Equal(t, model.AgentWithdrawalStatusPending, withdrawal.Status)
	require.Equal(t, int(math.Ceil(3.5/7*common.QuotaPerUnit)), withdrawal.AmountQuota)
	require.InDelta(t, 3.5, withdrawal.AmountMoney, 0.000001)

	balance, err := GetBalance(1)
	require.NoError(t, err)
	require.InDelta(t, 3.5, balance.PendingWithdrawalAmount, 0.000001)
	require.InDelta(t, 3.5, balance.AvailableAmount, 0.000001)
}

func TestSubmitWithdrawalRejectsOverAvailable(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, SettleConsume(&BillingSnapshot{AgentID: 1}, 100, 200, 1000, 1100))

	_, err := SubmitWithdrawal(1, 101, 1.01, "bank account")
	require.Error(t, err)
}

func TestAgentDomainAndGroupRatioCRUD(t *testing.T) {
	setupAgentTestDB(t)

	agent := &model.Agent{
		OwnerUserId:        10,
		Name:               "Agent One",
		Slug:               "agent-one",
		Status:             model.AgentStatusEnabled,
		PriceMode:          model.AgentPriceModeMultiplier,
		DefaultMarkup:      1.2,
		SettlementCurrency: "USD",
	}
	require.NoError(t, model.DB.Create(agent).Error)

	domain, err := CreateDomain(agent.Id, " Agent.Example.com:443 ")
	require.NoError(t, err)
	require.Equal(t, "agent.example.com", domain.Domain)
	require.Equal(t, model.AgentDomainStatusPending, domain.Status)
	require.NotEmpty(t, domain.VerifyToken)

	require.NoError(t, ActivateDomain(agent.Id, domain.Id))
	ctx, err := ResolveByHost("agent.example.com")
	require.NoError(t, err)
	require.NotNil(t, ctx)
	require.Equal(t, agent.Id, ctx.AgentID)

	rule, err := UpsertGroupRatio(agent.Id, "default", 1.45)
	require.NoError(t, err)
	require.Equal(t, 1.45, rule.Ratio)

	rules, err := ListGroupRatios(agent.Id)
	require.NoError(t, err)
	require.NotEmpty(t, rules)
}

func TestCreateDomainReturnsBusinessErrorForDuplicateDomain(t *testing.T) {
	setupAgentTestDB(t)

	agent := &model.Agent{
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1.2,
	}
	require.NoError(t, model.DB.Create(agent).Error)

	_, err := CreateDomain(agent.Id, "aiiqq.com")
	require.NoError(t, err)

	_, err = CreateDomain(agent.Id, " AIIQQ.com:443 ")
	require.Error(t, err)
	require.Equal(t, "agent domain already exists", err.Error())
	require.False(t, strings.Contains(err.Error(), "UNIQUE"))
	require.False(t, strings.Contains(err.Error(), "Duplicate entry"))
}
