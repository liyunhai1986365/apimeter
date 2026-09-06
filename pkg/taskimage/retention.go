// Package taskimage removes only image payloads from saved task JSON. RawMessage
// keeps unrelated numbers, provider fields and billing snapshots lossless.
package taskimage

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type Summary struct {
	Base64  int
	URLs    int
	Expired int
}

func IsDataURI(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "data:image/") && strings.Contains(value, ";base64,")
}

func HasPayload(body []byte) bool {
	for _, marker := range [][]byte{[]byte("b64_json"), []byte("base64"), []byte("Base64"), []byte("BASE64"), []byte("image_expired"), []byte("inlineData"), []byte("inline_data")} {
		if bytes.Contains(body, marker) {
			return true
		}
	}
	return false
}

func Transform(body []byte, imageTask, remove bool) ([]byte, Summary, error) {
	var summary Summary
	if len(bytes.TrimSpace(body)) == 0 {
		return body, summary, nil
	}
	updated, changed, err := walk(body, imageTask, remove, false, &summary)
	if !changed {
		return body, summary, err
	}
	return updated, summary, err
}

func stringValue(raw json.RawMessage) string {
	var s string
	_ = common.Unmarshal(raw, &s)
	return s
}

func httpURL(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "/")
}

func walk(raw json.RawMessage, imageContext, remove, imageString bool, summary *Summary) (json.RawMessage, bool, error) {
	switch common.GetJsonType(raw) {
	case "object":
		var obj map[string]json.RawMessage
		if err := common.Unmarshal(raw, &obj); err != nil {
			return nil, false, err
		}
		mime := strings.ToLower(stringValue(obj["mimeType"]))
		if mime == "" {
			mime = strings.ToLower(stringValue(obj["mime_type"]))
		}
		// Inline audio/video and opaque non-image blocks must not be touched.
		if mime != "" && !strings.HasPrefix(mime, "image/") {
			return raw, false, nil
		}
		inlineImage := strings.HasPrefix(mime, "image/")
		imageContext = imageContext || inlineImage || len(obj["b64_json"]) > 0
		changed, removed := false, false
		hasURL := false
		if stringValue(obj["image_expired"]) == "true" || bytes.Equal(obj["image_expired"], []byte("true")) {
			summary.Expired++
		}
		for key, value := range obj {
			lower := strings.ToLower(key)
			// These are textual/opaque bookkeeping fields, not image containers.
			// In particular, never edit provider or billing metadata by key name.
			switch lower {
			case "metadata", "usage", "properties", "billing_context", "billing", "error", "fail_reason", "message", "prompt", "signature", "thinking":
				continue
			}
			if strings.Contains(lower, "audio") || strings.Contains(lower, "video") {
				continue
			}
			text := stringValue(value)
			payload := (imageContext && text != "" && (lower == "b64_json" || lower == "base64" || lower == "image_base64" || lower == "imagebase64")) ||
				(inlineImage && lower == "data" && text != "") || IsDataURI(text)
			if payload {
				summary.Base64++
				if remove {
					delete(obj, key)
					changed, removed = true, true
				}
				continue
			}
			if (imageContext || lower == "image_url") && (lower == "url" || lower == "image_url") && httpURL(text) {
				hasURL = true
			}
			// Media fields outside an image object may contain huge audio/video
			// strings. Their names are not evidence that their data is an image.
			childImage := imageContext || lower == "images" || lower == "image" || lower == "image_url" || lower == "resulturls"
			v, didChange, err := walk(value, childImage, remove, lower == "images" || lower == "image" || lower == "image_url" || lower == "url" || lower == "resulturls", summary)
			if err != nil {
				return nil, false, err
			}
			if didChange {
				obj[key] = v
				changed = true
			}
		}
		if hasURL {
			summary.URLs++
		}
		if removed {
			obj["image_expired"] = json.RawMessage("true")
		}
		if !changed {
			return raw, false, nil
		}
		out, err := common.Marshal(obj)
		return out, true, err
	case "array":
		var items []json.RawMessage
		if err := common.Unmarshal(raw, &items); err != nil {
			return nil, false, err
		}
		changed := false
		for i, item := range items {
			v, didChange, err := walk(item, imageContext, remove, imageString, summary)
			if err != nil {
				return nil, false, err
			}
			if didChange {
				items[i] = v
				changed = true
			}
		}
		if !changed {
			return raw, false, nil
		}
		out, err := common.Marshal(items)
		return out, true, err
	case "string":
		value := stringValue(raw)
		if imageString && IsDataURI(value) {
			summary.Base64++
			if remove {
				return json.RawMessage(`{"image_expired":true}`), true, nil
			}
		} else if imageString && httpURL(value) {
			summary.URLs++
		}
	}
	return raw, false, nil
}
