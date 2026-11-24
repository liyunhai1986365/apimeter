package model

import (
	"context"
	"errors"
	"strconv"
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

func TestColdQuotaReserveOutlivesImageGuard(t *testing.T) {
	for _, kind := range []string{"wallet", "token"} {
		t.Run(kind, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			key, id, table := getUserCacheKey(user.Id), user.Id, "users"
			balance := func() int {
				cached, err := GetUserCache(user.Id)
				require.NoError(t, err)
				return cached.Quota
			}
			reserve := func(n int) (bool, error) { return TryReserveUserQuota(user.Id, n) }
			if kind == "token" {
				key, id, table = getTokenCacheKey(token.Key), token.Id, "tokens"
				balance = func() int {
					cached, err := GetTokenByKey(token.Key, false)
					require.NoError(t, err)
					return cached.RemainQuota
				}
				reserve = func(n int) (bool, error) { return TryReserveTokenQuota(token.Id, token.Key, n, false) }
			}
			server.Del(key)
			imageGuard, err := beginImageQuotaCacheMutation(t.Context(), key, id, kind)
			require.NoError(t, err)
			result, err := imageGuard.reserve(t.Context(), 20, false)
			require.NoError(t, err)
			require.Equal(t, cacheQuotaMiss, result)
			if kind == "token" {
				require.NoError(t, DB.Model(&token).Updates(map[string]any{"remain_quota": 80, "used_quota": 20}).Error)
			} else {
				require.NoError(t, DB.Model(&user).Update("quota", 80).Error)
			}
			// The image has committed, but hasn't released its guard. The next
			// reservation must go to SQL. Interleave another reader before its write.
			ran := false
			callback := "quota-cold-reserve-pause"
			require.NoError(t, DB.Callback().Update().Before("gorm:begin_transaction").Register(callback, func(tx *gorm.DB) {
				if !ran && tx.Statement.Table == table {
					ran = true
					imageGuard.finish(true)
					require.Equal(t, 80, balance())
					require.False(t, server.Exists(key), "the in-flight SQL debit must still fence hydration")
				}
			}))
			t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callback)) })
			ok, err := reserve(30)
			require.NoError(t, err)
			require.True(t, ok)
			require.True(t, ran)
			require.Equal(t, 50, balance())
			ok, err = reserve(51)
			require.NoError(t, err)
			require.False(t, ok)
			if kind == "token" {
				require.Equal(t, 50, getTokenFromDB(t, id).RemainQuota)
			} else {
				require.Equal(t, 50, getUserQuotaFromDB(t, id))
			}
		})
	}
}

type imageQuotaCleanupOutage struct {
	imageQuotaCacheInterleave
	down atomic.Bool
}

func (h *imageQuotaCleanupOutage) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	args := cmd.Args()
	if h.down.Load() && len(args) > 3 && args[0] == "eval" && args[1] == imageQuotaCacheFinishScript {
		return ctx, errors.New("cleanup temporarily unreachable")
	}
	return ctx, nil
}

