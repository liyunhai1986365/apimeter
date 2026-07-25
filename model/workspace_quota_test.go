package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserWorkspaceQuotaResetConfigAppliesInitialResetAndSchedulesNextReset(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 15, 30, 0, 0, time.UTC)
	workspace := Workspace{Id: 501, UserId: 1001, Name: "Quota Alpha", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&workspace).Error)
	require.NoError(t, DB.Create(&Token{
		Id:          601,
		UserId:      1001,
		WorkspaceId: workspace.Id,
		Name:        "alpha-initial",
		Key:         "alpha-initial-key",
		RemainQuota: 10,
		UsedQuota:   200,
		Status:      common.TokenStatusExhausted,
	}).Error)

	config, err := UpdateUserWorkspaceQuotaResetConfig(
		1001,
		workspace.Id,
		WorkspaceQuotaResetConfigRequest{
			Enabled: true,
			Period:  WorkspaceQuotaResetPeriodWeekly,
			Amount:  2500000,
		},
		now,
	)

	require.NoError(t, err)
	require.True(t, config.Enabled)
	require.Equal(t, WorkspaceQuotaResetPeriodWeekly, config.Period)
	require.Equal(t, 2500000, config.Amount)
	require.Equal(t, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC).Unix(), config.NextAt)
	require.Equal(t, now.Unix(), config.LastAppliedAt)

	var token Token
	require.NoError(t, DB.First(&token, 601).Error)
	require.Equal(t, 2500000, token.RemainQuota)
	require.Equal(t, 200, token.UsedQuota)
	require.Equal(t, common.TokenStatusEnabled, token.Status)
}

func TestUpdateUserWorkspaceQuotaResetConfigClearsTokenQuotaWhenDisabled(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 15, 45, 0, 0, time.UTC)
	workspace := Workspace{
		Id:                      505,
		UserId:                  1001,
		Name:                    "Quota Disable",
		Status:                  WorkspaceStatusEnabled,
		QuotaResetEnabled:       true,
		QuotaResetPeriod:        WorkspaceQuotaResetPeriodDaily,
		QuotaResetAmount:        3000000,
		QuotaResetLastAppliedAt: now.Add(-time.Hour).Unix(),
		QuotaResetNextAt:        now.Add(time.Hour).Unix(),
	}
	otherWorkspace := Workspace{Id: 506, UserId: 1001, Name: "Quota Other", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{workspace, otherWorkspace}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 605, UserId: 1001, WorkspaceId: workspace.Id, Name: "disable-main", Key: "disable-main-key", RemainQuota: 1200, UsedQuota: 200, UnlimitedQuota: false, Status: common.TokenStatusExhausted},
		{Id: 606, UserId: 1001, WorkspaceId: workspace.Id, Name: "disable-side", Key: "disable-side-key", RemainQuota: 800, UsedQuota: 300, UnlimitedQuota: false, Status: common.TokenStatusEnabled},
		{Id: 607, UserId: 1001, WorkspaceId: workspace.Id, Name: "disable-subscription", Key: "disable-sub-key", RemainQuota: 700, UsedQuota: 400, UnlimitedQuota: false, Status: common.TokenStatusEnabled, BillingSource: "subscription"},
		{Id: 608, UserId: 1001, WorkspaceId: otherWorkspace.Id, Name: "other-main", Key: "other-main-key", RemainQuota: 600, UsedQuota: 500, UnlimitedQuota: false, Status: common.TokenStatusEnabled},
	}).Error)

	config, err := UpdateUserWorkspaceQuotaResetConfig(
		1001,
		workspace.Id,
		WorkspaceQuotaResetConfigRequest{
			Enabled: false,
			Period:  WorkspaceQuotaResetPeriodDaily,
			Amount:  0,
		},
		now,
	)

	require.NoError(t, err)
	require.False(t, config.Enabled)
	require.Equal(t, 0, config.Amount)
	require.Equal(t, int64(0), config.NextAt)

	var main Token
	require.NoError(t, DB.First(&main, 605).Error)
	require.Equal(t, 0, main.RemainQuota)
	require.Equal(t, 200, main.UsedQuota)
	require.True(t, main.UnlimitedQuota)
	require.Equal(t, common.TokenStatusEnabled, main.Status)

	var side Token
	require.NoError(t, DB.First(&side, 606).Error)
	require.Equal(t, 0, side.RemainQuota)
	require.Equal(t, 300, side.UsedQuota)
	require.True(t, side.UnlimitedQuota)

	var subscription Token
	require.NoError(t, DB.First(&subscription, 607).Error)
	require.Equal(t, 700, subscription.RemainQuota)
	require.False(t, subscription.UnlimitedQuota)

	var other Token
	require.NoError(t, DB.First(&other, 608).Error)
	require.Equal(t, 600, other.RemainQuota)
	require.False(t, other.UnlimitedQuota)
}

