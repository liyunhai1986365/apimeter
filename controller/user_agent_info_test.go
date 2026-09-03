package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserAgentInfoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Agent{}, &model.AgentUser{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestDecorateAgentUserInfoUsesBrandSiteNameAndActiveMemberships(t *testing.T) {
	db := setupUserAgentInfoTestDB(t)
	brandedAgent := model.Agent{
		Name:     "Internal Agent Name",
		Slug:     "branded-agent",
		Status:   model.AgentStatusEnabled,
		Branding: `{"site_name":"Public Agent Site"}`,
	}
	fallbackAgent := model.Agent{
		Name:   "Fallback Agent Name",
		Slug:   "fallback-agent",
		Status: model.AgentStatusEnabled,
	}
	require.NoError(t, db.Create(&brandedAgent).Error)
	require.NoError(t, db.Create(&fallbackAgent).Error)

	users := []*model.User{{Id: 101}, {Id: 102}, {Id: 103}}
	require.NoError(t, db.Create(&[]model.AgentUser{
		{AgentId: brandedAgent.Id, UserId: 101, Source: model.AgentUserSourceDomain, Status: model.AgentUserStatusEnabled},
		{AgentId: fallbackAgent.Id, UserId: 102, Source: model.AgentUserSourceInvite, Status: model.AgentUserStatusEnabled},
		{AgentId: brandedAgent.Id, UserId: 103, Source: model.AgentUserSourceDomain, Status: model.AgentUserStatusDisabled},
	}).Error)

	require.NoError(t, decorateAgentUserInfo(users))

	assert.True(t, users[0].IsAgentUser)
	assert.Equal(t, "Public Agent Site", users[0].AgentSiteName)
	assert.True(t, users[1].IsAgentUser)
	assert.Equal(t, "Fallback Agent Name", users[1].AgentSiteName)
	assert.False(t, users[2].IsAgentUser)
}
