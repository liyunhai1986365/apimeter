package model_setting

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ModelFallbackModeInherit  = "inherit"
	ModelFallbackModeCustom   = "custom"
	ModelFallbackModeDisabled = "disabled"
)

type ModelFallbackRule struct {
	PrimaryModel  string `json:"primary_model"`
	FallbackModel string `json:"fallback_model"`
	Enabled       bool   `json:"enabled"`
}

type ModelFallbackSetting struct {
	Enabled            bool                `json:"enabled"`
	AllowUserOverride  bool                `json:"allow_user_override"`
	FailureStatusCodes string              `json:"failure_status_codes"`
	Rules              []ModelFallbackRule `json:"rules"`
}

type UserModelFallbackSetting struct {
	Mode    string              `json:"mode,omitempty"`
	Enabled bool                `json:"enabled,omitempty"`
	Rules   []ModelFallbackRule `json:"rules,omitempty"`
}

var modelFallbackSetting = ModelFallbackSetting{
	Enabled:            false,
	AllowUserOverride:  true,
	FailureStatusCodes: defaultModelFallbackFailureStatusCodes(),
	Rules:              []ModelFallbackRule{},
}

type StatusCodeRange struct {
	Start int
	End   int
}

func init() {
	config.GlobalConfig.Register("model_fallback", &modelFallbackSetting)
}

func GetModelFallbackSetting() *ModelFallbackSetting {
	return &modelFallbackSetting
}

func NormalizeModelFallbackMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ModelFallbackModeCustom:
		return ModelFallbackModeCustom
	case ModelFallbackModeDisabled:
		return ModelFallbackModeDisabled
	default:
		return ModelFallbackModeInherit
	}
}

func FindModelFallbackRule(rules []ModelFallbackRule, primaryModel string) (ModelFallbackRule, bool) {
	primaryModel = strings.TrimSpace(primaryModel)
	if primaryModel == "" {
		return ModelFallbackRule{}, false
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if strings.TrimSpace(rule.PrimaryModel) != primaryModel {
			continue
		}
		fallbackModel := strings.TrimSpace(rule.FallbackModel)
		if fallbackModel == "" || fallbackModel == primaryModel {
			continue
		}
		rule.PrimaryModel = primaryModel
		rule.FallbackModel = fallbackModel
		return rule, true
	}
	return ModelFallbackRule{}, false
}

func ValidateModelFallbackRulesJSON(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "[]"
	}
	var rules []ModelFallbackRule
	if err := common.UnmarshalJsonStr(raw, &rules); err != nil {
		return fmt.Errorf("模型备选规则必须是 JSON 数组: %w", err)
	}
	for index, rule := range rules {
		if err := ValidateModelFallbackRule(rule); err != nil {
			return fmt.Errorf("第 %d 条模型备选规则无效: %w", index+1, err)
		}
	}
	return nil
}

func ValidateModelFallbackFailureStatusCodes(raw string) error {
	_, err := ParseHTTPStatusCodeRanges(raw)
	return err
}

func ShouldFallbackByStatusCode(code int) bool {
	if code == 504 || code == 524 {
		return false
	}
	ranges, err := ParseHTTPStatusCodeRanges(modelFallbackSetting.FailureStatusCodes)
	if err != nil || len(ranges) == 0 {
		ranges, _ = ParseHTTPStatusCodeRanges(defaultModelFallbackFailureStatusCodes())
	}
	return shouldMatchStatusCodeRanges(ranges, code)
}

func ParseHTTPStatusCodeRanges(input string) ([]StatusCodeRange, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	input = strings.NewReplacer("，", ",").Replace(input)
	segments := strings.Split(input, ",")

	var ranges []StatusCodeRange
	var invalid []string

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		statusCodeRange, err := parseHTTPStatusCodeToken(segment)
		if err != nil {
			invalid = append(invalid, segment)
			continue
		}
		ranges = append(ranges, statusCodeRange)
	}

	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid http status code rules: %s", strings.Join(invalid, ", "))
	}
	if len(ranges) == 0 {
		return nil, nil
	}

	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Start == ranges[j].Start {
			return ranges[i].End < ranges[j].End
		}
		return ranges[i].Start < ranges[j].Start
	})

	merged := []StatusCodeRange{ranges[0]}
	for _, statusCodeRange := range ranges[1:] {
		last := &merged[len(merged)-1]
		if statusCodeRange.Start <= last.End+1 {
			if statusCodeRange.End > last.End {
				last.End = statusCodeRange.End
			}
			continue
		}
		merged = append(merged, statusCodeRange)
	}

	return merged, nil
}

func parseHTTPStatusCodeToken(token string) (StatusCodeRange, error) {
	token = strings.TrimSpace(token)
	token = strings.ReplaceAll(token, " ", "")
	if token == "" {
		return StatusCodeRange{}, fmt.Errorf("empty token")
	}

	if strings.Contains(token, "-") {
		parts := strings.Split(token, "-")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return StatusCodeRange{}, fmt.Errorf("invalid range")
		}
		start, startErr := strconv.Atoi(parts[0])
		end, endErr := strconv.Atoi(parts[1])
		if startErr != nil || endErr != nil || !isValidHTTPStatusCode(start) || !isValidHTTPStatusCode(end) || start > end {
			return StatusCodeRange{}, fmt.Errorf("invalid range")
		}
		return StatusCodeRange{Start: start, End: end}, nil
	}

	code, err := strconv.Atoi(token)
	if err != nil || !isValidHTTPStatusCode(code) {
		return StatusCodeRange{}, fmt.Errorf("invalid status code")
	}
	return StatusCodeRange{Start: code, End: code}, nil
}

func isValidHTTPStatusCode(code int) bool {
	return code >= 100 && code <= 599
}

func shouldMatchStatusCodeRanges(ranges []StatusCodeRange, code int) bool {
	if !isValidHTTPStatusCode(code) {
		return false
	}
	for _, statusCodeRange := range ranges {
		if code < statusCodeRange.Start {
			return false
		}
		if code <= statusCodeRange.End {
			return true
		}
	}
	return false
}

func defaultModelFallbackFailureStatusCodes() string {
	return "100-199,300-399,401-407,409-499,500-503,505-523,525-599"
}

func ValidateModelFallbackRule(rule ModelFallbackRule) error {
	primaryModel := strings.TrimSpace(rule.PrimaryModel)
	fallbackModel := strings.TrimSpace(rule.FallbackModel)
	if primaryModel == "" {
		return fmt.Errorf("主模型不能为空")
	}
	if fallbackModel == "" {
		return fmt.Errorf("备选模型不能为空")
	}
	if primaryModel == fallbackModel {
		return fmt.Errorf("主模型和备选模型不能相同")
	}
	return nil
}

func ResolveModelFallbackRule(primaryModel string, userSetting UserModelFallbackSetting) (ModelFallbackRule, string, bool) {
	global := GetModelFallbackSetting()
	mode := NormalizeModelFallbackMode(userSetting.Mode)

	if mode == ModelFallbackModeDisabled {
		return ModelFallbackRule{}, mode, false
	}

	if global.AllowUserOverride && mode == ModelFallbackModeCustom {
		if !userSetting.Enabled {
			return ModelFallbackRule{}, mode, false
		}
		rule, ok := FindModelFallbackRule(userSetting.Rules, primaryModel)
		return rule, mode, ok
	}

	if !global.Enabled {
		return ModelFallbackRule{}, ModelFallbackModeInherit, false
	}
	rule, ok := FindModelFallbackRule(global.Rules, primaryModel)
	return rule, ModelFallbackModeInherit, ok
}
