package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 上游地址
const (
	upstreamModelsURL  = "https://basellm.github.io/llm-metadata/api/newapi/models.json"
	upstreamVendorsURL = "https://basellm.github.io/llm-metadata/api/newapi/vendors.json"
	aihubmixSource     = "aihubmix"
	aihubmixModelsURL  = "https://aihubmix.com/call/mdl_info_pagination"
)

func normalizeLocale(locale string) (string, bool) {
	l := strings.ToLower(strings.TrimSpace(locale))
	switch l {
	case "en", "zh-CN", "zh-TW", "ja":
		return l, true
	default:
		return "", false
	}
}

func getUpstreamBase() string {
	return common.GetEnvOrDefaultString("SYNC_UPSTREAM_BASE", "https://basellm.github.io/llm-metadata")
}

func getUpstreamURLs(locale string) (modelsURL, vendorsURL string) {
	base := strings.TrimRight(getUpstreamBase(), "/")
	if l, ok := normalizeLocale(locale); ok && l != "" {
		return fmt.Sprintf("%s/api/i18n/%s/newapi/models.json", base, l),
			fmt.Sprintf("%s/api/i18n/%s/newapi/vendors.json", base, l)
	}
	return fmt.Sprintf("%s/api/newapi/models.json", base), fmt.Sprintf("%s/api/newapi/vendors.json", base)
}

type upstreamEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    []T    `json:"data"`
}

type upstreamModel struct {
	Description string          `json:"description"`
	Endpoints   json.RawMessage `json:"endpoints"`
	Icon        string          `json:"icon"`
	ModelName   string          `json:"model_name"`
	NameRule    int             `json:"name_rule"`
	Status      int             `json:"status"`
	Tags        string          `json:"tags"`
	Category    string          `json:"category"`
	VendorName  string          `json:"vendor_name"`
}

type upstreamVendor struct {
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
}

type aihubmixModel struct {
	BillingConfig  string  `json:"billing_config"`
	CacheRatio     float64 `json:"cache_ratio"`
	CompletionRate float64 `json:"completion_ratio"`
	Desc           string  `json:"desc"`
	DescEn         string  `json:"desc_en"`
	Developer      string  `json:"developer"`
	DisplayInput   string  `json:"display_input"`
	DisplayOutput  string  `json:"display_output"`
	Endpoints      string  `json:"endpoints"`
	Features       string  `json:"features"`
	Flag           int     `json:"flag"`
	ImgPriceConfig string  `json:"img_price_config"`
	Model          string  `json:"model"`
	ModelName      string  `json:"model_name"`
	ModelRatio     float64 `json:"model_ratio"`
	Modalities     string  `json:"modalities"`
	ContextWindow  int     `json:"context_window"`
}

type aihubmixEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Total   int             `json:"total"`
	Data    []aihubmixModel `json:"data"`
}

var (
	etagCache  = make(map[string]string)
	bodyCache  = make(map[string][]byte)
	cacheMutex sync.RWMutex
)

type overwriteField struct {
	ModelName string   `json:"model_name"`
	Fields    []string `json:"fields"`
}

type syncRequest struct {
	Overwrite    []overwriteField `json:"overwrite"`
	OverwriteAll bool             `json:"overwrite_all"`
	Locale       string           `json:"locale"`
	Source       string           `json:"source"`
}

