package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

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
