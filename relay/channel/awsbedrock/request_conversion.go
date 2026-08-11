package awsbedrock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const AnthropicVersion = "bedrock-2023-05-31"

type systemTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type URLLoader func(c *gin.Context, sourceURL string) (base64Data string, mimeType string, err error)

type RequestConversionOptions struct {
	PreserveModel            bool
	PreserveStream           bool
	PreserveAnthropicVersion bool
	SkipBetaConversion       bool
	BetaModelNames           []string
}

// ConvertRequestBody applies only the compatibility fixes required by Bedrock
// Runtime's Anthropic Messages format. Compliant request bytes stay untouched.
func ConvertRequestBody(c *gin.Context, body []byte, header http.Header) ([]byte, bool, error) {
	return convertRequestBody(c, body, header, loadURLSource, RequestConversionOptions{})
}

func ConvertRequestBodyWithURLLoader(c *gin.Context, body []byte, header http.Header, urlLoader URLLoader) ([]byte, bool, error) {
	return convertRequestBody(c, body, header, urlLoader, RequestConversionOptions{})
}

func ConvertRequestBodyWithOptions(c *gin.Context, body []byte, header http.Header, options RequestConversionOptions) ([]byte, bool, error) {
	return convertRequestBody(c, body, header, loadURLSource, options)
}

func convertRequestBody(c *gin.Context, body []byte, header http.Header, urlLoader URLLoader, options RequestConversionOptions) ([]byte, bool, error) {
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, false, fmt.Errorf("decode AWS Bedrock request body: %w", err)
	}

	changed := false
	betaModelNames := append([]string(nil), options.BetaModelNames...)
	if modelRaw, exists := payload["model"]; exists {
		var bodyModel string
		if common.Unmarshal(modelRaw, &bodyModel) == nil && bodyModel != "" {
			betaModelNames = append(betaModelNames, bodyModel)
		}
	}
	if _, exists := payload["model"]; exists && !options.PreserveModel {
		delete(payload, "model")
		changed = true
	}
	if _, exists := payload["stream"]; exists && !options.PreserveStream {
		delete(payload, "stream")
		changed = true
	}

	if !options.PreserveAnthropicVersion {
		var anthropicVersion string
		versionRaw, versionExists := payload["anthropic_version"]
		if !versionExists || common.Unmarshal(versionRaw, &anthropicVersion) != nil || anthropicVersion != AnthropicVersion {
			encodedVersion, err := common.Marshal(AnthropicVersion)
			if err != nil {
				return nil, false, fmt.Errorf("encode AWS Bedrock anthropic_version: %w", err)
			}
			payload["anthropic_version"] = encodedVersion
			changed = true
		}
	}

	if !options.SkipBetaConversion {
		betaChanged, err := normalizeBeta(payload, header, betaModelNames...)
		if err != nil {
			return nil, false, err
		}
		changed = changed || betaChanged
	}

	messagesRaw, hasMessages := payload["messages"]
	if hasMessages {
		var messages []map[string]json.RawMessage
		if err := common.Unmarshal(messagesRaw, &messages); err != nil {
			return nil, false, fmt.Errorf("decode AWS Bedrock messages: %w", err)
		}

		messagesChanged, err := promoteLeadingSystemMessage(payload, &messages)
		if err != nil {
			return nil, false, err
		}
		urlChanged, err := convertURLSources(c, messages, urlLoader)
		if err != nil {
			return nil, false, err
		}
		if messagesChanged || urlChanged {
			encodedMessages, err := common.Marshal(messages)
			if err != nil {
				return nil, false, fmt.Errorf("encode AWS Bedrock messages: %w", err)
			}
			payload["messages"] = encodedMessages
			changed = true
		}
	}

	if !changed {
		return body, false, nil
	}
	converted, err := common.Marshal(payload)
	if err != nil {
		return nil, false, fmt.Errorf("encode AWS Bedrock request body: %w", err)
	}
	return converted, true, nil
}

