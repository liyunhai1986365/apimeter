package configurable

import (
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"testing"
)

func TestModelsellResponseValidationUsesAuthoritativeFields(t *testing.T) {
	a := &TaskAdaptor{}
	a.Init(seedanceMaxRelayInfo("seedance2-modelsell"))
	for _, tc := range []struct {
		name, body           string
		validID, validStatus bool
	}{
		{"wrapped_provider_id", `{"data":{"task_id":"owner","status":"SUCCESS","data":{"task":{"id":"provider-id","status":"completed"}}}}`, true, true},
		{"flat_fallback", `{"id":"owner","status":"succeeded"}`, true, true},
		{"wrong_primary_id", `{"id":"owner","status":"succeeded","data":{"task_id":"other","status":"SUCCESS"}}`, false, true},
		{"null_primary_id", `{"id":"owner","status":"succeeded","data":{"task_id":null,"status":"SUCCESS"}}`, false, true},
		{"numeric_primary_id", `{"id":"owner","status":"succeeded","data":{"task_id":123,"status":"SUCCESS"}}`, false, true},
		{"blank_primary_id", `{"id":"owner","status":"succeeded","data":{"task_id":"","status":"SUCCESS"}}`, false, true},
		{"missing_id", `{"status":"succeeded"}`, false, true},
		{"missing_wrapped_id", `{"id":"owner","data":{"status":"SUCCESS"}}`, false, true},
		{"wrong_official_id", `{"id":"owner","upstream_task_id":"other","status":"succeeded"}`, false, true},
		{"null_primary_status", `{"id":"owner","status":"succeeded","data":{"task_id":"owner","status":null}}`, true, false},
		{"unknown_primary_status", `{"id":"owner","status":"succeeded","data":{"task_id":"owner","status":"unknown"}}`, true, false},
		{"numeric_primary_status", `{"id":"owner","status":"succeeded","data":{"task_id":"owner","status":123}}`, true, false},
		{"blank_primary_status", `{"id":"owner","status":"succeeded","data":{"task_id":"owner","status":""}}`, true, false},
		{"missing_status", `{"id":"owner"}`, true, false},
		{"missing_wrapped_status", `{"status":"succeeded","data":{"task_id":"owner"}}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			parsed, err := a.ParseTaskResult(body)
			require.NoError(t, err)
			require.Equal(t, tc.validID, relaycommon.ValidateSeedanceTaskIdentityForAdaptor(a, body, parsed, "owner", "official") == nil)
			require.Equal(t, tc.validStatus, relaycommon.ValidateSeedanceTaskStatusForAdaptor(a, body) == nil)
		})
	}
	// Fetch variants must override both parser and validator identity paths.
	response := a.profile.videoFetch().Response
	response.TaskIDPath = "task.id"
	a.selectedFetchResp = &response
	body := []byte(`{"task":{"id":"other"},"data":{"task_id":"owner","status":"SUCCESS"}}`)
	parsed, err := a.ParseTaskResult(body)
	require.NoError(t, err)
	require.Equal(t, "other", parsed.TaskID)
	require.Error(t, relaycommon.ValidateSeedanceTaskIdentityForAdaptor(a, body, parsed, "owner", ""))
}

func TestSeedanceNativeResponsePreservesExtensionsAndAbsentUsage(t *testing.T) {
	task := &model.Task{TaskID: "task_internal", PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-original"}, Status: model.TaskStatusSuccess}
	for _, raw := range []string{
		`{"id":"cgt-original","status":"succeeded","content":{"video_url":"https://example.com/v","last_frame_url":"https://example.com/f"},"usage":{"total_tokens":9007199254740993,"details":{"zero":0,"flag":false}},"future":{"preserve":true}}`,
		`{"id":"cgt-original","status":"running","future":{"preserve":false}}`,
		`{"id":"cgt-original","status":"failed","error":{"code":"TaskFailed","message":"example","future":1}}`,
	} {
		body, err := BuildVolcengineVideoTaskResponse(task, []byte(raw))
		require.NoError(t, err)
		require.JSONEq(t, raw, string(body))
		if gjson.Get(raw, "usage").Exists() {
			require.Equal(t, "9007199254740993", gjson.GetBytes(body, "usage.total_tokens").Raw)
		}
	}
}

func TestSeedanceWrappedUsagePreservesNumericPrecision(t *testing.T) {
	task := &model.Task{TaskID: "task_internal", PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-original"}, Status: model.TaskStatusSuccess}
	for _, path := range []string{"usage", "task.usage", "task.metadata.usage", "data.usage", "data.data.task.usage", "data.data.task.metadata.usage"} {
		t.Run(path, func(t *testing.T) {
			// No flat id: force wrapper conversion rather than the native raw fast path.
			upstream, err := sjson.SetRaw(`{"task":{"id":"cgt-original","status":"succeeded"}}`, path, `{"total_tokens":9007199254740993,"completion_tokens":"9223372036854775807","details":{"counter":18446744073709551615,"decimal":0.1234567890123456789,"flag":false,"zero":0},"unknown":[9007199254740993]}`)
			require.NoError(t, err)
			body, err := BuildVolcengineVideoTaskResponse(task, []byte(upstream))
			require.NoError(t, err)
			for field, expected := range map[string]string{"total_tokens": "9007199254740993", "completion_tokens": "9223372036854775807", "details.counter": "18446744073709551615", "details.decimal": "0.1234567890123456789", "details.flag": "false", "details.zero": "0", "unknown.0": "9007199254740993"} {
				require.Equal(t, expected, gjson.GetBytes(body, "usage."+field).Raw, field)
			}
		})
	}
}

func TestSeedanceFetchVariantUsesOneStatusSource(t *testing.T) {
	a := &TaskAdaptor{}
	a.Init(seedanceMaxRelayInfo("seedance2-service-inference"))
	for _, path := range []string{"task.status", "data.status", "data.data.task.status"} {
		t.Run(path, func(t *testing.T) {
			response := a.profile.videoFetch().Response
			response.StatusPath = path
			a.selectedFetchResp = &response
			for _, raw := range []string{`"failed"`, `null`, `123`, `"unknown"`} {
				body, err := sjson.SetRaw(`{"id":"cgt-owner","status":"succeeded"}`, path, raw)
				require.NoError(t, err)
				if raw != `"failed"` {
					require.Error(t, a.ValidateTaskStatus([]byte(body)))
					continue
				}
				require.NoError(t, a.ValidateTaskStatus([]byte(body)))
				parsed, err := a.ParseTaskResult([]byte(body))
				require.NoError(t, err)
				require.Equal(t, model.TaskStatusFailure, parsed.Status)
				output, err := a.ConvertToNativeFetchResponse(&model.Task{Status: model.TaskStatusFailure, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner"}}, []byte(body))
				require.NoError(t, err)
				require.Equal(t, "failed", gjson.GetBytes(output, "status").String())
			}
		})
	}
}
