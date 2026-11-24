package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestSharedRoutingAndRegroupIsolation(t *testing.T) {
	SetupChannelBatchGroupTestDBForTest(t)
	checkSharedRoutingAndRegroupIsolation(t)
}

func checkSharedRoutingAndRegroupIsolation(t *testing.T) {
	orig, groups, ids, configs := common.MemoryCacheEnabled, group2model2channels, channelsIDM, channel2advancedCustomConfig
	t.Cleanup(func() {
		common.MemoryCacheEnabled, group2model2channels, channelsIDM, channel2advancedCustomConfig = orig, groups, ids, configs
	})
	tag := "mixed"
	platform := Channel{Group: "default", Models: "shared", Tag: &tag}
	require.NoError(t, platform.Insert())
	personal := Channel{Scope: ChannelScopeUserOwned, OwnerUserId: 77, Models: "shared,private", Tag: &tag, Priority: common.GetPointer(int64(99))}
	require.NoError(t, InsertUserOwnedProvider(&personal))

	for _, edit := range []func() error{
		func() error { return BatchSetChannelGroups([]int{platform.Id, personal.Id}, "vip") },
		func() error {
			return EditChannelByTag(tag, common.GetPointer("renamed"), nil, nil, common.GetPointer("vip"), nil, nil, nil, nil)
		},
		func() error { return (&Channel{Id: personal.Id, Group: "vip"}).Update() },
		func() error { return (&Channel{Id: personal.Id, Scope: ChannelScopePlatform, Group: "vip"}).Update() },
	} {
		require.ErrorIs(t, edit(), ErrPlatformChannelRequired)
	}
	current, err := GetChannelById(platform.Id, true)
	require.NoError(t, err)
	require.Equal(t, "default", current.Group)
	require.Equal(t, tag, current.GetTag())
	owned, err := GetUserOwnedProviderChannelForGroup(77, personal.Group, "shared")
	require.NoError(t, err)
	require.Equal(t, personal.Id, owned.Id)
	_, err = GetUserOwnedProviderChannelForGroup(78, personal.Group, "shared")
	require.Error(t, err)

	// Previously written shared abilities must not influence priority, filters, or affinity.
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: personal.Id, Group: "default", Model: "shared", Priority: personal.Priority, Enabled: true},
		{ChannelId: personal.Id, Group: "default", Model: "private", Priority: personal.Priority, Enabled: true},
	}).Error)
	for _, cached := range []bool{false, true} {
		t.Run("cache="+strconv.FormatBool(cached), func(t *testing.T) {
			common.MemoryCacheEnabled = cached
			InitChannelCache()
			for _, filter := range []ChannelFilter{nil, func(ch *Channel) bool {
				require.NotEqual(t, personal.Id, ch.Id)
				return true
			}} {
				for _, retry := range []int{0, 1} {
					selected, err := GetRandomSatisfiedChannelWithFilter("default", "shared", retry, filter)
					require.NoError(t, err)
					require.NotNil(t, selected)
					require.Equal(t, platform.Id, selected.Id)
				}
			}
			selected, err := GetRandomSatisfiedChannel("default", "private", 0)
			require.NoError(t, err)
			require.Nil(t, selected)
			require.False(t, IsChannelEnabledForGroupModel("default", "shared", personal.Id))
			require.True(t, IsChannelEnabledForGroupModel("default", "shared", platform.Id))
			require.True(t, IsChannelEnabledForGroupModel(personal.Group, "shared", personal.Id))
		})
	}
	// A tag rename combined with a platform group edit must rebuild exactly the matched rows.
	platformTag, nextTag, nextGroup := "platform-only", "renamed", "vip"
	require.NoError(t, DB.Model(&platform).Update("tag", platformTag).Error)
	require.NoError(t, EditChannelByTag(platformTag, &nextTag, nil, nil, &nextGroup, nil, nil, nil, nil))
	var rebuilt []Ability
	require.NoError(t, DB.Where("channel_id = ?", platform.Id).Find(&rebuilt).Error)
	require.Len(t, rebuilt, 1)
	require.Equal(t, nextGroup, rebuilt[0].Group)
	require.Equal(t, &nextTag, rebuilt[0].Tag)
}

func TestSharedChannelCacheUsesOnlyCurrentEnabledAbilities(t *testing.T) {
	SetupChannelBatchGroupTestDBForTest(t)
	orig, groups, ids, configs := common.MemoryCacheEnabled, group2model2channels, channelsIDM, channel2advancedCustomConfig
	t.Cleanup(func() {
		common.MemoryCacheEnabled, group2model2channels, channelsIDM, channel2advancedCustomConfig = orig, groups, ids, configs
	})
	common.MemoryCacheEnabled = true
	ch := Channel{Group: "one,two", Models: "good,disabled,good"}
	require.NoError(t, ch.Insert())
	missing := Channel{Group: "missing", Models: "good"}
	require.NoError(t, DB.Create(&missing).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("model = ?", "disabled").Update("enabled", false).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: ch.Id, Group: "old", Model: "good", Enabled: true},
		{ChannelId: ch.Id, Group: "one", Model: "old", Enabled: true},
		{ChannelId: 99999, Group: "one", Model: "orphan", Enabled: true},
	}).Error)
	off := Channel{Group: "one", Models: "off"}
	require.NoError(t, off.Insert())
	require.NoError(t, DB.Model(&off).Update("status", common.ChannelStatusManuallyDisabled).Error)
	p := Channel{Scope: ChannelScopeUserOwned, OwnerUserId: 77, Models: "private", Key: "alpha\nbeta",
		ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeyMode: constant.MultiKeyModePolling}}
	require.NoError(t, InsertUserOwnedProvider(&p))
	InitChannelCache()
	require.Equal(t, map[string]map[string][]int{
		"one": {"good": {ch.Id}}, "two": {"good": {ch.Id}},
	}, group2model2channels)
	require.NotContains(t, channelsIDM, p.Id)
	for _, want := range []string{"alpha", "beta", "alpha"} {
		current, err := CacheGetChannel(p.Id)
		require.NoError(t, err)
		key, _, apiErr := current.GetNextEnabledKey()
		require.Nil(t, apiErr)
		require.Equal(t, want, key)
	}
	require.True(t, IsChannelEnabledForGroupModel(p.Group, "private", p.Id))
	require.False(t, IsChannelEnabledForGroupModel(p.Group, "private", ch.Id))
	for _, useCache := range []bool{true, false} {
		common.MemoryCacheEnabled = useCache
		found, err := GetRandomSatisfiedChannel(p.Group, "private", 0)
		require.NoError(t, err)
		require.Nil(t, found)
	}
}

