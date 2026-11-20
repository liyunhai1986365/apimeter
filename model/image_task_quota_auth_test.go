package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type imageQuotaAuthPublishFailure struct {
	imageQuotaCacheInterleave
}

func (h *imageQuotaAuthPublishFailure) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	args := cmd.Args()
	// Fail only the metadata publication after User.Update commits. The
	// authentication fence and quota scripts must still reach Redis.
	if len(args) > 3 && args[0] == "eval" && args[2] == 5 && args[3] == h.key && h.ran.CompareAndSwap(false, true) {
		return ctx, errors.New("temporary auth cache publication failure")
	}
	return ctx, nil
}

func TestImageTaskLeaseCannotReviveDisabledUser(t *testing.T) {
	for _, cacheSeconds := range []int{2, 60} {
		for _, imageFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("cache=%d/image_first=%v", cacheSeconds, imageFirst), func(t *testing.T) {
				truncateTables(t)
				resetBatchUpdateTestState(t)
				server := useUserCacheMiniRedis(t)
				common.SyncFrequency = cacheSeconds
				user := createReserveTestUser(t, 100)
				require.NoError(t, populateUserCache(user))
				task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
				require.NoError(t, task.Insert())
				reserve := func() {
					require.NoError(t, reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{UserID: user.Id, SkipToken: true}, 20))
				}
				if imageFirst {
					reserve()
				}
				hook := &imageQuotaAuthPublishFailure{imageQuotaCacheInterleave: imageQuotaCacheInterleave{key: getUserCacheKey(user.Id)}}
				common.RDB.AddHook(hook)
				user.Status = common.UserStatusDisabled
				require.ErrorContains(t, user.Update(false), "temporary auth cache publication failure")
				require.True(t, hook.ran.Load())
				stored, err := GetUserById(user.Id, false)
				require.NoError(t, err)
				require.Equal(t, common.UserStatusDisabled, stored.Status)
				require.EqualValues(t, 2, stored.AuthVersion)

				elapsed := time.Duration(0)
				if !imageFirst {
					// A previously authenticated worker can reach its extra
					// reservation after the user's disable transaction.
					elapsed = time.Duration(cacheSeconds/2) * time.Second
					server.FastForward(elapsed)
					reserve()
				}
				require.Greater(t, server.TTL(getUserAuthFenceKey(user.Id)), server.TTL(getUserCacheKey(user.Id)))
				quota, err := common.RDB.HGet(t.Context(), getUserCacheKey(user.Id), "Quota").Int()
				require.NoError(t, err)
				require.Equal(t, 80, quota, "auth protection must preserve the reserved balance")

				// Pass the old, fixed-duration fence's expiry. The new fence
				// must still reject the stale enabled hash or that hash must
				// already have expired, in either ordering of the operations.
				server.FastForward(time.Duration(userAuthFenceTTLSeconds()+1)*time.Second - elapsed)
				cached, err := GetUserCache(user.Id)
				require.NoError(t, err)
				require.Equal(t, common.UserStatusDisabled, cached.Status)
				require.EqualValues(t, 2, cached.AuthVersion)
				require.Equal(t, 80, cached.Quota)
			})
		}
	}
}

func TestImageTaskLeaseAuthFenceRollbackStillExpires(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	server := useUserCacheMiniRedis(t)
	user := createReserveTestUser(t, 100)
	common.BatchUpdateEnabled = true
	reserved, err := TryReserveUserQuota(user.Id, 20)
	require.NoError(t, err)
	require.True(t, reserved)
	require.Equal(t, 100, getUserQuotaFromDB(t, user.Id), "the cached balance includes a pending batch debit")

	rollback := errors.New("roll back auth update")
	require.ErrorIs(t, DB.Transaction(func(tx *gorm.DB) error {
		_, err := IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		return rollback
	}), rollback)

	for range 2 {
		guard, err := beginImageQuotaCacheMutation(t.Context(), getUserCacheKey(user.Id), user.Id, "wallet")
		require.NoError(t, err)
		guard.finish(false)
		fenceTTL := server.TTL(getUserAuthFenceKey(user.Id))
		require.Greater(t, fenceTTL, server.TTL(getUserCacheKey(user.Id)))
		// Retrying a fence publication must not reset an extended lifetime
		// back to the configured, shorter authentication TTL.
		require.NoError(t, SetUserAuthVersionFence(user.Id, 2))
		require.GreaterOrEqual(t, server.TTL(getUserAuthFenceKey(user.Id)), fenceTTL)
		quota, err := common.RDB.HGet(t.Context(), getUserCacheKey(user.Id), "Quota").Int()
		require.NoError(t, err)
		require.Equal(t, 80, quota)
		_, err = GetUserCache(user.Id)
		require.ErrorIs(t, err, ErrUserAuthCachePending)
		server.FastForward(30 * time.Second)
	}
	batchUpdate()
	server.FastForward(server.TTL(getUserAuthFenceKey(user.Id)) + time.Second)
	require.False(t, server.Exists(getUserAuthFenceKey(user.Id)))
	require.False(t, server.Exists(getUserCacheKey(user.Id)))
	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	require.Equal(t, common.UserStatusEnabled, cached.Status)
	require.EqualValues(t, 1, cached.AuthVersion)
	require.Equal(t, 80, cached.Quota)
}
