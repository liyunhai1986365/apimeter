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