func newHTTPClient() *http.Client {
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 10)
	dialer := &net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}
	transport := &http.Transport{
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   time.Duration(timeoutSec) * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: time.Duration(timeoutSec) * time.Second,
	}
	if common.TLSInsecureSkipVerify {
		transport.TLSClientConfig = common.InsecureTLSConfig
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		if strings.HasSuffix(host, "github.io") {
			if conn, err := dialer.DialContext(ctx, "tcp4", addr); err == nil {
				return conn, nil
			}
			return dialer.DialContext(ctx, "tcp6", addr)
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return &http.Client{Transport: transport}
}

var (
	httpClientOnce sync.Once
	httpClient     *http.Client
)

func getHTTPClient() *http.Client {
	httpClientOnce.Do(func() {
		httpClient = newHTTPClient()
	})
	return httpClient
}

func fetchJSON[T any](ctx context.Context, url string, out *upstreamEnvelope[T]) error {
	var lastErr error
	attempts := common.GetEnvOrDefault("SYNC_HTTP_RETRY", 3)
	if attempts < 1 {
		attempts = 1
	}
	baseDelay := 200 * time.Millisecond
	maxMB := common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10)
	maxBytes := int64(maxMB) << 20
	for attempt := 0; attempt < attempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		// ETag conditional request
		cacheMutex.RLock()
		if et := etagCache[url]; et != "" {
			req.Header.Set("If-None-Match", et)
		}
		cacheMutex.RUnlock()

		resp, err := getHTTPClient().Do(req)
		if err != nil {
			lastErr = err
			// backoff with jitter
			sleep := baseDelay * time.Duration(1<<attempt)
			jitter := time.Duration(rand.Intn(150)) * time.Millisecond
			time.Sleep(sleep + jitter)
			continue
		}
		func() {
			defer resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				// read body into buffer for caching and flexible decode
				limited := io.LimitReader(resp.Body, maxBytes)
				buf, err := io.ReadAll(limited)
				if err != nil {
					lastErr = err
					return
				}
				// cache body and ETag
				cacheMutex.Lock()
				if et := resp.Header.Get("ETag"); et != "" {
					etagCache[url] = et
				}
				bodyCache[url] = buf
				cacheMutex.Unlock()

				// Try decode as envelope first
				if err := common.Unmarshal(buf, out); err != nil {
					// Try decode as pure array
					var arr []T
					if err2 := common.Unmarshal(buf, &arr); err2 != nil {
						lastErr = err
						return
					}
					out.Success = true
					out.Data = arr
					out.Message = ""
				} else {
					if !out.Success && len(out.Data) == 0 && out.Message == "" {
						out.Success = true
					}
				}
				lastErr = nil
			case http.StatusNotModified:
				// use cache
				cacheMutex.RLock()
				buf := bodyCache[url]
				cacheMutex.RUnlock()
				if len(buf) == 0 {
					lastErr = errors.New("cache miss for 304 response")
					return
				}
				if err := common.Unmarshal(buf, out); err != nil {
					var arr []T
					if err2 := common.Unmarshal(buf, &arr); err2 != nil {
						lastErr = err
						return
					}
					out.Success = true
					out.Data = arr
					out.Message = ""
				} else {
					if !out.Success && len(out.Data) == 0 && out.Message == "" {
						out.Success = true
					}
				}
				lastErr = nil
			default:
				lastErr = errors.New(resp.Status)
			}
		}()
		if lastErr == nil {
			return nil
		}
		sleep := baseDelay * time.Duration(1<<attempt)
		jitter := time.Duration(rand.Intn(150)) * time.Millisecond
		time.Sleep(sleep + jitter)
	}
	return lastErr
}

func ensureVendorID(vendorName string, vendorByName map[string]upstreamVendor, vendorIDCache map[string]int, createdVendors *int) int {
	if vendorName == "" {
		return 0
	}
	if id, ok := vendorIDCache[vendorName]; ok {
		return id
	}
	var existing model.Vendor
	if err := model.DB.Where("name = ?", vendorName).First(&existing).Error; err == nil {
		vendorIDCache[vendorName] = existing.Id
		return existing.Id
	}
	uv := vendorByName[vendorName]
	v := &model.Vendor{
		Name:        vendorName,
		Description: uv.Description,
		Icon:        coalesce(uv.Icon, ""),
		Status:      chooseStatus(uv.Status, 1),
	}
	if err := v.Insert(); err == nil {
		*createdVendors++
		vendorIDCache[vendorName] = v.Id
		return v.Id
	}
	vendorIDCache[vendorName] = 0
	return 0
}

func isAIHubMixSyncSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return source == aihubmixSource || source == "inferera" || source == "推理时代"
}

func getAIHubMixModelsURL(page, pageSize int) string {
	base := strings.TrimRight(common.GetEnvOrDefaultString("SYNC_AIHUBMIX_MODELS_URL", aihubmixModelsURL), "/")
	return fmt.Sprintf("%s?p=%d&page_size=%d", base, page, pageSize)
}

