package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetChannelOperationRecords(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	channelID, _ := strconv.Atoi(c.Query("channel"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	records, total, err := model.GetChannelOperationRecords(model.ChannelOperationRecordQuery{
		ChannelID:      channelID,
		Action:         c.Query("action"),
		Source:         c.Query("source"),
		ModelName:      c.Query("model_name"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		Page:           pageInfo.GetPage(),
		PageSize:       pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}
