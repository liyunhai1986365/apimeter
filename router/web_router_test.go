package router

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRenderIndexPageUsesSystemNameForInitialTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "ModelSell"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	html := []byte(`<title>New API</title><meta name="title" content="New API" />`)

	rendered := renderIndexPage(c, html)

	require.Contains(t, string(rendered), `<title>ModelSell</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="ModelSell" />`)
	require.NotContains(t, string(rendered), `<title>New API</title>`)
}

func TestRenderIndexPageUsesAgentSiteNameForInitialTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "ModelSell"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		Branding: `{"site_name":"Agent & Site"}`,
	})
	html := []byte(`<title>New API</title><meta name="title" content="New API" />`)

	rendered := renderIndexPage(c, html)

	require.Contains(t, string(rendered), `<title>Agent &amp; Site</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="Agent &amp; Site" />`)
	require.NotContains(t, string(rendered), `<title>New API</title>`)
}

func TestRenderIndexPageReplacesLoadingPlaceholderTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "ModelSell"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	html := []byte(`<title>loading</title><meta name="title" content="loading" />`)

	rendered := renderIndexPage(c, html)

	require.Contains(t, string(rendered), `<title>ModelSell</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="ModelSell" />`)
	require.NotContains(t, string(rendered), `<title>loading</title>`)
}