func TestResetDueWorkspaceQuotaResetsWorkspaceTokens(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 0, 5, 0, 0, time.UTC)
	alpha := Workspace{
		Id:                      511,
		UserId:                  1001,
		Name:                    "Quota Reset Alpha",
		Status:                  WorkspaceStatusEnabled,
		QuotaResetEnabled:       true,
		QuotaResetPeriod:        WorkspaceQuotaResetPeriodDaily,
		QuotaResetAmount:        1500000,
		QuotaResetNextAt:        now.Add(-time.Minute).Unix(),
		QuotaResetLastAppliedAt: 0,
	}
	beta := Workspace{Id: 512, UserId: 1001, Name: "Quota Reset Beta", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{alpha, beta}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 611, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-main", Key: "alpha-main-key", RemainQuota: 10, UsedQuota: 200, Status: common.TokenStatusExhausted, UnlimitedQuota: true},
		{Id: 612, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-side", Key: "alpha-side-key", RemainQuota: 20, UsedQuota: 300, Status: common.TokenStatusEnabled},
		{Id: 613, UserId: 1001, WorkspaceId: alpha.Id, Name: "alpha-subscription", Key: "alpha-sub-key", RemainQuota: 30, UsedQuota: 400, Status: common.TokenStatusEnabled, BillingSource: "subscription"},
		{Id: 614, UserId: 1001, WorkspaceId: beta.Id, Name: "beta-main", Key: "beta-main-key", RemainQuota: 40, UsedQuota: 500, Status: common.TokenStatusEnabled},
	}).Error)

	resetCount, err := ResetDueWorkspaceQuotaConfigs(10, now)

	require.NoError(t, err)
	require.Equal(t, 1, resetCount)

	var alphaMain Token
	require.NoError(t, DB.First(&alphaMain, 611).Error)
	require.Equal(t, 1500000, alphaMain.RemainQuota)
	require.Equal(t, 200, alphaMain.UsedQuota)
	require.False(t, alphaMain.UnlimitedQuota)
	require.Equal(t, common.TokenStatusEnabled, alphaMain.Status)

	var alphaSide Token
	require.NoError(t, DB.First(&alphaSide, 612).Error)
	require.Equal(t, 1500000, alphaSide.RemainQuota)
	require.Equal(t, 300, alphaSide.UsedQuota)

	var subscription Token
	require.NoError(t, DB.First(&subscription, 613).Error)
	require.Equal(t, 30, subscription.RemainQuota)
	require.Equal(t, 400, subscription.UsedQuota)

	var betaMain Token
	require.NoError(t, DB.First(&betaMain, 614).Error)
	require.Equal(t, 40, betaMain.RemainQuota)

	var updated Workspace
	require.NoError(t, DB.First(&updated, alpha.Id).Error)
	require.Equal(t, now.Unix(), updated.QuotaResetLastAppliedAt)
	require.Equal(t, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC).Unix(), updated.QuotaResetNextAt)
}

