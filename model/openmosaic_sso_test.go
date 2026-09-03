package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOpenMosaicSSOTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalLogDB := LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &OpenMosaicSSOTokenBinding{}, &OpenMosaicSSOAuthorizationCode{}))
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestOpenMosaicSSOReusesDedicatedToken(t *testing.T) {
	setupOpenMosaicSSOTestDB(t)
	user := User{Username: "sso-user", DisplayName: "SSO User", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Email: "user@example.com", EmailVerifiedAt: time.Now().Unix()}
	require.NoError(t, DB.Create(&user).Error)

	first, err := EnsureOpenMosaicSSOToken(DB, user.Id)
	require.NoError(t, err)
	second, err := EnsureOpenMosaicSSOToken(DB, user.Id)
	require.NoError(t, err)

	require.Equal(t, first.Id, second.Id)
	require.Equal(t, first.Key, second.Key)
	require.Equal(t, "OpenMosaic 默认 Key", first.Name)
	require.True(t, first.UnlimitedQuota)
}

func TestOpenMosaicSSORotatesDeletedDedicatedToken(t *testing.T) {
	setupOpenMosaicSSOTestDB(t)
	user := User{Username: "sso-rotate", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(&user).Error)
	first, err := EnsureOpenMosaicSSOToken(DB, user.Id)
	require.NoError(t, err)
	require.NoError(t, DB.Delete(&Token{}, first.Id).Error)

	rotated, err := EnsureOpenMosaicSSOToken(DB, user.Id)
	require.NoError(t, err)
	require.NotEqual(t, first.Id, rotated.Id)
	require.NotEqual(t, first.Key, rotated.Key)

	var binding OpenMosaicSSOTokenBinding
	require.NoError(t, DB.Where("user_id = ?", user.Id).First(&binding).Error)
	require.Equal(t, rotated.Id, binding.TokenId)
}

func TestOpenMosaicSSOAuthorizationCodeIsSingleUse(t *testing.T) {
	setupOpenMosaicSSOTestDB(t)
	user := User{Username: "sso-user", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Email: "verified@example.com", EmailVerifiedAt: time.Now().Unix()}
	require.NoError(t, DB.Create(&user).Error)
	token, err := EnsureOpenMosaicSSOToken(DB, user.Id)
	require.NoError(t, err)

	rawCode, err := CreateOpenMosaicSSOAuthorizationCode(DB, user.Id, "https://openmosaic.example/auth/modelsell/callback", time.Minute)
	require.NoError(t, err)
	claim, err := ConsumeOpenMosaicSSOAuthorizationCode(DB, rawCode, "https://openmosaic.example/auth/modelsell/callback", time.Now())
	require.NoError(t, err)
	require.Equal(t, user.Id, claim.UserID)
	require.Equal(t, token.Key, claim.APIKey)
	require.True(t, claim.EmailVerified)

	_, err = ConsumeOpenMosaicSSOAuthorizationCode(DB, rawCode, "https://openmosaic.example/auth/modelsell/callback", time.Now())
	require.ErrorIs(t, err, ErrOpenMosaicSSOCodeInvalid)
}

func TestOpenMosaicSSORejectsExpiredOrMismatchedCode(t *testing.T) {
	setupOpenMosaicSSOTestDB(t)
	user := User{Username: "sso-user", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(&user).Error)
	_, err := EnsureOpenMosaicSSOToken(DB, user.Id)
	require.NoError(t, err)

	rawCode, err := CreateOpenMosaicSSOAuthorizationCode(DB, user.Id, "https://openmosaic.example/auth/modelsell/callback", time.Minute)
	require.NoError(t, err)
	_, err = ConsumeOpenMosaicSSOAuthorizationCode(DB, rawCode, "https://attacker.example/callback", time.Now())
	require.ErrorIs(t, err, ErrOpenMosaicSSOCodeInvalid)

	expiredCode, err := CreateOpenMosaicSSOAuthorizationCode(DB, user.Id, "https://openmosaic.example/auth/modelsell/callback", -time.Second)
	require.NoError(t, err)
	_, err = ConsumeOpenMosaicSSOAuthorizationCode(DB, expiredCode, "https://openmosaic.example/auth/modelsell/callback", time.Now())
	require.ErrorIs(t, err, ErrOpenMosaicSSOCodeInvalid)
}

func TestOpenMosaicSSOCodeIsBoundToProxySiteOrigin(t *testing.T) {
	setupOpenMosaicSSOTestDB(t)
	user := User{Username: "proxy-sso", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, DB.Create(&user).Error)
	rawCode, err := CreateOpenMosaicSSOAuthorizationCodeForSite(DB, user.Id, "https://openmosaic.example/auth/modelsell/callback", "https://proxy.example", time.Minute)
	require.NoError(t, err)
	_, err = ConsumeOpenMosaicSSOAuthorizationCodeForSite(DB, rawCode, "https://openmosaic.example/auth/modelsell/callback", "https://attacker.example", time.Now())
	require.ErrorIs(t, err, ErrOpenMosaicSSOCodeInvalid)
}
