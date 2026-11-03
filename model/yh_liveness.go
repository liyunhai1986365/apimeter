package model

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm/clause"
)

// Reserve short supplier names across nodes. These names remain reserved after
// the local query window expires because the upstream group can still exist.
func ReserveYHSessionName(ctx context.Context, session AssetBinding) error {
	ctx, cancel := context.WithTimeout(ctx, AssetOperationTimeout)
	defer cancel()
	if session.SessionName == "" || session.Kind != "session" || session.ID == "" {
		return fmt.Errorf("invalid YH session")
	}
	now := common.GetTimestamp()
	state := ConfigurableResourceState{ProfileID: "yh-liveness-name-v1", ResourceID: "name", PreRequestID: session.Scope, StateKey: session.SessionName,
		StateValue: assetBindingKey(session.ID), Status: ConfigurableResourceStateStatusActive, CreatedAt: now, UpdatedAt: now, LastUsedAt: now}
	result := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&state)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("YH session name is already reserved")
	}
	return nil
}
