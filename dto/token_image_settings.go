package dto

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	TokenImageFormatFollowRequest = "follow_request"
	TokenImageFormatURL           = "url"
	TokenImageFormatB64JSON       = "b64_json"

	TokenImageStoreDefault                = "default"
	TokenImageStoreKeepEndpointURL        = "keep_endpoint_url"
	TokenImageStoreOnlyStoreBase64        = "only_store_base64"
	TokenImageStoreForceStoreURLAndBase64 = "force_store_url_and_base64"
)

type TokenImageSettings struct {
	Format string `json:"format,omitempty"`
	Store  string `json:"store,omitempty"`
}

func (s TokenImageSettings) Normalized() TokenImageSettings {
	switch strings.ToLower(strings.TrimSpace(s.Format)) {
	case TokenImageFormatURL:
		s.Format = TokenImageFormatURL
	case TokenImageFormatB64JSON, "base64":
		s.Format = TokenImageFormatB64JSON
	default:
		s.Format = TokenImageFormatFollowRequest
	}

	switch strings.ToLower(strings.TrimSpace(s.Store)) {
	case TokenImageStoreKeepEndpointURL:
		s.Store = TokenImageStoreKeepEndpointURL
	case TokenImageStoreOnlyStoreBase64:
		s.Store = TokenImageStoreOnlyStoreBase64
	case TokenImageStoreForceStoreURLAndBase64:
		s.Store = TokenImageStoreForceStoreURLAndBase64
	default:
		s.Store = TokenImageStoreDefault
	}
	return s
}

func (s TokenImageSettings) Value() (driver.Value, error) {
	s = s.Normalized()
	if s.Format == TokenImageFormatFollowRequest && s.Store == TokenImageStoreDefault {
		return "", nil
	}
	bytes, err := common.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(bytes), nil
}

func (s *TokenImageSettings) Scan(value interface{}) error {
	if s == nil {
		return nil
	}
	if value == nil {
		*s = TokenImageSettings{}.Normalized()
		return nil
	}
	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("unsupported token image settings type: %T", value)
	}
	if strings.TrimSpace(string(data)) == "" {
		*s = TokenImageSettings{}.Normalized()
		return nil
	}
	if err := common.Unmarshal(data, s); err != nil {
		return err
	}
	*s = s.Normalized()
	return nil
}
