package suno

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func TestDuomiRequestBodyUsesJsonWithNumericFlags(t *testing.T) {
	customMode := dto.BoolValue(true)
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.wike.cc"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			Action: constant.SunoActionMusic,
		},
	}
	ctx := &gin.Context{}
	ctx.Set("task_request", &dto.SunoSubmitReq{
		CustomMode:       &customMode,
		MakeInstrumental: dto.BoolValue(false),
		Prompt:           "写一首充满怀旧、回忆感觉的流行歌",
		Mv:               "chirp-v4",
		Title:            "一路向北",
		Tags:             "pop",
		NegativeTags:     "dance pop",
	})

	body, err := adaptor.BuildRequestBody(ctx, info)
	if err != nil {
		t.Fatalf("BuildRequestBody error: %v", err)
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}

	var payload map[string]any
	if err := common.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("request body is not json: %v, body: %s", err, string(raw))
	}
	if got := payload["custom_mode"]; got != float64(1) {
		t.Fatalf("unexpected custom_mode: %#v, body: %s", got, string(raw))
	}
	if got := payload["make_instrumental"]; got != float64(0) {
		t.Fatalf("unexpected make_instrumental: %#v, body: %s", got, string(raw))
	}
	if got := payload["prompt"]; got != "写一首充满怀旧、回忆感觉的流行歌" {
		t.Fatalf("unexpected prompt: %#v", got)
	}
}

func TestSunoSubmitReqAcceptsDuomiNumericFlags(t *testing.T) {
	var req dto.SunoSubmitReq
	if err := common.Unmarshal([]byte(`{"custom_mode":1,"make_instrumental":0,"prompt":"hello"}`), &req); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if req.CustomMode == nil || !bool(*req.CustomMode) {
		t.Fatalf("custom_mode numeric flag was not parsed as true: %#v", req.CustomMode)
	}
	if bool(req.MakeInstrumental) {
		t.Fatalf("make_instrumental numeric zero should parse as false")
	}
}

func TestDuomiRequestConfig(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://api.wike.cc",
			ApiKey:         "duomi-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			Action: constant.SunoActionMusic,
		},
	}
	adaptor.Init(info)

	url, err := adaptor.BuildRequestURL(info)
	if err != nil {
		t.Fatalf("BuildRequestURL error: %v", err)
	}
	if url != "https://api.wike.cc/api/suno/generate" {
		t.Fatalf("unexpected request url: %s", url)
	}

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatalf("NewRequest error: %v", err)
	}
	ctx := &gin.Context{Request: &http.Request{Header: http.Header{"Content-Type": []string{"application/json"}}}}
	if err := adaptor.BuildRequestHeader(ctx, req, info); err != nil {
		t.Fatalf("BuildRequestHeader error: %v", err)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected Content-Type header: %q", got)
	}
	if got := req.Header.Get("Authorization"); got != "duomi-key" {
		t.Fatalf("unexpected Authorization header: %q", got)
	}
}

func TestDuomiSubmitResponseUsesFirstUpstreamTaskID(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.wike.cc"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}
	adaptor.Init(info)

	w := &responseRecorder{header: http.Header{}}
	ctx := &gin.Context{Writer: w}
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewBufferString(`{"code":200,"msg":"ok","data":["upstream_1","upstream_2"]}`)),
	}

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr: %v", taskErr)
	}
	if taskID != "upstream_1,upstream_2" {
		t.Fatalf("unexpected task id: %s", taskID)
	}
	if w.status != http.StatusOK {
		t.Fatalf("unexpected response status: %d", w.status)
	}
	if !bytes.Contains(w.body.Bytes(), []byte(`"data":"task_public"`)) {
		t.Fatalf("public task id not returned to client: %s", w.body.String())
	}
}

func TestDuomiSubmitResponseStoresMultipleUpstreamTaskIDs(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.wike.cc"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}
	adaptor.Init(info)

	w := &responseRecorder{header: http.Header{}}
	ctx := &gin.Context{Writer: w}
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewBufferString(`{"code":200,"msg":"ok","data":["upstream_1","upstream_2"]}`)),
	}

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr: %v", taskErr)
	}
	if taskID != "upstream_1,upstream_2" {
		t.Fatalf("unexpected stored task id: %s", taskID)
	}
}

