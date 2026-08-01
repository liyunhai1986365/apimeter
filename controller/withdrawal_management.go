package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/gin-gonic/gin"
)

type withdrawalManagementStatusRequest struct {
	Status      string `json:"status"`
	AdminRemark string `json:"admin_remark"`
}

func AdminListWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListWithdrawalManagement(
		c.Query("source"),
		c.Query("status"),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func AdminCompleteWithdrawal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid withdrawal id")
		return
	}
	var req withdrawalManagementStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	switch c.Param("source") {
	case model.WithdrawalSourceUser:
		err = model.CompleteAffiliateWithdrawal(id, req.Status, req.AdminRemark)
	case model.WithdrawalSourceAgent:
		err = agentservice.CompleteWithdrawal(id, req.Status, req.AdminRemark)
	default:
		err = model.ErrInvalidWithdrawalSource
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"id":     id,
		"source": c.Param("source"),
		"status": req.Status,
	})
}
