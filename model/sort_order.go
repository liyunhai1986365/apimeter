package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const DefaultSortOrder = 100

type SortOrderUpdate struct {
	ID        int `json:"id"`
	SortOrder int `json:"sort_order"`
}

func updateSortOrders(table any, updates []SortOrderUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			if update.ID <= 0 {
				continue
			}
			if err := tx.Model(table).Where("id = ?", update.ID).Updates(map[string]interface{}{
				"sort_order":   update.SortOrder,
				"updated_time": now,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func UpdateVendorSortOrders(updates []SortOrderUpdate) error {
	return updateSortOrders(&Vendor{}, updates)
}

func UpdateModelSortOrders(updates []SortOrderUpdate) error {
	return updateSortOrders(&Model{}, updates)
}