func failNextImageUpdate(t *testing.T, table string) {
	t.Helper()
	const callback = "test:fail_image_update"
	var failed atomic.Bool
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == table && failed.CompareAndSwap(false, true) {
			tx.AddError(errors.New("injected image reservation update failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callback)) })
}

func TestImageQuotaRecoversFromUnreachableCleanup(t *testing.T) {
	for _, outcome := range []string{"rollback", "commit", "commit with lost receipt"} {
		t.Run(outcome, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			common.SyncFrequency = 600
			common.BatchUpdateEnabled = true
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			ok, err := TryReserveUserQuota(user.Id, 12)
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = TryReserveTokenQuota(token.Id, token.Key, 12, false)
			require.NoError(t, err)
			require.True(t, ok)
			task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 12}
			require.NoError(t, task.Insert())
			outage := &imageQuotaCleanupOutage{}
			outage.down.Store(true)
			common.RDB.AddHook(outage)
			if outcome == "rollback" {
				failNextImageUpdate(t, "tokens")
			}
			err = reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}, 20)
			want := 68
			if outcome == "rollback" {
				require.ErrorIs(t, err, ErrImageTaskTokenReservation)
				// A timeout worker loaded the task before the debit failed.
				// It must reload the rollback receipt before publishing failure.
				task.Status = TaskStatusFailure
				won, updateErr := task.UpdateWithStatus(TaskStatusInProgress)
				require.NoError(t, updateErr)
				require.False(t, won)
				var fresh Task
				require.NoError(t, DB.First(&fresh, task.ID).Error)
				fresh.Status = TaskStatusFailure
				won, updateErr = fresh.UpdateWithStatus(TaskStatusInProgress)
				require.NoError(t, updateErr)
				require.True(t, won)
				want = 88
			} else {
				require.NoError(t, err)
			}
			if outcome == "commit with lost receipt" {
				// A mixed-version worker reconstructs private_data without the
				// new field. Losing a receipt must never refund an actual debit.
				require.NoError(t, DB.Model(&Task{}).Where("id = ?", task.ID).UpdateColumn("private_data", TaskPrivateData{}).Error)
			}
			require.Equal(t, "68", server.HGet(getUserCacheKey(user.Id), "Quota"))
			require.Equal(t, "68", server.HGet(getTokenCacheKey(token.Key), "RemainQuota"))
			// Replace the client: recovery cannot depend on the failed worker.
			oldClient := common.RDB
			common.RDB = redis.NewClient(&redis.Options{Addr: server.Addr()})
			require.NoError(t, oldClient.Close())
			server.FastForward(imageQuotaCacheLease + time.Second)
			balance, err := GetUserQuota(user.Id, false)
			require.NoError(t, err)
			require.Equal(t, want, balance)
			// Direct token reservations must consult receipts on insufficiency
			// even if the caller hasn't read the token cache beforehand.
			ok, err = TryReserveTokenQuota(token.Id, token.Key, 80, false)
			require.NoError(t, err)
			require.Equal(t, outcome == "rollback", ok)
			wantToken := want
			if ok {
				wantToken -= 80
			}
			cached, err := GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			require.Equal(t, wantToken, cached.RemainQuota)
			recoverImageQuota(getUserCacheKey(user.Id), "wallet")
			require.Equal(t, strconv.Itoa(want), server.HGet(getUserCacheKey(user.Id), "Quota"), "retry must not credit twice")
			batchUpdate()
			require.Equal(t, wantToken, getTokenFromDB(t, token.Id).RemainQuota)
			require.Equal(t, want, getUserQuotaFromDB(t, user.Id))
		})
	}
}

func TestUnknownTokenLookupDoesNotCreateCacheKeys(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	for range 3 {
		_, err := GetTokenByKey(common.GetRandomString(48), false)
		require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	}
	require.Empty(t, server.Keys())
	token := createReserveTestToken(t, 100)
	_, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.Equal(t, "100", server.HGet(getTokenCacheKey(token.Key), "RemainQuota"))
	require.Positive(t, server.TTL(quotaCacheEpochKey(getTokenCacheKey(token.Key))))
}

