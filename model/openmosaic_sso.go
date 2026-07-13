package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrOpenMosaicSSOCodeInvalid = errors.New("openmosaic sso authorization code is invalid")

type OpenMosaicSSOTokenBinding struct {
	Id        int `gorm:"primaryKey"`
	UserId    int `gorm:"not null;uniqueIndex"`
	TokenId   int `gorm:"not null;uniqueIndex"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type OpenMosaicSSOAuthorizationCode struct {
	Id          int        `gorm:"primaryKey"`
	CodeHash    string     `gorm:"type:char(64);not null;uniqueIndex"`
	UserId      int        `gorm:"not null;index"`
	RedirectURI string     `gorm:"type:varchar(512);not null"`
	ExpiresAt   time.Time  `gorm:"not null;index"`
	ConsumedAt  *time.Time `gorm:"index"`
	CreatedAt   time.Time
}

type OpenMosaicSSOClaim struct {
	UserID        int
	Username      string
	DisplayName   string
	Email         string
	EmailVerified bool
	APIKey        string
}

func EnsureOpenMosaicSSOToken(tx *gorm.DB, userID int) (*Token, error) {
	var result Token
	err := tx.Transaction(func(db *gorm.DB) error {
		var binding OpenMosaicSSOTokenBinding
		err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&binding).Error
		if err == nil {
			tokenErr := db.Where("id = ? AND user_id = ?", binding.TokenId, userID).First(&result).Error
			if tokenErr == nil {
				if result.Status != common.TokenStatusEnabled {
					result.Status = common.TokenStatusEnabled
					if err := db.Model(&result).Update("status", common.TokenStatusEnabled).Error; err != nil {
						return err
					}
				}
				return nil
			}
			if !errors.Is(tokenErr, gorm.ErrRecordNotFound) {
				return tokenErr
			}
			if err := db.Delete(&binding).Error; err != nil {
				return err
			}
			err = gorm.ErrRecordNotFound
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		key, err := common.GenerateKey()
		if err != nil {
			return err
		}
		result = Token{
			UserId:         userID,
			Name:           "OpenMosaic 默认 Key",
			Key:            key,
			Status:         common.TokenStatusEnabled,
			CreatedTime:    common.GetTimestamp(),
			AccessedTime:   common.GetTimestamp(),
			ExpiredTime:    -1,
			UnlimitedQuota: true,
		}
		if err := db.Create(&result).Error; err != nil {
			return err
		}
		return db.Create(&OpenMosaicSSOTokenBinding{UserId: userID, TokenId: result.Id}).Error
	})
	return &result, err
}

func CreateOpenMosaicSSOAuthorizationCode(tx *gorm.DB, userID int, redirectURI string, ttl time.Duration) (string, error) {
	redirectURI = strings.TrimSpace(redirectURI)
	if userID <= 0 || redirectURI == "" {
		return "", ErrOpenMosaicSSOCodeInvalid
	}
	// Keep the short-lived handoff table bounded without requiring a scheduler.
	_ = tx.Where("expires_at < ? OR consumed_at IS NOT NULL", time.Now().Add(-time.Hour)).Delete(&OpenMosaicSSOAuthorizationCode{}).Error
	rawCode, err := common.GenerateRandomKey(48)
	if err != nil {
		return "", err
	}
	code := OpenMosaicSSOAuthorizationCode{
		CodeHash:    openMosaicSSOCodeHash(rawCode),
		UserId:      userID,
		RedirectURI: redirectURI,
		ExpiresAt:   time.Now().Add(ttl),
	}
	if err := tx.Create(&code).Error; err != nil {
		return "", err
	}
	return rawCode, nil
}

func ConsumeOpenMosaicSSOAuthorizationCode(tx *gorm.DB, rawCode string, redirectURI string, now time.Time) (*OpenMosaicSSOClaim, error) {
	if strings.TrimSpace(rawCode) == "" || strings.TrimSpace(redirectURI) == "" {
		return nil, ErrOpenMosaicSSOCodeInvalid
	}
	var claim OpenMosaicSSOClaim
	err := tx.Transaction(func(db *gorm.DB) error {
		var code OpenMosaicSSOAuthorizationCode
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code_hash = ?", openMosaicSSOCodeHash(rawCode)).First(&code).Error; err != nil {
			return ErrOpenMosaicSSOCodeInvalid
		}
		if code.ConsumedAt != nil || !code.ExpiresAt.After(now) || code.RedirectURI != redirectURI {
			return ErrOpenMosaicSSOCodeInvalid
		}
		consumedAt := now
		result := db.Model(&OpenMosaicSSOAuthorizationCode{}).
			Where("id = ? AND consumed_at IS NULL", code.Id).
			Update("consumed_at", consumedAt)
		if result.Error != nil || result.RowsAffected != 1 {
			return ErrOpenMosaicSSOCodeInvalid
		}

		var user User
		if err := db.Where("id = ?", code.UserId).First(&user).Error; err != nil || user.Status != common.UserStatusEnabled {
			return ErrOpenMosaicSSOCodeInvalid
		}
		token, err := EnsureOpenMosaicSSOToken(db, user.Id)
		if err != nil {
			return err
		}
		claim = OpenMosaicSSOClaim{
			UserID:        user.Id,
			Username:      user.Username,
			DisplayName:   user.DisplayName,
			Email:         user.Email,
			EmailVerified: user.Email != "" && (user.EmailVerifiedAt > 0 || strings.EqualFold(strings.TrimSpace(os.Getenv("OPENMOSAIC_SSO_TRUST_LEGACY_EMAILS")), "true")),
			APIKey:        token.Key,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrOpenMosaicSSOCodeInvalid) {
			return nil, ErrOpenMosaicSSOCodeInvalid
		}
		return nil, err
	}
	return &claim, nil
}

func openMosaicSSOCodeHash(rawCode string) string {
	sum := sha256.Sum256([]byte(rawCode))
	return hex.EncodeToString(sum[:])
}
