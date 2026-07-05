package operation_setting

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

const (
	RetryPolicyActionRetry     = "retry"
	RetryPolicyActionFailover  = "failover"
	RetryPolicyActionSkipRetry = "skip_retry"

	RetryPolicySourceChannel = "channel"
	RetryPolicySourceGlobal  = "global"
)

type RetryPolicyRule struct {
	Name        string                `json:"name,omitempty"`
	Enabled     *bool                 `json:"enabled,omitempty"`
	Priority    int                   `json:"priority,omitempty"`
	Action      string                `json:"action"`
	Conditions  RetryPolicyConditions `json:"conditions,omitempty"`
	Targets     RetryPolicyTargets    `json:"targets,omitempty"`
	Strategy    RetryPolicyStrategy   `json:"strategy,omitempty"`
	RetryGroups []string              `json:"retry_groups,omitempty"`
	MaxRetries  int                   `json:"max_retries,omitempty"`

	Models          []string          `json:"models,omitempty"`
	Groups          []string          `json:"groups,omitempty"`
	RequestPaths    []string          `json:"request_paths,omitempty"`
	Stream          *bool             `json:"stream,omitempty"`
	TokenIDs        []int             `json:"token_ids,omitempty"`
	WorkspaceIDs    []int             `json:"workspace_ids,omitempty"`
	ChannelIDs      []int             `json:"channel_ids,omitempty"`
	ChannelTypes    []int             `json:"channel_types,omitempty"`
	ErrorTypes      []types.ErrorType `json:"error_types,omitempty"`
	ErrorCodes      []types.ErrorCode `json:"error_codes,omitempty"`
	StatusCodes     string            `json:"status_codes,omitempty"`
	MessageContains []string          `json:"message_contains,omitempty"`
}

type RetryPolicyConditions struct {
	Models          []string          `json:"models,omitempty"`
	Groups          []string          `json:"groups,omitempty"`
	RequestPaths    []string          `json:"request_paths,omitempty"`
	Stream          *bool             `json:"stream,omitempty"`
	TokenIDs        []int             `json:"token_ids,omitempty"`
	WorkspaceIDs    []int             `json:"workspace_ids,omitempty"`
	ChannelIDs      []int             `json:"channel_ids,omitempty"`
	ChannelTypes    []int             `json:"channel_types,omitempty"`
	ErrorTypes      []types.ErrorType `json:"error_types,omitempty"`
	ErrorCodes      []types.ErrorCode `json:"error_codes,omitempty"`
	StatusCodes     string            `json:"status_codes,omitempty"`
	MessageContains []string          `json:"message_contains,omitempty"`
}

type RetryPolicyTargets struct {
	Groups      []string `json:"groups,omitempty"`
	ChannelIDs  []int    `json:"channel_ids,omitempty"`
	ChannelTags []string `json:"channel_tags,omitempty"`
	Model       string   `json:"model,omitempty"`
}

type RetryPolicyStrategy struct {
	MaxRetries           int   `json:"max_retries,omitempty"`
	ExcludeFailedChannel bool  `json:"exclude_failed_channel,omitempty"`
	PreferHealthy        bool  `json:"prefer_healthy,omitempty"`
	ProtectLast          bool  `json:"protect_last,omitempty"`
	RecordRequestLog     *bool `json:"record_request_log,omitempty"`
	SampleRate           int   `json:"sample_rate,omitempty"`
}

type RetryPolicyInput struct {
	ModelName    string
	Group        string
	RequestPath  string
	IsStream     bool
	TokenID      int
	WorkspaceID  int
	ChannelID    int
	ChannelType  int
	ErrorType    types.ErrorType
	ErrorCode    types.ErrorCode
	StatusCode   int
	ErrorMessage string
	ChannelRules []RetryPolicyRule
}

type RetryPolicyDecision struct {
	Matched          bool
	ShouldRetry      bool
	Action           string
	Source           string
	RuleName         string
	RetryGroups      []string
	MaxRetries       int
	Targets          RetryPolicyTargets
	TargetChannelIDs []int
	TargetTags       []string
	TargetModel      string
	ExcludeChannelID int
}

