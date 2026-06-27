package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateDBHandlesLegacySQLiteDecimalAgentTables(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
	})

	db, err := gorm.Open(sqlite.Open("file:legacy_agent_decimal?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	require.NoError(t, db.Exec("CREATE TABLE `agents` (`id` integer,`owner_user_id` integer,`name` varchar(64),`slug` varchar(64),`status` integer,`price_mode` varchar(32) DEFAULT \"multiplier\",`default_markup` decimal(10,6) DEFAULT 1.000000,`min_withdraw_amount` integer DEFAULT 0,`withdraw_fee_rate` decimal(10,6) DEFAULT 0.000000,`settlement_currency` varchar(16) DEFAULT \"USD\",`branding` text,`created_at` integer,`updated_at` integer,PRIMARY KEY (`id`))").Error)
	require.NoError(t, db.Exec("CREATE TABLE `agent_pricing_rules` (`id` integer,`agent_id` integer,`model_pattern` varchar(128),`markup` decimal(10,6) DEFAULT 1.000000,`enabled` numeric,`created_at` integer,`updated_at` integer,PRIMARY KEY (`id`))").Error)
	require.NoError(t, db.Exec("CREATE TABLE `agent_group_ratios` (`id` integer,`agent_id` integer,`group_name` varchar(64),`ratio` decimal(10,6) DEFAULT 1.000000,`created_at` integer,`updated_at` integer,PRIMARY KEY (`id`))").Error)
	require.NoError(t, db.Exec("CREATE TABLE `agent_user_group_configs` (`id` integer,`agent_id` integer,`group_name` varchar(64),`visible_groups` text,`created_at` integer,`updated_at` integer,PRIMARY KEY (`id`))").Error)
	require.NoError(t, db.Exec("CREATE TABLE `agent_withdrawals` (`id` integer,`agent_id` integer,`amount_quota` integer DEFAULT 0,`amount_money` decimal(18,6) DEFAULT 0.000000,`fee` decimal(18,6) DEFAULT 0.000000,`status` varchar(32) DEFAULT \"pending\",`account_info` text,`admin_remark` varchar(255),`created_at` integer,`processed_at` integer DEFAULT 0,PRIMARY KEY (`id`))").Error)

	require.NoError(t, migrateDB())

	for _, table := range []string{"agents", "agent_pricing_rules", "agent_group_ratios", "agent_withdrawals"} {
		var createSQL string
		require.NoError(t, db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&createSQL).Error)
		require.NotContains(t, strings.ToLower(createSQL), "decimal(")
	}

	require.True(t, db.Migrator().HasColumn(&AgentUserGroupConfig{}, "group_ratios"))
}
