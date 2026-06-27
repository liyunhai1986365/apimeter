package routing_strategy_setting

import "github.com/QuantumNous/new-api/setting/config"

type RoutingStrategySetting struct {
	Enabled               bool    `json:"enabled"`
	UpdateIntervalMinutes int     `json:"update_interval_minutes"`
	WindowHours           int     `json:"window_hours"`
	MinRequestCount       int64   `json:"min_request_count"`
	SmartPriceWeight      float64 `json:"smart_price_weight"`
	SmartSpeedWeight      float64 `json:"smart_speed_weight"`
	SmartSuccessWeight    float64 `json:"smart_success_weight"`
	ExcludedGroups        string  `json:"excluded_groups"`
	PinnedGroups          string  `json:"pinned_groups"`
}

var routingStrategySetting = RoutingStrategySetting{
	Enabled:               true,
	UpdateIntervalMinutes: 10,
	WindowHours:           24,
	MinRequestCount:       3,
	SmartPriceWeight:      0.4,
	SmartSpeedWeight:      0.25,
	SmartSuccessWeight:    0.35,
	ExcludedGroups:        "",
	PinnedGroups:          "",
}

func init() {
	config.GlobalConfig.Register("routing_strategy_setting", &routingStrategySetting)
}

func GetSetting() RoutingStrategySetting {
	return routingStrategySetting
}

func GetUpdateIntervalMinutes() int {
	if routingStrategySetting.UpdateIntervalMinutes <= 0 {
		return 10
	}
	return routingStrategySetting.UpdateIntervalMinutes
}

func GetWindowHours() int {
	if routingStrategySetting.WindowHours <= 0 {
		return 24
	}
	return routingStrategySetting.WindowHours
}