func fetchAIHubMixModels(ctx context.Context) ([]aihubmixModel, string, error) {
	pageSize := common.GetEnvOrDefault("SYNC_AIHUBMIX_PAGE_SIZE", 200)
	if pageSize <= 0 {
		pageSize = 200
	}
	maxPages := common.GetEnvOrDefault("SYNC_AIHUBMIX_MAX_PAGES", 120)
	if maxPages <= 0 {
		maxPages = 120
	}

	items := make([]aihubmixModel, 0)
	firstURL := getAIHubMixModelsURL(1, pageSize)
	for page := 1; page <= maxPages; page++ {
		url := getAIHubMixModelsURL(page, pageSize)
		var envelope aihubmixEnvelope
		if err := fetchAIHubMixPage(ctx, url, &envelope); err != nil {
			return nil, firstURL, err
		}
		if !envelope.Success && len(envelope.Data) == 0 {
			return nil, firstURL, errors.New(coalesce(envelope.Message, "AIHubMix returned unsuccessful response"))
		}
		items = append(items, envelope.Data...)
		if len(envelope.Data) == 0 || (envelope.Total > 0 && len(items) >= envelope.Total) {
			break
		}
	}
	return items, firstURL, nil
}

func fetchAIHubMixPage(ctx context.Context, url string, out *aihubmixEnvelope) error {
	maxMB := common.GetEnvOrDefault("SYNC_HTTP_MAX_MB", 10)
	maxBytes := int64(maxMB) << 20
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := getHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New(resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return err
	}
	return common.Unmarshal(body, out)
}

func buildAIHubMixUpstreamModel(item aihubmixModel, locale string) upstreamModel {
	description := strings.TrimSpace(item.Desc)
	if strings.EqualFold(strings.TrimSpace(locale), "en") && strings.TrimSpace(item.DescEn) != "" {
		description = strings.TrimSpace(item.DescEn)
	}
	tags := mergeCommaTokens(item.Features, item.Modalities, contextTag(item.ContextWindow))
	endpoints := buildAIHubMixEndpoints(item.Endpoints)
	return upstreamModel{
		Description: description,
		Endpoints:   endpoints,
		ModelName:   strings.TrimSpace(item.Model),
		Status:      chooseStatus(item.Flag, 1),
		Tags:        tags,
		Category:    aiHubMixCategory(item.Modalities),
		VendorName:  strings.TrimSpace(item.Developer),
	}
}

func contextTag(contextWindow int) string {
	if contextWindow <= 0 {
		return ""
	}
	return fmt.Sprintf("context:%d", contextWindow)
}

func mergeCommaTokens(values ...string) string {
	seen := make(map[string]struct{})
	tokens := make([]string, 0)
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			tokens = append(tokens, token)
		}
	}
	return strings.Join(tokens, ",")
}

func buildAIHubMixEndpoints(raw string) json.RawMessage {
	endpoints := make(map[string]bool)
	for _, endpoint := range strings.Split(raw, ",") {
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		endpoints[endpoint] = true
	}
	if len(endpoints) == 0 {
		return nil
	}
	bytes, err := common.Marshal(endpoints)
	if err != nil {
		return nil
	}
	return bytes
}

func aiHubMixCategory(modalities string) string {
	for _, modality := range strings.Split(modalities, ",") {
		switch strings.TrimSpace(modality) {
		case "image":
			return "image"
		case "audio":
			return "audio"
		case "video":
			return "video"
		}
	}
	return "text"
}

func buildAIHubMixSyncData(items []aihubmixModel, locale string) (map[string]upstreamModel, map[string]upstreamVendor) {
	modelByName := make(map[string]upstreamModel, len(items))
	vendorByName := make(map[string]upstreamVendor)
	for _, item := range items {
		up := buildAIHubMixUpstreamModel(item, locale)
		if up.ModelName == "" {
			continue
		}
		modelByName[up.ModelName] = up
		if up.VendorName != "" {
			vendorByName[up.VendorName] = upstreamVendor{
				Name:        up.VendorName,
				Description: up.VendorName,
				Status:      1,
			}
		}
	}
	return modelByName, vendorByName
}

