package model

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Test fixtures have no concurrent database read. Production callers must
// capture the hydration epoch before their query and use the AtEpoch helpers.
func cacheInitToken(token Token) (int, error) {
	if !common.RedisEnabled {
		return 0, nil
	}
	epoch, err := quotaCacheHydrationEpoch(getTokenCacheKey(token.Key))
	if err != nil {
		return 0, err
	}
	return cacheInitTokenAtEpoch(token, epoch)
}

func populateUserCache(user User) error {
	return writeUserCache(user.ToBaseUser(), true)
}

func writeUserCache(user *UserBase, includeQuota bool) error {
	if !common.RedisEnabled {
		return nil
	}
	epoch, err := quotaCacheHydrationEpoch(getUserCacheKey(user.Id))
	if err != nil {
		return err
	}
	return writeUserCacheAtEpoch(user, includeQuota, epoch)
}

// Pause one Redis command to run the other request in a reproducible order.
type imageQuotaCacheInterleave struct {
	script, key string
	before      func()
	ran         atomic.Bool
}

func (h *imageQuotaCacheInterleave) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	args := cmd.Args()
	if len(args) > 3 && args[0] == "eval" && args[1] == h.script && args[3] == h.key && h.ran.CompareAndSwap(false, true) {
		h.before()
	}
	return ctx, nil
}

func (*imageQuotaCacheInterleave) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (*imageQuotaCacheInterleave) BeforeProcessPipeline(ctx context.Context, _ []redis.Cmder) (context.Context, error) {
	return ctx, nil
}
func (*imageQuotaCacheInterleave) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestImageTaskWalletHydrationAfterCommitDoesNotDoubleDebit(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
	require.NoError(t, task.Insert())
	hook := &imageQuotaCacheInterleave{script: imageQuotaCacheFinishScript, key: getUserCacheKey(user.Id)}
	hook.before = func() {
		// The database is committed but cache cleanup has not run. A new
		// request can use the balance without publishing it across the guard.
		cached, err := GetUserCache(user.Id)
		require.NoError(t, err)
		require.Equal(t, 80, cached.Quota)
	}
	common.RDB.AddHook(hook)
	require.NoError(t, reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{
		UserID: user.Id, TokenID: token.Id, TokenKey: token.Key,
	}, 20))
	require.True(t, hook.ran.Load())
	batchUpdate()
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, 80, cached.Quota)
	require.Equal(t, 80, getUserQuotaFromDB(t, user.Id))
	reserved, err := TryReserveUserQuota(user.Id, 70)
	require.NoError(t, err)
	require.True(t, reserved, "a real balance of 80 must admit the next reservation of 70")
}

