package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func TestGetPricingQuotationRejectsMissingOrInvalidUserGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		if err := setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})
	if err := setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [{"group": "member", "order": 10, "user_group": true}]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	for _, testCase := range []struct {
		name string
		url  string
	}{
		{name: "missing", url: "/api/pricing/quotation"},
		{name: "invalid", url: "/api/pricing/quotation?user_group=supplier"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, testCase.url, nil)
			response := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(response)
			context.Request = request

			GetPricingQuotation(context)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d", response.Code)
			}
			var body struct {
				Success bool `json:"success"`
			}
			if err := common.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if body.Success {
				t.Fatal("expected validation failure")
			}
		})
	}
}
