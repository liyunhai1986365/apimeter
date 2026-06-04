package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type WebhookSetting struct {
	Enabled                  bool    `json:"enabled"`
	URL                      string  `json:"url"`
	Secret                   string  `json:"secret"`
	IntervalMinutes          int     `json:"interval_minutes"`
	SuppressMinutes          int     `json:"suppress_minutes"`
	NotifyOnEmptyResult      bool    `json:"notify_on_empty_result"`
	ModelErrorCheckEnabled   bool    `json:"model_error_check_enabled"`
	ModelErrorWindowMinutes  int     `json:"model_error_window_minutes"`
	ModelErrorThreshold      int     `json:"model_error_threshold"`
	ModelErrorMinRequests    int     `json:"model_error_min_requests"`
	ModelErrorRate           float64 `json:"model_error_rate"`
	ChannelTestCheckEnabled  bool    `json:"channel_test_check_enabled"`
	ChannelTestWindowMinutes int     `json:"channel_test_window_minutes"`
}

var webhookSetting = WebhookSetting{
	Enabled:                  false,
	URL:                      "",
	Secret:                   "",
	IntervalMinutes:          30,
	SuppressMinutes:          30,
	NotifyOnEmptyResult:      false,
	ModelErrorCheckEnabled:   true,
	ModelErrorWindowMinutes:  10,
	ModelErrorThreshold:      3,
	ModelErrorMinRequests:    5,
	ModelErrorRate:           50,
	ChannelTestCheckEnabled:  true,
	ChannelTestWindowMinutes: 60,
}

func init() {
	config.GlobalConfig.Register("webhook_setting", &webhookSetting)
}

func GetWebhookSetting() *WebhookSetting {
	return &webhookSetting
}
