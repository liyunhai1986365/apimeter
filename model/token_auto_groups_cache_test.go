package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenGroupPolicyColdAndWarmCache(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	for _, policy := range []string{`{"type":"ordered","groups":["ch_GPT特价","gemini"]}`, ""} {
		token := createReserveTestToken(t, 100)
		token.Group = "ch_GPT特价"
		token.GroupPolicy = policy
		require.NoError(t, DB.Save(&token).Error)

		cold, err := GetTokenByKey(token.Key, false)
		require.NoError(t, err)
		assert.Equal(t, policy, cold.GroupPolicy)
		warm, err := cacheGetTokenByKey(token.Key)
		require.NoError(t, err)
		assert.Equal(t, token.Group, warm.Group)
		assert.Equal(t, policy, warm.GroupPolicy)
	}
}

func TestLegacyTokenGroupPolicyBackfillPreservesPendingDebit(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	ctx := context.Background()
	token := createReserveTestToken(t, 100)
	token.Group = "ch_GPT特价"
	token.GroupPolicy = `{"type":"ordered","groups":["ch_GPT特价","gemini"]}`
	require.NoError(t, DB.Save(&token).Error)
	require.NoError(t, cacheSetTokenForTest(token))
	cacheKey := getTokenCacheKey(token.Key)
	require.EqualValues(t, 1, common.RDB.HDel(ctx, cacheKey, "GroupPolicy").Val())
	_, err := cacheGetTokenByKey(token.Key)
	require.Error(t, err)
	status, err := cacheApplyTokenQuotaDelta(token.Id, token.Key, -70)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, status)

	reloaded, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.Equal(t, token.GroupPolicy, reloaded.GroupPolicy)
	assert.Equal(t, 30, reloaded.RemainQuota)
	assert.Equal(t, 70, reloaded.UsedQuota)

	stale := token
	stale.GroupPolicy = ""
	require.NoError(t, cacheSetTokenForTest(stale))
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, token.GroupPolicy, cached.GroupPolicy)
	assert.Equal(t, 30, cached.RemainQuota)
	assert.Equal(t, 70, cached.UsedQuota)
}

func TestTokenAutoGroupsRoundTripThroughRedisHashCache(t *testing.T) {
	useUserCacheMiniRedis(t)
	token := Token{
		Id:         42,
		UserId:     7,
		Key:        "token-auto-groups-cache-key",
		Name:       "auto-cache",
		Group:      "auto",
		AutoGroups: `["vip","default"]`,
	}

	require.NoError(t, cacheSetTokenForTest(token))
	cached, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.Equal(t, token.AutoGroups, cached.AutoGroups)
	groups, err := cached.GetAutoGroups()
	require.NoError(t, err)
	assert.Equal(t, []string{"vip", "default"}, groups)
}

func TestTokenUpdateSynchronouslyNarrowsPreheatedAutoGroupsCache(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)
	token := Token{
		UserId:          7,
		Key:             "token-auto-groups-update-cache-key",
		Name:            "auto-cache-update",
		Status:          common.TokenStatusEnabled,
		ExpiredTime:     -1,
		UnlimitedQuota:  true,
		Group:           "auto",
		CrossGroupRetry: true,
		AutoGroups:      `["default","vip"]`,
	}
	require.NoError(t, token.Insert())
	require.NoError(t, cacheSetTokenForTest(token))

	preheated, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	assert.JSONEq(t, `["default","vip"]`, preheated.AutoGroups)

	require.NoError(t, token.SetAutoGroups([]string{"vip"}))
	require.NoError(t, token.Update())
	// Update 是限制性变更：写库前删除缓存并设置 fence。缓存不再提供旧的
	// 宽分组值，下一次读取必须看到收紧后的分组。
	_, cacheErr := cacheGetTokenByKey(token.Key)
	require.Error(t, cacheErr, "the pre-update cache entry must be invalidated")
	reloaded, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.JSONEq(t, `["vip"]`, reloaded.AutoGroups)
}

// cacheSetTokenForTest 以测试身份写入完整 token 缓存（含额度字段），
// 模拟“已水合”的缓存状态。
func cacheSetTokenForTest(token Token) error {
	_, err := cacheInitToken(token)
	return err
}
