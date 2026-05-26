package model

const (
	NewAPISupplierChannelSyncStatusPending  = "pending"
	NewAPISupplierChannelSyncStatusCreated  = "created"
	NewAPISupplierChannelSyncStatusSynced   = "synced"
	NewAPISupplierChannelSyncStatusFailed   = "failed"
	NewAPISupplierChannelSyncStatusDisabled = "disabled"

	NewAPISupplierChannelStatusAvailable   = "available"
	NewAPISupplierChannelStatusUnavailable = "unavailable"

	NewAPISupplierProfileModelStatusUnknown     = "unknown"
	NewAPISupplierProfileModelStatusAvailable   = "available"
	NewAPISupplierProfileModelStatusUnavailable = "unavailable"
)

type NewAPISupplierChannelProfile struct {
	Id                   int      `json:"id"`
	SupplierID           int      `json:"supplier_id" gorm:"index:idx_supplier_profile_identity,unique;index"`
	SupplierNameSnapshot string   `json:"supplier_name_snapshot" gorm:"type:varchar(128)"`
	BaseURLSnapshot      string   `json:"base_url_snapshot" gorm:"type:varchar(512)"`
	UpstreamGroup        string   `json:"upstream_group" gorm:"type:varchar(64);index:idx_supplier_profile_identity,unique;index"`
	UpstreamGroupDesc    string   `json:"upstream_group_desc" gorm:"type:varchar(255)"`
	UpstreamGroupRatio   string   `json:"upstream_group_ratio" gorm:"type:varchar(32);index"`
	ModelSource          string   `json:"model_source" gorm:"type:varchar(64)"`
	ChannelType          int      `json:"channel_type" gorm:"default:1;index:idx_supplier_profile_identity,unique;index"`
	EndpointType         string   `json:"endpoint_type" gorm:"type:varchar(64);index:idx_supplier_profile_identity,unique;index"`
	LocalGroup           string   `json:"local_group" gorm:"type:varchar(64);index:idx_supplier_profile_identity,unique;index"`
	ChannelNameTemplate  string   `json:"channel_name_template" gorm:"type:varchar(255)"`
	Tag                  string   `json:"tag" gorm:"type:varchar(128);index"`
	Weight               *uint    `json:"weight" gorm:"default:0"`
	Priority             *int64   `json:"priority" gorm:"bigint;default:0"`
	AutoBan              int      `json:"auto_ban" gorm:"default:1"`
	BalanceThreshold     int      `json:"balance_threshold" gorm:"default:0"`
	ChannelRatio         *float64 `json:"channel_ratio" gorm:"default:1"`
	ChannelID            *int     `json:"channel_id" gorm:"index"`
	SyncMode             string   `json:"sync_mode" gorm:"type:varchar(32);default:'managed';index"`
	SyncStatus           string   `json:"sync_status" gorm:"type:varchar(32);default:'pending';index"`
	ChannelStatus        string   `json:"channel_status" gorm:"type:varchar(32);default:'available';index"`
	ConfigHash           string   `json:"config_hash" gorm:"type:varchar(64)"`
	ModelSetHash         string   `json:"model_set_hash" gorm:"type:varchar(64)"`
	LastCheckedAt        int64    `json:"last_checked_at" gorm:"bigint"`
	LastSyncedAt         int64    `json:"last_synced_at" gorm:"bigint"`
	LastError            string   `json:"last_error" gorm:"type:text"`
	CreatedTime          int64    `json:"created_time" gorm:"bigint"`
	UpdatedTime          int64    `json:"updated_time" gorm:"bigint"`
}

type NewAPISupplierChannelProfileModel struct {
	Id               int    `json:"id"`
	ProfileID        int    `json:"profile_id" gorm:"index:idx_supplier_profile_model,unique;index"`
	SupplierID       int    `json:"supplier_id" gorm:"index"`
	UpstreamGroup    string `json:"upstream_group" gorm:"type:varchar(64);index"`
	ModelName        string `json:"model_name" gorm:"type:varchar(255);index:idx_supplier_profile_model,unique;index"`
	ModelProvider    string `json:"model_provider" gorm:"type:varchar(128);index"`
	AvailableStatus  string `json:"available_status" gorm:"type:varchar(32);default:'unknown';index"`
	LastTestSuccess  bool   `json:"last_test_success" gorm:"default:false"`
	LastTestTime     int64  `json:"last_test_time" gorm:"bigint"`
	LastResponseTime int    `json:"last_response_time" gorm:"default:0"`
	LastError        string `json:"last_error" gorm:"type:text"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint"`
}