var AutomaticRetryPolicyRules []RetryPolicyRule

func AutomaticRetryPolicyRulesToString() string {
	if len(AutomaticRetryPolicyRules) == 0 {
		return "[]"
	}
	bytes, err := common.Marshal(AutomaticRetryPolicyRules)
	if err != nil {
		return "[]"
	}
	return string(bytes)
}

func AutomaticRetryPolicyRulesFromString(s string) error {
	rules, err := ParseRetryPolicyRules(s)
	if err != nil {
		return err
	}
	AutomaticRetryPolicyRules = rules
	return nil
}

func ParseRetryPolicyRules(s string) ([]RetryPolicyRule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var rules []RetryPolicyRule
	if err := common.UnmarshalJsonStr(s, &rules); err != nil {
		return nil, fmt.Errorf("retry policy rules must be a JSON array: %w", err)
	}
	for i, rule := range rules {
		if err := ValidateRetryPolicyRule(rule); err != nil {
			return nil, fmt.Errorf("retry policy rule %d invalid: %w", i+1, err)
		}
	}
	return rules, nil
}

func ValidateRetryPolicyRules(rules []RetryPolicyRule) error {
	for i, rule := range rules {
		if err := ValidateRetryPolicyRule(rule); err != nil {
			return fmt.Errorf("retry policy rule %d invalid: %w", i+1, err)
		}
	}
	return nil
}

func ValidateRetryPolicyRulesJSON(s string) error {
	_, err := ParseRetryPolicyRules(s)
	return err
}

func ValidateRetryPolicyRule(rule RetryPolicyRule) error {
	action := normalizeRetryPolicyAction(rule.Action)
	if action == "" {
		return fmt.Errorf("action must be %q, %q, or %q", RetryPolicyActionRetry, RetryPolicyActionFailover, RetryPolicyActionSkipRetry)
	}
	maxRetries := effectiveRetryPolicyMaxRetries(rule)
	if maxRetries < 0 {
		return fmt.Errorf("max_retries must be greater than or equal to 0")
	}
	targets := normalizedRetryPolicyTargets(rule)
	if action == RetryPolicyActionSkipRetry && (len(cleanRetryGroups(rule.RetryGroups)) > 0 || maxRetries > 0 || !targets.empty()) {
		return fmt.Errorf("retry targets and max_retries are only valid for retry or failover rules")
	}
	if action == RetryPolicyActionFailover && len(targets.Groups) == 0 && len(targets.ChannelIDs) == 0 && len(targets.ChannelTags) == 0 {
		return fmt.Errorf("failover rules must configure at least one channel target")
	}
	conditions := rule.normalizedConditions()
	if conditions.StatusCodes != "" {
		if _, err := ParseHTTPStatusCodeRanges(conditions.StatusCodes); err != nil {
			return err
		}
	}
	return nil
}

func ShouldRetryByPolicy(input RetryPolicyInput) RetryPolicyDecision {
	if decision, ok := matchRetryPolicyRules(input, input.ChannelRules, RetryPolicySourceChannel); ok {
		return decision
	}
	if decision, ok := matchRetryPolicyRules(input, AutomaticRetryPolicyRules, RetryPolicySourceGlobal); ok {
		return decision
	}
	return RetryPolicyDecision{}
}

func matchRetryPolicyRules(input RetryPolicyInput, rules []RetryPolicyRule, source string) (RetryPolicyDecision, bool) {
	orderedRules := orderedRetryPolicyRules(rules)
	for _, rule := range orderedRules {
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		if normalizeRetryPolicyAction(rule.Action) != RetryPolicyActionSkipRetry || !rule.matches(input) {
			continue
		}
		return buildRetryPolicyDecision(input, rule, source), true
	}
	for _, rule := range orderedRules {
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		if !rule.matches(input) {
			continue
		}
		return buildRetryPolicyDecision(input, rule, source), true
	}
	return RetryPolicyDecision{}, false
}

