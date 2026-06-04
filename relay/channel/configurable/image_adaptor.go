package configurable

import (
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type ImageAdaptor struct {
	openai.Adaptor
	apiKey             string
	baseURL            string
	profile            *Profile
	selectedSubmitPath string
}

func (a *ImageAdaptor) Init(info *relaycommon.RelayInfo) {
	a.Adaptor.Init(info)
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
	if info.ChannelMeta != nil {
		a.apiKey = info.ChannelMeta.ApiKey
		a.baseURL = info.ChannelMeta.ChannelBaseUrl
	}
	a.profile = resolveProfile(info)
}

func (a *ImageAdaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	profile, err := a.requireImageProfile()
	if err != nil {
		return "", err
	}
	path := profile.Submit.Path
	if strings.TrimSpace(a.selectedSubmitPath) != "" {
		path = a.selectedSubmitPath
	}
	return joinURL(a.baseURL, path), nil
}

func (a *ImageAdaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	header.Set("Content-Type", "application/json")
	header.Set("Accept", "application/json")
	if strings.TrimSpace(a.apiKey) != "" {
		header.Set("Authorization", "Bearer "+a.apiKey)
	}
	if a.profile != nil {
		req := &http.Request{Header: *header}
		ApplyConfiguredHeaders(req, a.profile.Submit.Headers, a.apiKey, "")
		*header = req.Header
	}
	return nil
}

func (a *ImageAdaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	profile, err := a.requireImageProfile()
	if err != nil {
		return nil, err
	}
	source, err := imageRequestSource(c, info, request)
	if err != nil {
		return nil, err
	}
	a.selectedSubmitPath = selectEndpointPath(profile.Submit, source, info)
	payload, err := buildMappedMap(profile.Submit.Body.Fields, source, info)
	if err != nil {
		return nil, err
	}
	normalizeImagePayload(payload)
	return payload, nil
}

func (a *ImageAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *ImageAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	profile, err := a.requireImageProfile()
	if err != nil {
		return &dto.Usage{}, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadRequest)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &dto.Usage{}, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &dto.Usage{}, types.NewOpenAIError(fmt.Errorf("upstream status %d: %s", resp.StatusCode, string(responseBody)), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	}

	taskID := strings.TrimSpace(gjson.GetBytes(responseBody, profile.Submit.Response.TaskIDPath).String())
	if taskID == "" {
		return &dto.Usage{}, types.NewOpenAIError(fmt.Errorf("task_id is empty"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	output := map[string]any{
		"id":      taskID,
		"object":  "image.task",
		"created": time.Now().Unix(),
		"model":   info.OriginModelName,
	}
	if statusPath := strings.TrimSpace(profile.Submit.Response.StatusPath); statusPath != "" {
		if status := strings.TrimSpace(gjson.GetBytes(responseBody, statusPath).String()); status != "" {
			output["status"] = status
		}
	}
	data, err := common.Marshal(output)
	if err != nil {
		return &dto.Usage{}, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json", data)
	return &dto.Usage{}, nil
}

func (a *ImageAdaptor) GetChannelName() string {
	if a.profile != nil && a.profile.Name != "" {
		return a.profile.Name
	}
	return "Configurable Image"
}

func (a *ImageAdaptor) requireImageProfile() (*Profile, error) {
	if a.profile == nil {
		return nil, fmt.Errorf("configurable image profile not found")
	}
	if !strings.EqualFold(strings.TrimSpace(a.profile.MediaType), "image") {
		return nil, fmt.Errorf("configurable profile %s is not an image profile", a.profile.ID)
	}
	return a.profile, nil
}

func selectEndpointPath(endpoint EndpointConfig, source map[string]any, info *relaycommon.RelayInfo) string {
	for _, variant := range endpoint.PathVariants {
		if pathVariantMatches(variant, source, info) {
			if path := strings.TrimSpace(variant.Path); path != "" {
				return path
			}
		}
	}
	return endpoint.Path
}

func pathVariantMatches(variant PathVariant, source map[string]any, info *relaycommon.RelayInfo) bool {
	if strings.TrimSpace(variant.Field) == "" {
		return false
	}
	value := valueFromSource(variant.Field, source, info)
	if variant.NonEmpty != nil {
		return !isEmptyValue(value) == *variant.NonEmpty
	}
	if variant.Equals != nil {
		return fmt.Sprint(value) == fmt.Sprint(variant.Equals)
	}
	return !isEmptyValue(value)
}

func imageRequestSource(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (map[string]any, error) {
	data, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	var source map[string]any
	if err := common.Unmarshal(data, &source); err != nil {
		return nil, err
	}
	for key, raw := range request.Extra {
		if len(raw) == 0 {
			continue
		}
		var value any
		if err := common.Unmarshal(raw, &value); err == nil {
			source[key] = value
		}
	}
	imageURLs, err := imageRequestURLs(c, info, request)
	if err != nil {
		return nil, err
	}
	source["image_urls"] = imageURLs
	return source, nil
}

func imageRequestURLs(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) ([]string, error) {
	var imageURLs []string
	appendRaw := func(raw []byte) error {
		values, err := rawImageURLs(raw)
		if err != nil {
			return err
		}
		imageURLs = append(imageURLs, values...)
		return nil
	}
	if err := appendRaw(request.Image); err != nil {
		return nil, err
	}
	if err := appendRaw(request.Images); err != nil {
		return nil, err
	}
	for _, key := range []string{"image", "images", "image_urls"} {
		if err := appendRaw(request.Extra[key]); err != nil {
			return nil, err
		}
	}
	if raw := request.Extra["input"]; len(raw) > 0 {
		var input struct {
			ImageURLs []string `json:"image_urls"`
		}
		if err := common.Unmarshal(raw, &input); err == nil {
			imageURLs = append(imageURLs, input.ImageURLs...)
		}
	}
	if len(imageURLs) == 0 && info != nil && info.RelayMode == relayconstant.RelayModeImagesEdits && !isJSONImageRequest(c) {
		var err error
		imageURLs, err = multipartImageURLs(c)
		if err != nil {
			return nil, err
		}
	}
	imageURLs = compactStrings(imageURLs)
	if len(imageURLs) > 16 {
		return nil, fmt.Errorf("image_urls supports 1-16 URLs")
	}
	return imageURLs, nil
}

func rawImageURLs(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var values []string
	if err := common.Unmarshal(raw, &values); err == nil {
		return compactStrings(values), nil
	}
	var single string
	if err := common.Unmarshal(raw, &single); err == nil {
		return compactStrings([]string{single}), nil
	}
	var objects []struct {
		URL string `json:"url"`
	}
	if err := common.Unmarshal(raw, &objects); err == nil {
		for _, object := range objects {
			values = append(values, object.URL)
		}
		return compactStrings(values), nil
	}
	var object struct {
		URL string `json:"url"`
	}
	if err := common.Unmarshal(raw, &object); err == nil && strings.TrimSpace(object.URL) != "" {
		return []string{strings.TrimSpace(object.URL)}, nil
	}
	return nil, nil
}

func multipartImageURLs(c *gin.Context) ([]string, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("parse image edit form failed: %w", err)
	}
	if form == nil || form.File == nil {
		return nil, fmt.Errorf("image is required")
	}
	files := multipartImageFiles(form.File)
	if len(files) == 0 {
		return nil, fmt.Errorf("image is required")
	}
	if len(files) > 16 {
		return nil, fmt.Errorf("image_urls supports 1-16 URLs")
	}
	imageURLs := make([]string, 0, len(files))
	for _, fileHeader := range files {
		imageURL, err := service.SaveTemporaryImage(c, fileHeader)
		if err != nil {
			return nil, err
		}
		imageURLs = append(imageURLs, imageURL)
	}
	return imageURLs, nil
}

func multipartImageFiles(files map[string][]*multipart.FileHeader) []*multipart.FileHeader {
	var imageFiles []*multipart.FileHeader
	for _, field := range []string{"image", "image[]"} {
		if values := files[field]; len(values) > 0 {
			imageFiles = append(imageFiles, values...)
		}
	}
	for field, values := range files {
		if field != "image[]" && strings.HasPrefix(field, "image[") && len(values) > 0 {
			imageFiles = append(imageFiles, values...)
		}
	}
	return imageFiles
}

func imageMode(value any) string {
	if len(stringValues(value)) > 0 {
		return "image-to-image"
	}
	return "text-to-image"
}

func imageAspectRatio(value any) string {
	input := firstString(value)
	if input == "" {
		return ""
	}
	if isSupportedImageAspectRatio(input) {
		return strings.ToLower(strings.TrimSpace(input))
	}
	aspectRatio, _, ok := imageSizeToAspectRatioAndResolution(input)
	if !ok {
		return ""
	}
	return aspectRatio
}

func imageResolution(value any) string {
	input := firstString(value)
	if input == "" {
		return ""
	}
	if resolution := normalizeImageResolution(input); resolution != "" {
		return resolution
	}
	_, resolution, ok := imageSizeToAspectRatioAndResolution(input)
	if !ok {
		return ""
	}
	return resolution
}

func imageQuality(value any) string {
	quality := strings.ToLower(strings.TrimSpace(firstString(value)))
	switch quality {
	case "":
		return ""
	case "auto":
		return "medium"
	case "low", "medium", "high":
		return quality
	default:
		return "medium"
	}
}

func imageSizeToAspectRatioAndResolution(size string) (string, string, bool) {
	size = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x")))
	if size == "auto" {
		return "auto", "1k", true
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return "", "", false
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || width <= 0 {
		return "", "", false
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || height <= 0 {
		return "", "", false
	}

	ratio := float64(width) / float64(height)
	aspectRatio := "auto"
	bestDelta := math.MaxFloat64
	for _, candidate := range imageAspectRatioCandidates {
		delta := math.Abs(ratio - candidate.ratio)
		if delta < bestDelta {
			bestDelta = delta
			aspectRatio = candidate.value
		}
	}
	maxSide := width
	if height > maxSide {
		maxSide = height
	}
	resolution := "4k"
	if maxSide <= 1024 {
		resolution = "1k"
	} else if maxSide <= 2048 {
		resolution = "2k"
	}
	return aspectRatio, resolution, true
}

type imageAspectRatioCandidate struct {
	value string
	ratio float64
}

var imageAspectRatioCandidates = []imageAspectRatioCandidate{
	{value: "1:1", ratio: 1},
	{value: "4:3", ratio: 4.0 / 3.0},
	{value: "3:4", ratio: 3.0 / 4.0},
	{value: "16:9", ratio: 16.0 / 9.0},
	{value: "9:16", ratio: 9.0 / 16.0},
}

func isSupportedImageAspectRatio(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "auto" {
		return true
	}
	for _, candidate := range imageAspectRatioCandidates {
		if value == candidate.value {
			return true
		}
	}
	return false
}

func normalizeImageResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1k", "2k", "4k":
		return strings.ToLower(strings.TrimSpace(value))
	case "auto":
		return "1k"
	default:
		return ""
	}
}

func firstString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		if len(v) == 0 {
			return ""
		}
		return strings.TrimSpace(v[0])
	case []any:
		if len(v) == 0 {
			return ""
		}
		if s, ok := v[0].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func normalizeImagePayload(payload map[string]any) {
	input, ok := payload["input"].(map[string]any)
	if !ok {
		return
	}
	if strings.EqualFold(strings.TrimSpace(firstString(input["aspect_ratio"])), "auto") {
		input["resolution"] = "1k"
	}
}

func isJSONImageRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	contentType := c.Request.Header.Get("Content-Type")
	return strings.HasPrefix(contentType, "application/json")
}

func ImageTaskResponseFromProfile(taskID string, profile *Profile, upstream []byte) (dto.ImageTaskResponse, bool, error) {
	if profile == nil || !strings.EqualFold(strings.TrimSpace(profile.MediaType), "image") {
		return dto.ImageTaskResponse{}, false, nil
	}
	resp := profile.Fetch.Response
	if strings.TrimSpace(resp.TaskIDPath) == "" || strings.TrimSpace(resp.StatusPath) == "" {
		return dto.ImageTaskResponse{}, false, nil
	}
	response := dto.ImageTaskResponse{
		ID:     common.GetStringIfEmpty(strings.TrimSpace(gjson.GetBytes(upstream, resp.TaskIDPath).String()), taskID),
		State:  strings.ToLower(mapStatus(resp.StatusMap, gjson.GetBytes(upstream, resp.StatusPath).String())),
		Action: constant.TaskActionGenerate,
	}
	if response.State == "" {
		response.State = "unknown"
	}
	if progress := gjson.GetBytes(upstream, resp.ProgressPath); progress.Exists() {
		response.Progress = int(progress.Int())
	}
	if createTime := imageTaskUnixTime(gjson.GetBytes(upstream, resp.CreateTimePath)); createTime != 0 {
		response.CreateTime = createTime
	}
	if updateTime := imageTaskUnixTime(gjson.GetBytes(upstream, resp.UpdateTimePath)); updateTime != 0 {
		response.UpdateTime = updateTime
	}
	if action := strings.TrimSpace(gjson.GetBytes(upstream, resp.ActionPath).String()); action != "" {
		response.Action = action
	}
	resultSource := upstream
	if resultJSONPath := strings.TrimSpace(resp.ResultJSONPath); resultJSONPath != "" {
		resultJSON := strings.TrimSpace(gjson.GetBytes(upstream, resultJSONPath).String())
		if resultJSON != "" {
			resultSource = []byte(resultJSON)
		}
	}
	if resultURLPath := strings.TrimSpace(resp.ResultURLPath); resultURLPath != "" {
		for _, imageURL := range stringsFromGJSON(gjson.GetBytes(resultSource, resultURLPath)) {
			response.Data.Images = append(response.Data.Images, dto.ImageTaskImage{URL: imageURL})
		}
	}
	if isFailureImageState(response.State) {
		if reason := strings.TrimSpace(gjson.GetBytes(upstream, resp.ReasonPath).String()); reason != "" {
			response.Error = map[string]any{"message": reason}
		}
	}
	normalized, err := common.Marshal(response)
	if err != nil {
		return dto.ImageTaskResponse{}, true, err
	}
	var parsed dto.ImageTaskResponse
	if err := common.Unmarshal(normalized, &parsed); err != nil {
		return dto.ImageTaskResponse{}, true, err
	}
	return parsed, true, nil
}

func imageTaskUnixTime(result gjson.Result) int64 {
	if !result.Exists() {
		return 0
	}
	value := result.Int()
	if value == 0 {
		parsed, err := strconv.ParseInt(strings.TrimSpace(result.String()), 10, 64)
		if err != nil {
			return 0
		}
		value = parsed
	}
	switch {
	case value >= 1_000_000_000_000_000:
		return value / 1_000_000
	case value >= 100_000_000_000:
		return value / 1_000
	default:
		return value
	}
}

func isFailureImageState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "failed", "failure", "error", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

func stringsFromGJSON(result gjson.Result) []string {
	if !result.Exists() {
		return nil
	}
	if result.IsArray() {
		values := make([]string, 0)
		for _, item := range result.Array() {
			if value := strings.TrimSpace(item.String()); value != "" {
				values = append(values, value)
			}
		}
		return values
	}
	if value := strings.TrimSpace(result.String()); value != "" {
		return []string{value}
	}
	return nil
}

func ImageTaskResponseBytesFromProfile(taskID string, profile *Profile, upstream []byte) ([]byte, bool, error) {
	response, ok, err := ImageTaskResponseFromProfile(taskID, profile, upstream)
	if !ok || err != nil {
		return nil, ok, err
	}
	data, err := common.Marshal(response)
	return data, true, err
}
