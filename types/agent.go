package types

type AgentContext struct {
	AgentID       int
	Domain        string
	OwnerUserID   int
	DefaultMarkup float64
	Branding      string
	Groups        map[string]AgentGroup
	UserGroups    map[string]AgentUserGroup
	GroupRatios   map[string]float64
}

type AgentGroup struct {
	GroupName       string
	SystemGroupName string
	Description     string
	AgentRatio      float64
	SystemRatio     float64
	ConfiguredRatio float64
	EffectiveRatio  float64
	Visible         bool
	Available       bool
}

type AgentUserGroup struct {
	GroupName     string
	VisibleGroups []string
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
