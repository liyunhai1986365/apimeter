package controller

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tokenAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type tokenPageResponse struct {
	Items []tokenResponseItem `json:"items"`
	Total int                 `json:"total"`
}

type tokenResponseItem struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Key             string `json:"key"`
	Status          int    `json:"status"`
	WorkspaceID     int    `json:"workspace_id"`
	WorkspaceName   string `json:"workspace_name"`
	Group           string `json:"group"`
	GroupPolicy     string `json:"group_policy"`
	CrossGroupRetry bool   `json:"cross_group_retry"`
}

type workspaceResponseItem struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	TokenCount  int64  `json:"token_count"`
}

type tokenKeyResponse struct {
	Key string `json:"key"`
}

type tokenFilterOptionsResponse struct {
	Workspaces []struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		IsDefault bool   `json:"is_default"`
	} `json:"workspaces"`
	Tokens []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		WorkspaceID int    `json:"workspace_id"`
	} `json:"tokens"`
}

type sqliteColumnInfo struct {
	Name string `gorm:"column:name"`
	Type string `gorm:"column:type"`
}

type legacyToken struct {
	Id                 int    `gorm:"primaryKey"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"column:key;type:char(48);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int    `gorm:"default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int     `gorm:"default:0"`
	Group              string  `gorm:"column:group;default:''"`
	CrossGroupRetry    bool
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (legacyToken) TableName() string {
	return "tokens"
}

func openTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColForTest()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func migrateTokenControllerTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.AutoMigrate(&model.Workspace{}, &model.Token{}, &model.Log{}); err != nil {
		t.Fatalf("failed to migrate token table: %v", err)
	}
}

func setupTokenControllerFilterOptionsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openTokenControllerTestDB(t)
	if err := db.AutoMigrate(&model.Workspace{}, &model.Token{}); err != nil {
		t.Fatalf("failed to migrate filter options tables: %v", err)
	}
	return db
}

func setupTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openTokenControllerTestDB(t)
	migrateTokenControllerTestDB(t, db)
	return db
}

func setupAgentTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openTokenControllerTestDB(t)
	if err := db.AutoMigrate(
		&model.Workspace{},
		&model.Token{},
		&model.Log{},
		&model.User{},
		&model.Agent{},
		&model.AgentUser{},
		&model.AgentGroupRatio{},
		&model.AgentUserGroupConfig{},
	); err != nil {
		t.Fatalf("failed to migrate agent token tables: %v", err)
	}
	return db
}

func openTokenControllerExternalDB(t *testing.T, dialect string, dsn string) (*gorm.DB, *bool) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.UsingSQLite = false
	common.UsingMySQL = dialect == "mysql"
	common.UsingPostgreSQL = dialect == "postgres"

	var (
		db  *gorm.DB
		err error
	)
	switch dialect {
	case "mysql":
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	if err != nil {
		t.Fatalf("failed to open %s db: %v", dialect, err)
	}

	model.DB = db
	model.LOG_DB = db

	if db.Migrator().HasTable("tokens") {
		t.Skipf("refusing to run %s migration compatibility test against external database because tokens table already exists", dialect)
	}

	managedTokensTable := new(bool)

	t.Cleanup(func() {
		if *managedTokensTable && db.Migrator().HasTable("tokens") {
			_ = db.Migrator().DropTable("tokens")
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db, managedTokensTable
}

func seedToken(t *testing.T, db *gorm.DB, userID int, name string, rawKey string) *model.Token {
	t.Helper()

	workspace, err := model.EnsureDefaultWorkspace(userID)
	if err != nil {
		t.Fatalf("failed to ensure default workspace: %v", err)
	}
	token := &model.Token{
		UserId:         userID,
		WorkspaceId:    workspace.Id,
		Name:           name,
		Key:            rawKey,
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "default",
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	return token
}

func seedWorkspace(t *testing.T, userID int, name string) *model.Workspace {
	t.Helper()

	workspace := &model.Workspace{
		UserId:      userID,
		Name:        name,
		Description: name + " description",
		Status:      model.WorkspaceStatusEnabled,
		CreatedTime: common.GetTimestamp(),
		UpdatedTime: common.GetTimestamp(),
	}
	if err := model.DB.Create(workspace).Error; err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	return workspace
}

func newAuthenticatedContext(t *testing.T, method string, target string, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, requestBody)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userID)
	return ctx, recorder
}

func decodeAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) tokenAPIResponse {
	t.Helper()

	var response tokenAPIResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode api response: %v", err)
	}
	return response
}

