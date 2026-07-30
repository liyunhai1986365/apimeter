package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGetConfigurableProtocolProfilesIncludesEmbeddedProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/protocol_profiles", nil)

	GetConfigurableProtocolProfiles(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.Bytes()
	require.True(t, gjson.GetBytes(body, "success").Bool(), string(body))
	require.Equal(t, "seedance2-service-inference", gjson.GetBytes(body, `data.#(id=="seedance2-service-inference").id`).String(), string(body))
	require.Equal(t, "Seedance2 Service Inference", gjson.GetBytes(body, `data.#(id=="seedance2-service-inference").name`).String(), string(body))
	require.Equal(t, "video", gjson.GetBytes(body, `data.#(id=="seedance2-service-inference").media_type`).String(), string(body))
	require.Equal(t, "openai.video.generations", gjson.GetBytes(body, `data.#(id=="seedance2-service-inference").accepted_modes.0`).String(), string(body))
	require.Equal(t, "seedance2-modelsell", gjson.GetBytes(body, `data.#(id=="seedance2-modelsell").id`).String(), string(body))
	require.Equal(t, "Seedance 2.0 Modelsell", gjson.GetBytes(body, `data.#(id=="seedance2-modelsell").name`).String(), string(body))
}
