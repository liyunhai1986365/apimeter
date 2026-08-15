package imageconv

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func RequestHasImageInput(request *dto.ImageRequest) bool {
	if request == nil {
		return false
	}
	if rawHasImageValue(request.Image) || rawHasImageValue(request.Images) {
		return true
	}
	for _, key := range []string{"image", "images", "image_urls"} {
		if rawHasImageValue(request.Extra[key]) {
			return true
		}
	}
	if raw := request.Extra["input"]; len(raw) > 0 {
		var input struct {
			ImageURLs []string `json:"image_urls"`
		}
		if err := common.Unmarshal(raw, &input); err == nil {
			for _, value := range input.ImageURLs {
				if strings.TrimSpace(value) != "" {
					return true
				}
			}
		}
	}
	return false
}

func RequestImageReferences(request dto.ImageRequest) ([]string, error) {
	var refs []string
	appendRaw := func(raw json.RawMessage) error {
		values, err := rawImageReferences(raw)
		if err != nil {
			return err
		}
		refs = append(refs, values...)
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
			refs = append(refs, input.ImageURLs...)
		}
	}
	return compactStrings(refs), nil
}

func RequestImageURLs(c *gin.Context, request dto.ImageRequest, includeMultipartFiles bool) ([]string, error) {
	refs, err := RequestImageReferences(request)
	if err != nil {
		return nil, err
	}
	imageURLs, err := ImageReferencesToURLs(c, refs)
	if err != nil {
		return nil, err
	}
	if len(imageURLs) == 0 && includeMultipartFiles {
		imageURLs, err = MultipartImageURLs(c)
		if err != nil {
			return nil, err
		}
	}
	if len(imageURLs) > 16 {
		return nil, fmt.Errorf("image_urls supports 1-16 URLs")
	}
	return imageURLs, nil
}

func ImageReferencesToURLs(c *gin.Context, refs []string) ([]string, error) {
	result := make([]string, 0, len(refs))
	for _, ref := range compactStrings(refs) {
		if IsHTTPReference(ref) {
			result = append(result, ref)
			continue
		}
		imageURL, err := service.SaveTemporaryImageBase64(c, ref)
		if err != nil {
			return nil, err
		}
		result = append(result, imageURL)
	}
	return result, nil
}

func MultipartImageURLs(c *gin.Context) ([]string, error) {
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		parseErr := fmt.Errorf("parse image edit form failed: %w", err)
		if common.IsMultipartRequestError(err) {
			return nil, types.MarkRequestError(parseErr)
		}
		return nil, parseErr
	}
	if form == nil || form.File == nil {
		return nil, fmt.Errorf("image is required")
	}
	files := MultipartImageFiles(form.File)
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

