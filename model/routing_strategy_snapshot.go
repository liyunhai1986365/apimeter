package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RoutingStrategySmartAuto    = "smart_auto"
	RoutingStrategyPriceFirst   = "price_first"
	RoutingStrategySpeedFirst   = "speed_first"
	RoutingStrategySuccessFirst = "success_first"
)

type RoutingStrategySnapshot struct {
	Id             int    `json:"id" gorm:"primaryKey"`
	Strategy       string `json:"strategy" gorm:"type:varchar(32);uniqueIndex:idx_routing_strategy_user_group,priority:1;index"`
	UserGroup      string `json:"user_group" gorm:"type:varchar(64);uniqueIndex:idx_routing_strategy_user_group,priority:2;index"`
	Groups         string `json:"groups" gorm:"type:text"`
	Scores         string `json:"scores" gorm:"type:text"`
	Config         string `json:"config" gorm:"type:text"`
	ManualOverride bool   `json:"manual_override" gorm:"default:false"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;index"`
}

func ValidRoutingStrategy(strategy string) bool {
	switch strategy {
	case RoutingStrategySmartAuto, RoutingStrategyPriceFirst, RoutingStrategySpeedFirst, RoutingStrategySuccessFirst:
		return true
	default:
		return false
	}
}

func RoutingStrategies() []string {
	return []string{
		RoutingStrategySmartAuto,
		RoutingStrategyPriceFirst,
		RoutingStrategySpeedFirst,
		RoutingStrategySuccessFirst,
	}
}

func UpsertRoutingStrategySnapshot(snapshot *RoutingStrategySnapshot) error {
	if snapshot == nil {
		return nil
	}
	if snapshot.UpdatedAt == 0 {
		snapshot.UpdatedAt = common.GetTimestamp()
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "strategy"},
			{Name: "user_group"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"groups",
			"scores",
			"config",
			"manual_override",
			"updated_at",
		}),
	}).Create(snapshot).Error
}

func GetRoutingStrategySnapshot(strategy string, userGroup string) (*RoutingStrategySnapshot, error) {
	var snapshot RoutingStrategySnapshot
	err := DB.Where("strategy = ? AND user_group = ?", strategy, userGroup).First(&snapshot).Error
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}

func RoutingStrategySnapshotHasManualOverride(strategy string, userGroup string) (bool, error) {
	var snapshot RoutingStrategySnapshot
	err := DB.Select("manual_override").Where("strategy = ? AND user_group = ?", strategy, userGroup).First(&snapshot).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return snapshot.ManualOverride, nil
}

func ListRoutingStrategySnapshots() ([]RoutingStrategySnapshot, error) {
	var snapshots []RoutingStrategySnapshot
	err := DB.Order("strategy ASC, user_group ASC").Find(&snapshots).Error
	return snapshots, err
}