func syncAIHubMixPricing(items []aihubmixModel) (int, error) {
	modelRatios := ratio_setting.GetModelRatioCopy()
	completionRatios := ratio_setting.GetCompletionRatioCopy()
	cacheRatios := ratio_setting.GetCacheRatioCopy()

	updated := 0
	for _, item := range items {
		name := strings.TrimSpace(item.Model)
		if name == "" || item.ModelRatio <= 0 {
			continue
		}
		modelRatios[name] = item.ModelRatio
		if item.CompletionRate > 0 {
			completionRatios[name] = item.CompletionRate
		}
		if item.CacheRatio > 0 {
			cacheRatios[name] = item.CacheRatio
		}
		updated++
	}
	if updated == 0 {
		return 0, nil
	}

	modelRatioBytes, err := common.Marshal(modelRatios)
	if err != nil {
		return 0, err
	}
	completionRatioBytes, err := common.Marshal(completionRatios)
	if err != nil {
		return 0, err
	}
	cacheRatioBytes, err := common.Marshal(cacheRatios)
	if err != nil {
		return 0, err
	}

	return updated, model.UpdateOptionsBulk(map[string]string{
		"ModelRatio":      string(modelRatioBytes),
		"CompletionRatio": string(completionRatioBytes),
		"CacheRatio":      string(cacheRatioBytes),
	})
}

func buildOverwriteFieldMap(overwrites []overwriteField) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{}, len(overwrites))
	for _, overwrite := range overwrites {
		fields := make(map[string]struct{}, len(overwrite.Fields))
		for _, field := range overwrite.Fields {
			field = strings.ToLower(strings.TrimSpace(field))
			if field == "" {
				continue
			}
			fields[field] = struct{}{}
		}
		if len(fields) > 0 {
			result[overwrite.ModelName] = fields
		}
	}
	return result
}

func shouldApplyAIHubMixField(fields map[string]struct{}, field string, selective bool) bool {
	if !selective {
		return true
	}
	_, ok := fields[field]
	return ok
}

func applyUpstreamModelFields(local *model.Model, up upstreamModel, vendorID int, fields map[string]struct{}, selective bool) bool {
	changed := false
	if shouldApplyAIHubMixField(fields, "description", selective) {
		local.Description = up.Description
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "icon", selective) {
		local.Icon = up.Icon
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "tags", selective) {
		local.Tags = up.Tags
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "category", selective) {
		local.Category = up.Category
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "vendor", selective) {
		local.VendorID = vendorID
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "endpoints", selective) {
		local.Endpoints = string(up.Endpoints)
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "status", selective) {
		local.Status = chooseStatus(up.Status, local.Status)
		changed = true
	}
	if shouldApplyAIHubMixField(fields, "name_rule", selective) {
		local.NameRule = up.NameRule
		changed = true
	}
	return changed
}

func applyAIHubMixModelFields(local *model.Model, up upstreamModel, vendorID int, fields map[string]struct{}, selective bool) bool {
	return applyUpstreamModelFields(local, up, vendorID, fields, selective)
}

func syncAIHubMixModels(c *gin.Context, req syncRequest) {
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 30)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	items, sourceURL, err := fetchAIHubMixModels(ctx)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取推理时代模型失败: " + err.Error(), "source_urls": gin.H{"models_url": sourceURL}})
		return
	}
	modelByName, vendorByName := buildAIHubMixSyncData(items, req.Locale)

	createdModels := 0
	createdVendors := 0
	updatedModels := 0
	skipped := make([]string, 0)
	createdList := make([]string, 0)
	updatedList := make([]string, 0)
	vendorIDCache := make(map[string]int)
	overwriteByModel := buildOverwriteFieldMap(req.Overwrite)
	selectiveOverwrite := len(req.Overwrite) > 0 && !req.OverwriteAll

	for name, up := range modelByName {
		var local model.Model
		if err := model.DB.Where("model_name = ?", name).First(&local).Error; err == nil {
			fields, hasOverwrite := overwriteByModel[name]
			if !req.OverwriteAll && !selectiveOverwrite {
				continue
			}
			if local.SyncOfficial == 0 && !req.OverwriteAll {
				skipped = append(skipped, name)
				continue
			}
			if selectiveOverwrite && !hasOverwrite {
				continue
			}
			vendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &createdVendors)
			if !applyAIHubMixModelFields(&local, up, vendorID, fields, selectiveOverwrite) {
				continue
			}
			if err := local.Update(); err == nil {
				updatedModels++
				updatedList = append(updatedList, name)
			} else {
				skipped = append(skipped, name)
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			skipped = append(skipped, name)
			continue
		}

		mi := &model.Model{
			ModelName:    name,
			Description:  up.Description,
			Icon:         up.Icon,
			Tags:         up.Tags,
			Category:     up.Category,
			VendorID:     ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &createdVendors),
			Endpoints:    string(up.Endpoints),
			Status:       chooseStatus(up.Status, 1),
			SyncOfficial: 1,
			NameRule:     up.NameRule,
		}
		if err := mi.Insert(); err == nil {
			createdModels++
			createdList = append(createdList, name)
		} else {
			skipped = append(skipped, name)
		}
	}

	updatedPrices, err := syncAIHubMixPricing(items)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "同步推理时代价格失败: " + err.Error(), "source_urls": gin.H{"models_url": sourceURL}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"created_models":  createdModels,
			"created_vendors": createdVendors,
			"updated_models":  updatedModels,
			"updated_prices":  updatedPrices,
			"skipped_models":  skipped,
			"created_list":    createdList,
			"updated_list":    updatedList,
			"source": gin.H{
				"source":     aihubmixSource,
				"locale":     req.Locale,
				"models_url": sourceURL,
			},
		},
	})
}

