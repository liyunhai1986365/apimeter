package dto

type TaskImageAvailability struct {
	ImageHasURL    bool   `json:"image_has_url,omitempty"`
	ImageStatus    string `json:"image_status,omitempty"`
	ImageExpiresAt int64  `json:"image_expires_at,omitempty"`
	ImageMessage   string `json:"image_message,omitempty"`
}
