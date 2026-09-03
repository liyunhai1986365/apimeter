package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestResolveEffectiveGroupRatioPrecedence(t *testing.T) {
	originalGroupRatio := GroupRatio2JSONString()
	originalGroupGroupRatio := GroupGroupRatio2JSONString()
	originalGroupModelRatio := GroupModelRatio2JSONString()
	originalUserGroupModelRatio := UserGroupModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateGroupRatioByJSONString(originalGroupRatio))
		require.NoError(t, UpdateGroupGroupRatioByJSONString(originalGroupGroupRatio))
		require.NoError(t, UpdateGroupModelRatioByJSONString(originalGroupModelRatio))
		require.NoError(t, UpdateUserGroupModelRatioByJSONString(originalUserGroupModelRatio))
	})

	require.NoError(t, UpdateGroupRatioByJSONString(`{"alibaba":1}`))
	require.NoError(t, UpdateGroupGroupRatioByJSONString(`{"vip":{"alibaba":0.8}}`))
	require.NoError(t, UpdateGroupModelRatioByJSONString(`{"alibaba":{"glm-5.2":0.7,"qwen3.7":0.5}}`))
	require.NoError(t, UpdateUserGroupModelRatioByJSONString(`{"vip":{"alibaba":{"glm-5.2":0.62,"free-model":0}}}`))

	tests := []struct {
		name           string
		userGroup      string
		group          string
		model          string
		wantRatio      float64
		wantSource     types.GroupRatioSource
		wantUserScoped bool
	}{
		{name: "user group model override", userGroup: "vip", group: "alibaba", model: "glm-5.2", wantRatio: 0.62, wantSource: types.GroupRatioSourceUserGroupModel, wantUserScoped: true},
		{name: "explicit zero override", userGroup: "vip", group: "alibaba", model: "free-model", wantRatio: 0, wantSource: types.GroupRatioSourceUserGroupModel, wantUserScoped: true},
		{name: "group model inherited by user group", userGroup: "vip", group: "alibaba", model: "qwen3.7", wantRatio: 0.5, wantSource: types.GroupRatioSourceGroupModel},
		{name: "group model default", userGroup: "default", group: "alibaba", model: "glm-5.2", wantRatio: 0.7, wantSource: types.GroupRatioSourceGroupModel},
		{name: "user group fallback", userGroup: "vip", group: "alibaba", model: "other", wantRatio: 0.8, wantSource: types.GroupRatioSourceUserGroup, wantUserScoped: true},
		{name: "group fallback", userGroup: "default", group: "alibaba", model: "other", wantRatio: 1, wantSource: types.GroupRatioSourceGroup},
		{name: "missing group fallback", userGroup: "default", group: "missing", model: "other", wantRatio: 1, wantSource: types.GroupRatioSourceDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveEffectiveGroupRatio(tt.userGroup, tt.group, tt.model)
			require.Equal(t, tt.wantRatio, got.Ratio)
			require.Equal(t, tt.wantSource, got.Source)
			require.Equal(t, tt.wantUserScoped, got.IsUserSpecific)
		})
	}
}

func TestModelSpecificRatioValidation(t *testing.T) {
	require.Error(t, CheckGroupModelRatio(`{"alibaba":{"glm-5.2":-0.1}}`))
	require.Error(t, CheckGroupModelRatio(`{"":{"glm-5.2":0.7}}`))
	require.Error(t, CheckUserGroupModelRatio(`{"vip":{"alibaba":{"":0.5}}}`))
	require.NoError(t, CheckGroupModelRatio(`{"alibaba":{"glm-5.2":0}}`))
	require.NoError(t, CheckUserGroupModelRatio(`{"vip":{"alibaba":{"glm-5.2":0.62}}}`))
}
