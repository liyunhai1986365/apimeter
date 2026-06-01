package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type newAPISupplierChannelProfileModelTestResponse struct {
	Success       bool                                     `json:"success"`
	Message       string                                   `json:"message"`
	UpstreamGroup string                                   `json:"upstream_group"`
	Model         string                                   `json:"model"`
	Time          float64                                  `json:"time"`
	ErrorCode     string                                   `json:"error_code,omitempty"`
	ProfileModel  *model.NewAPISupplierChannelProfileModel `json:"profile_model,omitempty"`
}

type newAPISupplierChannelProfileBatchTestResponse struct {
	ProfileID     int                                             `json:"profile_id"`
	SupplierID    int                                             `json:"supplier_id"`
	UpstreamGroup string                                          `json:"upstream_group"`
	ChannelStatus string                                          `json:"channel_status"`
	Total         int                                             `json:"total"`
	Passed        int                                             `json:"passed"`
	Failed        int                                             `json:"failed"`
	Results       []newAPISupplierChannelProfileModelTestResponse `json:"results"`
}

type newAPISupplierChannelProfileTestModelsRequest struct {
	Stream bool     `json:"stream"`
	Models []string `json:"models"`
}

type newAPISupplierChannelProfileTestAllRequest struct {
	Stream     bool  `json:"stream"`
	ProfileIDs []int `json:"profile_ids"`
}

type newAPISupplierChannelProfileResponse struct {
	model.NewAPISupplierChannelProfile
	ModelNames []string `json:"model_names"`
}

func GetAllNewAPISuppliers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	suppliers, total, err := model.GetAllNewAPISuppliers(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(suppliers)
	common.ApiSuccess(c, pageInfo)
}

func SearchNewAPISuppliers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	suppliers, total, err := model.SearchNewAPISuppliers(c.Query("keyword"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(suppliers)
	common.ApiSuccess(c, pageInfo)
}

func GetNewAPISupplier(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	common.ApiSuccess(c, supplier)
}

func CreateNewAPISupplier(c *gin.Context) {
	var supplier model.NewAPISupplier
	if err := c.ShouldBindJSON(&supplier); err != nil {
		common.ApiError(c, err)
		return
	}
	supplier.Normalize()
	if supplier.Name == "" {
		common.ApiErrorMsg(c, "供应商名称不能为空")
		return
	}
	if supplier.BaseURL == "" {
		common.ApiErrorMsg(c, "供应商 Base URL 不能为空")
		return
	}
	if supplier.AccessToken == "" && (supplier.Username == "" || supplier.Password == "") {
		common.ApiErrorMsg(c, "供应商账号密码或系统访问令牌不能为空")
		return
	}
	if supplier.AccessToken != "" && supplier.UpstreamUserID == 0 {
		common.ApiErrorMsg(c, "使用系统访问令牌时必须填写上游用户 ID")
		return
	}
	if err := supplier.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, &supplier)
}

