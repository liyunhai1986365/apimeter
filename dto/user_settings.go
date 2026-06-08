package dto

import "strings"

type UserSetting struct {
	NotifyType                       string                  `json:"notify_type,omitempty"`                          // QuotaWarningType 额度预警类型
	QuotaWarningThreshold            float64                 `json:"quota_warning_threshold,omitempty"`              // QuotaWarningThreshold 额度预警阈值
	WebhookUrl                       string                  `json:"webhook_url,omitempty"`                          // WebhookUrl webhook地址
	WebhookSecret                    string                  `json:"webhook_secret,omitempty"`                       // WebhookSecret webhook密钥
	NotificationEmail                string                  `json:"notification_email,omitempty"`                   // NotificationEmail 通知邮箱地址
	BarkUrl                          string                  `json:"bark_url,omitempty"`                             // BarkUrl Bark推送URL
	GotifyUrl                        string                  `json:"gotify_url,omitempty"`                           // GotifyUrl Gotify服务器地址
	GotifyToken                      string                  `json:"gotify_token,omitempty"`                         // GotifyToken Gotify应用令牌
	GotifyPriority                   int                     `json:"gotify_priority"`                                // GotifyPriority Gotify消息优先级
	UpstreamModelUpdateNotifyEnabled bool                    `json:"upstream_model_update_notify_enabled,omitempty"` // 是否接收上游模型更新定时检测通知（仅管理员）
	AcceptUnsetRatioModel            bool                    `json:"accept_unset_model_ratio_model,omitempty"`       // AcceptUnsetRatioModel 是否接受未设置价格的模型
	RecordIpLog                      *bool                   `json:"record_ip_log,omitempty"`                        // 是否记录请求和错误日志IP，未设置时默认开启
	SidebarModules                   string                  `json:"sidebar_modules,omitempty"`                      // SidebarModules 左侧边栏模块配置
	BillingPreference                string                  `json:"billing_preference,omitempty"`                   // BillingPreference 扣费策略（订阅/钱包）
	Language                         string                  `json:"language,omitempty"`                             // Language 用户语言偏好 (zh, en)
	ModelFallback                    any                     `json:"model_fallback,omitempty"`                       // ModelFallback 用户模型备选配置
	ImageStorage                     UserImageStorageSetting `json:"image_storage,omitempty"`                        // ImageStorage 用户图片转存配置
}

const (
	UserImageStorageTypeAliyunOSS    = "aliyun_oss"
	UserImageStorageTypeCloudflareR2 = "cloudflare_r2"
)

type UserImageStorageSetting struct {
	Enabled                   bool   `json:"enabled,omitempty"`
	Type                      string `json:"type,omitempty"`
	Endpoint                  string `json:"endpoint,omitempty"`
	AccelerateEndpoint        string `json:"accelerate_endpoint,omitempty"`
	Bucket                    string `json:"bucket,omitempty"`
	AccessKeyID               string `json:"access_key_id,omitempty"`
	AccessKeySecret           string `json:"access_key_secret,omitempty"`
	AccessKeySecretConfigured bool   `json:"access_key_secret_configured,omitempty"`
	ObjectPrefix              string `json:"object_prefix,omitempty"`
	PublicBaseURL             string `json:"public_base_url,omitempty"`
}

func (setting UserImageStorageSetting) IsAliyunOSSReady() bool {
	return setting.Enabled &&
		setting.Type == UserImageStorageTypeAliyunOSS &&
		strings.TrimSpace(setting.Endpoint) != "" &&
		strings.TrimSpace(setting.Bucket) != "" &&
		strings.TrimSpace(setting.AccessKeyID) != "" &&
		strings.TrimSpace(setting.AccessKeySecret) != ""
}

func (setting UserImageStorageSetting) IsCloudflareR2Ready() bool {
	return setting.Enabled &&
		setting.Type == UserImageStorageTypeCloudflareR2 &&
		strings.TrimSpace(setting.Endpoint) != "" &&
		strings.TrimSpace(setting.Bucket) != "" &&
		strings.TrimSpace(setting.AccessKeyID) != "" &&
		strings.TrimSpace(setting.AccessKeySecret) != ""
}

func (setting UserImageStorageSetting) IsReady() bool {
	return setting.IsAliyunOSSReady() || setting.IsCloudflareR2Ready()
}

func (setting UserImageStorageSetting) Redacted() UserImageStorageSetting {
	if setting.AccessKeySecret != "" {
		setting.AccessKeySecretConfigured = true
		setting.AccessKeySecret = ""
	}
	return setting
}

var (
	NotifyTypeEmail   = "email"   // Email 邮件
	NotifyTypeWebhook = "webhook" // Webhook
	NotifyTypeBark    = "bark"    // Bark 推送
	NotifyTypeGotify  = "gotify"  // Gotify 推送
)

func (setting UserSetting) RecordIpLogEnabled() bool {
	return setting.RecordIpLog == nil || *setting.RecordIpLog
}
