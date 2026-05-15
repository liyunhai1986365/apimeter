package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetRatioConfig(c *gin.Context) {
	if !ratio_setting.IsExposeRatioEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "倍率配置接口未启用",
		})
		return
	}

	data := ratio_setting.GetExposedData()
	if groupRatios, ok := data["group_ratio"].(map[string]float64); ok {
		data["group_ratio"] = applyAgentGroupRatios(c, groupRatios)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}
