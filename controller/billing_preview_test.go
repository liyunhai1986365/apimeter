package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPreviewBillingExpression(t *testing.T) {
	origUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = origUnit })
	for _, tc := range []struct {
		name, expression, format string
		counts                   map[string]int
		unit                     float64
		quota                    int
		tier                     string
	}{
		{"openai cache", `tier("base",p*2+cr*0.5)`, "openai", map[string]int{"p": 100, "cr": 20}, 500000, 85, "base"},
		{"claude cache", `tier("base",p*2+cr*0.5)`, "anthropic", map[string]int{"p": 100, "cr": 20}, 500000, 105, "base"},
		{"openai context", `len>110 ? tier("long",p*4+cr) : tier("short",p*2+cr)`, "openai", map[string]int{"p": 100, "cr": 20}, 500000, 90, "short"},
		{"claude context", `len>110 ? tier("long",p*4+cr) : tier("short",p*2+cr)`, "anthropic", map[string]int{"p": 100, "cr": 20}, 500000, 210, "long"},
		{"claude split writes", `tier("write",len+cc+cc1h*3)`, "anthropic", map[string]int{"p": 10, "cc": 3, "cc1h": 2}, 500000, 12, "write"},
		{"claude legacy write price", `tier("write",cc*20)`, "anthropic", map[string]int{"cc": 3, "cc1h": 7}, 500000, 142, "write"},
		{"claude explicit free long writes", `tier("write",cc*20+cc1h*0)`, "anthropic", map[string]int{"cc": 3, "cc1h": 7}, 500000, 30, "write"},
		{"claude input fallback", `tier("input",len == 115 ? p*20 : 0)`, "anthropic", map[string]int{"p": 100, "cr": 5, "cc": 3, "cc1h": 7}, 500000, 1192, "input"},
		{"openai split", `tier("write",p*2+cc*3+cc12h*8)`, "openai", map[string]int{"p": 10000, "cc": 3000, "cc12h": 4000}, 500000, 23500, "write"},
		{"openai long free", `tier("write",p*2+cc*3+cc12h*0)`, "openai", map[string]int{"p": 10000, "cc": 3000, "cc12h": 4000}, 500000, 7500, "write"},
		{"openai old expression", `tier("write",p*2+cc*3)`, "openai", map[string]int{"p": 10000, "cc": 3000, "cc12h": 4000}, 500000, 19500, "write"},
		{"openai input fallback", `len == 10000 ? tier("base",p*2) : 0`, "openai", map[string]int{"p": 10000, "cc": 3000, "cc12h": 4000}, 500000, 14000, "base"},
		{"openai output", `tier("base",c*2+ao*10)`, "openai", map[string]int{"c": 100, "ao": 20}, 500000, 180, "base"},
		{"images and audio", `tier("media",p+c+img*2+img_o*4+ai*8)`, "openai", map[string]int{"p": 25, "c": 8, "img": 2, "img_o": 3, "ai": 4}, 500000, 36, "media"},
		{"caret", `tier("base",p ^ 2)`, "openai", map[string]int{"p": 10}, 500000, 50, "base"},
		{"keyword", `p>0 and c>0 ? tier("both",p+c) : tier("no",0)`, "openai", map[string]int{"p": 20, "c": 10}, 500000, 15, "both"},
		{"custom quota", `v1:tier("base",p*2+c*10)`, "openai", map[string]int{"p": 100, "c": 20}, 251250, 101, "base"},
		{"request and pixels", `param("size")==nil and header("opt")=="" and response("size")==nil and pixels("2K")==pixels("2048x2048") ? tier("base",1) : 0`, "openai", nil, 500000, 1, "base"},
		{"saturation", `tier("large",1e20)`, "openai", nil, 500000, 2147483647, "large"},
		{"invalid format", "1", "unknown", nil, 500000, 0, ""},
		{"invalid count", "p", "openai", map[string]int{"p": -1}, 500000, 0, ""},
		{"unknown version", "v2:p", "openai", nil, 500000, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaPerUnit = tc.unit
			sample := gin.H{"expression": tc.expression, "format": tc.format}
			for k, v := range tc.counts {
				sample[k] = v
			}
			body, err := common.Marshal(sample)
			require.NoError(t, err)
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/billing-preview", bytes.NewReader(body))
			PreviewBillingExpression(ctx)
			var actual struct {
				Success bool
				Message string
				Data    struct {
					Quota int
					Tier  string
				}
			}
			require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &actual))
			require.Equal(t, tc.tier != "", actual.Success, actual.Message)
			if actual.Success {
				require.Equal(t, tc.quota, actual.Data.Quota)
				require.Equal(t, tc.tier, actual.Data.Tier)
			} else {
				require.NotEmpty(t, actual.Message)
			}
		})
	}
}