func TestResetUserWorkspaceQuotaNowKeepsNextSchedule(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 10, 0, 0, 0, time.UTC)
	nextAt := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC).Unix()
	workspace := Workspace{
		Id:                521,
		UserId:            1001,
		Name:              "Manual Reset",
		Status:            WorkspaceStatusEnabled,
		QuotaResetEnabled: true,
		QuotaResetPeriod:  WorkspaceQuotaResetPeriodDaily,
		QuotaResetAmount:  1800000,
		QuotaResetNextAt:  nextAt,
	}
	require.NoError(t, DB.Create(&workspace).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 621, UserId: 1001, WorkspaceId: workspace.Id, Name: "manual-main", Key: "manual-main-key", RemainQuota: 10, UsedQuota: 200},
		{Id: 622, UserId: 1001, WorkspaceId: workspace.Id, Name: "manual-sub", Key: "manual-sub-key", RemainQuota: 20, UsedQuota: 300, BillingSource: "subscription"},
	}).Error)

	config, err := ResetUserWorkspaceQuotaNow(1001, workspace.Id, now)

	require.NoError(t, err)
	require.Equal(t, now.Unix(), config.LastAppliedAt)
	require.Equal(t, nextAt, config.NextAt)

	var normal Token
	require.NoError(t, DB.First(&normal, 621).Error)
	require.Equal(t, 1800000, normal.RemainQuota)
	require.Equal(t, 200, normal.UsedQuota)

	var subscription Token
	require.NoError(t, DB.First(&subscription, 622).Error)
	require.Equal(t, 20, subscription.RemainQuota)
}

func TestResetUserTokenWorkspaceQuotaOnlyResetsSelectedToken(t *testing.T) {
	truncateTables(t)

	now := time.Date(2026, 6, 10, 10, 30, 0, 0, time.UTC)
	workspace := Workspace{
		Id:                531,
		UserId:            1001,
		Name:              "Single Reset",
		Status:            WorkspaceStatusEnabled,
		QuotaResetEnabled: true,
		QuotaResetPeriod:  WorkspaceQuotaResetPeriodDaily,
		QuotaResetAmount:  2100000,
		QuotaResetNextAt:  now.Add(12 * time.Hour).Unix(),
	}
	require.NoError(t, DB.Create(&workspace).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 631, UserId: 1001, WorkspaceId: workspace.Id, Name: "single-main", Key: "single-main-key", RemainQuota: 0, UsedQuota: 200, Status: common.TokenStatusExhausted, UnlimitedQuota: true},
		{Id: 632, UserId: 1001, WorkspaceId: workspace.Id, Name: "single-side", Key: "single-side-key", RemainQuota: 20, UsedQuota: 300, Status: common.TokenStatusEnabled},
	}).Error)

	token, err := ResetUserTokenWorkspaceQuota(1001, 631, now, nil)

	require.NoError(t, err)
	require.Equal(t, 2100000, token.RemainQuota)
	require.Equal(t, 200, token.UsedQuota)
	require.False(t, token.UnlimitedQuota)
	require.Equal(t, common.TokenStatusEnabled, token.Status)
	require.Equal(t, workspace.Name, token.WorkspaceName)

	var other Token
	require.NoError(t, DB.First(&other, 632).Error)
	require.Equal(t, 20, other.RemainQuota)
	require.Equal(t, 300, other.UsedQuota)
}

func TestDeleteUserWorkspaceMovesTokensToDefaultWorkspace(t *testing.T) {
	truncateTables(t)

	defaultWorkspace, err := EnsureDefaultWorkspace(1001)
	require.NoError(t, err)
	project := Workspace{Id: 701, UserId: 1001, Name: "Delete Project", Status: WorkspaceStatusEnabled}
	otherUserWorkspace := Workspace{Id: 702, UserId: 2002, Name: "Other User", Status: WorkspaceStatusEnabled}
	require.NoError(t, DB.Create(&[]Workspace{project, otherUserWorkspace}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 801, UserId: 1001, WorkspaceId: project.Id, Name: "project-main", Key: "project-main-key"},
		{Id: 802, UserId: 1001, WorkspaceId: project.Id, Name: "project-side", Key: "project-side-key"},
		{Id: 803, UserId: 2002, WorkspaceId: otherUserWorkspace.Id, Name: "other-main", Key: "other-main-key"},
	}).Error)

	err = DeleteUserWorkspace(1001, project.Id)

	require.NoError(t, err)
	var deleted Workspace
	require.Error(t, DB.First(&deleted, project.Id).Error)
	var moved []Token
	require.NoError(t, DB.Where("user_id = ? AND id IN ?", 1001, []int{801, 802}).Find(&moved).Error)
	require.Len(t, moved, 2)
	for _, token := range moved {
		require.Equal(t, defaultWorkspace.Id, token.WorkspaceId)
	}
	var other Token
	require.NoError(t, DB.First(&other, 803).Error)
	require.Equal(t, otherUserWorkspace.Id, other.WorkspaceId)
}
