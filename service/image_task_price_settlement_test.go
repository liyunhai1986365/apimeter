package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

func TestImageTaskPricePollingWithIncompleteUsage(t *testing.T) {
	for _, tc := range []struct {
		name                string
		requested, reserved int
		actual, delivered   int
		group, quality      float64
		images              string
	}{
		{name: "single image", requested: 1, reserved: 10000, actual: 10000, delivered: 1, group: 1, quality: 1,
			images: `[{"url":"https://example.invalid/image.png"}]`},
		{name: "partial result with frozen ratios", requested: 3, reserved: 90000, actual: 60000, delivered: 2, group: 2, quality: 1.5,
			images: `[{"url":"https://example.invalid/image.png"},{"b64_json":"aW1hZ2U="},{"url":" "}]`},
		{name: "legacy result without image list", requested: 3, reserved: 90000, actual: 90000, delivered: 3, group: 2, quality: 1.5,
			images: `[]`},
	} {
		for _, funding := range []string{BillingSourceWallet, BillingSourceSubscription} {
			for _, usageCase := range []struct {
				name, field string
				total       int
			}{
				{name: "total only", field: `,"usage":{"total_tokens":258}`, total: 258},
				{name: "omitted"},
				{name: "null", field: `,"usage":null`},
				{name: "empty", field: `,"usage":{}`},
			} {
				t.Run(tc.name+"/"+funding+"/"+usageCase.name, func(t *testing.T) {
					truncate(t)
					const initial = 200000
					walletBefore := initial
					if funding == BillingSourceWallet {
						walletBefore -= tc.reserved
					}
					seedUser(t, 941, walletBefore)
					seedToken(t, 942, 941, "image-task-total-only", initial-tc.reserved)
					seedChannel(t, 943)
					seedSubscription(t, 944, 941, initial, int64(tc.reserved))
					task := makeTask(941, 943, tc.reserved, 942, funding, 944)
					task.Status = model.TaskStatusSubmitted
					task.PrivateData.AsyncImage = true
					bc := task.PrivateData.BillingContext
					bc.PerCallBilling = true
					bc.ModelPrice = .02
					bc.GroupRatio = tc.group
					bc.OtherRatios = map[string]float64{"n": float64(tc.requested), "quality": tc.quality}
					requestBody, err := common.Marshal(map[string]int{"n": tc.requested})
					require.NoError(t, err)
					bc.BillingRequestInput = &billingexpr.RequestInput{Body: requestBody}
					require.NoError(t, model.DB.Create(task).Error)

					// Exercise polling, response normalization and the terminal-state
					// update: this upstream reports no input/output token breakdown.
					body := []byte(`{"state":"succeeded","data":{"images":` + tc.images + `}` + usageCase.field + `}`)
					originalFetch := ConfigurableImageTaskFetcher
					t.Cleanup(func() { ConfigurableImageTaskFetcher = originalFetch })
					ConfigurableImageTaskFetcher = func(_ *model.Channel, _ *model.Task, _, _, _ string) ([]byte, int, bool, error) {
						return body, http.StatusOK, true, nil
					}
					ch := &model.Channel{Id: 943, Type: 1, Key: "dummy"}
					require.NoError(t, updateImageSingleTask(context.Background(), ch, task.TaskID, map[string]*model.Task{task.TaskID: task}))
					// Repeated successful polling must neither refund nor log twice.
					require.NoError(t, updateImageSingleTask(context.Background(), ch, task.TaskID, map[string]*model.Task{task.TaskID: task}))
					require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
					require.Equal(t, tc.actual, task.Quota)
					require.Equal(t, initial-tc.actual, getTokenRemainQuota(t, 942))
					if funding == BillingSourceWallet {
						require.Equal(t, initial-tc.actual, getUserQuota(t, 941))
					} else {
						require.Equal(t, walletBefore, getUserQuota(t, 941))
						require.EqualValues(t, tc.actual, getSubscriptionUsed(t, 944))
					}
					var saved model.Task
					require.NoError(t, model.DB.First(&saved, task.ID).Error)
					require.Equal(t, tc.actual, saved.Quota)
					require.JSONEq(t, string(requestBody), string(saved.PrivateData.BillingContext.BillingRequestInput.Body))
					log := getLastLog(t)
					require.NotNil(t, log)
					require.Equal(t, tc.actual, log.Quota)
					require.Zero(t, log.PromptTokens)
					require.Zero(t, log.CompletionTokens)
					var other map[string]any
					require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
					require.Equal(t, float64(usageCase.total), other["actual_total_tokens"])
					require.Equal(t, float64(tc.actual), other["actual_quota"])
					require.Equal(t, float64(tc.delivered), other["n"])
					require.Equal(t, tc.quality, other["quality"])
					require.EqualValues(t, 1, countLogs(t))
					require.EqualValues(t, tc.actual, getChannelUsedQuota(t, 943))
					usedQuota, requests := getUserUsageAccounting(t, 941)
					require.Equal(t, tc.actual, usedQuota)
					require.Equal(t, 1, requests)
				})
			}
		}
	}
}

