package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

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
