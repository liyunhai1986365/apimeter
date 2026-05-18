package suno

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
