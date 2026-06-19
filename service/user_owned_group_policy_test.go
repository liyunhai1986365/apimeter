package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserOwnedGroupPolicyTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	model.DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()

	t.Cleanup(func() {
		model.DB = originDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		model.InitColForTest()
	})
}

func TestNormalizeTokenGroupPolicyForUserAllowsOwnedProviderGroup(t *testing.T) {
	setupUserOwnedGroupPolicyTestDB(t)

	group := model.BuildUserOwnedProviderGroup(1001, 2002)
	ratio := 1.0
	channel := model.Channel{
		Id:           2002,
		Type:         1,
		Key:          "sk-user-owned",
		Name:         "User OpenAI",
		Models:       "gpt-4o",
		Group:        group,
		ChannelRatio: &ratio,
		Status:       common.ChannelStatusEnabled,
		Scope:        model.ChannelScopeUserOwned,
		OwnerUserId:  1001,
	}
	require.NoError(t, channel.Insert())

	raw := `{"type":"ordered","groups":["` + group + `"]}`
	tokenGroup, policy, err := NormalizeTokenGroupPolicyForUser(raw, "default", 1001)
	require.NoError(t, err)
	require.Equal(t, group, tokenGroup)
	require.Contains(t, policy, group)

	err = ValidateExplicitTokenGroupForUser(group, "default", 1002)
	require.Error(t, err)
}