func TestImageTaskRollbackDoesNotCreditRebuiltTokenCache(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	_, err := GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	sub := UserSubscription{UserId: user.Id, AmountTotal: 100, AmountUsed: 90, Status: "active"}
	require.NoError(t, DB.Create(&sub).Error)
	task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
	require.NoError(t, task.Insert())
	hook := &imageQuotaCacheInterleave{script: imageQuotaCacheFinishScript, key: getTokenCacheKey(token.Key)}
	hook.before = func() {
		server.Del(getTokenCacheKey(token.Key))
		loaded, err := GetTokenByKey(token.Key, true)
		require.NoError(t, err)
		require.Equal(t, 100, loaded.RemainQuota)
	}
	common.RDB.AddHook(hook)
	require.ErrorIs(t, reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{
		UserID: user.Id, TokenID: token.Id, TokenKey: token.Key, SubscriptionID: sub.Id,
	}, 20), ErrImageTaskSubscriptionReservation)
	require.True(t, hook.ran.Load())
	cached, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.Equal(t, 100, cached.RemainQuota)
	require.Zero(t, cached.UsedQuota)
	reserved, err := TryReserveTokenQuota(token.Id, token.Key, 110, false)
	require.NoError(t, err)
	require.False(t, reserved, "a rejected image must not create extra token quota")
	require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestImageTaskRollbackPreservesPendingBatchDebits(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	won, err := TryReserveUserQuota(user.Id, 20)
	require.NoError(t, err)
	require.True(t, won)
	won, err = TryReserveTokenQuota(token.Id, token.Key, 20, false)
	require.NoError(t, err)
	require.True(t, won)
	task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 20}
	require.NoError(t, task.Insert())
	require.NoError(t, DB.Exec(`CREATE TRIGGER image_token_outage BEFORE UPDATE ON tokens BEGIN SELECT RAISE(ABORT, 'temporary write failure'); END`).Error)
	err = reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}, 30)
	require.ErrorIs(t, err, ErrImageTaskTokenReservation)
	require.NoError(t, DB.Exec("DROP TRIGGER image_token_outage").Error)
	userCache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	tokenCache, err := cacheGetTokenByKey(token.Key)
	require.NoError(t, err)
	require.Equal(t, 80, userCache.Quota)
	require.Equal(t, 80, tokenCache.RemainQuota)
	require.Equal(t, 20, tokenCache.UsedQuota)
	require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
	batchUpdate()
	require.Equal(t, 80, getUserQuotaFromDB(t, user.Id))
	require.Equal(t, 80, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestImageTaskHydrationRejectsSnapshotsAcrossTransaction(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	userEpoch, err := quotaCacheHydrationEpoch(getUserCacheKey(user.Id))
	require.NoError(t, err)
	tokenEpoch, err := quotaCacheHydrationEpoch(getTokenCacheKey(token.Key))
	require.NoError(t, err)
	// These snapshots were read before the image transaction. Their cache
	// writes are delayed until either during the transaction or after commit.
	staleUser, err := GetUserById(user.Id, false)
	require.NoError(t, err)
	staleToken := getTokenFromDB(t, token.Id)
	task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
	require.NoError(t, task.Insert())
	callback := "image-quota-test-hydration"
	callbackRan := false
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "tokens" || callbackRan {
			return
		}
		callbackRan = true
		server.Del(getTokenCacheKey(token.Key))
		server.Del(getUserCacheKey(user.Id))
		// A concurrent DB reader can start after begin and still see the old
		// committed snapshot. The active guard must reject that fill too.
		_, err := quotaCacheHydrationEpoch(getUserCacheKey(user.Id))
		require.ErrorIs(t, err, errQuotaCacheMutationPending)
		_, err = quotaCacheHydrationEpoch(getTokenCacheKey(token.Key))
		require.ErrorIs(t, err, errQuotaCacheMutationPending)
		currentUserEpoch, err := common.RDB.Get(t.Context(), quotaCacheEpochKey(getUserCacheKey(user.Id))).Result()
		require.NoError(t, err)
		currentTokenEpoch, err := common.RDB.Get(t.Context(), quotaCacheEpochKey(getTokenCacheKey(token.Key))).Result()
		require.NoError(t, err)
		require.NoError(t, writeUserCacheAtEpoch(staleUser.ToBaseUser(), true, currentUserEpoch))
		_, err = cacheInitTokenAtEpoch(staleToken, currentTokenEpoch)
		require.NoError(t, err)
		require.False(t, server.Exists(getUserCacheKey(user.Id)))
		require.False(t, server.Exists(getTokenCacheKey(token.Key)))
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callback)) })
	require.NoError(t, reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}, 20))
	require.True(t, callbackRan)
	// An arbitrarily delayed pre-transaction reader remains rejected after
	// guards are released, even when the new hashes have not yet been created.
	require.NoError(t, writeUserCacheAtEpoch(staleUser.ToBaseUser(), true, userEpoch))
	_, err = cacheInitTokenAtEpoch(staleToken, tokenEpoch)
	require.NoError(t, err)
	require.False(t, server.Exists(getUserCacheKey(user.Id)))
	require.False(t, server.Exists(getTokenCacheKey(token.Key)))
	userCache, err := GetUserCache(user.Id)
	require.NoError(t, err)
	tokenCache, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.Equal(t, 80, userCache.Quota)
	require.Equal(t, 80, tokenCache.RemainQuota)
}

func TestImageTaskCacheGuardOwnershipAndAuthFence(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	key := getUserCacheKey(user.Id)
	first, err := beginImageQuotaCacheMutation(context.Background(), key, user.Id, "wallet")
	require.NoError(t, err)
	second, err := beginImageQuotaCacheMutation(context.Background(), key, user.Id, "wallet")
	require.NoError(t, err)
	first.finish(false)
	_, err = quotaCacheHydrationEpoch(key)
	require.ErrorIs(t, err, errQuotaCacheMutationPending)
	epoch, err := common.RDB.Get(t.Context(), quotaCacheEpochKey(key)).Result()
	require.NoError(t, err)
	require.NoError(t, writeUserCacheAtEpoch(user.ToBaseUser(), true, epoch))
	require.False(t, server.Exists(key), "finishing one transaction must leave another guard active")
	second.finish(false)
	first.finish(false) // A repeated cleanup must advance the epoch, not reuse it.
	require.NoError(t, writeUserCacheAtEpoch(user.ToBaseUser(), true, epoch))
	require.False(t, server.Exists(key))
	third, err := beginImageQuotaCacheMutation(context.Background(), key, user.Id, "wallet")
	require.NoError(t, err)
	t.Cleanup(func() { third.finish(false) })
	require.NoError(t, SetUserAuthVersionFence(user.Id, user.AuthVersion+1))
	require.ErrorIs(t, writeUserCacheAtEpoch(user.ToBaseUser(), true, epoch), ErrUserAuthCachePending,
		"quota coordination must never mask a stricter authentication fence")
}