func TestDuomiSubmitResponseAcceptsObjectData(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.wike.cc"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}
	adaptor.Init(info)

	w := &responseRecorder{header: http.Header{}}
	ctx := &gin.Context{Writer: w}
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewBufferString(`{"code":200,"msg":"ok","data":{"task_id":"upstream_1"}}`)),
	}

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr: %v", taskErr)
	}
	if taskID != "upstream_1" {
		t.Fatalf("unexpected stored task id: %s", taskID)
	}
}

func TestDuomiSubmitResponseAcceptsObjectArrayData(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.wike.cc"},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}
	adaptor.Init(info)

	w := &responseRecorder{header: http.Header{}}
	ctx := &gin.Context{Writer: w}
	resp := &http.Response{
		Body: io.NopCloser(bytes.NewBufferString(`{"code":200,"msg":"ok","data":[{"task_id":"upstream_1"},{"task_id":"upstream_2"}]}`)),
	}

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse taskErr: %v", taskErr)
	}
	if taskID != "upstream_1,upstream_2" {
		t.Fatalf("unexpected stored task id: %s", taskID)
	}
}

func TestDuomiFeedResponseConvertsToSunoDataResponse(t *testing.T) {
	body := []byte(`{"code":200,"msg":"ok","data":{"task_id":"upstream_1","state":"succeeded","title":"Song","audio_url":"https://cdn.example/song.mp3","image_url":"https://cdn.example/img.jpg","clip_id":"clip_1","prompt":"lyrics","gpt_description_prompt":"desc","mv":"chirp-v4-5","tags":"pop","duration":123}}`)

	items, err := parseDuomiFetchResponse("upstream_1", body)
	if err != nil {
		t.Fatalf("parseDuomiFetchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	item := items[0]
	if item.TaskID != "upstream_1" || item.Status != "SUCCESS" {
		t.Fatalf("unexpected task status: %+v", item)
	}

	var songs []dto.SunoSong
	if err := common.Unmarshal(item.Data, &songs); err != nil {
		t.Fatalf("unmarshal song data error: %v", err)
	}
	if len(songs) != 1 || songs[0].AudioURL != "https://cdn.example/song.mp3" || songs[0].ID != "clip_1" {
		t.Fatalf("unexpected song data: %+v", songs)
	}
}

func TestDuomiFeedResponseMapsNumericStatus(t *testing.T) {
	body := []byte(`{"code":200,"msg":"ok","data":{"task_id":"upstream_1","status":"3","title":"Song","audio_url":"https://cdn.example/song.mp3","image_url":"https://cdn.example/img.jpg","prompt":"lyrics","mv":"chirp-v4"}}`)

	items, err := parseDuomiFetchResponse("upstream_1", body)
	if err != nil {
		t.Fatalf("parseDuomiFetchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].Status != "SUCCESS" {
		t.Fatalf("unexpected status: %+v", items[0])
	}
}

func TestDuomiFeedResponseParsesEndpointSuccessShape(t *testing.T) {
	body := []byte(`{"code":200,"msg":"成功","data":{"status":"3","task_id":"fa421931-b29e-4a57-b409-4d14a5a6cbad","prompt":"[Verse]\nBright light shining in the sky","title":"Rays of Fire","gpt_description_prompt":"Write a song about the sun","audio_url":"https://cdn1.suno.ai/fa421931-b29e-4a57-b409-4d14a5a6cbad.mp3","image_url":"https://cdn1.suno.ai/image_fa421931-b29e-4a57-b409-4d14a5a6cbad.png","video_url":"https://cdn1.suno.ai/fa421931-b29e-4a57-b409-4d14a5a6cbad.mp4","image_large_url":"https://cdn1.suno.ai/image_large_fa421931-b29e-4a57-b409-4d14a5a6cbad.png","tags":null,"mv":"chirp-v3-0","continue_clip_id":null,"continue_at":null},"exec_time":0.554264,"ip":"117.173.160.45"}`)

	items, err := parseDuomiFetchResponse("fa421931-b29e-4a57-b409-4d14a5a6cbad", body)
	if err != nil {
		t.Fatalf("parseDuomiFetchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	item := items[0]
	if item.TaskID != "fa421931-b29e-4a57-b409-4d14a5a6cbad" || item.Status != "SUCCESS" {
		t.Fatalf("unexpected task status: %+v", item)
	}

	var songs []dto.SunoSong
	if err := common.Unmarshal(item.Data, &songs); err != nil {
		t.Fatalf("unmarshal song data error: %v", err)
	}
	if len(songs) != 1 {
		t.Fatalf("unexpected song count: %d", len(songs))
	}
	song := songs[0]
	if song.AudioURL != "https://cdn1.suno.ai/fa421931-b29e-4a57-b409-4d14a5a6cbad.mp3" ||
		song.ImageURL != "https://cdn1.suno.ai/image_fa421931-b29e-4a57-b409-4d14a5a6cbad.png" ||
		song.VideoURL != "https://cdn1.suno.ai/fa421931-b29e-4a57-b409-4d14a5a6cbad.mp4" ||
		song.Title != "Rays of Fire" {
		t.Fatalf("unexpected song data: %+v", song)
	}
}

func TestDuomiFeedResponseDoesNotReturnContentBeforeStatusThree(t *testing.T) {
	body := []byte(`{"code":200,"msg":"ok","data":{"task_id":"upstream_1","status":"2","title":"Song","audio_url":"https://cdn.example/song.mp3","image_url":"https://cdn.example/img.jpg","prompt":"lyrics","mv":"chirp-v4"}}`)

	items, err := parseDuomiFetchResponse("upstream_1", body)
	if err != nil {
		t.Fatalf("parseDuomiFetchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].Status != "IN_PROGRESS" {
		t.Fatalf("unexpected status: %+v", items[0])
	}
	if len(items[0].Data) != 0 {
		t.Fatalf("status != 3 should not return content, got: %s", string(items[0].Data))
	}
}

func TestDuomiFeedResponseMapsStatusTwoWithMessageToFailure(t *testing.T) {
	body := []byte(`{"code":200,"msg":"success","data":{"task_id":"upstream_1","status":"2","msg":"Song generation is temporarily unavailable. Please try again shortly."}}`)

	items, err := parseDuomiFetchResponse("upstream_1", body)
	if err != nil {
		t.Fatalf("parseDuomiFetchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	item := items[0]
	if item.Status != "FAILURE" {
		t.Fatalf("unexpected status: %+v", item)
	}
	if item.FailReason != "Song generation is temporarily unavailable. Please try again shortly." {
		t.Fatalf("unexpected fail reason: %+v", item)
	}
	if item.FinishTime == 0 {
		t.Fatalf("failure response should set finish time: %+v", item)
	}
}

func TestDuomiFeedResponsePrefersStatusOverStateForSuccess(t *testing.T) {
	body := []byte(`{"code":200,"msg":"ok","data":{"task_id":"upstream_1","state":"succeeded","status":"2","title":"Song","audio_url":"https://cdn.example/song.mp3","image_url":"https://cdn.example/img.jpg","prompt":"lyrics","mv":"chirp-v4"}}`)

	items, err := parseDuomiFetchResponse("upstream_1", body)
	if err != nil {
		t.Fatalf("parseDuomiFetchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected item count: %d", len(items))
	}
	if items[0].Status != "IN_PROGRESS" {
		t.Fatalf("status field should decide success when present: %+v", items[0])
	}
	if len(items[0].Data) != 0 {
		t.Fatalf("status != 3 should not return content, got: %s", string(items[0].Data))
	}
}

func TestFetchDuomiTasksKeepsBatchWhenOneTaskIsNotFound(t *testing.T) {
	service.InitHttpClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskID := r.URL.Query().Get("task_id")
		w.Header().Set("Content-Type", "application/json")
		switch taskID {
		case "upstream_ok":
			_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"task_id":"upstream_ok","state":"succeeded","title":"Song","audio_url":"https://cdn.example/song.mp3"}}`))
		case "upstream_missing":
			_, _ = w.Write([]byte(`{"code":404,"msg":"This task was not found","data":{"task_id":"upstream_missing"}}`))
		default:
			t.Fatalf("unexpected task id: %s", taskID)
		}
	}))
	defer server.Close()

	resp, err := fetchDuomiTasks(server.URL, "duomi-key", map[string]any{
		"ids": []string{"upstream_ok", "upstream_missing"},
	}, "")
	if err != nil {
		t.Fatalf("fetchDuomiTasks error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response status: %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	var result dto.TaskResponse[[]dto.SunoDataResponse]
	if err := common.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal error: %v, body: %s", err, string(raw))
	}
	if !result.IsSuccess() {
		t.Fatalf("unexpected task response: %+v", result)
	}
	if len(result.Data) != 2 {
		t.Fatalf("unexpected item count: %d, body: %s", len(result.Data), string(raw))
	}
	if result.Data[0].TaskID != "upstream_ok" || result.Data[0].Status != "SUCCESS" {
		t.Fatalf("unexpected success item: %+v", result.Data[0])
	}
	if result.Data[1].TaskID != "upstream_missing" || result.Data[1].Status != "FAILURE" || result.Data[1].FailReason != "This task was not found" {
		t.Fatalf("unexpected missing item: %+v", result.Data[1])
	}
}

func TestFetchDuomiTasksKeepsBatchForArbitraryUpstreamErrors(t *testing.T) {
	service.InitHttpClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskID := r.URL.Query().Get("task_id")
		w.Header().Set("Content-Type", "application/json")
		switch taskID {
		case "upstream_ok":
			_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"task_id":"upstream_ok","state":"succeeded","title":"Song","audio_url":"https://cdn.example/song.mp3"}}`))
		case "upstream_500":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`internal upstream error`))
		case "upstream_business_error":
			_, _ = w.Write([]byte(`{"code":503,"msg":"upstream overloaded","data":{"task_id":"upstream_business_error"}}`))
		case "upstream_bad_json":
			_, _ = w.Write([]byte(`not-json`))
		default:
			t.Fatalf("unexpected task id: %s", taskID)
		}
	}))
	defer server.Close()

	resp, err := fetchDuomiTasks(server.URL, "duomi-key", map[string]any{
		"ids": []string{"upstream_ok", "upstream_500", "upstream_business_error", "upstream_bad_json"},
	}, "")
	if err != nil {
		t.Fatalf("fetchDuomiTasks error: %v", err)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	var result dto.TaskResponse[[]dto.SunoDataResponse]
	if err := common.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal error: %v, body: %s", err, string(raw))
	}
	if len(result.Data) != 4 {
		t.Fatalf("unexpected item count: %d, body: %s", len(result.Data), string(raw))
	}
	if result.Data[0].TaskID != "upstream_ok" || result.Data[0].Status != "SUCCESS" {
		t.Fatalf("unexpected success item: %+v", result.Data[0])
	}
	for _, item := range result.Data[1:] {
		if item.Status != "FAILURE" || item.TaskID == "" || item.FailReason == "" {
			t.Fatalf("unexpected failure item: %+v", item)
		}
	}
}

