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

func ListModelMonitorModels(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	result, err := service.ListModelMonitorModels(
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
		pageInfo.GetPage(),
		c.Query("keyword"),
		service.ModelMonitorQuery{
			Window:         c.Query("window"),
			StartTimestamp: startTimestamp,
			EndTimestamp:   endTimestamp,
			Group:          c.Query("group"),
		},
		time.Now().Unix(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func GetModelChannelTestResults(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	results, err := model.GetLatestModelChannelTestResults(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    results,
	})
}