func getSQLiteColumnType(t *testing.T, db *gorm.DB, tableName string, columnName string) string {
	t.Helper()

	var columns []sqliteColumnInfo
	if err := db.Raw("PRAGMA table_info(" + tableName + ")").Scan(&columns).Error; err != nil {
		t.Fatalf("failed to inspect %s schema: %v", tableName, err)
	}

	for _, column := range columns {
		if column.Name == columnName {
			return strings.ToLower(column.Type)
		}
	}

	t.Fatalf("column %s not found in %s schema", columnName, tableName)
	return ""
}

func getTokenKeyColumnType(t *testing.T, db *gorm.DB, dialect string) string {
	t.Helper()

	switch dialect {
	case "sqlite":
		return getSQLiteColumnType(t, db, "tokens", "key")
	case "mysql":
		var columnType string
		if err := db.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Scan(&columnType).Error; err != nil {
			t.Fatalf("failed to inspect mysql token key column: %v", err)
		}
		return strings.ToLower(columnType)
	case "postgres":
		var dataType string
		var maxLength sql.NullInt64
		if err := db.Raw(`SELECT data_type, character_maximum_length
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Row().Scan(&dataType, &maxLength); err != nil {
			t.Fatalf("failed to inspect postgres token key column: %v", err)
		}
		switch strings.ToLower(dataType) {
		case "character varying":
			return fmt.Sprintf("varchar(%d)", maxLength.Int64)
		case "character":
			return fmt.Sprintf("char(%d)", maxLength.Int64)
		default:
			if maxLength.Valid {
				return fmt.Sprintf("%s(%d)", strings.ToLower(dataType), maxLength.Int64)
			}
			return strings.ToLower(dataType)
		}
	default:
		t.Fatalf("unsupported dialect %q", dialect)
		return ""
	}
}

func setTokenTestGroups(t *testing.T) {
	t.Helper()

	requireNoError := func(err error) {
		if err != nil {
			t.Fatalf("failed to set token test groups: %v", err)
		}
	}
	requireNoError(ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"backup":1}`))
	requireNoError(setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
	})
}

func runTokenMigrationCompatibilityTest(t *testing.T, db *gorm.DB, dialect string, managedTokensTable *bool) {
	t.Helper()

	legacyKey := strings.Repeat("a", 48)
	longKey := strings.Repeat("b", 64)

	if err := db.AutoMigrate(&legacyToken{}); err != nil {
		t.Fatalf("failed to create legacy token schema: %v", err)
	}
	if managedTokensTable != nil {
		*managedTokensTable = true
	}
	if err := db.Create(&legacyToken{
		UserId:             7,
		Key:                legacyKey,
		Status:             common.TokenStatusEnabled,
		Name:               "legacy-token",
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        100,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}).Error; err != nil {
		t.Fatalf("failed to seed legacy token row: %v", err)
	}

	if got := getTokenKeyColumnType(t, db, dialect); got != "char(48)" {
		t.Fatalf("expected legacy key column type char(48), got %q", got)
	}

	migrateTokenControllerTestDB(t, db)

	if got := getTokenKeyColumnType(t, db, dialect); got != "varchar(128)" {
		t.Fatalf("expected migrated key column type varchar(128), got %q", got)
	}

	var migratedToken model.Token
	if err := db.First(&migratedToken, "name = ?", "legacy-token").Error; err != nil {
		t.Fatalf("failed to load migrated token row: %v", err)
	}
	if migratedToken.Key != legacyKey {
		t.Fatalf("expected migrated token key %q, got %q", legacyKey, migratedToken.Key)
	}
	if migratedToken.Name != "legacy-token" {
		t.Fatalf("expected migrated token name to be preserved, got %q", migratedToken.Name)
	}

	inserted := model.Token{
		UserId:             8,
		Name:               "long-token",
		Key:                longKey,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        200,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}
	if err := db.Create(&inserted).Error; err != nil {
		t.Fatalf("failed to insert long token after migration: %v", err)
	}

	var fetched model.Token
	if err := db.First(&fetched, "id = ?", inserted.Id).Error; err != nil {
		t.Fatalf("failed to fetch long token after migration: %v", err)
	}
	if fetched.Key != longKey {
		t.Fatalf("expected long token key %q, got %q", longKey, fetched.Key)
	}
}

