package controller

import (
	"strconv"

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
	if supplier.APIKey == "" {
		supplier.APIKey = origin.APIKey
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
