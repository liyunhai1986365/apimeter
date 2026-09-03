package captcha

import (
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCaptchaTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalMainDBType := common.MainDatabaseType()
	originalLogDBType := common.LogDatabaseType()
	originalSessionSecret := common.SessionSecret

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AuthFlow{}))
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.SessionSecret = "captcha-test-session-secret"

	t.Cleanup(func() {
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainDBType, originalLogDBType)
		common.SessionSecret = originalSessionSecret
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
}

func challengePosition(t *testing.T, scene string) Position {
	t.Helper()
	var flow model.AuthFlow
	require.NoError(t, model.DB.Where(
		"purpose = ? AND provider = ?",
		model.AuthFlowPurposeCaptchaChallenge,
		scene,
	).Order("id DESC").First(&flow).Error)
	var payload challengePayload
	require.NoError(t, common.UnmarshalJsonStr(flow.Payload, &payload))

	return Position{X: payload.X, Y: payload.Y}
}

func TestCaptchaChallengeAndProofAreBoundAndOneTime(t *testing.T) {
	setupCaptchaTestDB(t)

	challenge, err := Generate(SceneLogin)
	require.NoError(t, err)
	require.NotEmpty(t, challenge.CaptchaKey)
	assert.True(t, strings.HasPrefix(challenge.Image, "data:image/"))
	assert.True(t, strings.HasPrefix(challenge.Tile, "data:image/"))
	assert.Positive(t, challenge.TileX)
	assert.GreaterOrEqual(t, challenge.TileY, 0)
	assert.GreaterOrEqual(t, challenge.TileWidth, 58)
	assert.LessOrEqual(t, challenge.TileWidth, 66)
	assert.Positive(t, challenge.TileHeight)
	assert.Positive(t, challenge.ExpiresAt)
	position := challengePosition(t, SceneLogin)
	assert.Equal(t, challenge.TileY, position.Y)
	assert.Less(t, challenge.TileX, position.X)
	assert.Greater(t, position.X, 0)
	assert.LessOrEqual(t, position.X, ImageWidth-challenge.TileWidth)

	proof, err := Verify(SceneLogin, challenge.CaptchaKey, position)
	require.NoError(t, err)
	require.NotEmpty(t, proof)

	err = ConsumeProof(SceneRegister, proof)
	assert.ErrorIs(t, err, model.ErrAuthFlowInvalid)
	require.NoError(t, ConsumeProof(SceneLogin, proof))
	assert.ErrorIs(t, ConsumeProof(SceneLogin, proof), model.ErrAuthFlowInvalid)

	var count int64
	require.NoError(t, model.DB.Model(&model.AuthFlow{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCaptchaFailedVerificationConsumesChallenge(t *testing.T) {
	setupCaptchaTestDB(t)

	challenge, err := Generate(SceneRegister)
	require.NoError(t, err)
	correctPosition := challengePosition(t, SceneRegister)
	wrongPosition := Position{X: -1000, Y: -1000}

	_, err = Verify(SceneRegister, challenge.CaptchaKey, wrongPosition)
	assert.ErrorIs(t, err, ErrVerificationFailed)
	_, err = Verify(SceneRegister, challenge.CaptchaKey, correctPosition)
	assert.ErrorIs(t, err, ErrInvalidChallenge)
}

func TestCaptchaRejectsInvalidScene(t *testing.T) {
	_, err := Generate("password-reset")
	assert.ErrorIs(t, err, ErrInvalidScene)
	assert.False(t, IsValidScene("password-reset"))
}

func TestSlideCaptchaGenerateConcurrent(t *testing.T) {
	capt, err := getSlideCaptcha()
	require.NoError(t, err)

	const workers = 12
	errors := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			data, generateErr := capt.Generate()
			if generateErr == nil && (data.GetData() == nil || data.GetMasterImage() == nil || data.GetTileImage() == nil) {
				generateErr = ErrInvalidChallenge
			}
			errors <- generateErr
		}()
	}
	group.Wait()
	close(errors)

	for generateErr := range errors {
		require.NoError(t, generateErr)
	}
}