func TestTokenAutoMigrateUsesVarchar128KeyColumn(t *testing.T) {
	db := setupTokenControllerTestDB(t)

	if got := getTokenKeyColumnType(t, db, "sqlite"); got != "varchar(128)" {
		t.Fatalf("expected key column type varchar(128), got %q", got)
	}
	if got := getSQLiteColumnType(t, db, "tokens", "group_policy"); got != "text" {
		t.Fatalf("expected group_policy column type text, got %q", got)
	}
	if got := getSQLiteColumnType(t, db, "tokens", "workspace_id"); got == "" {
		t.Fatalf("expected workspace_id column to exist")
	}
}

func TestEnsureDefaultWorkspaceBackfillsExistingTokens(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := &model.Token{
		UserId:         1,
		Name:           "legacy-token",
		Key:            "legacyworkspacekey",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "default",
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to create legacy token: %v", err)
	}

	workspace, err := model.EnsureDefaultWorkspace(1)
	if err != nil {
		t.Fatalf("failed to ensure default workspace: %v", err)
	}

	var fetched model.Token
	if err := db.First(&fetched, token.Id).Error; err != nil {
		t.Fatalf("failed to fetch token: %v", err)
	}
	if fetched.WorkspaceId != workspace.Id {
		t.Fatalf("expected token workspace %d, got %d", workspace.Id, fetched.WorkspaceId)
	}
}

func TestGetAllTokensFiltersByWorkspace(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	defaultWorkspace, err := model.EnsureDefaultWorkspace(1)
	if err != nil {
		t.Fatalf("failed to ensure default workspace: %v", err)
	}
	projectWorkspace := seedWorkspace(t, 1, "Project Alpha")
	otherWorkspace := seedWorkspace(t, 2, "Other User")

	defaultToken := seedToken(t, db, 1, "default-token", "defaultworkspacekey")
	projectToken := seedToken(t, db, 1, "project-token", "projectworkspacekey")
	projectToken.WorkspaceId = projectWorkspace.Id
	if err := db.Save(projectToken).Error; err != nil {
		t.Fatalf("failed to move token to project workspace: %v", err)
	}
	otherToken := seedToken(t, db, 2, "other-token", "otherworkspacekey")
	otherToken.WorkspaceId = otherWorkspace.Id
	if err := db.Save(otherToken).Error; err != nil {
		t.Fatalf("failed to move token to other workspace: %v", err)
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/?workspace_id=%d&p=1&size=10", projectWorkspace.Id), nil, 1)
	GetAllTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}
	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode token page response: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("expected one workspace token, got total=%d items=%d", page.Total, len(page.Items))
	}
	if page.Items[0].ID != projectToken.Id {
		t.Fatalf("expected project token %d, got %d", projectToken.Id, page.Items[0].ID)
	}
	if page.Items[0].WorkspaceID != projectWorkspace.Id || page.Items[0].WorkspaceName != "Project Alpha" {
		t.Fatalf("expected workspace metadata for Project Alpha, got id=%d name=%q", page.Items[0].WorkspaceID, page.Items[0].WorkspaceName)
	}
	if page.Items[0].ID == defaultToken.Id || page.Items[0].WorkspaceID == defaultWorkspace.Id {
		t.Fatalf("workspace filter returned default workspace token")
	}
}

func TestGetTokenFilterOptionsDoesNotRequireLogTable(t *testing.T) {
	db := setupTokenControllerFilterOptionsTestDB(t)
	projectWorkspace := seedWorkspace(t, 1, "Project Filter")
	projectToken := seedToken(t, db, 1, "project-filter-token", "projectfiltertoken")
	projectToken.WorkspaceId = projectWorkspace.Id
	if err := db.Save(projectToken).Error; err != nil {
		t.Fatalf("failed to move token to project workspace: %v", err)
	}
	seedToken(t, db, 2, "other-user-token", "otherfiltertoken")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/filter-options", nil, 1)
	GetTokenFilterOptions(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}
	var options tokenFilterOptionsResponse
	if err := common.Unmarshal(response.Data, &options); err != nil {
		t.Fatalf("failed to decode filter options response: %v", err)
	}
	if len(options.Tokens) != 1 {
		t.Fatalf("expected one token option, got %d", len(options.Tokens))
	}
	if options.Tokens[0].Name != "project-filter-token" || options.Tokens[0].WorkspaceID != projectWorkspace.Id {
		t.Fatalf("unexpected token option: %+v", options.Tokens[0])
	}
	if len(options.Workspaces) == 0 {
		t.Fatalf("expected workspace options")
	}
	for _, token := range options.Tokens {
		if token.Name == "other-user-token" {
			t.Fatalf("filter options leaked another user's token")
		}
	}
}

