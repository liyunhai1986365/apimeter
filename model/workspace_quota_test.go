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
