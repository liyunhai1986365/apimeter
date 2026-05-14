package agent

import (
	"errors"
	"math"
	"net"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Context = types.AgentContext
type BillingSnapshot = types.AgentBillingSnapshot

func NormalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return ""
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	} else if idx := strings.LastIndex(host, ":"); idx > -1 {
		host = host[:idx]
	}
	host = strings.TrimSuffix(host, ".")
	return host
}

func ResolveByRequest(c *gin.Context) (*Context, error) {
	if c == nil || c.Request == nil {
		return nil, nil
	}
	return ResolveByHost(c.Request.Host)
}

func ResolveByHost(host string) (*Context, error) {
	if model.DB == nil {
		return nil, nil
	}
	domain := NormalizeHost(host)
	if domain == "" {
		return nil, nil
	}
	agentWithDomain, err := model.GetActiveAgentByDomain(domain)
	if err != nil || agentWithDomain == nil {
		return nil, err
	}
	return &Context{
		AgentID:       agentWithDomain.Id,
		Domain:        agentWithDomain.Domain,
		OwnerUserID:   agentWithDomain.OwnerUserId,
		DefaultMarkup: agentWithDomain.DefaultMarkup,
		Branding:      agentWithDomain.Branding,
	}, nil
}

func RequireUserInAgent(ctx *Context, userID int) error {
	if ctx == nil || ctx.AgentID == 0 {
		return nil
	}
	ok, err := model.IsUserInAgent(ctx.AgentID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return model.ErrAgentUserNotFound
	}
	return nil
}

func BindUser(ctx *Context, userID int, source string) error {
	if ctx == nil || ctx.AgentID == 0 {
		return nil
	}
	return model.BindUserToAgent(ctx.AgentID, userID, source)
}

func ApplyPricing(ctx *Context, modelName string, baseQuota int) (chargedQuota int, snapshot *BillingSnapshot, err error) {
	if ctx == nil || ctx.AgentID == 0 {
		return baseQuota, nil, nil
	}
	markup := ctx.DefaultMarkup
	if ruleMarkup, ok, err := model.GetAgentPricingMarkup(ctx.AgentID, modelName); err != nil {
		return 0, nil, err
	} else if ok {
		markup = ruleMarkup
	}
	if markup <= 0 {
		return 0, nil, errors.New("agent markup must be greater than 0")
	}
	chargedQuota = int(math.Round(float64(baseQuota) * markup))
	snapshot = &BillingSnapshot{
		AgentID:               ctx.AgentID,
		Domain:                ctx.Domain,
		Markup:                markup,
		BaseEstimatedQuota:    baseQuota,
		ChargedEstimatedQuota: chargedQuota,
	}
	return chargedQuota, snapshot, nil
}

func ApplySnapshot(snapshot *BillingSnapshot, baseQuota int) int {
	if snapshot == nil || snapshot.Markup <= 0 {
		return baseQuota
	}
	return int(math.Round(float64(baseQuota) * snapshot.Markup))
}

func SettleConsume(snapshot *BillingSnapshot, userID int, logID int, baseQuota int, chargedQuota int) error {
	if snapshot == nil || snapshot.AgentID == 0 {
		return nil
	}
	profitQuota := chargedQuota - baseQuota
	if profitQuota <= 0 {
		return nil
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var totalProfit int
		if err := tx.Model(&model.AgentLedger{}).
			Select("COALESCE(SUM(profit_quota), 0)").
			Where("agent_id = ?", snapshot.AgentID).
			Scan(&totalProfit).Error; err != nil {
			return err
		}
		ledger := &model.AgentLedger{
			AgentId:      snapshot.AgentID,
			UserId:       userID,
			LogId:        logID,
			Type:         model.AgentLedgerTypeConsumeProfit,
			BaseQuota:    baseQuota,
			ChargedQuota: chargedQuota,
			ProfitQuota:  profitQuota,
			BalanceAfter: totalProfit + profitQuota,
		}
		return tx.Create(ledger).Error
	})
}
