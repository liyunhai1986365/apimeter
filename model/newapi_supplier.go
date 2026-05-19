package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	NewAPISupplierStatusEnabled   = 1
	NewAPISupplierStatusDisabled  = 0
	NewAPISupplierStatusError     = 2
	NewAPISupplierSyncModeManaged = "managed"
)

type NewAPISupplier struct {
	Id                   int            `json:"id"`
	Name                 string         `json:"name" gorm:"size:128;not null;index"`
	BaseURL              string         `json:"base_url" gorm:"type:varchar(512);not null"`
	Username             string         `json:"username" gorm:"type:varchar(128)"`
	Password             string         `json:"password,omitempty" gorm:"type:varchar(255)"`
	AccessToken          string         `json:"access_token,omitempty" gorm:"type:varchar(255)"`
	APIKey               string         `json:"api_key,omitempty" gorm:"type:varchar(255)"`
	UpstreamUserID       int            `json:"upstream_user_id"`
	UpstreamUsername     string         `json:"upstream_username" gorm:"type:varchar(128)"`
	Status               int            `json:"status" gorm:"default:1"`
	Quota                int            `json:"quota" gorm:"default:0"`
	UsedQuota            int            `json:"used_quota" gorm:"default:0"`
	GroupsJSON           string         `json:"groups_json" gorm:"type:text"`
	GroupModelsJSON      string         `json:"group_models_json" gorm:"type:text"`
	GroupKeysJSON        string         `json:"group_keys_json,omitempty" gorm:"type:text"`
	ModelSource          string         `json:"model_source" gorm:"type:varchar(64)"`
	DefaultLocalGroup    string         `json:"default_local_group" gorm:"type:varchar(64);default:'default'"`
	SelectedModelsJSON   string         `json:"selected_models_json" gorm:"type:text"`
	Tag                  string         `json:"tag" gorm:"type:varchar(128);index"`
	ChannelType          int            `json:"channel_type" gorm:"default:1"`
	Weight               *uint          `json:"weight" gorm:"default:0"`
	Priority             *int64         `json:"priority" gorm:"bigint;default:0"`
	AutoBan              int            `json:"auto_ban" gorm:"default:1"`
	BalanceThreshold     int            `json:"balance_threshold" gorm:"default:0"`
	LastCheckTime        int64          `json:"last_check_time" gorm:"bigint"`
	LastConfigureTime    int64          `json:"last_configure_time" gorm:"bigint"`
	LastError            string         `json:"last_error" gorm:"type:text"`
	Remark               string         `json:"remark" gorm:"type:varchar(255)"`
	CreatedTime          int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime          int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
	BoundChannelCount    int64          `json:"bound_channel_count" gorm:"-"`
	ConfiguredChannelIDs []int          `json:"configured_channel_ids" gorm:"-"`
}

type NewAPISupplierChannel struct {
	Id            int    `json:"id"`
	SupplierID    int    `json:"supplier_id" gorm:"index:idx_newapi_supplier_channel,unique"`
	ChannelID     int    `json:"channel_id" gorm:"index:idx_newapi_supplier_channel,unique;index"`
	UpstreamGroup string `json:"upstream_group" gorm:"type:varchar(64);index"`
	ChannelType   int    `json:"channel_type" gorm:"default:1;index"`
	LocalGroup    string `json:"local_group" gorm:"type:varchar(64)"`
	Models        string `json:"models" gorm:"type:text"`
	SyncMode      string `json:"sync_mode" gorm:"type:varchar(32);default:'managed'"`
	CreatedTime   int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime   int64  `json:"updated_time" gorm:"bigint"`
}

func (supplier *NewAPISupplier) Normalize() {
	supplier.Name = strings.TrimSpace(supplier.Name)
	supplier.BaseURL = strings.TrimRight(strings.TrimSpace(supplier.BaseURL), "/")
	supplier.Username = strings.TrimSpace(supplier.Username)
	supplier.AccessToken = strings.TrimSpace(supplier.AccessToken)
	supplier.APIKey = strings.TrimSpace(supplier.APIKey)
	supplier.DefaultLocalGroup = strings.TrimSpace(supplier.DefaultLocalGroup)
	if supplier.DefaultLocalGroup == "" {
		supplier.DefaultLocalGroup = "default"
	}
	supplier.Tag = strings.TrimSpace(supplier.Tag)
	if supplier.Tag == "" {
		supplier.Tag = supplier.Name
	}
	if supplier.ChannelType == 0 {
		supplier.ChannelType = 1
	}
}

func (supplier *NewAPISupplier) Insert() error {
	supplier.Normalize()
	if supplier.Status == NewAPISupplierStatusDisabled {
		supplier.Status = NewAPISupplierStatusEnabled
	}
	now := common.GetTimestamp()
	supplier.CreatedTime = now
	supplier.UpdatedTime = now
	return DB.Create(supplier).Error
}

func (supplier *NewAPISupplier) Update() error {
	supplier.Normalize()
	supplier.UpdatedTime = common.GetTimestamp()
	return DB.Model(&NewAPISupplier{}).Where("id = ?", supplier.Id).Updates(supplier).Error
}

func GetNewAPISupplierByID(id int) (*NewAPISupplier, error) {
	var supplier NewAPISupplier
	if err := DB.First(&supplier, id).Error; err != nil {
		return nil, err
	}
	fillNewAPISupplierCounts([]*NewAPISupplier{&supplier})
	return &supplier, nil
}

func GetAllNewAPISuppliers(offset int, limit int) ([]*NewAPISupplier, int64, error) {
	var total int64
	if err := DB.Model(&NewAPISupplier{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var suppliers []*NewAPISupplier
	if err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&suppliers).Error; err != nil {
		return nil, 0, err
	}
	fillNewAPISupplierCounts(suppliers)
	return suppliers, total, nil
}

func SearchNewAPISuppliers(keyword string, offset int, limit int) ([]*NewAPISupplier, int64, error) {
	db := DB.Model(&NewAPISupplier{})
	if strings.TrimSpace(keyword) != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		db = db.Where("name LIKE ? OR base_url LIKE ? OR username LIKE ? OR remark LIKE ?", like, like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var suppliers []*NewAPISupplier
	if err := db.Order("id DESC").Offset(offset).Limit(limit).Find(&suppliers).Error; err != nil {
		return nil, 0, err
	}
	fillNewAPISupplierCounts(suppliers)
	return suppliers, total, nil
}

func fillNewAPISupplierCounts(suppliers []*NewAPISupplier) {
	if len(suppliers) == 0 {
		return
	}
	ids := make([]int, 0, len(suppliers))
	byID := make(map[int]*NewAPISupplier, len(suppliers))
	for _, supplier := range suppliers {
		ids = append(ids, supplier.Id)
		byID[supplier.Id] = supplier
	}
	var rows []struct {
		SupplierID int
		Count      int64
	}
	if err := DB.Model(&NewAPISupplierChannel{}).
		Select("supplier_id, count(*) as count").
		Where("supplier_id IN ?", ids).
		Group("supplier_id").
		Scan(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		if supplier := byID[row.SupplierID]; supplier != nil {
			supplier.BoundChannelCount = row.Count
		}
	}
}
