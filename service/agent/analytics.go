package agent

import "github.com/QuantumNous/new-api/model"

func GetAnalytics(agentID int, startTimestamp, endTimestamp, bucketSeconds int64) (*model.AgentAnalytics, error) {
	analytics, err := model.GetAgentAnalytics(agentID, startTimestamp, endTimestamp, bucketSeconds)
	if err != nil {
		return nil, err
	}
	currency, err := getSettlementCurrencyWithTx(model.DB, agentID)
	if err != nil {
		return nil, err
	}
	analytics.Summary.Currency = normalizeSettlementCurrency(currency)
	analytics.Summary.ProfitAmount = quotaToSettlementAmount(int(analytics.Summary.ProfitQuota), currency)
	return analytics, nil
}