type imageQuotaCacheFailure struct {
	imageQuotaCacheInterleave
	after bool
	calls atomic.Int32
}

func (h *imageQuotaCacheFailure) matches(cmd redis.Cmder) bool {
	args := cmd.Args()
	return len(args) > 3 && args[0] == "eval" && args[1] == h.script && args[3] == h.key
}

func (h *imageQuotaCacheFailure) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if h.matches(cmd) {
		h.calls.Add(1)
		if !h.after && h.ran.CompareAndSwap(false, true) {
			return ctx, errors.New("cache unavailable before script")
		}
	}
	return ctx, nil
}

func (h *imageQuotaCacheFailure) AfterProcess(_ context.Context, cmd redis.Cmder) error {
	if h.after && h.matches(cmd) && h.ran.CompareAndSwap(false, true) {
		return errors.New("script succeeded but its reply was lost")
	}
	return nil
}

func TestImageTaskCacheFailureLeavesReservationUnchanged(t *testing.T) {
	for _, stage := range []string{"begin_failure", "begin_reply_lost", "reserve_reply_lost", "rollback_reply_lost"} {
		t.Run(stage, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			useUserCacheMiniRedis(t)
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			_, err := GetTokenByKey(token.Key, true)
			require.NoError(t, err)
			sub := UserSubscription{UserId: user.Id, AmountTotal: 100, AmountUsed: 90, Status: "active"}
			require.NoError(t, DB.Create(&sub).Error)
			task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
			require.NoError(t, task.Insert())
			version := task.ImageBase64Version
			script := imageQuotaCacheBeginScript
			if stage == "reserve_reply_lost" {
				script = imageQuotaCacheReserveScript
			} else if stage == "rollback_reply_lost" {
				script = imageQuotaCacheFinishScript
			}
			hook := &imageQuotaCacheFailure{
				imageQuotaCacheInterleave: imageQuotaCacheInterleave{script: script, key: getTokenCacheKey(token.Key)},
				after:                     stage != "begin_failure",
			}
			common.RDB.AddHook(hook)
			require.Error(t, reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{
				UserID: user.Id, TokenID: token.Id, TokenKey: token.Key, SubscriptionID: sub.Id,
			}, 20))
			require.True(t, hook.ran.Load())
			if stage == "rollback_reply_lost" {
				require.GreaterOrEqual(t, hook.calls.Load(), int32(2), "retry must not credit the same marker twice")
			}
			require.NoError(t, DB.First(&task, task.ID).Error)
			require.Equal(t, 10, task.Quota)
			require.Equal(t, version, task.ImageBase64Version)
			require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
			cached, err := GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			require.Equal(t, 100, cached.RemainQuota)
			require.Zero(t, cached.UsedQuota)
			require.NoError(t, DB.First(&sub, sub.Id).Error)
			require.EqualValues(t, 90, sub.AmountUsed)
		})
	}
}

func TestImageTaskHydrationStartedDuringGuardCannotOutliveLease(t *testing.T) {
	for _, kind := range []string{"wallet", "token"} {
		t.Run(kind, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			key, id, table := getUserCacheKey(user.Id), user.Id, "users"
			if kind == "token" {
				key, id, table = getTokenCacheKey(token.Key), token.Id, "tokens"
			}
			guard, err := beginImageQuotaCacheMutation(t.Context(), key, id, kind)
			require.NoError(t, err)
			t.Cleanup(func() { guard.finish(true) })
			callback := "image-quota-test-expired-guard"
			ran := false
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != table || ran {
					return
				}
				ran = true
				// The reader already has the old row. Simulate COMMIT followed
				// by a worker crash, so no finish call advances the epoch.
				if kind == "token" {
					require.NoError(t, DB.Model(&Token{}).Where("id = ?", id).Update("remain_quota", 80).Error)
				} else {
					require.NoError(t, DB.Model(&User{}).Where("id = ?", id).Update("quota", 80).Error)
				}
				server.FastForward(imageQuotaCacheLease + time.Second)
			}))
			t.Cleanup(func() { require.NoError(t, DB.Callback().Query().Remove(callback)) })
			if kind == "token" {
				stale, err := GetTokenByKey(token.Key, true)
				require.NoError(t, err)
				require.Equal(t, 100, stale.RemainQuota)
			} else {
				stale, err := GetUserCache(user.Id)
				require.NoError(t, err)
				require.Equal(t, 100, stale.Quota)
			}
			require.True(t, ran)
			require.False(t, server.Exists(key), "a snapshot read under a guard must never be cached after its lease")
			if kind == "token" {
				fresh, err := GetTokenByKey(token.Key, false)
				require.NoError(t, err)
				require.Equal(t, 80, fresh.RemainQuota)
			} else {
				fresh, err := GetUserCache(user.Id)
				require.NoError(t, err)
				require.Equal(t, 80, fresh.Quota)
			}
		})
	}
}
