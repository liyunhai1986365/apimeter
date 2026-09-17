package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/QuantumNous/new-api/common"
)

// AssetCredentials is write-only API input. It is never part of Channel JSON.
type AssetCredentials struct {
	APIKey          string `json:"api_key,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

func assetCredentialCipher() (cipher.AEAD, error) {
	if os.Getenv("CRYPTO_SECRET") == "" && os.Getenv("SESSION_SECRET") == "" {
		return nil, fmt.Errorf("configure a persistent CRYPTO_SECRET or SESSION_SECRET before saving asset credentials")
	}
	key := sha256.Sum256([]byte(common.CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (ch *Channel) SetAssetCredentials(credentials *AssetCredentials) error {
	if credentials == nil {
		return nil
	}
	aead, err := assetCredentialCipher()
	if err != nil {
		return err
	}
	raw, err := common.Marshal(credentials)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	ch.AssetSecret = base64.StdEncoding.EncodeToString(aead.Seal(nonce, nonce, raw, []byte("channel-asset-credentials-v1")))
	return nil
}

func (ch *Channel) GetAssetCredentials() (*AssetCredentials, error) {
	if ch.AssetSecret == "" {
		return nil, fmt.Errorf("asset credentials are not configured")
	}
	aead, err := assetCredentialCipher()
	if err != nil {
		return nil, err
	}
	sealed, err := base64.StdEncoding.DecodeString(ch.AssetSecret)
	if err != nil || len(sealed) < aead.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted asset credentials")
	}
	raw, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("channel-asset-credentials-v1"))
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt asset credentials; check CRYPTO_SECRET")
	}
	var credentials AssetCredentials
	if err = common.Unmarshal(raw, &credentials); err != nil {
		return nil, fmt.Errorf("invalid asset credentials")
	}
	return &credentials, nil
}
