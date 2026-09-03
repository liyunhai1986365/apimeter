// 用于迁移检测的旧键，该文件下个版本会删除。
package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// MigrateConsoleSetting migrates legacy console options to console_setting.*.
func MigrateConsoleSetting(c *gin.Context) {
	opts, err := model.AllOption()
	if err != nil {
		common.SysError("failed to get all options: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "获取配置失败，请稍后重试"})
		return
	}
	values := make(map[string]string, len(opts))
	for _, option := range opts {
		values[option.Key] = option.Value
	}

	if value := values["ApiInfo"]; value != "" {
		var items []map[string]interface{}
		if err := common.UnmarshalJsonStr(value, &items); err == nil {
			if len(items) > 50 {
				items = items[:50]
			}
			if data, err := common.Marshal(items); err == nil {
				model.UpdateOption("console_setting.api_info", string(data))
			}
		}
		model.UpdateOption("ApiInfo", "")
	}
	if value := values["Announcements"]; value != "" {
		model.UpdateOption("console_setting.announcements", value)
		model.UpdateOption("Announcements", "")
	}
	if value := values["FAQ"]; value != "" {
		var items []map[string]interface{}
		if err := common.UnmarshalJsonStr(value, &items); err == nil {
			output := make([]map[string]interface{}, 0, len(items))
			for _, item := range items {
				question, _ := item["question"].(string)
				if question == "" {
					question, _ = item["title"].(string)
				}
				answer, _ := item["answer"].(string)
				if answer == "" {
					answer, _ = item["content"].(string)
				}
				if question != "" && answer != "" {
					output = append(output, map[string]interface{}{"question": question, "answer": answer})
				}
			}
			if len(output) > 50 {
				output = output[:50]
			}
			if data, err := common.Marshal(output); err == nil {
				model.UpdateOption("console_setting.faq", string(data))
			}
		}
		model.UpdateOption("FAQ", "")
	}

	urlValue, slug := values["UptimeKumaUrl"], values["UptimeKumaSlug"]
	if urlValue != "" && slug != "" {
		groups := []map[string]interface{}{{
			"id": 1, "categoryName": "old", "url": urlValue, "slug": slug, "description": "",
		}}
		if data, err := common.Marshal(groups); err == nil {
			model.UpdateOption("console_setting.uptime_kuma_groups", string(data))
		}
	}
	if urlValue != "" {
		model.UpdateOption("UptimeKumaUrl", "")
	}
	if slug != "" {
		model.UpdateOption("UptimeKumaSlug", "")
	}

	oldKeys := []string{"ApiInfo", "Announcements", "FAQ", "UptimeKumaUrl", "UptimeKumaSlug"}
	model.DB.Where("key IN ?", oldKeys).Delete(&model.Option{})
	model.InitOptionMap()
	common.SysLog("console setting migrated")
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "migrated"})
}
