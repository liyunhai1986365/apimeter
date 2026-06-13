package model

import (
	"fmt"
	"strings"
)

func billingLogAmount(log Log) (lineType string, amount int64, ok bool) {
	switch log.Type {
	case LogTypeTopup:
		return AccountLedgerEntryTypeTopup, int64(log.Quota), true
	case LogTypeConsume:
		return AccountLedgerEntryTypeConsume, -int64(log.Quota), true
	case LogTypeRefund:
		return AccountLedgerEntryTypeRefund, int64(log.Quota), true
	case LogTypeManage:
		if log.Quota == 0 {
			return "", 0, false
		}
		return AccountLedgerEntryTypeAdjustment, int64(log.Quota), true
	default:
		return "", 0, false
	}
}

func billingValueString(values map[string]interface{}, key string, fallback string) string {
	if values == nil {
		return fallback
	}
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func billingValueFloat(values map[string]interface{}, key string, fallback float64) float64 {
	if values == nil {
		return fallback
	}
	switch value := values[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func billingValueInt(values map[string]interface{}, key string, fallback int) int {
	if values == nil {
		return fallback
	}
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case float32:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}