func TestPersonalChannelTransactionsRollbackAbilityFailures(t *testing.T) {
	SetupChannelBatchGroupTestDBForTest(t)
	p := Channel{Scope: ChannelScopeUserOwned, OwnerUserId: 77, Models: "first"}
	require.NoError(t, InsertUserOwnedProvider(&p))
	require.Error(t, DeleteUserOwnedProvider(78, p.Id))
	require.NoError(t, DB.Migrator().DropTable(&Ability{}))
	require.Error(t, DeleteUserOwnedProvider(77, p.Id))
	second := Channel{Scope: ChannelScopeUserOwned, OwnerUserId: 77, Models: "second"}
	require.Error(t, InsertUserOwnedProvider(&second))
	var kept []int
	require.NoError(t, DB.Model(&Channel{}).Pluck("id", &kept).Error)
	require.Equal(t, []int{p.Id}, kept)
}

func TestUserOwnedProviderGroupRoundTripAndOwnership(t *testing.T) {
	SetupChannelBatchGroupTestDBForTest(t)

	group := BuildUserOwnedProviderGroup(1001, 2002)
	require.Equal(t, "user_owned:1001:2002", group)

	userID, channelID, ok := ParseUserOwnedProviderGroup(group)
	require.True(t, ok)
	require.Equal(t, 1001, userID)
	require.Equal(t, 2002, channelID)

	require.False(t, IsUserOwnedProviderGroup("default"))
	require.True(t, IsUserOwnedProviderGroup(group))

	ratio := 1.0
	channel := Channel{
		Id:            2002,
		Type:          1,
		Key:           "sk-user-owned",
		Name:          "User OpenAI",
		Models:        "gpt-4o,gpt-4.1",
		Group:         group,
		ChannelRatio:  &ratio,
		Status:        common.ChannelStatusEnabled,
		Scope:         ChannelScopeUserOwned,
		OwnerUserId:   1001,
		CreatedTime:   1,
		RetryEnabled:  common.GetPointer(true),
		ChannelInfo:   ChannelInfo{},
		OtherSettings: "",
	}
	require.NoError(t, channel.Insert())

	ok, err := UserOwnsProviderGroup(1001, group)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = UserOwnsProviderGroup(1002, group)
	require.NoError(t, err)
	require.False(t, ok)

	owned, err := GetUserOwnedProviderChannelForGroup(1001, group, "gpt-4o")
	require.NoError(t, err)
	require.Equal(t, channel.Id, owned.Id)

	_, err = GetUserOwnedProviderChannelForGroup(1001, group, "claude-3-5-sonnet")
	require.Error(t, err)
}

func TestGetUserOwnedProviderStatsAggregatesCurrentUserConsumeLogs(t *testing.T) {
	truncateTables(t)

	stats, err := GetUserOwnedProviderStats(1001, []int{2002, 2003})
	require.NoError(t, err)
	require.Empty(t, stats)

	require.NoError(t, LOG_DB.Create(&[]Log{
		{Id: 1, UserId: 1001, Type: LogTypeConsume, ChannelId: 2002, Quota: 500, PromptTokens: 10, CompletionTokens: 20},
		{Id: 2, UserId: 1001, Type: LogTypeConsume, ChannelId: 2002, Quota: 700, PromptTokens: 30, CompletionTokens: 40},
		{Id: 3, UserId: 1001, Type: LogTypeConsume, ChannelId: 2003, Quota: 300, PromptTokens: 5, CompletionTokens: 6},
		{Id: 4, UserId: 1002, Type: LogTypeConsume, ChannelId: 2002, Quota: 900, PromptTokens: 90, CompletionTokens: 90},
		{Id: 5, UserId: 1001, Type: LogTypeError, ChannelId: 2002, Quota: 800, PromptTokens: 80, CompletionTokens: 80},
		{Id: 6, UserId: 1001, Type: LogTypeConsume, ChannelId: 3001, Quota: 600, PromptTokens: 60, CompletionTokens: 60},
	}).Error)

	stats, err = GetUserOwnedProviderStats(1001, []int{2002, 2003})
	require.NoError(t, err)

	require.Equal(t, int64(2), stats[2002].RequestCount)
	require.Equal(t, 1200, stats[2002].Quota)
	require.Equal(t, 40, stats[2002].PromptTokens)
	require.Equal(t, 60, stats[2002].CompletionTokens)

	require.Equal(t, int64(1), stats[2003].RequestCount)
	require.Equal(t, 300, stats[2003].Quota)
	require.Equal(t, 5, stats[2003].PromptTokens)
	require.Equal(t, 6, stats[2003].CompletionTokens)
}