func TestImageTaskPriceUsesDeliveredCountAndFrozenRatios(t *testing.T) {
	for _, funding := range []string{BillingSourceWallet, BillingSourceSubscription, BillingSourceUserOwnedProvider} {
		t.Run(funding, func(t *testing.T) {
			truncate(t)
			// $0.02/image * group 2 * quality 1.5 = 30000 quota/image.
			// Three images reserved, only one delivered.
			const initial, reserved, actual = 200000, 90000, 30000
			seedUser(t, 941, initial-reserved)
			seedToken(t, 942, 941, "image-task-price", initial-reserved)
			seedChannel(t, 943)
			seedSubscription(t, 944, 941, initial, reserved)
			task := makeTask(941, 943, reserved, 942, funding, 944)
			if funding == BillingSourceUserOwnedProvider {
				task.Quota = 0
			}
			bc := task.PrivateData.BillingContext
			bc.PerCallBilling = true
			bc.GroupRatio = 2
			bc.OtherRatios = map[string]float64{"n": 3, "quality": 1.5}
			bc.BillingRequestInput = &billingexpr.RequestInput{Body: []byte(`{"n":3}`)}
			task.Data = []byte(`{"data":{"images":[{"url":"https://example.invalid/image.png"},{"url":""}]}}`)
			require.NoError(t, model.DB.Create(task).Error)
			SettleImageTaskUsage(context.Background(), task, &dto.Usage{InputTokens: 1, TotalTokens: 1}, nil)
			want := actual
			switch funding {
			case BillingSourceWallet:
				require.Equal(t, initial-actual, getUserQuota(t, 941))
				require.Equal(t, initial-actual, getTokenRemainQuota(t, 942))
			case BillingSourceSubscription:
				require.EqualValues(t, actual, getSubscriptionUsed(t, 944))
				require.Equal(t, initial-reserved, getUserQuota(t, 941))
				require.Equal(t, initial-actual, getTokenRemainQuota(t, 942))
			case BillingSourceUserOwnedProvider:
				want = 0
				require.Equal(t, initial-reserved, getUserQuota(t, 941))
				require.Equal(t, initial-reserved, getTokenRemainQuota(t, 942))
			}
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.Equal(t, want, saved.Quota)
			require.JSONEq(t, `{"n":3}`, string(saved.PrivateData.BillingContext.BillingRequestInput.Body))
			log := getLastLog(t)
			require.NotNil(t, log)
			require.Equal(t, want, log.Quota)
			if want > 0 {
				var other map[string]any
				require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
				require.Equal(t, float64(reserved), other["pre_consumed_quota"])
				require.Equal(t, float64(actual), other["actual_quota"])
				require.Equal(t, float64(1), other["n"])
				require.Equal(t, 1.5, other["quality"])
			}
		})
	}
}

func TestImageTaskMissingUsageKeepsTokenAndExpressionReservations(t *testing.T) {
	for _, billingMode := range []string{"tokens", "tiered_expr"} {
		for _, usage := range []*dto.Usage{nil, {}} {
			t.Run(billingMode, func(t *testing.T) {
				truncate(t)
				const remaining, reserved = 90000, 10000
				seedUser(t, 941, remaining)
				seedToken(t, 942, 941, "image-missing-usage", remaining)
				seedChannel(t, 943)
				task := makeTask(941, 943, reserved, 942, BillingSourceWallet, 0)
				bc := task.PrivateData.BillingContext
				bc.ModelRatio = 1
				bc.CompletionRatio = 1
				if billingMode == "tiered_expr" {
					bc.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
						BillingMode: "tiered_expr", ExprString: `p + c`,
						GroupRatio: 1, QuotaPerUnit: common.QuotaPerUnit,
					}
				}
				task.Data = []byte(`{"data":{"images":[{"url":"https://example.invalid/image.png"}]}}`)
				require.NoError(t, model.DB.Create(task).Error)
				SettleImageTaskUsage(context.Background(), task, usage, task.Data)
				require.Equal(t, reserved, task.Quota)
				require.Equal(t, remaining, getUserQuota(t, 941))
				require.Equal(t, remaining, getTokenRemainQuota(t, 942))
				require.Zero(t, countLogs(t))
			})
		}
	}
}

func TestImageTaskPriceWithoutUsageKeepsUserOwnedProviderFree(t *testing.T) {
	truncate(t)
	const initial = 200000
	seedUser(t, 941, initial)
	seedToken(t, 942, 941, "image-owned-without-usage", initial)
	seedChannel(t, 943)
	task := makeTask(941, 943, 0, 942, BillingSourceUserOwnedProvider, 0)
	task.PrivateData.BillingContext.PerCallBilling = true
	task.PrivateData.BillingContext.OtherRatios = map[string]float64{"n": 3}
	task.Data = []byte(`{"data":{"images":[{"url":"https://example.invalid/image.png"}]}}`)
	require.NoError(t, model.DB.Create(task).Error)
	SettleImageTaskUsage(context.Background(), task, nil, task.Data)
	require.Zero(t, task.Quota)
	require.Equal(t, initial, getUserQuota(t, 941))
	require.Equal(t, initial, getTokenRemainQuota(t, 942))
	log := getLastLog(t)
	require.NotNil(t, log)
	require.Zero(t, log.Quota)
}
