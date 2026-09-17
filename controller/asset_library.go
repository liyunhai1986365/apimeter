package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	volc "github.com/volcengine/volc-sdk-golang/base"
)

var errUnsupportedAssetOperation = errors.New("selected asset library does not support this operation")

func configurableResourceBaseURL(ch *model.Channel, resource *configurable.ResourceConfig) string {
	if resource.AssetLibrary {
		return assetBaseURL(ch)
	}
	return ch.GetBaseURL()
}

func authorizeConfigurableResourceRequest(c *gin.Context, ch *model.Channel, resource *configurable.ResourceConfig, req *http.Request) error {
	if resource.AssetLibrary {
		return authorizeAssetRequest(c, ch, req)
	}
	return nil
}

func configurableResourceHasIndependentHealth(ch *model.Channel, resource *configurable.ResourceConfig) bool {
	return resource.AssetLibrary && assetLibraryHasIndependentHealth(ch)
}

func configurableResourceStateKey(ch *model.Channel, resource *configurable.ResourceConfig, key string) string {
	if resource.AssetLibrary {
		return ch.AssetStateKey(key)
	}
	return key
}

func assetLibrary(ch *model.Channel) *relaydto.AssetLibrarySettings {
	if p := ch.GetSetting().Protocol; p != nil {
		return p.AssetLibrary
	}
	return nil
}

func assetBaseURL(ch *model.Channel) string {
	if cfg := assetLibrary(ch); cfg != nil && cfg.Backend != "inherit" && cfg.Backend != "disabled" && cfg.Backend != "" {
		if cfg.BaseURL != "" {
			return strings.TrimRight(cfg.BaseURL, "/")
		}
		if cfg.Backend == configurable.OfficialAssetBackend {
			return "https://ark.cn-beijing.volcengineapi.com"
		}
	}
	return ch.GetBaseURL()
}

// Independent asset services must not affect the health of video credentials.
func assetLibraryHasIndependentHealth(ch *model.Channel) bool {
	cfg := assetLibrary(ch)
	return cfg != nil && cfg.Backend != "" && cfg.Backend != "inherit" && cfg.Backend != "disabled" &&
		(cfg.AuthMode != "channel_key" || strings.TrimRight(assetBaseURL(ch), "/") != strings.TrimRight(ch.GetBaseURL(), "/"))
}

// Called after all request transformations. No caller may mutate signed fields afterwards.
func authorizeAssetRequest(c *gin.Context, ch *model.Channel, req *http.Request) error {
	cfg := assetLibrary(ch)
	if cfg == nil || cfg.Backend == "" || cfg.Backend == "inherit" {
		return nil
	}
	if cfg.AuthMode == "channel_key" {
		if ch.ChannelInfo.IsMultiKey {
			return fmt.Errorf("asset libraries require a dedicated key when the video channel uses multiple keys")
		}
		req.Header.Set("Authorization", "Bearer "+common.GetContextKeyString(c, constant.ContextKeyChannelKey))
		return nil
	}
	credentials, err := ch.GetAssetCredentials()
	if err != nil {
		return err
	}
	if cfg.AuthMode == "api_key" {
		req.Header.Set("Authorization", "Bearer "+credentials.APIKey)
		return nil
	}
	if cfg.AuthMode != "aksk" {
		return fmt.Errorf("unsupported asset authentication mode")
	}
	region := cfg.Region
	if region == "" {
		region = "cn-beijing"
	}
	req.Header.Del("Authorization")
	volc.Credentials{AccessKeyID: credentials.AccessKeyID, SecretAccessKey: credentials.SecretAccessKey, Region: region, Service: "ark"}.Sign(req)
	return nil
}

func validateAssetLibrary(ch *model.Channel, credentials *model.AssetCredentials) error {
	cfg := assetLibrary(ch)
	if cfg == nil {
		return nil
	}
	if ch.Type != constant.ChannelTypeConfigurable {
		return fmt.Errorf("asset library settings require a configurable channel")
	}
	if cfg.Backend == "inherit" || cfg.Backend == "disabled" {
		return nil
	}
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	if !ok {
		return fmt.Errorf("unknown asset library backend")
	}
	if cfg.BaseURL != "" {
		u, err := url.Parse(cfg.BaseURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("asset Base URL must be an HTTP(S) URL without credentials, query or fragment")
		}
	}
	if profile.ID == configurable.OfficialAssetBackend || profile.ID == "seedance-tgxmaas" {
		if strings.TrimSpace(ch.GetSetting().Protocol.ProjectName) == "" {
			return fmt.Errorf("asset library ProjectName is required")
		}
	}
	if cfg.Backend == configurable.OfficialAssetBackend && cfg.AuthMode != "aksk" {
		return fmt.Errorf("official Volcengine assets require AK/SK authentication")
	}
	switch cfg.AuthMode {
	case "channel_key":
		if ch.ChannelInfo.IsMultiKey {
			return fmt.Errorf("use dedicated asset credentials for a multi-key video channel")
		}
	case "api_key", "aksk":
		if cfg.AuthMode == "aksk" && cfg.Backend != configurable.OfficialAssetBackend {
			return fmt.Errorf("AK/SK authentication requires the official asset backend")
		}
		if credentials == nil {
			var err error
			credentials, err = ch.GetAssetCredentials()
			if err != nil {
				return err
			}
		}
		if cfg.AuthMode == "api_key" && strings.TrimSpace(credentials.APIKey) == "" {
			return fmt.Errorf("asset API key is required")
		}
		if cfg.AuthMode == "aksk" && (strings.TrimSpace(credentials.AccessKeyID) == "" || strings.TrimSpace(credentials.SecretAccessKey) == "") {
			return fmt.Errorf("asset AccessKey ID and Secret Access Key are required")
		}
	default:
		return fmt.Errorf("unknown asset authentication mode")
	}
	return nil
}

// Keep secrets out of setting JSON and all channel read/export responses.
func prepareAssetCredentials(ch *model.Channel, original *model.Channel, credentials *model.AssetCredentials) error {
	validationChannel := *ch
	if original != nil {
		validationChannel.AssetSecret = original.AssetSecret
		if validationChannel.Setting == nil {
			validationChannel.Setting = original.Setting
		}
		if validationChannel.Type == 0 {
			validationChannel.Type = original.Type
		}
	}
	if err := validateAssetLibrary(&validationChannel, credentials); err != nil {
		return err
	}
	if credentials != nil {
		cfg := assetLibrary(&validationChannel)
		if cfg == nil || (cfg.AuthMode != "api_key" && cfg.AuthMode != "aksk") {
			return fmt.Errorf("select dedicated asset authentication before supplying credentials")
		}
		return ch.SetAssetCredentials(credentials)
	}
	if original != nil {
		// GORM omits empty fields. Do not rewrite a stale snapshot of a secret
		// that another request may have rotated since original was loaded.
		ch.AssetSecret = ""
	}
	return nil
}