// SyncUpstreamModels 同步上游模型与供应商：
// - 默认仅创建「未配置模型」
// - 可通过 overwrite_all 覆盖所有本地已有模型字段
// - 可通过 overwrite 选择性覆盖更新本地已有模型的字段（前提：sync_official <> 0）
func SyncUpstreamModels(c *gin.Context) {
	var req syncRequest
	// 允许空体
	_ = c.ShouldBindJSON(&req)
	if isAIHubMixSyncSource(req.Source) {
		syncAIHubMixModels(c, req)
		return
	}
	// 1) 获取未配置模型列表
	missing, err := model.GetMissingModels()
	if err != nil {
		common.SysError("failed to get missing models: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取模型列表失败，请稍后重试"})
		return
	}

	// 若既无缺失模型需要创建，也未指定覆盖更新字段，则无需请求上游数据，直接返回
	if len(missing) == 0 && len(req.Overwrite) == 0 && !req.OverwriteAll {
		modelsURL, vendorsURL := getUpstreamURLs(req.Locale)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"created_models":  0,
				"created_vendors": 0,
				"updated_models":  0,
				"skipped_models":  []string{},
				"created_list":    []string{},
				"updated_list":    []string{},
				"source": gin.H{
					"locale":      req.Locale,
					"models_url":  modelsURL,
					"vendors_url": vendorsURL,
				},
			},
		})
		return
	}

	// 2) 拉取上游 vendors 与 models
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 15)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	modelsURL, vendorsURL := getUpstreamURLs(req.Locale)
	var vendorsEnv upstreamEnvelope[upstreamVendor]
	var modelsEnv upstreamEnvelope[upstreamModel]
	var fetchErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		// vendor 失败不拦截
		_ = fetchJSON(ctx, vendorsURL, &vendorsEnv)
	}()
	go func() {
		defer wg.Done()
		if err := fetchJSON(ctx, modelsURL, &modelsEnv); err != nil {
			fetchErr = err
		}
	}()
	wg.Wait()
	if fetchErr != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取上游模型失败: " + fetchErr.Error(), "locale": req.Locale, "source_urls": gin.H{"models_url": modelsURL, "vendors_url": vendorsURL}})
		return
	}

	// 建立映射
	vendorByName := make(map[string]upstreamVendor)
	for _, v := range vendorsEnv.Data {
		if v.Name != "" {
			vendorByName[v.Name] = v
		}
	}
	modelByName := make(map[string]upstreamModel)
	for _, m := range modelsEnv.Data {
		if m.ModelName != "" {
			modelByName[m.ModelName] = m
		}
	}

	// 3) 执行同步：覆盖已有模型并创建缺失模型；若上游缺失该模型则跳过
	createdModels := 0
	createdVendors := 0
	updatedModels := 0
	skipped := make([]string, 0)
	createdList := make([]string, 0)
	updatedList := make([]string, 0)

	// 本地缓存：vendorName -> id
	vendorIDCache := make(map[string]int)

	if req.OverwriteAll {
		upstreamNames := make([]string, 0, len(modelByName))
		for name := range modelByName {
			upstreamNames = append(upstreamNames, name)
		}
		var locals []model.Model
		if len(upstreamNames) > 0 {
			_ = model.DB.Where("model_name IN ?", upstreamNames).Find(&locals).Error
		}
		for _, local := range locals {
			up := modelByName[local.ModelName]
			vendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &createdVendors)
			if !applyUpstreamModelFields(&local, up, vendorID, nil, false) {
				continue
			}
			if err := local.Update(); err == nil {
				updatedModels++
				updatedList = append(updatedList, local.ModelName)
			} else {
				skipped = append(skipped, local.ModelName)
			}
		}
	}

	for _, name := range missing {
		up, ok := modelByName[name]
		if !ok {
			skipped = append(skipped, name)
			continue
		}

		// 若本地已存在且设置为不同步，则跳过（极端情况：缺失列表与本地状态不同步时）
		var existing model.Model
		if err := model.DB.Where("model_name = ?", name).First(&existing).Error; err == nil {
			if existing.SyncOfficial == 0 {
				skipped = append(skipped, name)
				continue
			}
		}

		// 确保 vendor 存在
		vendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &createdVendors)

		// 创建模型
		mi := &model.Model{
			ModelName:   name,
			Description: up.Description,
			Icon:        up.Icon,
			Tags:        up.Tags,
			Category:    up.Category,
			VendorID:    vendorID,
			Endpoints:   string(up.Endpoints),
			Status:      chooseStatus(up.Status, 1),
			NameRule:    up.NameRule,
		}
		if err := mi.Insert(); err == nil {
			createdModels++
			createdList = append(createdList, name)
		} else {
			skipped = append(skipped, name)
		}
	}

	// 4) 处理可选覆盖（更新本地已有模型的差异字段）
	if len(req.Overwrite) > 0 && !req.OverwriteAll {
		// vendorIDCache 已用于创建阶段，可复用
		for _, ow := range req.Overwrite {
			up, ok := modelByName[ow.ModelName]
			if !ok {
				continue
			}
			var local model.Model
			if err := model.DB.Where("model_name = ?", ow.ModelName).First(&local).Error; err != nil {
				continue
			}

			// 跳过被禁用官方同步的模型
			if local.SyncOfficial == 0 {
				continue
			}

			// 映射 vendor
			newVendorID := ensureVendorID(up.VendorName, vendorByName, vendorIDCache, &createdVendors)

			fields := make(map[string]struct{}, len(ow.Fields))
			for _, field := range ow.Fields {
				field = strings.ToLower(strings.TrimSpace(field))
				if field != "" {
					fields[field] = struct{}{}
				}
			}

			_ = model.DB.Transaction(func(tx *gorm.DB) error {
				if !applyUpstreamModelFields(&local, up, newVendorID, fields, true) {
					return nil
				}
				if err := tx.Save(&local).Error; err != nil {
					return err
				}
				updatedModels++
				updatedList = append(updatedList, ow.ModelName)
				return nil
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"created_models":  createdModels,
			"created_vendors": createdVendors,
			"updated_models":  updatedModels,
			"skipped_models":  skipped,
			"created_list":    createdList,
			"updated_list":    updatedList,
			"source": gin.H{
				"locale":      req.Locale,
				"models_url":  modelsURL,
				"vendors_url": vendorsURL,
			},
		},
	})
}

