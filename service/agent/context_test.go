package agent

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

func TestApplyPricingUsesModelRuleBeforeDefaultMarkup(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.AgentPricingRule{
		AgentId:      1,
		ModelPattern: "gpt-4.1",
		Markup:       1.5,
		Enabled:      true,
	}).Error)

	charged, snapshot, err := ApplyPricing(&Context{AgentID: 1, Domain: "agent.example.com", DefaultMarkup: 1.2}, "gpt-4.1", 1000)
	require.NoError(t, err)
	require.Equal(t, 1500, charged)
	require.NotNil(t, snapshot)
	require.Equal(t, 1.5, snapshot.Markup)
	require.Equal(t, 1000, snapshot.BaseEstimatedQuota)
	require.Equal(t, 1500, snapshot.ChargedEstimatedQuota)

	charged, snapshot, err = ApplyPricing(&Context{AgentID: 1, Domain: "agent.example.com", DefaultMarkup: 1.2}, "gpt-3.5", 1000)
	require.NoError(t, err)
	require.Equal(t, 1200, charged)
	require.NotNil(t, snapshot)
	require.Equal(t, 1.2, snapshot.Markup)

	charged, snapshot, err = ApplyPricing(nil, "gpt-4.1", 1000)
	require.NoError(t, err)
	require.Equal(t, 1000, charged)
	require.Nil(t, snapshot)
}

func TestSettleConsumeWritesProfitLedger(t *testing.T) {
	setupAgentTestDB(t)

	err := SettleConsume(&BillingSnapshot{AgentID: 1, Domain: "agent.example.com", Markup: 1.2}, 100, 200, 1000, 1200)
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

	require.NoError(t, SettleConsume(&BillingSnapshot{AgentID: 1, Domain: "agent.example.com", Markup: 1.3}, 100, 200, 1000, 1300))

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

func TestSubmitWithdrawalRejectsOverAvailable(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, SettleConsume(&BillingSnapshot{AgentID: 1, Markup: 1.1}, 100, 200, 1000, 1100))

	_, err := SubmitWithdrawal(1, 101, 1.01, "bank account")
	require.Error(t, err)
}

func TestAgentDomainAndPricingRuleCRUD(t *testing.T) {
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

	rule, err := UpsertPricingRule(agent.Id, "gpt-4.1", 1.45, true)
	require.NoError(t, err)
	require.Equal(t, 1.45, rule.Markup)

	rules, total, err := ListPricingRules(agent.Id, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rules, 1)
}
