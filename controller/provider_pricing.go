package controller

import (
	"crypto/hmac"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const (
	hvoyProviderPricingSchemaVersion = "1.1"
	hvoyProviderPricingTokenUnit     = "per_1m_tokens"
	hvoyProviderPricingCallUnit      = "per_call"
	hvoySignatureMaxAgeSeconds       = int64(60)
	claudeCacheCreation1hMultiplier  = 6.0 / 3.75
)

type hvoyProviderPricingResponse struct {
	SchemaVersion string                   `json:"schema_version"`
	Success       bool                     `json:"success"`
	Message       string                   `json:"message,omitempty"`
	Data          *hvoyProviderPricingData `json:"data,omitempty"`
}

type hvoyProviderPricingData struct {
	Currency   string                     `json:"currency"`
	PriceUnit  string                     `json:"price_unit"`
	SiteName   string                     `json:"site_name,omitempty"`
	SiteDomain string                     `json:"site_domain,omitempty"`
	UpdatedAt  string                     `json:"updated_at"`
	Models     []hvoyProviderPricingModel `json:"models"`
}

type hvoyProviderPricingModel struct {
	ModelName          string   `json:"model_name"`
	GroupName          string   `json:"group_name"`
	PriceUnit          string   `json:"price_unit,omitempty"`
	InputPrice         *float64 `json:"input_price,omitempty"`
	OutputPrice        *float64 `json:"output_price,omitempty"`
	CacheInputPrice    *float64 `json:"cache_input_price,omitempty"`
	CacheCreatePrice   *float64 `json:"cache_create_price,omitempty"`
	CacheCreatePrice1h *float64 `json:"cache_create_price_1h,omitempty"`
	UnitPrice          *float64 `json:"unit_price,omitempty"`
	Enabled            bool     `json:"enabled"`
	Note               string   `json:"note"`
}

func GetHvoyProviderPricing(c *gin.Context) {
	if err := verifyHvoyProviderPricingSignature(c.Request.Header, common.HvoyAuthSecret, time.Now()); err != nil {
		c.JSON(http.StatusUnauthorized, hvoyProviderPricingResponse{
			SchemaVersion: hvoyProviderPricingSchemaVersion,
			Success:       false,
			Message:       err.Error(),
		})
		return
	}

	// Price is the amount of CNY charged for one system USD of credit, so it
	// converts the final internal model cost into Hvoy's required currency.
	cnyPerUSD := operation_setting.Price
	if !isFiniteNonNegative(cnyPerUSD) || cnyPerUSD == 0 {
		cnyPerUSD = operation_setting.USDExchangeRate
	}
	if !isFiniteNonNegative(cnyPerUSD) || cnyPerUSD == 0 {
		cnyPerUSD = 1
	}

	usableGroups := service.GetUserUsableGroups("")
	pricing := filterPricingByUsableGroups(model.GetPricing(), usableGroups)
	groupRatios := ratio_setting.GetGroupRatioCopy()
	for group := range groupRatios {
		if _, ok := usableGroups[group]; !ok {
			delete(groupRatios, group)
		}
	}

	response := buildHvoyProviderPricingResponse(
		pricing,
		groupRatios,
		ratio_setting.GetGroupModelRatio,
		cnyPerUSD,
		common.SystemName,
		hvoyProviderPricingSiteDomain(c.Request),
		time.Now(),
	)
	c.JSON(http.StatusOK, response)
}

func verifyHvoyProviderPricingSignature(header http.Header, secret string, now time.Time) error {
	if secret == "" {
		return nil
	}

	timestampText := strings.TrimSpace(header.Get("X-Hvoy-Ts"))
	signature := strings.TrimSpace(header.Get("X-Hvoy-Sign"))
	if timestampText == "" || signature == "" {
		return errors.New("missing Hvoy signature headers")
	}

	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil {
		return errors.New("invalid Hvoy timestamp")
	}
	nowUnix := now.Unix()
	if timestamp < nowUnix-hvoySignatureMaxAgeSeconds || timestamp > nowUnix+hvoySignatureMaxAgeSeconds {
		return errors.New("expired Hvoy timestamp")
	}

	expected := common.GenerateHMACWithKey([]byte(secret), timestampText)
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(signature))) {
		return errors.New("invalid Hvoy signature")
	}
	return nil
}