func TestImageQuotaRecoveryBeforeAuthorizationOrNextImage(t *testing.T) {
	for _, entry := range []string{"token validation", "image reservation", "direct token reservation", "direct wallet reservation"} {
		t.Run(entry, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			common.SyncFrequency = 600
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			_, err := GetUserCache(user.Id)
			require.NoError(t, err)
			task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, PrivateData: TaskPrivateData{AsyncImage: true}}
			require.NoError(t, task.Insert())
			outage := &imageQuotaCleanupOutage{}
			outage.down.Store(true)
			common.RDB.AddHook(outage)
			failNextImageUpdate(t, "tokens")
			accounts := ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}
			require.ErrorIs(t, reserveImageTaskTestDelta(&task, accounts, 100), ErrImageTaskTokenReservation)
			require.Equal(t, "0", server.HGet(getTokenCacheKey(token.Key), "RemainQuota"))
			// The failed image finishes before its Redis lease expires.
			var failed Task
			require.NoError(t, DB.First(&failed, task.ID).Error)
			failed.Status = TaskStatusFailure
			won, err := failed.UpdateWithStatus(TaskStatusInProgress)
			require.NoError(t, err)
			require.True(t, won)
			outage.down.Store(false)
			server.FastForward(imageQuotaCacheLease + time.Second)
			// A competing request resolves the hold just after this caller saw
			// the old balance. Both the read and reservation paths must retry.
			for kind, key := range map[string]string{"wallet": getUserCacheKey(user.Id), "token": getTokenCacheKey(token.Key)} {
				hook := &imageQuotaCacheInterleave{script: expiredImageQuotaHoldsScript, key: key, before: func() { recoverImageQuota(key, kind) }}
				common.RDB.AddHook(hook)
				if entry == "token validation" || entry == "direct "+kind+" reservation" {
					t.Cleanup(func() { require.True(t, hook.ran.Load()) })
				}
			}
			switch entry {
			case "token validation":
				valid, err := ValidateUserToken(token.Key)
				require.NoError(t, err)
				require.Equal(t, 100, valid.RemainQuota)
				quota, err := GetUserQuota(user.Id, false)
				require.NoError(t, err)
				require.Equal(t, 100, quota)
			case "direct token reservation":
				ok, err := TryReserveTokenQuota(token.Id, token.Key, 90, false)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 10, getTokenFromDB(t, token.Id).RemainQuota)
			case "direct wallet reservation":
				ok, err := TryReserveUserQuota(user.Id, 90)
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
			case "image reservation":
				next := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, PrivateData: TaskPrivateData{AsyncImage: true}}
				require.NoError(t, next.Insert())
				require.NoError(t, reserveImageTaskTestDelta(&next, accounts, 90))
				quota, err := GetUserQuota(user.Id, false)
				require.NoError(t, err)
				require.Equal(t, 10, quota)
				require.Equal(t, 10, getTokenFromDB(t, token.Id).RemainQuota)
			}
		})
	}
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
	failNextImageUpdate(t, "tokens")
	err = reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}, 30)
	require.ErrorIs(t, err, ErrImageTaskTokenReservation)
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

func TestQuotaCacheEpochExpiryAndLegacyWriters(t *testing.T) {
	server := useUserCacheMiniRedis(t)
	ctx := t.Context()
	token := Token{Id: 17, Key: "epoch-lifetime", RemainQuota: 100}
	key := getTokenCacheKey(token.Key)
	generation := quotaCacheEpochKey(key)
	require.NoError(t, server.Set(generation, "3"))
	first, err := quotaCacheHydrationEpoch(key)
	require.NoError(t, err)
	require.NotEqual(t, "3", first)
	require.Positive(t, server.TTL(generation))
	for _, expire := range []bool{true, false} {
		if expire {
			server.FastForward(imageQuotaCacheLease + time.Millisecond)
		} else {
			server.Del(generation)
		}
		require.False(t, server.Exists(generation))
		for _, stale := range []string{"", "3", first} {
			_, err = cacheInitTokenAtEpoch(token, stale)
			require.NoError(t, err)
			require.False(t, server.Exists(key), "missing generation must reject stale snapshots")
		}
		next, err := quotaCacheHydrationEpoch(key)
		require.NoError(t, err)
		require.NotEqual(t, first, next)
		_, err = cacheInitTokenAtEpoch(token, first)
		require.NoError(t, err)
		require.False(t, server.Exists(key), "a fresh generation cannot authorize the old snapshot")
		_, err = common.RDB.Incr(ctx, generation).Result() // numeric preview writer
		require.NoError(t, err)
		_, err = cacheInitTokenAtEpoch(token, next)
		require.NoError(t, err)
		require.False(t, server.Exists(key))
		first, err = quotaCacheHydrationEpoch(key)
		require.NoError(t, err)
	}
	_, err = cacheInitTokenAtEpoch(token, first)
	require.NoError(t, err)
	require.Equal(t, "100", server.HGet(key, "RemainQuota"))
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
