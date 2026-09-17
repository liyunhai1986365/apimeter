package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Keep aliases in the existing persistent resource state store. They are scoped
// by gateway user, supplier channel and project; never resolve another user's
// original ID through a shared upstream credential.
func tgxMaasHandleKey(project, id string) string {
	if project == "" {
		project = "default"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(project+"\x00"+id)))
}

func SaveTgxMaasAssetHandle(channelID, userID int, project, originalID, providerID string) error {
	if originalID == "" || providerID == "" || originalID == providerID {
		return nil
	}
	return UpsertConfigurableResourceState(&ConfigurableResourceState{
		ChannelID: channelID, ProfileID: "seedance-tgxmaas", ResourceID: "asset_handle",
		PreRequestID: "original_id", UserID: userID,
		StateKey: tgxMaasHandleKey(project, originalID), StateValue: providerID,
	})
}

func ResolveTgxMaasAssetHandle(channelID, userID int, project, id string) (string, error) {
	if !strings.HasPrefix(id, "asset-") && !strings.HasPrefix(id, "group-") {
		return id, nil
	}
	state, err := FindActiveConfigurableResourceState(channelID, "seedance-tgxmaas", "asset_handle", "original_id", userID, 0, tgxMaasHandleKey(project, id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return id, nil
	} // New supplier versions may accept original handles directly.
	if err != nil {
		return "", err
	}
	return state.StateValue, nil
}

// Explicit backends have independent account/endpoint scopes. Legacy channels
// retain their existing lookup keys. Replacing dedicated credentials invalidates
// aliases conservatively rather than applying an old account's IDs to a new one.
func (ch *Channel) AssetHandleProject(project string) string {
	p := ch.GetSetting().Protocol
	if p == nil || p.AssetLibrary == nil || p.AssetLibrary.Backend == "" || p.AssetLibrary.Backend == "inherit" {
		return project
	}
	if project == "" {
		project = "default"
	}
	cfg := p.AssetLibrary
	endpoint := cfg.BaseURL
	if endpoint == "" {
		endpoint = ch.GetBaseURL()
	}
	credential := ch.AssetSecret
	if cfg.AuthMode == "channel_key" {
		credential = ch.Key
	}
	scope := sha256.Sum256([]byte(cfg.Backend + "\x00" + strings.TrimRight(endpoint, "/") + "\x00" + cfg.AuthMode + "\x00" + credential))
	return fmt.Sprintf("%s\x00%x", project, scope)
}

func (ch *Channel) AssetStateKey(key string) string {
	scoped := ch.AssetHandleProject(key)
	if scoped == key {
		return key
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(scoped)))
}
