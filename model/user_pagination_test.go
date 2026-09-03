package model

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUsersForPaginationTest(t *testing.T, total int) {
	t.Helper()
	for id := 1; id <= total; id++ {
		user := &User{
			Id:          id,
			Username:    fmt.Sprintf("user%02d", id),
			Password:    "password123",
			DisplayName: fmt.Sprintf("User %02d", id),
			Email:       fmt.Sprintf("user%02d@example.com", id),
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			Group:       "default",
			AffCode:     fmt.Sprintf("aff%02d", id),
		}
		require.NoError(t, DB.Create(user).Error)
	}
}

func collectUserIDs(users []*User) []int {
	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	return ids
}

func TestGetAllUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	pageOne, total, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, collectUserIDs(pageOne))

	pageTwo, total, err := GetAllUsers(&common.PageInfo{Page: 2, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(pageTwo))

	pageThree, total, err := GetAllUsers(&common.PageInfo{Page: 3, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{41, 42}, collectUserIDs(pageThree))
}

func TestSearchUsersSortsBeforePagination(t *testing.T) {
	truncateTables(t)
	insertUsersForPaginationTest(t, 42)

	users, total, err := SearchUsers(UserSearchFilters{Keyword: "user"}, 20, 20, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	assert.Equal(t, int64(42), total)
	assert.Equal(t, []int{21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}, collectUserIDs(users))
}

func TestSearchUsersSupportsManagementFilters(t *testing.T) {
	truncateTables(t)

	users := []*User{
		{Id: 101, Username: "alpha-user", Password: "password123", DisplayName: "Alpha User", Email: "alpha@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "vip", AffCode: "alpha-aff", InviterId: 501},
		{Id: 102, Username: "beta-user", Password: "password123", DisplayName: "Beta User", Email: "beta@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "beta-aff", InviterId: 502},
		{Id: 103, Username: "gamma-user", Password: "password123", DisplayName: "Gamma User", Email: "gamma@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "vip", AffCode: "gamma-aff", InviterId: 501},
	}
	for _, user := range users {
		require.NoError(t, DB.Create(user).Error)
	}

	alphaAgent := &Agent{OwnerUserId: 901, Name: "Alpha Agent", Slug: "alpha-agent-user-search", Status: AgentStatusEnabled, Branding: `{"site_name":"Alpha Portal"}`}
	betaAgent := &Agent{OwnerUserId: 902, Name: "Beta Agent", Slug: "beta-agent-user-search", Status: AgentStatusEnabled}
	require.NoError(t, DB.Create(alphaAgent).Error)
	require.NoError(t, DB.Create(betaAgent).Error)
	require.NoError(t, DB.Create(&[]AgentUser{
		{AgentId: alphaAgent.Id, UserId: 101, Status: AgentUserStatusEnabled},
		{AgentId: betaAgent.Id, UserId: 102, Status: AgentUserStatusEnabled},
		{AgentId: alphaAgent.Id, UserId: 103, Status: AgentUserStatusDisabled},
	}).Error)

	inviterId := 501
	tests := []struct {
		name     string
		filters  UserSearchFilters
		expected []int
	}{
		{name: "user id", filters: UserSearchFilters{Keyword: "102"}, expected: []int{102}},
		{name: "group", filters: UserSearchFilters{Group: "vip"}, expected: []int{101, 103}},
		{name: "agent name", filters: UserSearchFilters{Agent: "Alpha Agent"}, expected: []int{101}},
		{name: "agent site name", filters: UserSearchFilters{Agent: "Alpha Portal"}, expected: []int{101}},
		{name: "agent id", filters: UserSearchFilters{Agent: strconv.Itoa(betaAgent.Id)}, expected: []int{102}},
		{name: "inviter id", filters: UserSearchFilters{InviterId: &inviterId}, expected: []int{101, 103}},
		{name: "combined", filters: UserSearchFilters{Group: "vip", Agent: "Alpha", InviterId: &inviterId}, expected: []int{101}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, total, err := SearchUsers(tt.filters, 0, 20, NewUserSortOptions("id", "asc"))
			require.NoError(t, err)
			assert.Equal(t, int64(len(tt.expected)), total)
			assert.Equal(t, tt.expected, collectUserIDs(matched))
		})
	}
}