func buildHvoyProviderPricingResponse(
	pricing []model.Pricing,
	groupRatios map[string]float64,
	resolveGroupModelRatio func(group, modelName string) (float64, bool),
	cnyPerUSD float64,
	siteName string,
	siteDomain string,
	updatedAt time.Time,
) hvoyProviderPricingResponse {
	models := make([]hvoyProviderPricingModel, 0)
	if !isFiniteNonNegative(cnyPerUSD) {
		cnyPerUSD = 0
	}
	for _, item := range pricing {
		// Hvoy currently accepts only one fixed price per model and group. A
		// tiered/request-aware expression cannot be represented truthfully.
		if item.BillingMode == "tiered_expr" && strings.TrimSpace(item.BillingExpr) != "" {
			continue
		}
		if strings.TrimSpace(item.ModelName) == "" {
			continue
		}

		groups := expandHvoyProviderPricingGroups(item.EnableGroup, groupRatios)
		for _, group := range groups {
			groupRatio := 1.0
			if configuredRatio, ok := groupRatios[group]; ok {
				groupRatio = configuredRatio
			}
			if resolveGroupModelRatio != nil {
				if configuredRatio, ok := resolveGroupModelRatio(group, item.ModelName); ok {
					groupRatio = configuredRatio
				}
			}
			if !isFiniteNonNegative(groupRatio) {
				continue
			}

			priceScale := groupRatio * cnyPerUSD
			if !isFiniteNonNegative(priceScale) {
				continue
			}
			if item.QuotaType == 1 {
				unitPriceRaw := item.ModelPrice * priceScale
				if !isFiniteNonNegative(unitPriceRaw) || unitPriceRaw == 0 {
					continue
				}
				unitPrice := roundedHvoyPrice(unitPriceRaw)
				if unitPrice == 0 {
					continue
				}
				models = append(models, hvoyProviderPricingModel{
					ModelName: item.ModelName,
					GroupName: group,
					PriceUnit: hvoyProviderPricingCallUnit,
					UnitPrice: float64Pointer(unitPrice),
					Enabled:   true,
					Note:      "",
				})
				continue
			}

			inputPriceRaw := item.ModelRatio * 2 * priceScale
			outputPriceRaw := item.ModelRatio * 2 * item.CompletionRatio * priceScale
			if !isFiniteNonNegative(inputPriceRaw) || !isFiniteNonNegative(outputPriceRaw) {
				continue
			}
			inputPrice := roundedHvoyPrice(inputPriceRaw)
			outputPrice := roundedHvoyPrice(outputPriceRaw)
			entry := hvoyProviderPricingModel{
				ModelName:   item.ModelName,
				GroupName:   group,
				InputPrice:  float64Pointer(inputPrice),
				OutputPrice: float64Pointer(outputPrice),
				Enabled:     true,
				Note:        "",
			}
			if item.CacheRatio != nil {
				entry.CacheInputPrice = validHvoyPricePointer(inputPrice * *item.CacheRatio)
			}
			if item.CreateCacheRatio != nil {
				entry.CacheCreatePrice = validHvoyPricePointer(inputPrice * *item.CreateCacheRatio)
				if isClaudeModelName(item.ModelName) {
					entry.CacheCreatePrice1h = validHvoyPricePointer(inputPrice * *item.CreateCacheRatio * claudeCacheCreation1hMultiplier)
				}
			}
			models = append(models, entry)
		}
	}

	return hvoyProviderPricingResponse{
		SchemaVersion: hvoyProviderPricingSchemaVersion,
		Success:       true,
		Data: &hvoyProviderPricingData{
			Currency:   "CNY",
			PriceUnit:  hvoyProviderPricingTokenUnit,
			SiteName:   strings.TrimSpace(siteName),
			SiteDomain: strings.TrimSpace(siteDomain),
			UpdatedAt:  updatedAt.UTC().Format(time.RFC3339),
			Models:     models,
		},
	}
}

func expandHvoyProviderPricingGroups(groups []string, groupRatios map[string]float64) []string {
	seen := make(map[string]struct{})
	includeAll := false
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "all" {
			includeAll = true
			continue
		}
		if group != "" {
			seen[group] = struct{}{}
		}
	}
	if includeAll {
		for group := range groupRatios {
			group = strings.TrimSpace(group)
			if group != "" && group != "all" {
				seen[group] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for group := range seen {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func hvoyProviderPricingSiteDomain(request *http.Request) string {
	host := strings.TrimSpace(request.Host)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if host != "" {
		return host
	}
	if parsed, err := url.Parse(system_setting.ServerAddress); err == nil {
		return parsed.Hostname()
	}
	return ""
}

func isClaudeModelName(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "claude")
}

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func roundedHvoyPrice(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func validHvoyPricePointer(value float64) *float64 {
	if !isFiniteNonNegative(value) {
		return nil
	}
	value = roundedHvoyPrice(value)
	return float64Pointer(value)
}

func float64Pointer(value float64) *float64 {
	return &value
}
