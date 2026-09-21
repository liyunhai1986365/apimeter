package configurable

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Resolve original asset URIs using the same persistent aliases as material
// management. Preserve content order, media roles and all unrelated fields.
func resolveTgxMaasVideoAssets(body []byte, info *relaycommon.RelayInfo) ([]byte, error) {
	if info == nil || info.ChannelMeta == nil {
		return body, nil
	}
	if info.TaskRelayInfo != nil && info.AssetProject != nil {
		// The access layer checked native and generic/form project aliases.
		// Send one spelling with that exact project, including after mapping.
		var names []string
		gjson.ParseBytes(body).ForEach(func(key, _ gjson.Result) bool {
			if strings.EqualFold(key.String(), "ProjectName") || strings.EqualFold(key.String(), "project_name") {
				names = append(names, key.String())
			}
			return true
		})
		for _, name := range names {
			var err error
			body, err = sjson.DeleteBytes(body, name)
			if err != nil {
				return nil, err
			}
		}
		var err error
		body, err = sjson.SetBytes(body, "ProjectName", *info.AssetProject)
		if err != nil {
			return nil, err
		}
	} else if !gjson.GetBytes(body, "ProjectName").Exists() {
		project := ""
		if info.ChannelSetting.Protocol != nil {
			project = strings.TrimSpace(info.ChannelSetting.Protocol.ProjectName)
		}
		if project != "" {
			var err error
			body, err = sjson.SetBytes(body, "ProjectName", project)
			if err != nil {
				return nil, err
			}
		}
	}
	if p := info.ChannelSetting.Protocol; p != nil && p.AssetLibrary != nil && p.AssetLibrary.Backend != "" && p.AssetLibrary.Backend != "inherit" && p.AssetLibrary.Backend != "tgxmaas" {
		return body, nil
	}
	project := gjson.GetBytes(body, "ProjectName").String()
	if p := info.ChannelSetting.Protocol; p != nil && p.AssetLibrary != nil && p.AssetLibrary.Backend == "tgxmaas" {
		ch, err := model.GetChannelById(info.ChannelId, true)
		if err != nil {
			return nil, err
		}
		project = ch.AssetHandleProject(project)
	}
	for i, item := range gjson.GetBytes(body, "content").Array() {
		kind := item.Get("type").String()
		if kind != "image_url" && kind != "video_url" && kind != "audio_url" {
			continue
		}
		uri := item.Get(kind + ".url").String()
		if !strings.HasPrefix(uri, "asset://asset-") {
			continue
		}
		id := strings.TrimPrefix(uri, "asset://")
		resolved := id
		var err error
		if info.TaskRelayInfo != nil && info.AssetAliases != nil {
			if alias := info.AssetAliases[id]; alias != "" {
				resolved = alias
			}
		} else {
			resolved, err = model.ResolveTgxMaasAssetHandle(info.ChannelId, info.UserId, project, id)
			if err != nil {
				return nil, err
			}
		}
		if resolved != id {
			body, err = sjson.SetBytes(body, fmt.Sprintf("content.%d.%s.url", i, kind), "asset://"+resolved)
			if err != nil {
				return nil, err
			}
		}
	}
	return body, nil
}