func normalizeBeta(payload map[string]json.RawMessage, header http.Header, modelNames ...string) (bool, error) {
	changed := false
	if betaRaw, exists := payload["anthropic_beta"]; exists {
		if !bytes.Equal(bytes.TrimSpace(betaRaw), []byte("null")) {
			var betaValues []string
			if err := common.Unmarshal(betaRaw, &betaValues); err == nil {
				filtered := filterBetaValues(betaValues, modelNames...)
				if len(filtered) == 0 {
					delete(payload, "anthropic_beta")
					return true, nil
				}
				if equalStringSlices(betaValues, filtered) {
					return false, nil
				}
				encodedBeta, err := common.Marshal(filtered)
				if err != nil {
					return false, fmt.Errorf("encode AWS Bedrock anthropic_beta: %w", err)
				}
				payload["anthropic_beta"] = encodedBeta
				return true, nil
			}

			var betaValue string
			if err := common.Unmarshal(betaRaw, &betaValue); err != nil {
				return false, fmt.Errorf("anthropic_beta must be an array of strings")
			}
			betaValues = filterBetaValues([]string{betaValue}, modelNames...)
			if len(betaValues) == 0 {
				delete(payload, "anthropic_beta")
				return true, nil
			}
			encodedBeta, err := common.Marshal(betaValues)
			if err != nil {
				return false, fmt.Errorf("encode AWS Bedrock anthropic_beta: %w", err)
			}
			payload["anthropic_beta"] = encodedBeta
			return true, nil
		}
		delete(payload, "anthropic_beta")
		changed = true
	}

	betaValues := filterBetaValues(header.Values("anthropic-beta"), modelNames...)
	if len(betaValues) == 0 {
		return changed, nil
	}
	encodedBeta, err := common.Marshal(betaValues)
	if err != nil {
		return false, fmt.Errorf("encode AWS Bedrock anthropic_beta: %w", err)
	}
	payload["anthropic_beta"] = encodedBeta
	return true, nil
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func promoteLeadingSystemMessage(payload map[string]json.RawMessage, messages *[]map[string]json.RawMessage) (bool, error) {
	if len(*messages) == 0 {
		return false, nil
	}

	var role string
	if err := common.Unmarshal((*messages)[0]["role"], &role); err != nil || role != "system" {
		return false, nil
	}
	content, exists := (*messages)[0]["content"]
	if !exists {
		return false, fmt.Errorf("leading system message is missing content")
	}

	if existingSystem, exists := payload["system"]; exists && !bytes.Equal(bytes.TrimSpace(existingSystem), []byte("null")) {
		merged, err := mergeSystemValues(existingSystem, content)
		if err != nil {
			return false, err
		}
		payload["system"] = merged
	} else {
		payload["system"] = content
	}
	*messages = (*messages)[1:]
	return true, nil
}

func mergeSystemValues(existing json.RawMessage, leading json.RawMessage) (json.RawMessage, error) {
	existingBlocks, err := systemValueToBlocks(existing)
	if err != nil {
		return nil, fmt.Errorf("decode top-level system prompt: %w", err)
	}
	leadingBlocks, err := systemValueToBlocks(leading)
	if err != nil {
		return nil, fmt.Errorf("decode leading system message: %w", err)
	}
	return common.Marshal(append(existingBlocks, leadingBlocks...))
}

func systemValueToBlocks(value json.RawMessage) ([]json.RawMessage, error) {
	var text string
	if err := common.Unmarshal(value, &text); err == nil {
		block, err := common.Marshal(systemTextBlock{Type: "text", Text: text})
		if err != nil {
			return nil, err
		}
		return []json.RawMessage{block}, nil
	}

	var blocks []json.RawMessage
	if err := common.Unmarshal(value, &blocks); err != nil {
		return nil, fmt.Errorf("system prompt must be a string or an array of content blocks")
	}
	return blocks, nil
}

func loadURLSource(c *gin.Context, sourceURL string) (string, string, error) {
	return service.GetBase64Data(c, types.NewURLFileSource(sourceURL), "formatting URL media for AWS Bedrock")
}

func convertURLSources(c *gin.Context, messages []map[string]json.RawMessage, urlLoader URLLoader) (bool, error) {
	changed := false
	for messageIndex := range messages {
		contentRaw, exists := messages[messageIndex]["content"]
		if !exists {
			continue
		}

		var contentBlocks []map[string]json.RawMessage
		if err := common.Unmarshal(contentRaw, &contentBlocks); err != nil {
			continue
		}

		contentChanged := false
		for blockIndex := range contentBlocks {
			sourceRaw, exists := contentBlocks[blockIndex]["source"]
			if !exists {
				continue
			}

			var source map[string]json.RawMessage
			if err := common.Unmarshal(sourceRaw, &source); err != nil {
				continue
			}
			var sourceType string
			if err := common.Unmarshal(source["type"], &sourceType); err != nil || sourceType != "url" {
				continue
			}
			var sourceURL string
			if err := common.Unmarshal(source["url"], &sourceURL); err != nil || strings.TrimSpace(sourceURL) == "" {
				return false, fmt.Errorf("messages[%d].content[%d] URL source is missing url", messageIndex, blockIndex)
			}

			base64Data, mimeType, err := urlLoader(c, sourceURL)
			if err != nil {
				return false, fmt.Errorf("convert messages[%d].content[%d] URL source: %w", messageIndex, blockIndex, err)
			}
			sourceTypeRaw, err := common.Marshal("base64")
			if err != nil {
				return false, err
			}
			mimeTypeRaw, err := common.Marshal(mimeType)
			if err != nil {
				return false, err
			}
			dataRaw, err := common.Marshal(base64Data)
			if err != nil {
				return false, err
			}

			source["type"] = sourceTypeRaw
			source["media_type"] = mimeTypeRaw
			source["data"] = dataRaw
			delete(source, "url")
			encodedSource, err := common.Marshal(source)
			if err != nil {
				return false, fmt.Errorf("encode messages[%d].content[%d] source: %w", messageIndex, blockIndex, err)
			}
			contentBlocks[blockIndex]["source"] = encodedSource
			contentChanged = true
		}

		if contentChanged {
			encodedContent, err := common.Marshal(contentBlocks)
			if err != nil {
				return false, fmt.Errorf("encode messages[%d] content: %w", messageIndex, err)
			}
			messages[messageIndex]["content"] = encodedContent
			changed = true
		}
	}
	return changed, nil
}