func MultipartImageFiles(files map[string][]*multipart.FileHeader) []*multipart.FileHeader {
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

func BuildOpenAIJSONEditMultipart(c *gin.Context, request dto.ImageRequest) (*bytes.Buffer, error) {
	imageRefs, err := RequestImageReferences(request)
	if err != nil {
		return nil, err
	}
	if len(imageRefs) == 0 {
		return nil, types.MarkRequestError(errors.New("image is required"))
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	if err := writeOpenAIImageFormFields(writer, request); err != nil {
		return nil, err
	}

	fieldName := "image"
	if len(imageRefs) > 1 {
		fieldName = "image[]"
	}
	for i, ref := range imageRefs {
		data, mimeType, filename, err := imageReferenceBytes(c, ref, i+1)
		if err != nil {
			return nil, err
		}
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
		h.Set("Content-Type", mimeType)
		part, err := writer.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("create image form part failed: %w", err)
		}
		if _, err := part.Write(data); err != nil {
			return nil, fmt.Errorf("write image form part failed: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}
	if c != nil && c.Request != nil {
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	}
	return &requestBody, nil
}

func writeOpenAIImageFormFields(writer *multipart.Writer, request dto.ImageRequest) error {
	requestJSON, err := common.Marshal(request)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(requestJSON, &fields); err != nil {
		return err
	}
	for _, key := range []string{"image", "images", "image_urls"} {
		delete(fields, key)
	}
	if rawInput := fields["input"]; len(rawInput) > 0 {
		var input map[string]json.RawMessage
		if err := common.Unmarshal(rawInput, &input); err == nil {
			delete(input, "image_urls")
			if len(input) == 0 {
				delete(fields, "input")
			} else if fields["input"], err = common.Marshal(input); err != nil {
				return err
			}
		}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writeRawFormField(writer, key, fields[key]); err != nil {
			return err
		}
	}
	return nil
}

func EditsRequestPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "/v1/images/edits"
	}
	parts := strings.SplitN(path, "?", 2)
	originalBase := parts[0]
	parts[0] = strings.Replace(parts[0], "/v1/images/generations", "/v1/images/edits", 1)
	if parts[0] == originalBase {
		parts[0] = "/v1/images/edits"
	}
	if len(parts) == 2 {
		return parts[0] + "?" + parts[1]
	}
	return parts[0]
}

func IsHTTPReference(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

func rawImageReferences(raw json.RawMessage) ([]string, error) {
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
		URL     string `json:"url"`
		B64Json string `json:"b64_json"`
		Base64  string `json:"base64"`
	}
	if err := common.Unmarshal(raw, &objects); err == nil {
		for _, object := range objects {
			values = append(values, firstNonEmptyString(object.URL, object.B64Json, object.Base64))
		}
		return compactStrings(values), nil
	}
	var object struct {
		URL     string `json:"url"`
		B64Json string `json:"b64_json"`
		Base64  string `json:"base64"`
	}
	if err := common.Unmarshal(raw, &object); err == nil {
		return compactStrings([]string{firstNonEmptyString(object.URL, object.B64Json, object.Base64)}), nil
	}
	return nil, types.MarkRequestError(errors.New("invalid image reference"))
}

func rawHasImageValue(raw json.RawMessage) bool {
	refs, err := rawImageReferences(raw)
	return err == nil && len(refs) > 0
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func writeRawFormField(writer *multipart.Writer, key string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	value := strings.TrimSpace(string(raw))
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		value = strings.TrimSpace(text)
	}
	if value == "" {
		return nil
	}
	return writer.WriteField(key, value)
}

const imageReferenceCacheKey = "imageconv_reference_cache"

type cachedImageReference struct {
	data     []byte
	mimeType string
	err      error
}

type imageReferenceCache struct {
	mu      sync.Mutex
	entries map[string]cachedImageReference
}

func EnsureImageReferenceCache(c *gin.Context) {
	_ = imageReferenceCacheFromContext(c)
}

func ResetImageReferenceFailures(c *gin.Context) {
	if c == nil {
		return
	}
	value, ok := c.Get(imageReferenceCacheKey)
	if !ok {
		return
	}
	cache, ok := value.(*imageReferenceCache)
	if !ok {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for ref, entry := range cache.entries {
		if entry.err != nil {
			delete(cache.entries, ref)
		}
	}
}

func imageReferenceCacheFromContext(c *gin.Context) *imageReferenceCache {
	if c == nil {
		return nil
	}
	if value, ok := c.Get(imageReferenceCacheKey); ok {
		if cache, ok := value.(*imageReferenceCache); ok {
			return cache
		}
	}
	cache := &imageReferenceCache{entries: make(map[string]cachedImageReference)}
	c.Set(imageReferenceCacheKey, cache)
	return cache
}

func imageReferenceBytes(c *gin.Context, ref string, index int) ([]byte, string, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", "", types.MarkRequestError(errors.New("image is required"))
	}
	cache := imageReferenceCacheFromContext(c)
	if cache != nil {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		if cached, ok := cache.entries[ref]; ok {
			if cached.err != nil {
				return nil, "", "", cached.err
			}
			return cached.data, cached.mimeType, fmt.Sprintf("image-%d%s", index, imageExtensionForMimeType(cached.mimeType)), nil
		}
	}
	data, mimeType, err := loadImageReference(ref)
	if err != nil {
		if cache != nil {
			cache.entries[ref] = cachedImageReference{err: err}
		}
		return nil, "", "", err
	}
	if cache != nil {
		cache.entries[ref] = cachedImageReference{data: data, mimeType: mimeType}
	}
	return data, mimeType, fmt.Sprintf("image-%d%s", index, imageExtensionForMimeType(mimeType)), nil
}

func loadImageReference(ref string) ([]byte, string, error) {
	if IsHTTPReference(ref) {
		mimeType, base64Data, err := service.GetImageFromUrl(ref)
		if err != nil {
			return nil, "", err
		}
		data, err := base64.StdEncoding.DecodeString(base64Data)
		if err != nil {
			return nil, "", fmt.Errorf("decode image url data failed: %w", err)
		}
		return data, normalizeMimeType(mimeType), nil
	}
	mimeType, cleanBase64, err := service.DecodeBase64FileData(ref)
	if err != nil {
		return nil, "", types.MarkRequestError(err)
	}
	data, err := base64.StdEncoding.DecodeString(cleanBase64)
	if err != nil {
		return nil, "", types.MarkRequestError(fmt.Errorf("decode image base64 failed: %w", err))
	}
	return data, normalizeMimeType(mimeType), nil
}

func normalizeMimeType(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if strings.HasPrefix(mimeType, "image/") {
		return mimeType
	}
	return "image/png"
}

func imageExtensionForMimeType(mimeType string) string {
	switch normalizeMimeType(mimeType) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}
