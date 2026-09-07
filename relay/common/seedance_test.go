package common

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSeedanceOmniReferenceConstraints(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantCode string
	}{
		{"auto", `{"omni_reference_task_type":"auto","duration":5}`, ""},
		{"reference", `{"omni_reference_task_type":"reference","duration":5}`, ""},
		{"default auto duration", `{"duration":-1}`, ""},
		{"auto duration", `{"omni_reference_task_type":"auto","seconds":"-1"}`, ""},
		{"edit", `{"omni_reference_task_type":"edit","duration":-1,"metadata":{"ratio":"adaptive","content":[{"type":"video_url","role":"reference_video","video_url":{"url":"asset://source"}}]}}`, ""},
		{"extend shorthand", `{"omni_reference_task_type":"extend","duration":5,"metadata":{"ratio":"adaptive","video_url":["https://example.com/source.mp4"]}}`, ""},
		{"metadata options", `{"seconds":"-1","metadata":{"omni_reference_task_type":"edit","aspect_ratio":"adaptive","video_url":"asset://source"}}`, ""},
		{"top level precedence", `{"omni_reference_task_type":"reference","metadata":{"omni_reference_task_type":"edit"}}`, ""},
		{"invalid enum", `{"omni_reference_task_type":"unknown"}`, "invalid_request"},
		{"empty enum", `{"omni_reference_task_type":"","metadata":{"omni_reference_task_type":"auto"}}`, "invalid_request"},
		{"edit duration", `{"omni_reference_task_type":"edit","duration":5,"metadata":{"ratio":"adaptive","video_url":"asset://source"}}`, "invalid_request"},
		{"edit missing duration", `{"omni_reference_task_type":"edit","metadata":{"ratio":"adaptive","video_url":"asset://source"}}`, "invalid_request"},
		{"edit ratio", `{"omni_reference_task_type":"edit","duration":-1,"metadata":{"ratio":"16:9","video_url":"asset://source"}}`, "invalid_request"},
		{"extend missing video", `{"omni_reference_task_type":"extend","duration":5,"metadata":{"ratio":"adaptive"}}`, "invalid_request"},
		{"extend wrong role", `{"omni_reference_task_type":"extend","duration":5,"metadata":{"ratio":"adaptive","content":[{"type":"video_url","role":"first_frame","video_url":{"url":"asset://source"}}],"video_url":"asset://fallback"}}`, "invalid_request"},
		{"invalid content cannot use fallback", `{"omni_reference_task_type":"extend","duration":5,"metadata":{"ratio":"adaptive","content":{},"video_url":"asset://fallback"}}`, "invalid_request"},
		{"conflicting duration", `{"omni_reference_task_type":"edit","duration":-1,"seconds":"5","metadata":{"ratio":"adaptive","video_url":"asset://source"}}`, "invalid_request"},
		{"invalid seconds", `{"omni_reference_task_type":"reference","seconds":"invalid"}`, "invalid_request"},
		{"other negative duration", `{"duration":-2}`, "invalid_seconds"},
		{"duration over limit", `{"duration":31}`, "invalid_seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req TaskSubmitReq
			require.NoError(t, common.Unmarshal([]byte(tc.body), &req))
			req.Model = "doubao-seedance-2-5-260628"
			taskErr := ValidateTaskDurationBounds(req)
			if tc.wantCode != "" {
				require.NotNil(t, taskErr)
				require.Equal(t, tc.wantCode, taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})
	}
}

func TestSeedanceOmniReferenceOptionalJSON(t *testing.T) {
	for _, body := range []string{`{}`, `{"omni_reference_task_type":null}`, `{"metadata":{"omni_reference_task_type":null}}`} {
		var req TaskSubmitReq
		require.NoError(t, common.Unmarshal([]byte(body), &req))
		encoded, err := common.Marshal(req)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(encoded, "omni_reference_task_type").Exists())
	}
	for _, body := range []string{`{"omni_reference_task_type":5}`, `{"metadata":{"omni_reference_task_type":false}}`} {
		var req TaskSubmitReq
		require.Error(t, common.Unmarshal([]byte(body), &req))
	}
	var req TaskSubmitReq
	require.NoError(t, common.Unmarshal([]byte(`{"metadata":"{\"omni_reference_task_type\":\"edit\"}"}`), &req))
	require.NotNil(t, req.OmniReferenceTaskType)
	require.Equal(t, "edit", *req.OmniReferenceTaskType)
}

func TestSeedanceAutomaticDurationUsesChannelForAliases(t *testing.T) {
	req := TaskSubmitReq{Model: "video-alias", Duration: -1}
	for _, meta := range []*ChannelMeta{
		{ChannelType: constant.ChannelTypeVolcEngine, UpstreamModelName: "ep-123"},
		{ChannelType: constant.ChannelTypeConfigurable, UpstreamModelName: "ep-123", ChannelSetting: dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"}}},
	} {
		require.Nil(t, ValidateTaskDurationBoundsForRelay(req, &RelayInfo{ChannelMeta: meta}))
	}
	require.NotNil(t, ValidateTaskDurationBoundsForRelay(req, &RelayInfo{ChannelMeta: &ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}))
	require.NotNil(t, ValidateTaskDurationBounds(TaskSubmitReq{Model: "sora-2", Duration: 5, Seconds: "-1"}))
}
