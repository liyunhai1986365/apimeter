package dto

type ImageTaskResponse struct {
	ID         string         `json:"id"`
	State      string         `json:"state"`
	Progress   int            `json:"progress"`
	CreateTime int64          `json:"create_time"`
	UpdateTime int64          `json:"update_time"`
	Action     string         `json:"action"`
	Data       ImageTaskData  `json:"data"`
	Error      map[string]any `json:"error,omitempty"`
}

type ImageTaskData struct {
	Images []ImageTaskImage `json:"images"`
}

type ImageTaskImage struct {
	URL      string `json:"url"`
	FileName string `json:"file_name,omitempty"`
	B64Json  string `json:"b64_json,omitempty"`
}
