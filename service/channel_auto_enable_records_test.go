package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestFindAutoEnableOperationRecordCandidatesFiltersEligibleChannels(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := int64(1_700_000_000)
	autoBanEnabled := 1
	autoBanDisabled := 0

	require.NoError(t, model.DB.Create(&[]model.Channel{
		{Id: 11, Name: "eligible", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "gpt-test", AutoBan: &autoBanEnabled},
		{Id: 12, Name: "already-enabled", Type: 1, Status: common.ChannelStatusEnabled, Models: "gpt-test", AutoBan: &autoBanEnabled},
		{Id: 13, Name: "manual", Type: 1, Status: common.ChannelStatusManuallyDisabled, Models: "gpt-test", AutoBan: &autoBanEnabled},
		{Id: 14, Name: "auto-ban-off", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "gpt-test", AutoBan: &autoBanDisabled},
		{Id: 15, Name: "reenabled", Type: 1, Status: common.ChannelStatusAutoDisabled, Models: "gpt-test", AutoBan: &autoBanEnabled},
	}).Error)

	require.NoError(t, model.DB.Create(&[]model.ChannelOperationRecord{
		{ChannelID: 11, ChannelName: "eligible", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusAutoDisabled, ModelName: "old-model", Success: true, CreatedAt: now - 600},
		{ChannelID: 11, ChannelName: "eligible", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusAutoDisabled, ModelName: "gpt-test", Success: true, CreatedAt: now - 300},
		{ChannelID: 12, ChannelName: "already-enabled", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusAutoDisabled, ModelName: "gpt-test", Success: true, CreatedAt: now - 300},
		{ChannelID: 13, ChannelName: "manual", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusAutoDisabled, ModelName: "gpt-test", Success: true, CreatedAt: now - 300},
		{ChannelID: 14, ChannelName: "auto-ban-off", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusAutoDisabled, ModelName: "gpt-test", Success: true, CreatedAt: now - 300},
		{ChannelID: 15, ChannelName: "reenabled", Action: model.ChannelOperationActionDisable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusAutoDisabled, ModelName: "gpt-test", Success: true, CreatedAt: now - 300},
		{ChannelID: 15, ChannelName: "reenabled", Action: model.ChannelOperationActionEnable, Source: model.ChannelOperationSourceAuto, Status: common.ChannelStatusEnabled, ModelName: "gpt-test", Success: true, CreatedAt: now - 200},
	}).Error)

	candidates, err := FindAutoEnableOperationRecordCandidates(AutoEnableOperationRecordCandidateQuery{
		Now:             now,
		CooldownSeconds: 60,
		Limit:           20,
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.EqualValues(t, 11, candidates[0].Channel.Id)
	require.Equal(t, "gpt-test", candidates[0].Record.ModelName)
}

func TestFindAutoEnableOperationRecordCandidatesHonorsCooldown(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := int64(1_700_000_000)
	autoBanEnabled := 1

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      11,
		Name:    "cooling-down",
		Type:    1,
		Status:  common.ChannelStatusAutoDisabled,
		Models:  "gpt-test",
		AutoBan: &autoBanEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.ChannelOperationRecord{
		ChannelID:   11,
		ChannelName: "cooling-down",
		Action:      model.ChannelOperationActionDisable,
		Source:      model.ChannelOperationSourceAuto,
		Status:      common.ChannelStatusAutoDisabled,
		ModelName:   "gpt-test",
		Success:     true,
		CreatedAt:   now - 30,
	}).Error)

	candidates, err := FindAutoEnableOperationRecordCandidates(AutoEnableOperationRecordCandidateQuery{
		Now:             now,
		CooldownSeconds: 60,
		Limit:           20,
	})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestFindAutoEnableOperationRecordCandidatesUsesLatestDisableBeforeCooldown(t *testing.T) {
	setupModelMonitorTestDB(t)
	now := int64(1_700_000_000)
	autoBanEnabled := 1

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      11,
		Name:    "recent-disable",
		Type:    1,
		Status:  common.ChannelStatusAutoDisabled,
		Models:  "gpt-test",
		AutoBan: &autoBanEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&[]model.ChannelOperationRecord{
		{
			ChannelID:   11,
			ChannelName: "recent-disable",
			Action:      model.ChannelOperationActionDisable,
			Source:      model.ChannelOperationSourceAuto,
			Status:      common.ChannelStatusAutoDisabled,
			ModelName:   "old-model",
			Success:     true,
			CreatedAt:   now - 600,
		},
		{
			ChannelID:   11,
			ChannelName: "recent-disable",
			Action:      model.ChannelOperationActionDisable,
			Source:      model.ChannelOperationSourceAuto,
			Status:      common.ChannelStatusAutoDisabled,
			ModelName:   "new-model",
			Success:     true,
			CreatedAt:   now - 30,
		},
	}).Error)

	candidates, err := FindAutoEnableOperationRecordCandidates(AutoEnableOperationRecordCandidateQuery{
		Now:             now,
		CooldownSeconds: 60,
		Limit:           20,
	})
	require.NoError(t, err)
	require.Empty(t, candidates)
}