func containsField(fields []string, key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, f := range fields {
		if strings.ToLower(strings.TrimSpace(f)) == key {
			return true
		}
	}
	return false
}

func coalesce(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func chooseStatus(primary, fallback int) int {
	if primary == 0 && fallback != 0 {
		return fallback
	}
	if primary != 0 {
		return primary
	}
	return 1
}

// SyncUpstreamPreview 预览上游与本地的差异（仅用于弹窗选择）
func SyncUpstreamPreview(c *gin.Context) {
	// 1) 拉取上游数据
	timeoutSec := common.GetEnvOrDefault("SYNC_HTTP_TIMEOUT_SECONDS", 15)
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	locale := c.Query("locale")
	source := c.Query("source")
	sourceInfo := gin.H{"locale": locale}
	modelByName := make(map[string]upstreamModel)
	vendorByName := make(map[string]upstreamVendor)
	upstreamNames := make([]string, 0)

	if isAIHubMixSyncSource(source) {
		items, sourceURL, err := fetchAIHubMixModels(ctx)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取推理时代模型失败: " + err.Error(), "locale": locale, "source_urls": gin.H{"models_url": sourceURL}})
			return
		}
		modelByName, vendorByName = buildAIHubMixSyncData(items, locale)
		sourceInfo["source"] = aihubmixSource
		sourceInfo["models_url"] = sourceURL
	} else {
		modelsURL, vendorsURL := getUpstreamURLs(locale)
		sourceInfo["models_url"] = modelsURL
		sourceInfo["vendors_url"] = vendorsURL

		var vendorsEnv upstreamEnvelope[upstreamVendor]
		var modelsEnv upstreamEnvelope[upstreamModel]
		var fetchErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = fetchJSON(ctx, vendorsURL, &vendorsEnv)
		}()
		go func() {
			defer wg.Done()
			if err := fetchJSON(ctx, modelsURL, &modelsEnv); err != nil {
				fetchErr = err
			}
		}()
		wg.Wait()
		if fetchErr != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "获取上游模型失败: " + fetchErr.Error(), "locale": locale, "source_urls": sourceInfo})
			return
		}

		for _, v := range vendorsEnv.Data {
			if v.Name != "" {
				vendorByName[v.Name] = v
			}
		}
		for _, m := range modelsEnv.Data {
			if m.ModelName != "" {
				modelByName[m.ModelName] = m
			}
		}
	}
	for name := range modelByName {
		upstreamNames = append(upstreamNames, name)
	}

	// 2) 本地已有模型
	var locals []model.Model
	if len(upstreamNames) > 0 {
		_ = model.DB.Where("model_name IN ? AND sync_official <> 0", upstreamNames).Find(&locals).Error
	}

	// 本地 vendor 名称映射
	vendorIdSet := make(map[int]struct{})
	for _, m := range locals {
		if m.VendorID != 0 {
			vendorIdSet[m.VendorID] = struct{}{}
		}
	}
	vendorIDs := make([]int, 0, len(vendorIdSet))
	for id := range vendorIdSet {
		vendorIDs = append(vendorIDs, id)
	}
	idToVendorName := make(map[int]string)
	if len(vendorIDs) > 0 {
		var dbVendors []model.Vendor
		_ = model.DB.Where("id IN ?", vendorIDs).Find(&dbVendors).Error
		for _, v := range dbVendors {
			idToVendorName[v.Id] = v.Name
		}
	}

	// 3) 缺失且上游存在的模型
	missingList, _ := model.GetMissingModels()
	var missing []string
	for _, name := range missingList {
		if _, ok := modelByName[name]; ok {
			missing = append(missing, name)
		}
	}

	// 4) 计算冲突字段
	type conflictField struct {
		Field    string      `json:"field"`
		Local    interface{} `json:"local"`
		Upstream interface{} `json:"upstream"`
	}
	type conflictItem struct {
		ModelName string          `json:"model_name"`
		Fields    []conflictField `json:"fields"`
	}

	var conflicts []conflictItem
	for _, local := range locals {
		up, ok := modelByName[local.ModelName]
		if !ok {
			continue
		}
		fields := make([]conflictField, 0, 6)
		if strings.TrimSpace(local.Description) != strings.TrimSpace(up.Description) {
			fields = append(fields, conflictField{Field: "description", Local: local.Description, Upstream: up.Description})
		}
		if strings.TrimSpace(local.Icon) != strings.TrimSpace(up.Icon) {
			fields = append(fields, conflictField{Field: "icon", Local: local.Icon, Upstream: up.Icon})
		}
		if strings.TrimSpace(local.Tags) != strings.TrimSpace(up.Tags) {
			fields = append(fields, conflictField{Field: "tags", Local: local.Tags, Upstream: up.Tags})
		}
		if strings.TrimSpace(local.Category) != strings.TrimSpace(up.Category) {
			fields = append(fields, conflictField{Field: "category", Local: local.Category, Upstream: up.Category})
		}
		// vendor 对比使用名称
		localVendor := idToVendorName[local.VendorID]
		if strings.TrimSpace(localVendor) != strings.TrimSpace(up.VendorName) {
			fields = append(fields, conflictField{Field: "vendor", Local: localVendor, Upstream: up.VendorName})
		}
		if local.NameRule != up.NameRule {
			fields = append(fields, conflictField{Field: "name_rule", Local: local.NameRule, Upstream: up.NameRule})
		}
		if local.Status != chooseStatus(up.Status, local.Status) {
			fields = append(fields, conflictField{Field: "status", Local: local.Status, Upstream: up.Status})
		}
		if len(fields) > 0 {
			conflicts = append(conflicts, conflictItem{ModelName: local.ModelName, Fields: fields})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"missing":   missing,
			"conflicts": conflicts,
			"source":    sourceInfo,
		},
	})
}