func UpdateNewAPISupplier(c *gin.Context) {
	var supplier model.NewAPISupplier
	if err := c.ShouldBindJSON(&supplier); err != nil {
		common.ApiError(c, err)
		return
	}
	if supplier.Id == 0 {
		common.ApiErrorMsg(c, "缺少供应商 ID")
		return
	}
	origin, err := model.GetNewAPISupplierByID(supplier.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if supplier.Password == "" {
		supplier.Password = origin.Password
	}
	if supplier.AccessToken == "" {
		supplier.AccessToken = origin.AccessToken
	}
	if supplier.UpstreamUserID == 0 {
		supplier.UpstreamUserID = origin.UpstreamUserID
	}
	if supplier.APIKey == "" {
		supplier.APIKey = origin.APIKey
	}
	supplier.Normalize()
	if supplier.AccessToken == "" && (supplier.Username == "" || supplier.Password == "") {
		common.ApiErrorMsg(c, "供应商账号密码或系统访问令牌不能为空")
		return
	}
	if supplier.AccessToken != "" && supplier.UpstreamUserID == 0 {
		common.ApiErrorMsg(c, "使用系统访问令牌时必须填写上游用户 ID")
		return
	}
	if err := supplier.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	updated, _ := model.GetNewAPISupplierByID(supplier.Id)
	common.ApiSuccess(c, updated)
}

func DeleteNewAPISupplier(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Delete(&model.NewAPISupplier{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func CheckNewAPISupplier(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	result, err := service.CheckNewAPISupplier(c.Request.Context(), supplier)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func QueryNewAPISupplierBalance(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	result, err := service.QueryNewAPISupplierBalance(c.Request.Context(), supplier)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func ConfigureNewAPISupplierChannels(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	var req service.NewAPISupplierConfigureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	results, err := service.ConfigureNewAPISupplierChannels(c.Request.Context(), supplier, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, results)
}

func TestNewAPISupplierModel(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	var req service.NewAPISupplierTestModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	prepared, err := service.PrepareNewAPISupplierGroupTestChannel(c.Request.Context(), supplier, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tik := time.Now()
	result := testChannel(prepared.Channel, prepared.Model, req.EndpointType, req.Stream)
	consumedTime := float64(time.Since(tik).Milliseconds()) / 1000.0
	resp := gin.H{
		"upstream_group": prepared.UpstreamGroup,
		"model":          prepared.Model,
		"time":           consumedTime,
	}
	if result.localErr != nil {
		resp["success"] = false
		resp["message"] = result.localErr.Error()
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	if result.newAPIError != nil {
		resp["success"] = false
		resp["message"] = result.newAPIError.Error()
		resp["error_code"] = result.newAPIError.GetErrorCode()
		c.JSON(http.StatusOK, resp)
		return
	}
	resp["success"] = true
	resp["message"] = ""
	c.JSON(http.StatusOK, resp)
}

func ListNewAPISupplierChannels(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	var bindings []model.NewAPISupplierChannel
	if err := model.DB.Where("supplier_id = ?", supplier.Id).Order("id DESC").Find(&bindings).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, bindings)
}

func ListNewAPISupplierChannelProfiles(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	db := newAPISupplierChannelProfileListQuery(c, false)
	var total int64
	if err := newAPISupplierChannelProfileListQuery(c, true).Count(&total).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var profiles []model.NewAPISupplierChannelProfile
	if err := db.Order("new_api_supplier_channel_profiles.id DESC").Offset(pageInfo.GetStartIdx()).Limit(pageInfo.GetPageSize()).Find(&profiles).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildNewAPISupplierChannelProfileResponses(profiles))
	common.ApiSuccess(c, pageInfo)
}

func buildNewAPISupplierChannelProfileResponses(profiles []model.NewAPISupplierChannelProfile) []newAPISupplierChannelProfileResponse {
	items := make([]newAPISupplierChannelProfileResponse, 0, len(profiles))
	if len(profiles) == 0 {
		return items
	}
	profileIDs := make([]int, 0, len(profiles))
	for _, profile := range profiles {
		profileIDs = append(profileIDs, profile.Id)
	}
	var profileModels []model.NewAPISupplierChannelProfileModel
	_ = model.DB.Where("profile_id IN ?", profileIDs).Order("model_name ASC").Find(&profileModels).Error
	modelsByProfileID := make(map[int][]string, len(profiles))
	for _, profileModel := range profileModels {
		modelsByProfileID[profileModel.ProfileID] = append(modelsByProfileID[profileModel.ProfileID], profileModel.ModelName)
	}
	for _, profile := range profiles {
		items = append(items, newAPISupplierChannelProfileResponse{
			NewAPISupplierChannelProfile: profile,
			ModelNames:                   modelsByProfileID[profile.Id],
		})
	}
	return items
}

func newAPISupplierChannelProfileListQuery(c *gin.Context, count bool) *gorm.DB {
	const profileTable = "new_api_supplier_channel_profiles"
	const profileModelTable = "new_api_supplier_channel_profile_models"

	db := model.DB.Model(&model.NewAPISupplierChannelProfile{})
	if count {
		db = db.Distinct(profileTable + ".id")
	} else {
		db = db.Select(profileTable + ".*").Distinct(profileTable + ".*")
	}
	if supplierID, err := strconv.Atoi(strings.TrimSpace(c.Query("supplier_id"))); err == nil && supplierID > 0 {
		db = db.Where(profileTable+".supplier_id = ?", supplierID)
	}
	if supplier := strings.TrimSpace(c.Query("supplier")); supplier != "" {
		if supplierID, err := strconv.Atoi(supplier); err == nil && supplierID > 0 {
			db = db.Where(profileTable+".supplier_id = ?", supplierID)
		} else {
			like := "%" + supplier + "%"
			db = db.Where(profileTable+".supplier_name_snapshot LIKE ? OR "+profileTable+".base_url_snapshot LIKE ?", like, like)
		}
	}
	if upstreamGroup := strings.TrimSpace(c.Query("upstream_group")); upstreamGroup != "" {
		db = db.Where(profileTable+".upstream_group = ?", upstreamGroup)
	}
	if localGroup := strings.TrimSpace(c.Query("local_group")); localGroup != "" {
		db = db.Where(profileTable+".local_group = ?", localGroup)
	}
	if syncStatus := strings.TrimSpace(c.Query("sync_status")); syncStatus != "" {
		db = db.Where(profileTable+".sync_status = ?", syncStatus)
	}
	if channelStatus := strings.TrimSpace(c.Query("channel_status")); channelStatus != "" {
		db = db.Where(profileTable+".channel_status = ?", channelStatus)
	}
	switch strings.TrimSpace(c.Query("managed_channel")) {
	case "linked":
		db = db.Where(profileTable + ".channel_id IS NOT NULL AND " + profileTable + ".channel_id > 0")
	case "unlinked":
		db = db.Where(profileTable + ".channel_id IS NULL OR " + profileTable + ".channel_id = 0")
	}
	if modelName := strings.TrimSpace(c.Query("model")); modelName != "" {
		db = db.Joins("JOIN "+profileModelTable+" ON "+profileModelTable+".profile_id = "+profileTable+".id").
			Where(profileModelTable+".model_name LIKE ?", "%"+modelName+"%")
	}
	return db
}

func ListNewAPISupplierChannelProfileModels(c *gin.Context) {
	profileID, err := strconv.Atoi(c.Param("profile_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var items []model.NewAPISupplierChannelProfileModel
	if err := model.DB.Where("profile_id = ?", profileID).Order("model_name ASC").Find(&items).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func UpdateNewAPISupplierChannelProfile(c *gin.Context) {
	profileID, err := strconv.Atoi(c.Param("profile_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req service.NewAPISupplierChannelProfileUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	profile, err := service.UpdateNewAPISupplierChannelProfile(profileID, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, profile)
}

func SyncNewAPISupplierChannelProfile(c *gin.Context) {
	profileID, err := strconv.Atoi(c.Param("profile_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	profile, err := service.SyncNewAPISupplierChannelProfile(c.Request.Context(), profileID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, profile)
}

func TestNewAPISupplierChannelProfileModel(c *gin.Context) {
	profileID, err := strconv.Atoi(c.Param("profile_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var profile model.NewAPISupplierChannelProfile
	if err := model.DB.First(&profile, profileID).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var req service.NewAPISupplierTestModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := testNewAPISupplierChannelProfileModel(c.Request.Context(), profile, req.Model, req.Stream)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func TestNewAPISupplierChannelProfileModels(c *gin.Context) {
	profileID, err := strconv.Atoi(c.Param("profile_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var profile model.NewAPISupplierChannelProfile
	if err := model.DB.First(&profile, profileID).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	var req newAPISupplierChannelProfileTestModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := testNewAPISupplierChannelProfileModels(c.Request.Context(), profile, req.Stream, req.Models)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func TestAllNewAPISupplierChannelProfiles(c *gin.Context) {
	var req newAPISupplierChannelProfileTestAllRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	var profiles []model.NewAPISupplierChannelProfile
	db := model.DB.Order("id ASC")
	if len(req.ProfileIDs) > 0 {
		db = db.Where("id IN ?", req.ProfileIDs)
	}
	if err := db.Find(&profiles).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	results := make([]gin.H, 0, len(profiles))
	total := 0
	passed := 0
	failed := 0
	for _, profile := range profiles {
		result, err := testNewAPISupplierChannelProfileModels(c.Request.Context(), profile, req.Stream, nil)
		if err != nil {
			results = append(results, gin.H{
				"profile_id":     profile.Id,
				"supplier_id":    profile.SupplierID,
				"upstream_group": profile.UpstreamGroup,
				"success":        false,
				"message":        err.Error(),
			})
			failed++
			continue
		}
		total += result.Total
		passed += result.Passed
		failed += result.Failed
		results = append(results, gin.H{
			"profile_id":     profile.Id,
			"supplier_id":    profile.SupplierID,
			"upstream_group": profile.UpstreamGroup,
			"channel_status": result.ChannelStatus,
			"total":          result.Total,
			"passed":         result.Passed,
			"failed":         result.Failed,
			"success":        result.Failed == 0,
			"model_results":  result.Results,
		})
	}
	common.ApiSuccess(c, gin.H{
		"total":    total,
		"passed":   passed,
		"failed":   failed,
		"profiles": results,
	})
}

func testNewAPISupplierChannelProfileModels(ctx context.Context, profile model.NewAPISupplierChannelProfile, stream bool, modelNames []string) (*newAPISupplierChannelProfileBatchTestResponse, error) {
	var models []model.NewAPISupplierChannelProfileModel
	db := model.DB.Where("profile_id = ?", profile.Id)
	if len(modelNames) > 0 {
		selectedModelNames := uniqueNonEmptyStrings(modelNames)
		if len(selectedModelNames) == 0 {
			db = db.Where("1 = 0")
		} else {
			db = db.Where("model_name IN ?", selectedModelNames)
		}
	}
	if err := db.Order("model_name ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	result := &newAPISupplierChannelProfileBatchTestResponse{
		ProfileID:     profile.Id,
		SupplierID:    profile.SupplierID,
		UpstreamGroup: profile.UpstreamGroup,
		Results:       make([]newAPISupplierChannelProfileModelTestResponse, 0, len(models)),
	}
	for _, item := range models {
		modelResult, err := testNewAPISupplierChannelProfileModel(ctx, profile, item.ModelName, stream)
		if err != nil {
			modelResult = &newAPISupplierChannelProfileModelTestResponse{
				Success:       false,
				Message:       err.Error(),
				UpstreamGroup: profile.UpstreamGroup,
				Model:         item.ModelName,
			}
		}
		result.Total++
		if modelResult.Success {
			result.Passed++
		} else {
			result.Failed++
		}
		result.Results = append(result.Results, *modelResult)
	}
	if err := service.RefreshNewAPISupplierChannelProfileStatus(profile.Id); err != nil {
		return nil, err
	}
	var refreshed model.NewAPISupplierChannelProfile
	if err := model.DB.First(&refreshed, profile.Id).Error; err != nil {
		return nil, err
	}
	result.ChannelStatus = refreshed.ChannelStatus
	return result, nil
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func testNewAPISupplierChannelProfileModel(ctx context.Context, profile model.NewAPISupplierChannelProfile, modelName string, stream bool) (*newAPISupplierChannelProfileModelTestResponse, error) {
	if modelName == "" {
		return nil, fmt.Errorf("模型名称不能为空")
	}
	supplier, err := model.GetNewAPISupplierByID(profile.SupplierID)
	if err != nil {
		return nil, err
	}
	req := service.NewAPISupplierTestModelRequest{
		UpstreamGroup: profile.UpstreamGroup,
		Model:         modelName,
		ChannelType:   profile.ChannelType,
		EndpointType:  profile.EndpointType,
		Stream:        stream,
	}
	prepared, err := service.PrepareNewAPISupplierGroupTestChannel(ctx, supplier, req)
	if err != nil {
		return nil, err
	}
	tik := time.Now()
	result := testChannel(prepared.Channel, prepared.Model, req.EndpointType, req.Stream)
	consumedTimeMS := int(time.Since(tik).Milliseconds())
	success := result.localErr == nil && result.newAPIError == nil
	message := ""
	errorCode := ""
	if result.localErr != nil {
		message = result.localErr.Error()
	}
	if result.newAPIError != nil {
		message = result.newAPIError.Error()
		errorCode = string(result.newAPIError.GetErrorCode())
	}
	modelResult, err := service.UpdateNewAPISupplierProfileModelTestResult(profile.Id, service.NewAPISupplierProfileModelTestResultRequest{
		ModelName:      prepared.Model,
		Success:        success,
		ResponseTimeMS: consumedTimeMS,
		Message:        message,
	})
	if err != nil {
		return nil, err
	}
	return &newAPISupplierChannelProfileModelTestResponse{
		Success:       success,
		Message:       message,
		UpstreamGroup: prepared.UpstreamGroup,
		Model:         prepared.Model,
		Time:          float64(consumedTimeMS) / 1000.0,
		ErrorCode:     errorCode,
		ProfileModel:  modelResult,
	}, nil
}

func SyncNewAPISupplierProfiles(c *gin.Context) {
	supplier, ok := getNewAPISupplierFromParam(c)
	if !ok {
		return
	}
	if supplier.ModelSource != "pricing" {
		common.ApiErrorMsg(c, "请先检查供应商，加载模型广场分组模型")
		return
	}
	var snapshots []service.NewAPISupplierGroupSnapshot
	if err := common.UnmarshalJsonStr(supplier.GroupModelsJSON, &snapshots); err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := service.SyncNewAPISupplierChannelProfiles(supplier, snapshots)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func getNewAPISupplierFromParam(c *gin.Context) (*model.NewAPISupplier, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	supplier, err := model.GetNewAPISupplierByID(id)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return supplier, true
}