func TestFetchDuomiTasksFetchesItemsConcurrently(t *testing.T) {
	service.InitHttpClient()

	var active int32
	var maxActive int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			observed := atomic.LoadInt32(&maxActive)
			if current <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, current) {
				break
			}
		}

		time.Sleep(50 * time.Millisecond)
		taskID := r.URL.Query().Get("task_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"ok","data":{"task_id":"` + taskID + `","state":"succeeded","title":"Song","audio_url":"https://cdn.example/song.mp3"}}`))
	}))
	defer server.Close()

	ids := make([]string, 12)
	for i := range ids {
		ids[i] = "upstream_" + strconv.Itoa(i)
	}
	startedAt := time.Now()
	resp, err := fetchDuomiTasks(server.URL, "duomi-key", map[string]any{
		"ids": ids,
	}, "")
	if err != nil {
		t.Fatalf("fetchDuomiTasks error: %v", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 350*time.Millisecond {
		t.Fatalf("fetchDuomiTasks was too slow, likely sequential: %s", elapsed)
	}
	if atomic.LoadInt32(&maxActive) < 2 {
		t.Fatalf("expected concurrent upstream fetches, max active: %d", maxActive)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	var result dto.TaskResponse[[]dto.SunoDataResponse]
	if err := common.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal error: %v, body: %s", err, string(raw))
	}
	if len(result.Data) != len(ids) {
		t.Fatalf("unexpected item count: %d", len(result.Data))
	}
	for i, item := range result.Data {
		if item.TaskID != ids[i] || item.Status != "SUCCESS" {
			t.Fatalf("unexpected item at %d: %+v", i, item)
		}
	}
}

func TestFetchStandardSunoTasksKeepsBatchForArbitraryUpstreamErrors(t *testing.T) {
	service.InitHttpClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/suno/fetch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload struct {
			IDs []string `json:"ids"`
		}
		if err := common.DecodeJson(r.Body, &payload); err != nil {
			t.Fatalf("DecodeJson error: %v", err)
		}
		if len(payload.IDs) != 1 {
			t.Fatalf("expected isolated single-id fetch, got: %#v", payload.IDs)
		}
		taskID := payload.IDs[0]
		w.Header().Set("Content-Type", "application/json")
		switch taskID {
		case "upstream_ok":
			_, _ = w.Write([]byte(`{"code":"success","data":[{"task_id":"upstream_ok","status":"SUCCESS","data":[{"id":"song_1","audio_url":"https://cdn.example/song.mp3"}]}]}`))
		case "upstream_500":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`internal upstream error`))
		case "upstream_business_error":
			_, _ = w.Write([]byte(`{"code":"failed","message":"upstream overloaded"}`))
		case "upstream_bad_json":
			_, _ = w.Write([]byte(`not-json`))
		default:
			t.Fatalf("unexpected task id: %s", taskID)
		}
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "suno-key", map[string]any{
		"ids": []string{"upstream_ok", "upstream_500", "upstream_business_error", "upstream_bad_json"},
	}, "")
	if err != nil {
		t.Fatalf("FetchTask error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response status: %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	var result dto.TaskResponse[[]dto.SunoDataResponse]
	if err := common.Unmarshal(raw, &result); err != nil {
		t.Fatalf("Unmarshal error: %v, body: %s", err, string(raw))
	}
	if len(result.Data) != 4 {
		t.Fatalf("unexpected item count: %d, body: %s", len(result.Data), string(raw))
	}
	if result.Data[0].TaskID != "upstream_ok" || result.Data[0].Status != "SUCCESS" {
		t.Fatalf("unexpected success item: %+v", result.Data[0])
	}
	for _, item := range result.Data[1:] {
		if item.Status != "FAILURE" || item.TaskID == "" || item.FailReason == "" {
			t.Fatalf("unexpected failure item: %+v", item)
		}
	}
}

func TestExtractDuomiTaskIDsSplitsStoredMultipleIDs(t *testing.T) {
	ids := extractDuomiTaskIDs([]string{"upstream_1,upstream_2", "upstream_3"})
	if len(ids) != 3 || ids[0] != "upstream_1" || ids[1] != "upstream_2" || ids[2] != "upstream_3" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

type responseRecorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (r *responseRecorder) WriteHeaderNow() {}
func (r *responseRecorder) WriteString(s string) (int, error) {
	return r.Write([]byte(s))
}
func (r *responseRecorder) Status() int              { return r.status }
func (r *responseRecorder) Size() int                { return r.body.Len() }
func (r *responseRecorder) Written() bool            { return r.status != 0 }
func (r *responseRecorder) WriteHeaderNowCalled()    {}
func (r *responseRecorder) Pusher() http.Pusher      { return nil }
func (r *responseRecorder) Flush()                   {}
func (r *responseRecorder) CloseNotify() <-chan bool { return make(chan bool) }
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}
