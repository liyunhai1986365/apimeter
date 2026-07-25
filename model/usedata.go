package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	NodeName  string `json:"node_name" gorm:"size:128;default:''"`
	TokenID   int    `json:"token_id" gorm:"default:0;index"`
	UseGroup  string `json:"use_group" gorm:"size:64;default:'';index"`
	ChannelID int    `json:"channel_id" gorm:"default:0;index"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed int    `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

type UsageDimensionTrendData struct {
	CreatedAt     int64  `json:"created_at"`
	TokenId       int    `json:"token_id"`
	TokenName     string `json:"token_name"`
	WorkspaceId   int    `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
	Count         int    `json:"count"`
	TokenUsed     int    `json:"token_used"`
	Quota         int    `json:"quota"`
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

type QuotaDataLogParams struct {
	UserID    int
	Username  string
	ModelName string
	Quota     int
	CreatedAt int64
	TokenUsed int
	TokenID   int
	UseGroup  string
	ChannelID int
	NodeName  string
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(params QuotaDataLogParams) {
	key := fmt.Sprintf("%d-%s-%s-%d-%d-%s-%d-%s",
		params.UserID,
		params.Username,
		params.ModelName,
		params.CreatedAt,
		params.TokenID,
		params.UseGroup,
		params.ChannelID,
		params.NodeName,
	)
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += params.Quota
		quotaData.TokenUsed += params.TokenUsed
	} else {
		quotaData = &QuotaData{
			UserID:    params.UserID,
			Username:  params.Username,
			NodeName:  params.NodeName,
			TokenID:   params.TokenID,
			UseGroup:  params.UseGroup,
			ChannelID: params.ChannelID,
			ModelName: params.ModelName,
			CreatedAt: params.CreatedAt,
			Count:     1,
			Quota:     params.Quota,
			TokenUsed: params.TokenUsed,
		}
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(params QuotaDataLogParams) {
	// 只精确到小时
	params.CreatedAt = params.CreatedAt - (params.CreatedAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(params)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").Where(
			"user_id = ? and username = ? and node_name = ? and token_id = ? and use_group = ? and channel_id = ? and model_name = ? and created_at = ?",
			quotaData.UserID,
			quotaData.Username,
			quotaData.NodeName,
			quotaData.TokenID,
			quotaData.UseGroup,
			quotaData.ChannelID,
			quotaData.ModelName,
			quotaData.CreatedAt,
		).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData, quotaData.Count, quotaData.Quota, quotaData.TokenUsed)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(quotaData *QuotaData, count int, quota int, tokenUsed int) {
	err := DB.Table("quota_data").Where(
		"user_id = ? and username = ? and node_name = ? and token_id = ? and use_group = ? and channel_id = ? and model_name = ? and created_at = ?",
		quotaData.UserID,
		quotaData.Username,
		quotaData.NodeName,
		quotaData.TokenID,
		quotaData.UseGroup,
		quotaData.ChannelID,
		quotaData.ModelName,
		quotaData.CreatedAt,
	).Updates(map[string]interface{}{
		"count":      gorm.Expr("count + ?", count),
		"quota":      gorm.Expr("quota + ?", quota),
		"token_used": gorm.Expr("token_used + ?", tokenUsed),
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

// applyQuotaDataWorkspaceScope restricts a quota_data query to the caller's workspaces.
// nil means unrestricted; a non-nil empty slice means nothing is visible. quota_data
// lives in the main DB, so the token set is expressed as a subquery instead of an
// unbounded id list.
func applyQuotaDataWorkspaceScope(query *gorm.DB, allowedWorkspaceIds []int) *gorm.DB {
	if allowedWorkspaceIds == nil {
		return query
	}
	if len(allowedWorkspaceIds) == 0 {
		return query.Where("1 = 0")
	}
	return query.Where("quota_data.token_id IN (?)",
		DB.Table("tokens").Select("id").Where("workspace_id IN ?", allowedWorkspaceIds))
}

func GetSelfTokenQuotaData(startTime int64, endTime int64, userId int, allowedWorkspaceIds []int) (trendData []*UsageDimensionTrendData, err error) {
	var rows []*UsageDimensionTrendData
	tx := DB.Table("quota_data").
		Select("token_id, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("user_id = ?", userId)
	tx = applyQuotaDataWorkspaceScope(tx, allowedWorkspaceIds)
	if startTime != 0 {
		tx = tx.Where("created_at >= ?", startTime)
	}
	if endTime != 0 {
		tx = tx.Where("created_at <= ?", endTime)
	}
	if err = tx.Group("token_id, created_at").Order("created_at asc, token_id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	enrichUsageDimensionTrendTokenNames(rows)
	return rows, nil
}

func enrichUsageDimensionTrendTokenNames(trendData []*UsageDimensionTrendData) {
	if len(trendData) == 0 {
		return
	}
	tokenIdSet := make(map[int]struct{})
	for _, item := range trendData {
		if item != nil && item.TokenId > 0 {
			tokenIdSet[item.TokenId] = struct{}{}
		}
	}
	if len(tokenIdSet) == 0 {
		return
	}
	tokenIds := make([]int, 0, len(tokenIdSet))
	for tokenId := range tokenIdSet {
		tokenIds = append(tokenIds, tokenId)
	}
	var tokens []Token
	if err := DB.Select("id", "name").Where("id IN ?", tokenIds).Find(&tokens).Error; err != nil {
		common.SysError("failed to query usage trend token names: " + err.Error())
		return
	}
	tokenNames := make(map[int]string, len(tokens))
	for _, token := range tokens {
		tokenNames[token.Id] = token.Name
	}
	for _, item := range trendData {
		if item == nil {
			continue
		}
		item.TokenName = tokenNames[item.TokenId]
	}
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}
