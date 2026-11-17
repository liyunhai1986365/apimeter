package common

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestRelayInfoConvOptionsUsesNormalizedGeminiSafetySettings(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	original := settings.SafetySettings
	t.Cleanup(func() {
		settings.SafetySettings = original
	})
	settings.SafetySettings = map[string]string{
		"HARM_CATEGORY_HATE_SPEECH":       "",
		"HARM_CATEGORY_DANGEROUS_CONTENT": "BLOCK_ONLY_HIGH",
	}

	options := (&RelayInfo{}).ConvOptions()

	assert.Equal(t, "OFF", options.Gemini.SafetySetting("HARM_CATEGORY_HATE_SPEECH"))
	assert.Equal(t, "BLOCK_ONLY_HIGH", options.Gemini.SafetySetting("HARM_CATEGORY_DANGEROUS_CONTENT"))
}

func TestGeminiSchemaCapabilityUsesRequestModelVersion(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	oldVersions, oldAdapter := settings.VersionSettings, settings.ThinkingAdapterEnabled
	t.Cleanup(func() { settings.VersionSettings = oldVersions; settings.ThinkingAdapterEnabled = oldAdapter })
	settings.VersionSettings = map[string]string{"default": "v1beta", "gemini-2.5-flash": "v1"}
	settings.ThinkingAdapterEnabled = true
	info := &RelayInfo{OriginModelName: "gemini-2.5-flash-high"}
	assert.False(t, info.ConvOptions().Gemini.SupportsJSONSchemaModel("gemini-2.5-flash-high"))
	assert.Equal(t, "gemini-2.5-flash", model_setting.GeminiRequestModelName(info.OriginModelName, "gemini-2.5-flash-high"))
}
