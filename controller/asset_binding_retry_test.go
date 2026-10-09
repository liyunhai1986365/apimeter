package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetBindingRetryNeverReplaysAcceptedOperation(t *testing.T) {
	for _, operation := range []string{"create", "delete"} {
		for _, exhausted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/exhausted=%t", operation, exhausted), func(t *testing.T) {
				preserveSmartRetryTestConfiguration(t)
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if operation == "delete" {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"Result":{"Id":"retry-asset","TaskId":"retry-task"}}`))
				}))
				defer upstream.Close()
				router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
				seedAssetOwnershipForTest(t, 20, 1, "group", "retry-group")
				groups, err := model.FindAssetBindings(1, "group", "retry-group")
				require.NoError(t, err)
				require.Len(t, groups, 1)
				asset := groups[0]
				asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "retry-asset", "retry-asset", "retry-group"
				task := asset
				task.Kind, task.ID = "task", "retry-task"
				if operation == "delete" {
					require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{asset, task}))
				}
				for _, profile := range []string{"seedance-tgxmaas", "seedance2-service-inference"} {
					require.NoError(t, model.UpsertConfigurableResourceState(&model.ConfigurableResourceState{
						ChannelID: 20, ProfileID: profile, ResourceID: "cached-resource", PreRequestID: "asset_group",
						UserID: 1, TokenID: 1, StateKey: "cache-key", StateValue: "retry-asset",
						Metadata: `{"cached":true}`,
					}))
				}
				var otherStateBefore []model.ConfigurableResourceState
				require.NoError(t, model.DB.Where("profile_id NOT IN ?", []string{"asset-access-v1", "asset-access-lock-v1"}).Order("id").Find(&otherStateBefore).Error)
				attempts := 0
				fail := func(tx *gorm.DB) {
					attempts++
					if exhausted || attempts == 1 {
						tx.AddError(&mysql.MySQLError{Number: 1213, Message: "injected binding deadlock"})
					}
				}
				if operation == "create" {
					require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register("test:binding_retry", func(tx *gorm.DB) {
						state, ok := tx.Statement.Dest.(*model.ConfigurableResourceState)
						if ok && state.ProfileID == "asset-access-v1" && state.ResourceID == "task" {
							fail(tx)
						}
					}))
					defer model.DB.Callback().Create().Remove("test:binding_retry")
				} else {
					require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("test:binding_retry", func(tx *gorm.DB) {
						values, ok := tx.Statement.Dest.(map[string]any)
						if !ok || values["status"] != model.ConfigurableResourceStateStatusInvalid {
							return
						}
						fail(tx)
					}))
					defer model.DB.Callback().Update().Remove("test:binding_retry")
				}
				method, path, body, expectedStatus := http.MethodPost, "/api/assets", `{"GroupId":"retry-group","URL":"https://example.com/a.png"}`, http.StatusOK
				if operation == "delete" {
					method, path, body, expectedStatus = http.MethodDelete, "/api/assets/retry-asset", "", http.StatusNoContent
				}
				if exhausted {
					expectedStatus = http.StatusBadGateway
				}
				response := seedanceCall(router, method, path, body, 1)
				require.Equal(t, expectedStatus, response.Code, response.Body.String())
				require.EqualValues(t, 1, calls.Load(), "local database retries must never repeat an accepted upstream operation")
				expectedAttempts := 2
				if exhausted {
					expectedAttempts = 4
					require.Contains(t, response.Body.String(), "asset_binding_failed")
				}
				require.Equal(t, expectedAttempts, attempts)
				for _, binding := range []model.AssetBinding{asset, task} {
					found, err := model.FindAssetBindings(1, binding.Kind, binding.ID)
					require.NoError(t, err)
					if (operation == "create" && !exhausted) || (operation == "delete" && exhausted) {
						require.Len(t, found, 1, "both aliases must remain together")
					} else {
						require.Empty(t, found, "no partial registration or revocation")
					}
				}
				remainingGroups, err := model.FindAssetBindings(1, "group", "retry-group")
				require.NoError(t, err)
				require.Equal(t, groups, remainingGroups)
				var otherStateAfter []model.ConfigurableResourceState
				require.NoError(t, model.DB.Where("profile_id NOT IN ?", []string{"asset-access-v1", "asset-access-lock-v1"}).Order("id").Find(&otherStateAfter).Error)
				require.Equal(t, otherStateBefore, otherStateAfter, "other profiles sharing the state table must be unchanged")
			})
		}
	}
}
