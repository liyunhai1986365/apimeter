package types

type AgentContext struct {
	AgentID       int
	Domain        string
	OwnerUserID   int
	DefaultMarkup float64
	Branding      string
}

type AgentBillingSnapshot struct {
	AgentID               int
	Domain                string
	Markup                float64
	BaseEstimatedQuota    int
	ChargedEstimatedQuota int
}