func TestAddTokenPersistsWorkspace(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	workspace := seedWorkspace(t, 1, "Project Beta")
	body := map[string]any{
		"name":                 "workspace-token",
		"workspace_id":         workspace.Id,
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "default",
		"cross_group_retry":    false,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected create response to succeed, got message: %s", response.Message)
	}
	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token create response: %v", err)
	}
	if detail.WorkspaceID != workspace.Id || detail.WorkspaceName != "Project Beta" {
		t.Fatalf("expected created token workspace metadata, got id=%d name=%q", detail.WorkspaceID, detail.WorkspaceName)
	}

	var token model.Token
	if err := db.First(&token, detail.ID).Error; err != nil {
		t.Fatalf("failed to load created token: %v", err)
	}
	if token.WorkspaceId != workspace.Id {
		t.Fatalf("expected stored workspace %d, got %d", workspace.Id, token.WorkspaceId)
	}
}

func TestListWorkspacesIncludesDefaultAndTokenCounts(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	defaultWorkspace, err := model.EnsureDefaultWorkspace(1)
	if err != nil {
		t.Fatalf("failed to ensure default workspace: %v", err)
	}
	projectWorkspace := seedWorkspace(t, 1, "Project Gamma")
	defaultToken := seedToken(t, db, 1, "default-token", "defaulttokenkey000")
	projectToken := seedToken(t, db, 1, "project-token", "projecttokenkey000")
	projectToken.WorkspaceId = projectWorkspace.Id
	if err := db.Save(projectToken).Error; err != nil {
		t.Fatalf("failed to move token to project workspace: %v", err)
	}
	_ = defaultToken

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/workspaces", nil, 1)
	ListWorkspaces(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}
	var workspaces []workspaceResponseItem
	if err := common.Unmarshal(response.Data, &workspaces); err != nil {
		t.Fatalf("failed to decode workspace response: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected two workspaces, got %d", len(workspaces))
	}
	counts := map[int]int64{}
	for _, workspace := range workspaces {
		counts[workspace.ID] = workspace.TokenCount
	}
	if counts[defaultWorkspace.Id] != 1 {
		t.Fatalf("expected default workspace token count 1, got %d", counts[defaultWorkspace.Id])
	}
	if counts[projectWorkspace.Id] != 1 {
		t.Fatalf("expected project workspace token count 1, got %d", counts[projectWorkspace.Id])
	}
}

func TestTokenMigrationFromChar48ToVarchar128(t *testing.T) {
	db := openTokenControllerTestDB(t)
	runTokenMigrationCompatibilityTest(t, db, "sqlite", nil)
}

func TestTokenMigrationFromChar48ToVarchar128MySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run mysql migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "mysql", dsn)
	runTokenMigrationCompatibilityTest(t, db, "mysql", managedTokensTable)
}

func TestTokenMigrationFromChar48ToVarchar128Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "postgres", dsn)
	runTokenMigrationCompatibilityTest(t, db, "postgres", managedTokensTable)
}

func TestGetAllTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "list-token", "abcd1234efgh5678")
	seedToken(t, db, 2, "other-user-token", "zzzz1234yyyy5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/?p=1&size=10", nil, 1)
	GetAllTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode token page response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one token, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("list response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestSearchTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "searchable-token", "ijkl1234mnop5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/search?keyword=searchable-token&p=1&size=10", nil, 1)
	SearchTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode search response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one search result, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked search key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("search response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestGetTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "detail-token", "qrst1234uvwx5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(token.Id), nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token detail response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked detail key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("detail response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestUpdateTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "editable-token", "yzab1234cdef5678")

	body := map[string]any{
		"id":                   token.Id,
		"name":                 "updated-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "default",
		"cross_group_retry":    false,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token update response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked update key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("update response leaked raw token key: %s", recorder.Body.String())
	}
}

func TestAddTokenReturnsCreatedTokenWithFullKey(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	body := map[string]any{
		"name":                 "created-token",
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": true,
		"model_limits":         "gpt-4o,gpt-4o-mini",
		"group":                "vip",
		"cross_group_retry":    false,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected create response to succeed, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token create response: %v", err)
	}
	if detail.ID == 0 {
		t.Fatalf("expected created token id in response")
	}
	if detail.Key == "" || strings.Contains(detail.Key, "*") {
		t.Fatalf("expected full created token key in response, got %q", detail.Key)
	}

	var token model.Token
	if err := db.First(&token, detail.ID).Error; err != nil {
		t.Fatalf("failed to load created token: %v", err)
	}
	if detail.Key != token.GetFullKey() {
		t.Fatalf("expected returned key %q to match stored key %q", detail.Key, token.GetFullKey())
	}
}

func TestAddTokenPersistsOrderedGroupPolicy(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	setTokenTestGroups(t)

	body := map[string]any{
		"name":                 "ordered-token",
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "vip",
		"group_policy":         `{"type":"ordered","groups":["vip","backup"]}`,
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	ctx.Set("group", "default")
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected create response to succeed, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token create response: %v", err)
	}
	if detail.Group != "vip" {
		t.Fatalf("expected legacy group to follow first group policy item, got %q", detail.Group)
	}
	if detail.GroupPolicy != `{"type":"ordered","groups":["vip","backup"]}` {
		t.Fatalf("expected ordered group policy to be returned, got %q", detail.GroupPolicy)
	}
	if !detail.CrossGroupRetry {
		t.Fatalf("expected ordered group policy to keep cross-group retry enabled")
	}

	var token model.Token
	if err := db.First(&token, detail.ID).Error; err != nil {
		t.Fatalf("failed to load created token: %v", err)
	}
	if token.Group != "vip" {
		t.Fatalf("expected stored legacy group vip, got %q", token.Group)
	}
	if token.GroupPolicy != `{"type":"ordered","groups":["vip","backup"]}` {
		t.Fatalf("expected stored ordered group policy, got %q", token.GroupPolicy)
	}
}

func TestAddTokenDefaultsEmptyGroupToAuto(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	setTokenTestGroups(t)

	body := map[string]any{
		"name":                 "auto-default-token",
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "",
		"group_policy":         "",
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	ctx.Set("group", "default")
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected create response to succeed, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token create response: %v", err)
	}
	if detail.Group != service.AutoGroupName {
		t.Fatalf("expected empty group to default to auto, got %q", detail.Group)
	}
	if detail.GroupPolicy != "" {
		t.Fatalf("expected empty group policy to remain empty, got %q", detail.GroupPolicy)
	}

	var token model.Token
	if err := db.First(&token, detail.ID).Error; err != nil {
		t.Fatalf("failed to load created token: %v", err)
	}
	if token.Group != service.AutoGroupName {
		t.Fatalf("expected stored group auto, got %q", token.Group)
	}
}

func TestAddTokenUsesCurrentUserGroupForSpecialUsableGroups(t *testing.T) {
	db := setupTokenControllerTestDB(t)

	requireNoError := func(err error) {
		if err != nil {
			t.Fatalf("failed to configure test groups: %v", err)
		}
	}
	requireNoError(db.AutoMigrate(&model.User{}))
	requireNoError(ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"starter":1,"hidden":1}`))
	requireNoError(setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))

	specialGroups := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup
	previousSpecialGroups := specialGroups.ReadAll()
	specialGroups.Clear()
	specialGroups.Set("starter", map[string]string{
		"+:hidden": "Hidden group for starter users",
	})
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		specialGroups.Clear()
		specialGroups.AddAll(previousSpecialGroups)
	})

	user := &model.User{
		Id:       1,
		Username: "starter-user",
		Password: "test-password",
		Group:    "starter",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleCommonUser,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create starter user: %v", err)
	}

	body := map[string]any{
		"name":                 "hidden-token",
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "hidden",
		"group_policy":         `{"type":"ordered","groups":["hidden"]}`,
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	ctx.Set("group", "default")
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected create response to use current DB user group, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token create response: %v", err)
	}
	if detail.Group != "hidden" {
		t.Fatalf("expected created token group hidden, got %q", detail.Group)
	}
}

func TestAddTokenAllowsVisibleAgentGroupPolicy(t *testing.T) {
	db := setupAgentTokenControllerTestDB(t)

	requireNoError := func(err error) {
		if err != nil {
			t.Fatalf("failed to configure agent token test: %v", err)
		}
	}
	requireNoError(ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1.25}`))
	requireNoError(setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
	})

	requireNoError(db.Create(&model.User{Id: 1, Username: "agent-user", Group: "default", Status: common.UserStatusEnabled}).Error)
	requireNoError(db.Create(&model.Agent{Id: 10, OwnerUserId: 1, Name: "代理站", Slug: "agent-site", Status: model.AgentStatusEnabled, DefaultMarkup: 1}).Error)
	requireNoError(db.Create(&model.AgentUser{AgentId: 10, UserId: 1, Status: model.AgentUserStatusEnabled, Group: "member"}).Error)
	_, err := agentservice.UpsertGroupRatio(10, "vip", "vip", "代理销售规则", 1.5, false)
	requireNoError(err)
	_, err = agentservice.UpsertUserGroupConfig(10, "member", []string{"vip"})
	requireNoError(err)
	groups, err := agentservice.EffectiveGroupMap(10)
	requireNoError(err)
	userGroups, err := agentservice.UserGroupConfigMap(10)
	requireNoError(err)

	body := map[string]any{
		"name":                 "agent-visible-token",
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "vip",
		"group_policy":         `{"type":"ordered","groups":["vip"]}`,
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	common.SetContextKey(ctx, constant.ContextKeyAgentContext, &types.AgentContext{
		AgentID:    10,
		Groups:     groups,
		UserGroups: userGroups,
	})
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected create response to allow visible agent group, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token create response: %v", err)
	}
	if detail.Group != "vip" {
		t.Fatalf("expected created token group vip, got %q", detail.Group)
	}
	if detail.GroupPolicy != `{"type":"ordered","groups":["vip"]}` {
		t.Fatalf("expected stored agent group policy, got %q", detail.GroupPolicy)
	}
}

