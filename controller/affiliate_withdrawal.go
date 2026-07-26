package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type affiliateWithdrawalRequest struct {
	AmountQuota int    `json:"amount_quota"`
	AccountInfo string `json:"account_info"`
}

type affiliateWithdrawalStatusRequest struct {
	Status      string `json:"status"`
	AdminRemark string `json:"admin_remark"`
}

func ListAffiliateWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	withdrawals, total, err := model.ListAffiliateWithdrawals(c.GetInt("id"), pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawals)
	common.ApiSuccess(c, pageInfo)
}

func SubmitAffiliateWithdrawal(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req affiliateWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	withdrawal, err := model.SubmitAffiliateWithdrawal(c.GetInt("id"), req.AmountQuota, req.AccountInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, withdrawal)
}

func CancelAffiliateWithdrawal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid withdrawal id")
		return
	}
	if err := model.CancelAffiliateWithdrawal(c.GetInt("id"), id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": model.AffiliateWithdrawalStatusCancelled})
}

func AdminListAffiliateWithdrawals(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userId, _ := strconv.Atoi(c.Query("user_id"))
	withdrawals, total, err := model.ListAffiliateWithdrawals(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(withdrawals)
	common.ApiSuccess(c, pageInfo)
}

func AdminCompleteAffiliateWithdrawal(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "invalid withdrawal id")
		return
	}
	var req affiliateWithdrawalStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CompleteAffiliateWithdrawal(id, req.Status, req.AdminRemark); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": req.Status})
}
