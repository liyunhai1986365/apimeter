package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func TestGetGroupsReturnsOnlyUserGroupDisplayEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`); err != nil {
			t.Fatalf("failed to reset group ratios: %v", err)
		}
		if err := setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"token-only":1,"vip":1}`); err != nil {
		t.Fatalf("failed to update group ratios: %v", err)
	}
	if err := setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "default", "order": 20, "user_group": true},
			{"group": "token-only", "order": 10},
			{"group": "vip", "order": 30, "user_group": true}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	router := gin.New()
	router.GET("/api/group/", GetGroups)

	req := httptest.NewRequest(http.MethodGet, "/api/group/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatalf("expected success response")
	}
	if len(body.Data) != 2 || body.Data[0] != "default" || body.Data[1] != "vip" {
		t.Fatalf("expected only user groups [default vip], got %+v", body.Data)
	}
}

func TestGetGroupsReturnsEmptyWhenDisplayEntriesHaveNoUserGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1}`); err != nil {
			t.Fatalf("failed to reset group ratios: %v", err)
		}
		if err := setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})

	if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"token-only":1}`); err != nil {
		t.Fatalf("failed to update group ratios: %v", err)
	}
	if err := setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "default", "order": 10},
			{"group": "token-only", "order": 20}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	router := gin.New()
	router.GET("/api/group/", GetGroups)

	req := httptest.NewRequest(http.MethodGet, "/api/group/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body struct {
		Success bool     `json:"success"`
		Data    []string `json:"data"`
	}
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatalf("expected success response")
	}
	if len(body.Data) != 0 {
		t.Fatalf("expected no user groups when none are flagged, got %+v", body.Data)
	}
}

func TestConfiguredUserGroupValidationUsesDisplayUserGroups(t *testing.T) {
	t.Cleanup(func() {
		if err := setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})

	if err := setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "supplier", "order": 10},
			{"group": "member", "order": 20, "user_group": true}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	if !isConfiguredUserGroup("member") {
		t.Fatal("expected configured user group to be accepted")
	}
	if isConfiguredUserGroup("supplier") {
		t.Fatal("expected token-only group to be rejected as a user group")
	}
}
