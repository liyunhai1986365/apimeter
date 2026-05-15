package types

type AgentContext struct {
	AgentID       int
	Domain        string
	OwnerUserID   int
	DefaultMarkup float64
	Branding      string
	GroupRatios   map[string]float64
}

type AgentBillingSnapshot struct {
	AgentID               int
	Domain                string
	Group                 string
	BaseGroupRatio        float64
	ChargedGroupRatio     float64
	BaseEstimatedQuota    int
	ChargedEstimatedQuota int
}