func buildRetryPolicyDecision(input RetryPolicyInput, rule RetryPolicyRule, source string) RetryPolicyDecision {
	action := normalizeRetryPolicyAction(rule.Action)
	targets := normalizedRetryPolicyTargets(rule)
	maxRetries := effectiveRetryPolicyMaxRetries(rule)
	retryGroups := cleanRetryGroups(rule.RetryGroups)
	if len(targets.Groups) > 0 {
		retryGroups = targets.Groups
	}
	excludeChannelID := 0
	if action == RetryPolicyActionFailover {
		excludeChannelID = input.ChannelID
	}
	return RetryPolicyDecision{
		Matched:          true,
		ShouldRetry:      action == RetryPolicyActionRetry || action == RetryPolicyActionFailover,
		Action:           action,
		Source:           source,
		RuleName:         rule.Name,
		RetryGroups:      retryGroups,
		MaxRetries:       maxRetries,
		Targets:          targets,
		TargetChannelIDs: targets.ChannelIDs,
		TargetTags:       targets.ChannelTags,
		TargetModel:      strings.TrimSpace(targets.Model),
		ExcludeChannelID: excludeChannelID,
	}
}

func orderedRetryPolicyRules(rules []RetryPolicyRule) []RetryPolicyRule {
	if len(rules) <= 1 {
		return rules
	}
	ordered := append([]RetryPolicyRule(nil), rules...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Priority > ordered[j].Priority
	})
	return ordered
}

func cleanRetryGroups(groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	clean := make([]string, 0, len(groups))
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		clean = append(clean, group)
	}
	return clean
}

func normalizedRetryPolicyTargets(rule RetryPolicyRule) RetryPolicyTargets {
	targets := rule.Targets
	if len(targets.Groups) == 0 {
		targets.Groups = rule.RetryGroups
	}
	targets.Groups = cleanRetryGroups(targets.Groups)
	targets.ChannelIDs = cleanRetryPolicyInts(targets.ChannelIDs)
	targets.ChannelTags = cleanRetryPolicyStrings(targets.ChannelTags)
	targets.Model = strings.TrimSpace(targets.Model)
	return targets
}

func effectiveRetryPolicyMaxRetries(rule RetryPolicyRule) int {
	if rule.Strategy.MaxRetries > 0 {
		return rule.Strategy.MaxRetries
	}
	return rule.MaxRetries
}

func (targets RetryPolicyTargets) empty() bool {
	return len(targets.Groups) == 0 && len(targets.ChannelIDs) == 0 && len(targets.ChannelTags) == 0 && strings.TrimSpace(targets.Model) == ""
}

func cleanRetryPolicyInts(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	clean := make([]int, 0, len(values))
	seen := make(map[int]bool, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		clean = append(clean, value)
	}
	return clean
}

func cleanRetryPolicyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clean := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		clean = append(clean, value)
	}
	return clean
}

func (r RetryPolicyRule) matches(input RetryPolicyInput) bool {
	conditions := r.normalizedConditions()
	if len(conditions.Models) > 0 && !containsFold(conditions.Models, input.ModelName) {
		return false
	}
	if len(conditions.Groups) > 0 && !containsFold(conditions.Groups, input.Group) {
		return false
	}
	if len(conditions.RequestPaths) > 0 && !containsString(conditions.RequestPaths, input.RequestPath) {
		return false
	}
	if conditions.Stream != nil && *conditions.Stream != input.IsStream {
		return false
	}
	if len(conditions.TokenIDs) > 0 && !containsInt(conditions.TokenIDs, input.TokenID) {
		return false
	}
	if len(conditions.WorkspaceIDs) > 0 && !containsInt(conditions.WorkspaceIDs, input.WorkspaceID) {
		return false
	}
	if len(conditions.ChannelIDs) > 0 && !containsInt(conditions.ChannelIDs, input.ChannelID) {
		return false
	}
	if len(conditions.ChannelTypes) > 0 && !containsInt(conditions.ChannelTypes, input.ChannelType) {
		return false
	}
	if len(conditions.ErrorTypes) > 0 && !containsErrorType(conditions.ErrorTypes, input.ErrorType) {
		return false
	}
	if len(conditions.ErrorCodes) > 0 && !containsErrorCode(conditions.ErrorCodes, input.ErrorCode) {
		return false
	}
	if strings.TrimSpace(conditions.StatusCodes) != "" && !matchesStatusCode(conditions.StatusCodes, input.StatusCode) {
		return false
	}
	if len(conditions.MessageContains) > 0 && !messageContainsAny(input.ErrorMessage, conditions.MessageContains) {
		return false
	}
	return true
}

