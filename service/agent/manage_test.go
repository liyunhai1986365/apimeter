package agent

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestUpdateBrandingUpdatesOnlyTargetAgent(t *testing.T) {
	setupAgentTestDB(t)

	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
		Branding:      `{"site_name":"Old","logo":"/old.png"}`,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            2,
		OwnerUserId:   20,
		Name:          "Agent Two",
		Slug:          "agent-two",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
		Branding:      `{"site_name":"Keep","logo":"/keep.png"}`,
	}).Error)

	updated, err := UpdateBranding(1, `{"site_name":"New","logo":"https://agent.example/logo.png"}`)

	require.NoError(t, err)
	require.Equal(t, `{"site_name":"New","logo":"https://agent.example/logo.png"}`, updated.Branding)

	var untouched model.Agent
	require.NoError(t, model.DB.First(&untouched, "id = ?", 2).Error)
	require.Equal(t, `{"site_name":"Keep","logo":"/keep.png"}`, untouched.Branding)
}
