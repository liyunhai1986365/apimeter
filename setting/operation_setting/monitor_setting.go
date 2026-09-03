package operation_setting

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled           bool                     `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes           float64                  `json:"auto_test_channel_minutes"`
	ChannelTestMode                  string                   `json:"channel_test_mode"`
	ChannelTestConcurrency           int                      `json:"channel_test_concurrency"`
	ChannelAutoOperationEnabled      bool                     `json:"channel_auto_operation_enabled"`
	ChannelAutoOperationThreshold    int                      `json:"channel_auto_operation_threshold"`
	ChannelAutoOperationWindowMins   int                      `json:"channel_auto_operation_window_minutes"`
	ChannelAutoOperationMinRequests  int                      `json:"channel_auto_operation_min_requests"`
	ChannelAutoOperationErrorRate    float64                  `json:"channel_auto_operation_error_rate"`
	ChannelAutoOperationProtectLast  bool                     `json:"channel_auto_operation_protect_last"`
	ChannelAutoDisableRules          []ChannelAutoDisableRule `json:"channel_auto_disable_rules"`
	ChannelAutoEnableCheckMinutes    int                      `json:"channel_auto_enable_check_minutes"`
	ChannelAutoEnableCooldownMinutes int                      `json:"channel_auto_enable_cooldown_minutes"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"

	ChannelTestConcurrencyOptionKey = "monitor_setting.channel_test_concurrency"
	DefaultChannelTestConcurrency   = 1
	MaxChannelTestConcurrency       = 32
)

type ChannelAutoDisableRule struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Enabled                  bool     `json:"enabled"`
	ModelNames               []string `json:"model_names"`
	ErrorTypes               []string `json:"error_types"`
	StatusCodes              string   `json:"status_codes"`
	FirstTokenSeconds        int      `json:"first_token_seconds"`
	FirstTokenCountThreshold int      `json:"first_token_count_threshold"`
	WindowMinutes            int      `json:"window_minutes"`
	MinRequests              int      `json:"min_requests"`
	ErrorCountThreshold      int      `json:"error_count_threshold"`
	ErrorRate                float64  `json:"error_rate"`
	PerMinuteErrorThreshold  int      `json:"per_minute_error_threshold"`
	ProtectLast              bool     `json:"protect_last"`
}

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:           false,
	AutoTestChannelMinutes:           10,
	ChannelTestMode:                  ChannelTestModeScheduledAll,
	ChannelTestConcurrency:           DefaultChannelTestConcurrency,
	ChannelAutoOperationEnabled:      false,
	ChannelAutoOperationThreshold:    3,
	ChannelAutoOperationWindowMins:   10,
	ChannelAutoOperationMinRequests:  5,
	ChannelAutoOperationErrorRate:    50,
	ChannelAutoOperationProtectLast:  true,
	ChannelAutoEnableCheckMinutes:    5,
	ChannelAutoEnableCooldownMinutes: 1,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	switch monitorSetting.ChannelTestMode {
	case ChannelTestModeAutoBanOnly, ChannelTestModePassiveRecovery:
	default:
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	monitorSetting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(monitorSetting.ChannelTestConcurrency)
	return &monitorSetting
}

func NormalizeChannelTestConcurrency(concurrency int) int {
	if concurrency < 1 {
		return DefaultChannelTestConcurrency
	}
	if concurrency > MaxChannelTestConcurrency {
		return MaxChannelTestConcurrency
	}
	return concurrency
}

func ValidateChannelTestConcurrency(value string) error {
	concurrency, err := strconv.Atoi(value)
	if err != nil || concurrency < 1 || concurrency > MaxChannelTestConcurrency {
		return fmt.Errorf("channel test concurrency must be between 1 and %d", MaxChannelTestConcurrency)
	}
	return nil
}

func ValidateChannelAutoDisableRules(rules []ChannelAutoDisableRule) error {
	seen := map[string]struct{}{}
	for i, rule := range rules {
		prefix := fmt.Sprintf("rule %d", i+1)
		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("%s id is required", prefix)
		}
		if _, ok := seen[rule.ID]; ok {
			return fmt.Errorf("%s id is duplicated", prefix)
		}
		seen[rule.ID] = struct{}{}
		if !rule.Enabled {
			continue
		}
		if rule.WindowMinutes <= 0 {
			return fmt.Errorf("%s window_minutes must be greater than 0", prefix)
		}
		if rule.MinRequests < 0 || rule.ErrorCountThreshold < 0 || rule.PerMinuteErrorThreshold < 0 || rule.FirstTokenSeconds < 0 || rule.FirstTokenCountThreshold < 0 {
			return fmt.Errorf("%s numeric thresholds cannot be negative", prefix)
		}
		if rule.ErrorRate < 0 || rule.ErrorRate > 100 {
			return fmt.Errorf("%s error_rate must be between 0 and 100", prefix)
		}
		if rule.FirstTokenCountThreshold > 0 && rule.FirstTokenSeconds <= 0 {
			return fmt.Errorf("%s first_token_seconds must be greater than 0 when first_token_count_threshold is configured", prefix)
		}
		if strings.TrimSpace(rule.StatusCodes) != "" {
			if _, err := ParseHTTPStatusCodeRanges(rule.StatusCodes); err != nil {
				return fmt.Errorf("%s status_codes invalid: %w", prefix, err)
			}
		}
		if rule.ErrorCountThreshold == 0 && rule.ErrorRate == 0 && rule.PerMinuteErrorThreshold == 0 && rule.FirstTokenCountThreshold == 0 {
			return fmt.Errorf("%s must configure at least one trigger threshold", prefix)
		}
	}
	return nil
}
