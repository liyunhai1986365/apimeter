package types

type AgentContext struct {
	AgentID       int
	Domain        string
	OwnerUserID   int
	DefaultMarkup float64
	Branding      string
	Groups        map[string]AgentGroup
	GroupRatios   map[string]float64
}

type AgentGroup struct {
	GroupName       string
	SystemGroupName string
	SystemRatio     float64
	ConfiguredRatio float64
	EffectiveRatio  float64
	Visible         bool
	Available       bool
	VisibleGroups   []string
	RemoveGroups    []string
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