func TestUpdateTokenPersistsOrderedGroupPolicyOrder(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	setTokenTestGroups(t)
	token := seedToken(t, db, 1, "ordered-edit-token", "edit1234token5678")

	body := map[string]any{
		"id":                   token.Id,
		"name":                 "ordered-updated-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "vip",
		"group_policy":         `{"type":"ordered","groups":["vip","backup"]}`,
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	ctx.Set("group", "default")
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected update response to succeed, got message: %s", response.Message)
	}

	var updated model.Token
	if err := db.First(&updated, token.Id).Error; err != nil {
		t.Fatalf("failed to load updated token: %v", err)
	}
	if updated.Group != "vip" {
		t.Fatalf("expected stored legacy group vip, got %q", updated.Group)
	}
	if updated.GroupPolicy != `{"type":"ordered","groups":["vip","backup"]}` {
		t.Fatalf("expected stored group policy order to be preserved, got %q", updated.GroupPolicy)
	}
}

func TestGetTokenKeyRequiresOwnershipAndReturnsFullKey(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "owned-token", "owner1234token5678")

	authorizedCtx, authorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 1)
	authorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(authorizedCtx)

	authorizedResponse := decodeAPIResponse(t, authorizedRecorder)
	if !authorizedResponse.Success {
		t.Fatalf("expected authorized key fetch to succeed, got message: %s", authorizedResponse.Message)
	}

	var keyData tokenKeyResponse
	if err := common.Unmarshal(authorizedResponse.Data, &keyData); err != nil {
		t.Fatalf("failed to decode token key response: %v", err)
	}
	if keyData.Key != token.GetFullKey() {
		t.Fatalf("expected full key %q, got %q", token.GetFullKey(), keyData.Key)
	}

	unauthorizedCtx, unauthorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 2)
	unauthorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(unauthorizedCtx)

	unauthorizedResponse := decodeAPIResponse(t, unauthorizedRecorder)
	if unauthorizedResponse.Success {
		t.Fatalf("expected unauthorized key fetch to fail")
	}
	if strings.Contains(unauthorizedRecorder.Body.String(), token.Key) {
		t.Fatalf("unauthorized key response leaked raw token key: %s", unauthorizedRecorder.Body.String())
	}
}
