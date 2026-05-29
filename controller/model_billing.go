package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func modelBillingSummaryQuery(c *gin.Context, userId int) model.ModelBillingSummaryQuery {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	return model.ModelBillingSummaryQuery{
		UserId:         userId,
		Period:         c.Query("period"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
		Group:          c.Query("group"),
		BillingSource:  c.Query("billing_source"),
	}
}

func GetSelfModelBillingSummary(c *gin.Context) {
	rows, err := model.GetModelBillingSummary(modelBillingSummaryQuery(c, c.GetInt("id")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func GetAdminModelBillingSummary(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	rows, err := model.GetModelBillingSummary(modelBillingSummaryQuery(c, userId))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

func BackfillModelBillingFromLogs(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	batchSize, _ := strconv.Atoi(c.Query("batch_size"))
	result, err := model.BackfillModelBillingFromLogs(model.ModelBillingBackfillOptions{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		BatchSize:      batchSize,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
