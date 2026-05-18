package model_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveModelFallbackRuleUsesGlobalWhenUserInherits(t *testing.T) {
	original := modelFallbackSetting
	t.Cleanup(func() { modelFallbackSetting = original })

	modelFallbackSetting = ModelFallbackSetting{
		Enabled:           true,
		AllowUserOverride: true,
		Rules: []ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "qwen-3.6", Enabled: true},
		},
	}

	rule, mode, ok := ResolveModelFallbackRule("gpt-5.5", UserModelFallbackSetting{Mode: ModelFallbackModeInherit})

	require.True(t, ok)
	require.Equal(t, ModelFallbackModeInherit, mode)
	require.Equal(t, "qwen-3.6", rule.FallbackModel)
}

func TestResolveModelFallbackRuleUsesUserCustomWhenAllowed(t *testing.T) {
	original := modelFallbackSetting
	t.Cleanup(func() { modelFallbackSetting = original })

	modelFallbackSetting = ModelFallbackSetting{
		Enabled:           true,
		AllowUserOverride: true,
		Rules: []ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "global-fallback", Enabled: true},
		},
	}

	rule, mode, ok := ResolveModelFallbackRule("gpt-5.5", UserModelFallbackSetting{
		Mode:    ModelFallbackModeCustom,
		Enabled: true,
		Rules: []ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "user-fallback", Enabled: true},
		},
	})

	require.True(t, ok)
	require.Equal(t, ModelFallbackModeCustom, mode)
	require.Equal(t, "user-fallback", rule.FallbackModel)
}

func TestResolveModelFallbackRuleFallsBackToGlobalWhenUserOverrideDisabled(t *testing.T) {
	original := modelFallbackSetting
	t.Cleanup(func() { modelFallbackSetting = original })

	modelFallbackSetting = ModelFallbackSetting{
		Enabled:           true,
		AllowUserOverride: false,
		Rules: []ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "global-fallback", Enabled: true},
		},
	}

	rule, mode, ok := ResolveModelFallbackRule("gpt-5.5", UserModelFallbackSetting{
		Mode:    ModelFallbackModeCustom,
		Enabled: true,
		Rules: []ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "user-fallback", Enabled: true},
		},
	})

	require.True(t, ok)
	require.Equal(t, ModelFallbackModeInherit, mode)
	require.Equal(t, "global-fallback", rule.FallbackModel)
}

func TestValidateModelFallbackRulesJSONRejectsInvalidRule(t *testing.T) {
	err := ValidateModelFallbackRulesJSON(`[{"primary_model":"gpt-5.5","fallback_model":"gpt-5.5","enabled":true}]`)

	require.Error(t, err)
	require.Contains(t, err.Error(), "不能相同")
}

func TestShouldFallbackByStatusCodeUsesConfiguredStatusCodes(t *testing.T) {
	original := modelFallbackSetting
	t.Cleanup(func() { modelFallbackSetting = original })

	modelFallbackSetting.FailureStatusCodes = "429,500-503"

	require.True(t, ShouldFallbackByStatusCode(429))
	require.True(t, ShouldFallbackByStatusCode(500))
	require.True(t, ShouldFallbackByStatusCode(503))
	require.False(t, ShouldFallbackByStatusCode(504))
	require.False(t, ShouldFallbackByStatusCode(400))
}

func TestShouldFallbackByStatusCodeDefaultsToRetryRules(t *testing.T) {
	original := modelFallbackSetting
	t.Cleanup(func() { modelFallbackSetting = original })

	modelFallbackSetting.FailureStatusCodes = ""

	require.True(t, ShouldFallbackByStatusCode(401))
	require.True(t, ShouldFallbackByStatusCode(429))
	require.True(t, ShouldFallbackByStatusCode(500))
	require.False(t, ShouldFallbackByStatusCode(400))
	require.False(t, ShouldFallbackByStatusCode(504))
	require.False(t, ShouldFallbackByStatusCode(524))
}