func (r RetryPolicyRule) normalizedConditions() RetryPolicyConditions {
	conditions := r.Conditions
	if len(conditions.Models) == 0 {
		conditions.Models = r.Models
	}
	if len(conditions.Groups) == 0 {
		conditions.Groups = r.Groups
	}
	if len(conditions.RequestPaths) == 0 {
		conditions.RequestPaths = r.RequestPaths
	}
	if conditions.Stream == nil {
		conditions.Stream = r.Stream
	}
	if len(conditions.TokenIDs) == 0 {
		conditions.TokenIDs = r.TokenIDs
	}
	if len(conditions.WorkspaceIDs) == 0 {
		conditions.WorkspaceIDs = r.WorkspaceIDs
	}
	if len(conditions.ChannelIDs) == 0 {
		conditions.ChannelIDs = r.ChannelIDs
	}
	if len(conditions.ChannelTypes) == 0 {
		conditions.ChannelTypes = r.ChannelTypes
	}
	if len(conditions.ErrorTypes) == 0 {
		conditions.ErrorTypes = r.ErrorTypes
	}
	if len(conditions.ErrorCodes) == 0 {
		conditions.ErrorCodes = r.ErrorCodes
	}
	if strings.TrimSpace(conditions.StatusCodes) == "" {
		conditions.StatusCodes = r.StatusCodes
	}
	if len(conditions.MessageContains) == 0 {
		conditions.MessageContains = r.MessageContains
	}
	return conditions
}

func normalizeRetryPolicyAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case RetryPolicyActionRetry:
		return RetryPolicyActionRetry
	case RetryPolicyActionFailover:
		return RetryPolicyActionFailover
	case RetryPolicyActionSkipRetry:
		return RetryPolicyActionSkipRetry
	default:
		return ""
	}
}

func matchesStatusCode(raw string, code int) bool {
	ranges, err := ParseHTTPStatusCodeRanges(raw)
	if err != nil {
		return false
	}
	return shouldMatchStatusCodeRanges(ranges, code)
}

func containsFold(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsErrorType(values []types.ErrorType, target types.ErrorType) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsErrorCode(values []types.ErrorCode, target types.ErrorCode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func messageContainsAny(message string, needles []string) bool {
	message = strings.ToLower(message)
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle == "" {
			continue
		}
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func RetryPolicyRuleSummary(rule RetryPolicyRule) string {
	if rule.Name != "" {
		return rule.Name
	}
	conditions := rule.normalizedConditions()
	parts := make([]string, 0, 4)
	if len(conditions.Models) > 0 {
		parts = append(parts, "models="+strings.Join(conditions.Models, "|"))
	}
	if conditions.StatusCodes != "" {
		parts = append(parts, "status="+conditions.StatusCodes)
	}
	if len(conditions.MessageContains) > 0 {
		parts = append(parts, "contains="+strings.Join(conditions.MessageContains, "|"))
	}
	if len(conditions.ChannelIDs) > 0 {
		ids := make([]string, 0, len(conditions.ChannelIDs))
		for _, id := range conditions.ChannelIDs {
			ids = append(ids, strconv.Itoa(id))
		}
		parts = append(parts, "channels="+strings.Join(ids, "|"))
	}
	return strings.Join(parts, ",")
}
