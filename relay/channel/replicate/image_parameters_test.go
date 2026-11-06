package replicate

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestImageExplicitParametersKeepReplicateInput(t *testing.T) {
	var request dto.ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"owner/image-model","prompt":"draw",
		"extra_fields":{"frame_count":99},"vendor_option":true,
		"complexity":"","frame_count":0,"frame_duration_ms":100,"safety_identifier":"user-hash",
		"svg":{"model":"author"},"markup":{"file_id":"file-markup"}
	}`), &request))
	adaptor := &Adaptor{}
	payload, err := adaptor.ConvertImageRequest(nil, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{},
	}, request)
	require.NoError(t, err)
	wire, err := common.Marshal(payload)
	require.NoError(t, err)
	require.JSONEq(t, `{"input":{
		"prompt":"draw","vendor_option":true,"complexity":"","frame_count":0,
		"frame_duration_ms":100,"safety_identifier":"user-hash",
		"svg":{"model":"author"},"markup":{"file_id":"file-markup"}
	}}`, string(wire))
}

func TestImagePromotedScalarNullOverrides(t *testing.T) {
	defaults := `"extra_fields":{"complexity":"high","frame_count":99,"frame_duration_ms":250,"safety_identifier":"fallback"}`
	nulls := `"complexity":null,"frame_count":null,"frame_duration_ms":null,"safety_identifier":null`
	for _, tc := range []struct{ name, fields, want string }{
		{"null overrides defaults", defaults + "," + nulls, nulls},
		{"null without defaults", nulls, nulls},
		{"absent keeps defaults", defaults, `"complexity":"high","frame_count":99,"frame_duration_ms":250,"safety_identifier":"fallback"`},
		{"zero overrides defaults", defaults + `,"complexity":"","frame_count":0,"frame_duration_ms":0,"safety_identifier":""`, `"complexity":"","frame_count":0,"frame_duration_ms":0,"safety_identifier":""`},
		{"nested input wins", nulls + `,"input":{"complexity":"low","frame_count":2,"frame_duration_ms":100,"safety_identifier":"nested"}`, `"complexity":"low","frame_count":2,"frame_duration_ms":100,"safety_identifier":"nested"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"owner/image-model","prompt":"draw",`+tc.fields+`}`), &request))
			// The relay makes a deep copy for each attempt; presence must survive it.
			attempt, err := common.DeepCopy(&request)
			require.NoError(t, err)
			payload, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{},
			}, *attempt)
			require.NoError(t, err)
			wire, err := common.Marshal(payload)
			require.NoError(t, err)
			require.JSONEq(t, `{"input":{"prompt":"draw",`+tc.want+`}}`, string(wire))
		})
	}
}

func TestImageNullMetadataIsolation(t *testing.T) {
	var request dto.ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"owner/image-model","prompt":"draw","complexity":null,"frame_count":null,"frame_duration_ms":null,"safety_identifier":null}`), &request))
	attempt, err := common.DeepCopy(&request)
	require.NoError(t, err)
	require.Len(t, attempt.ExplicitNullFields, 4)
	require.Empty(t, attempt.Extra, "nulls must not be reintroduced by fallback Extra merging")
	wire, err := common.Marshal(attempt)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"owner/image-model","prompt":"draw"}`, string(wire), "shared DTO serialization must omit scalar nulls and internal metadata")

	// Explicit values added on an attempt take priority over old null markers.
	attempt.Complexity = common.GetPointer("")
	attempt.FrameCount = common.GetPointer(0)
	attempt.FrameDurationMS = common.GetPointer(0)
	attempt.SafetyIdentifier = common.GetPointer("")
	payload, err := (&Adaptor{}).ConvertImageRequest(nil, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{},
	}, *attempt)
	require.NoError(t, err)
	wire, err = common.Marshal(payload)
	require.NoError(t, err)
	require.JSONEq(t, `{"input":{"prompt":"draw","complexity":"","frame_count":0,"frame_duration_ms":0,"safety_identifier":""}}`, string(wire))
	delete(attempt.ExplicitNullFields, "frame_count")
	require.True(t, request.ExplicitNullFields["frame_count"], "attempt metadata must not mutate the retry source")
	require.Nil(t, request.FrameCount)

	require.NoError(t, common.Unmarshal([]byte(`{"model":"owner/image-model","prompt":"next"}`), &request))
	require.Empty(t, request.ExplicitNullFields, "reusing a DTO must clear prior null markers")
}
