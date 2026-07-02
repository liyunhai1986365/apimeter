package configurable

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/*.yaml
var profileFS embed.FS

type Profile struct {
	ID            string               `yaml:"id"`
	Name          string               `yaml:"name"`
	MediaType     string               `yaml:"media_type"`
	AcceptedModes []string             `yaml:"accepted_modes"`
	UpstreamModes []string             `yaml:"upstream_modes"`
	Conversions   []ConversionRef      `yaml:"conversions"`
	Video         *VideoProtocolConfig `yaml:"video,omitempty"`
	Native        NativeConfig         `yaml:"native"`
	Billing       BillingConfig        `yaml:"billing"`
	Submit        EndpointConfig       `yaml:"submit"`
	Fetch         EndpointConfig       `yaml:"fetch"`
	Resources     []ResourceConfig     `yaml:"resources"`
}

type VideoProtocolConfig struct {
	Native  NativeConfig   `yaml:"native"`
	Billing BillingConfig  `yaml:"billing"`
	Submit  EndpointConfig `yaml:"submit"`
	Fetch   EndpointConfig `yaml:"fetch"`
}

type ConversionRef struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type EndpointConfig struct {
	Method         string         `yaml:"method"`
	Path           string         `yaml:"path"`
	PathVariants   []PathVariant  `yaml:"path_variants"`
	Headers        []HeaderConfig `yaml:"headers"`
	Body           BodyConfig     `yaml:"body"`
	Response       ResponseConfig `yaml:"response"`
	OpenAIResponse ResponseConfig `yaml:"openai_response"`
}

type PathVariant struct {
	Path       string          `yaml:"path"`
	Field      string          `yaml:"field"`
	NonEmpty   *bool           `yaml:"non_empty"`
	Equals     any             `yaml:"equals"`
	Conditions []PathCondition `yaml:"conditions"`
}

type PathCondition struct {
	Field    string `yaml:"field"`
	NonEmpty *bool  `yaml:"non_empty"`
	Equals   any    `yaml:"equals"`
}

type NativeConfig struct {
	Submit NativeEndpointConfig `yaml:"submit"`
	Fetch  NativeEndpointConfig `yaml:"fetch"`
}

type NativeEndpointConfig struct {
	Method         string         `yaml:"method"`
	Path           string         `yaml:"path"`
	Passthrough    bool           `yaml:"passthrough"`
	ResponseFormat string         `yaml:"response_format"`
	Request        BodyConfig     `yaml:"request"`
	Response       ResponseConfig `yaml:"response"`
}

type ResourceConfig struct {
	ID          string                `yaml:"id"`
	Name        string                `yaml:"name"`
	Model       string                `yaml:"model"`
	Billing     ResourceBillingConfig `yaml:"billing"`
	Public      EndpointConfig        `yaml:"public"`
	Aliases     []EndpointConfig      `yaml:"aliases"`
	Upstream    EndpointConfig        `yaml:"upstream"`
	PreRequests []PreRequestConfig    `yaml:"pre_requests"`
	PathParams  map[string]string     `yaml:"path_params"`
	Request     BodyConfig            `yaml:"request"`
	Response    ResponseConfig        `yaml:"response"`
}

type ResourceBillingConfig struct {
	Enabled bool `yaml:"enabled"`
}

type PreRequestConfig struct {
	ID            string              `yaml:"id"`
	SkipIfPresent string              `yaml:"skip_if_present"`
	ManagedState  *ManagedStateConfig `yaml:"managed_state"`
	Upstream      EndpointConfig      `yaml:"upstream"`
	Request       BodyConfig          `yaml:"request"`
	Response      ResponseConfig      `yaml:"response"`
}

type ManagedStateConfig struct {
	Key       string         `yaml:"key"`
	ValuePath string         `yaml:"value_path"`
	Validate  EndpointConfig `yaml:"validate"`
}

type BillingConfig struct {
	Ratios []BillingRatioConfig `yaml:"ratios"`
}

type BillingRatioConfig struct {
	Key      string  `yaml:"key"`
	From     string  `yaml:"from"`
	Value    float64 `yaml:"value"`
	Default  float64 `yaml:"default"`
	OmitZero bool    `yaml:"omit_zero"`
}

type HeaderConfig struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value"`
	Transform string `yaml:"transform"`
}

type BodyConfig struct {
	Fields []FieldMapping `yaml:"fields"`
}

type FieldMapping struct {
	To                string         `yaml:"to"`
	From              string         `yaml:"from"`
	FallbackFrom      string         `yaml:"fallback_from"`
	FallbackFroms     []string       `yaml:"fallback_froms"`
	Transform         string         `yaml:"transform"`
	MediaType         string         `yaml:"media_type"`
	FirstOnly         bool           `yaml:"first_only"`
	WhenModelContains string         `yaml:"when_model_contains"`
	Append            bool           `yaml:"append"`
	Value             any            `yaml:"value"`
	ValueMap          map[string]any `yaml:"value_map"`
	OmitEmpty         bool           `yaml:"omit_empty"`
}

type ResponseConfig struct {
	Passthrough          bool              `yaml:"passthrough"`
	Fields               []FieldMapping    `yaml:"fields"`
	TaskIDPath           string            `yaml:"task_id_path"`
	StatusPath           string            `yaml:"status_path"`
	ProgressPath         string            `yaml:"progress_path"`
	CreateTimePath       string            `yaml:"create_time_path"`
	UpdateTimePath       string            `yaml:"update_time_path"`
	ActionPath           string            `yaml:"action_path"`
	ResultURLPath        string            `yaml:"result_url_path"`
	ResultBase64Path     string            `yaml:"result_base64_path"`
	ResultJSONPath       string            `yaml:"result_json_path"`
	ReasonPath           string            `yaml:"reason_path"`
	TotalTokensPath      string            `yaml:"total_tokens_path"`
	CompletionTokensPath string            `yaml:"completion_tokens_path"`
	StatusMap            map[string]string `yaml:"status_map"`
}

var (
	profilesOnce sync.Once
	profilesByID map[string]*Profile
	profilesErr  error
)

func ListProfiles() []*Profile {
	profiles, err := loadProfiles()
	if err != nil {
		return nil
	}
	result := make([]*Profile, 0, len(profiles))
	for _, profile := range profiles {
		cp := *profile
		result = append(result, &cp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func GetProfile(id string) (*Profile, bool) {
	profiles, err := loadProfiles()
	if err != nil {
		return nil, false
	}
	profile, ok := profiles[strings.TrimSpace(id)]
	if !ok {
		return nil, false
	}
	cp := *profile
	return &cp, true
}

func (p *Profile) ResourceByID(id string) (*ResourceConfig, bool) {
	if p == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	for _, resource := range p.Resources {
		if strings.TrimSpace(resource.ID) == id {
			cp := resource
			return &cp, true
		}
	}
	return nil, false
}

func (p *Profile) ResourceForEndpoint(method, path string) (*ResourceConfig, bool) {
	if p == nil {
		return nil, false
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	for _, resource := range p.Resources {
		for _, endpoint := range resourcePublicEndpoints(resource) {
			if strings.ToUpper(strings.TrimSpace(endpoint.Method)) != method {
				continue
			}
			if !nativePathMatches(endpoint.Path, path) {
				continue
			}
			cp := resource
			return &cp, true
		}
	}
	return nil, false
}

func (p *Profile) videoNative() NativeConfig {
	if p != nil && p.Video != nil {
		return p.Video.Native
	}
	if p == nil {
		return NativeConfig{}
	}
	return p.Native
}

func (p *Profile) videoSubmit() EndpointConfig {
	if p != nil && p.Video != nil {
		return p.Video.Submit
	}
	if p == nil {
		return EndpointConfig{}
	}
	return p.Submit
}

func (p *Profile) videoFetch() EndpointConfig {
	if p != nil && p.Video != nil {
		return p.Video.Fetch
	}
	if p == nil {
		return EndpointConfig{}
	}
	return p.Fetch
}

func (p *Profile) videoBilling() BillingConfig {
	if p != nil && p.Video != nil {
		return p.Video.Billing
	}
	if p == nil {
		return BillingConfig{}
	}
	return p.Billing
}

func (p *Profile) VideoNativeSubmit() NativeEndpointConfig {
	return p.videoNative().Submit
}

func (p *Profile) VideoNativeFetch() NativeEndpointConfig {
	return p.videoNative().Fetch
}

func MatchResource(method, path string) (*Profile, *ResourceConfig, bool) {
	profiles, err := loadProfiles()
	if err != nil {
		return nil, nil, false
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	for _, profile := range profiles {
		for _, resource := range profile.Resources {
			for _, endpoint := range resourcePublicEndpoints(resource) {
				if strings.ToUpper(strings.TrimSpace(endpoint.Method)) != method {
					continue
				}
				if !nativePathMatches(endpoint.Path, path) {
					continue
				}
				profileCopy := *profile
				resourceCopy := resource
				return &profileCopy, &resourceCopy, true
			}
		}
	}
	return nil, nil, false
}

func resourcePublicEndpoints(resource ResourceConfig) []EndpointConfig {
	endpoints := make([]EndpointConfig, 0, 1+len(resource.Aliases))
	if strings.TrimSpace(resource.Public.Method) != "" && strings.TrimSpace(resource.Public.Path) != "" {
		endpoints = append(endpoints, resource.Public)
	}
	for _, alias := range resource.Aliases {
		if strings.TrimSpace(alias.Method) == "" || strings.TrimSpace(alias.Path) == "" {
			continue
		}
		endpoints = append(endpoints, alias)
	}
	return endpoints
}

func MatchNativeSubmit(method, path string) (*Profile, bool) {
	return matchNativeEndpoint(method, path, func(profile *Profile) NativeEndpointConfig {
		return profile.videoNative().Submit
	})
}

func MatchNativeFetch(method, path string) (*Profile, bool) {
	return matchNativeEndpoint(method, path, func(profile *Profile) NativeEndpointConfig {
		return profile.videoNative().Fetch
	})
}

func MatchNativeSubmitProfiles(method, path string) []*Profile {
	return matchNativeEndpointProfiles(method, path, func(profile *Profile) NativeEndpointConfig {
		return profile.videoNative().Submit
	})
}

func MatchNativeFetchProfiles(method, path string) []*Profile {
	return matchNativeEndpointProfiles(method, path, func(profile *Profile) NativeEndpointConfig {
		return profile.videoNative().Fetch
	})
}

func matchNativeEndpoint(method, path string, endpoint func(*Profile) NativeEndpointConfig) (*Profile, bool) {
	matches := matchNativeEndpointProfiles(method, path, endpoint)
	if len(matches) == 0 {
		return nil, false
	}
	return matches[0], true
}

func matchNativeEndpointProfiles(method, path string, endpoint func(*Profile) NativeEndpointConfig) []*Profile {
	profiles, err := loadProfiles()
	if err != nil {
		return nil
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	matches := make([]*Profile, 0)
	for _, profile := range profiles {
		native := endpoint(profile)
		if strings.ToUpper(strings.TrimSpace(native.Method)) != method {
			continue
		}
		if !nativePathMatches(native.Path, path) {
			continue
		}
		cp := *profile
		matches = append(matches, &cp)
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ID < matches[j].ID
	})
	return matches
}

func nativePathMatches(pattern, path string) bool {
	pattern = strings.Trim(pattern, "/")
	path = strings.Trim(path, "/")
	if pattern == "" || path == "" {
		return pattern == path
	}
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	if len(patternParts) != len(pathParts) {
		return false
	}
	for i, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			continue
		}
		if patternPart != pathParts[i] {
			return false
		}
	}
	return true
}

func loadProfiles() (map[string]*Profile, error) {
	profilesOnce.Do(func() {
		profilesByID = make(map[string]*Profile)
		profilesErr = fs.WalkDir(profileFS, "profiles", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
				return nil
			}
			data, err := profileFS.ReadFile(path)
			if err != nil {
				return err
			}
			var profile Profile
			if err := yaml.Unmarshal(data, &profile); err != nil {
				return fmt.Errorf("unmarshal %s: %w", path, err)
			}
			if strings.TrimSpace(profile.ID) == "" {
				return fmt.Errorf("profile %s missing id", path)
			}
			profilesByID[profile.ID] = &profile
			return nil
		})
	})
	return profilesByID, profilesErr
}
