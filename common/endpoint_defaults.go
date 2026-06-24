package common

import "github.com/QuantumNous/new-api/constant"

// EndpointInfo 描述单个端点的默认请求信息
// path: 上游路径
// method: HTTP 请求方式，例如 POST/GET
// 目前均为 POST，后续可扩展
//
// json 标签用于直接序列化到 API 输出
// 例如：{"path":"/v1/chat/completions","method":"POST"}

type EndpointInfo struct {
	Path    string                 `json:"path"`
	Method  string                 `json:"method"`
	Label   string                 `json:"label,omitempty"`
	DocsURL string                 `json:"docs_url,omitempty"`
	Config  map[string]interface{} `json:"config,omitempty"`
}

// defaultEndpointInfoMap 保存内置端点的默认 Path 与 Method
var defaultEndpointInfoMap = map[constant.EndpointType]EndpointInfo{
	constant.EndpointTypeOpenAI:                {Path: "/v1/chat/completions", Method: "POST"},
	constant.EndpointTypeOpenAIResponse:        {Path: "/v1/responses", Method: "POST"},
	constant.EndpointTypeOpenAIResponseCompact: {Path: "/v1/responses/compact", Method: "POST"},
	constant.EndpointTypeAnthropic:             {Path: "/v1/messages", Method: "POST"},
	constant.EndpointTypeGemini:                {Path: "/v1beta/models/{model}:generateContent", Method: "POST"},
	constant.EndpointTypeJinaRerank:            {Path: "/v1/rerank", Method: "POST"},
	constant.EndpointTypeImageGeneration:       {Path: "/v1/images/generations", Method: "POST"},
	constant.EndpointTypeOpenAIImageEdit:       {Path: "/v1/images/edits", Method: "POST", Label: "OpenAI Image Edit"},
	constant.EndpointTypeGeminiImageGeneration: {Path: "/v1beta/models/{model}:predict", Method: "POST", Label: "Gemini Native Image Generation"},
	constant.EndpointTypeEmbeddings:            {Path: "/v1/embeddings", Method: "POST"},
	constant.EndpointTypeOpenAIVideo:           {Path: "/v1/video/generations", Method: "POST", Label: "OpenAI Video"},
	constant.EndpointTypeSeedance2NativeVideo: {
		Path:   "/api/v3/contents/generations/tasks",
		Method: "POST",
		Label:  "Seedance2 Native Video",
		Config: map[string]interface{}{
			"protocol_profile_id": "seedance2-service-inference",
			"mode":                "native_video",
			"submit_path":         "/api/v3/contents/generations/tasks",
			"fetch_path":          "/api/v3/contents/generations/tasks/{task_id}",
			"path_params":         []interface{}{"task_id"},
			"parameters": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "array",
					"required":    true,
					"description": "Text, image_url, video_url, or audio_url content items",
				},
				"resolution": map[string]interface{}{
					"type": "string",
					"enum": []interface{}{"480p", "720p"},
				},
				"duration": map[string]interface{}{
					"type":    "integer",
					"default": 4,
				},
				"ratio": map[string]interface{}{
					"type": "string",
				},
				"watermark": map[string]interface{}{
					"type": "boolean",
				},
				"generate_audio": map[string]interface{}{
					"type": "boolean",
				},
				"return_last_frame": map[string]interface{}{
					"type": "boolean",
				},
			},
		},
	},
}

// GetDefaultEndpointInfo 返回指定端点类型的默认信息以及是否存在
func GetDefaultEndpointInfo(et constant.EndpointType) (EndpointInfo, bool) {
	info, ok := defaultEndpointInfoMap[et]
	return info, ok
}
