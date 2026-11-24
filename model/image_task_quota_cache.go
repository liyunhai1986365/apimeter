package model

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

const (
	imageQuotaTransactionTimeout = 30 * time.Second
	// The lease outlives the transaction's database context, including when a
	// worker exits without releasing it.
	imageQuotaCacheLease = 2 * time.Minute
)

// Decimal strings remain compatible with readers using GET and writers using
// INCR during rollout. Leave headroom for those writers and avoid small legacy
// counters when initializing an expired or evicted generation.
func newQuotaCacheEpoch() string {
	return strconv.FormatUint((rand.Uint64()>>3)|(1<<61), 10)
}

func quotaCacheEpochKey(cacheKey string) string   { return "quota:epoch:" + cacheKey }
func quotaCachePendingKey(cacheKey string) string { return "quota:pending:" + cacheKey }

var errQuotaCacheMutationPending = errors.New("quota cache mutation is pending")

// Capture before reading the database, never after obtaining the snapshot.
// An unavailable epoch disables hydration; the caller can still serve the DB.
func quotaCacheHydrationEpoch(cacheKey string) (string, error) {
	return quotaCacheEpoch(cacheKey, newQuotaCacheEpoch())
}

// A blank candidate only observes the epoch. A public token lookup must not
// allocate a key before the database confirms that the token exists.
func quotaCacheEpoch(cacheKey, candidate string) (string, error) {
	// Readers starting during a transaction must remain ineligible to fill
	// the cache even if a worker crashes after COMMIT and its guard expires
	// before their delayed snapshot is published.
	const script = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
if redis.call('ZCOUNT', KEYS[2], now, '+inf') > 0 then
  return false
end
local epoch = redis.call('GET', KEYS[1])
if not epoch or redis.call('PTTL', KEYS[1]) == -1 then
  if ARGV[1] == '' then return '' end
  epoch = ARGV[1]
  redis.call('SET', KEYS[1], epoch, 'PX', ARGV[2])
end
return epoch`
	epoch, err := common.RDB.Eval(context.Background(), script,
		[]string{quotaCacheEpochKey(cacheKey), quotaCachePendingKey(cacheKey)}, candidate, imageQuotaCacheLease.Milliseconds()).Text()
	if err == redis.Nil {
		return "", errQuotaCacheMutationPending
	}
	return epoch, err
}

// Shared by the user/token hydration scripts. Their last two keys are the
// epoch and pending-operation set, and their last argument is the epoch read
// before the database snapshot. Return 2 means no cache publication is needed.
const quotaCacheHydrationGuardScript = `
local epoch = redis.call('GET', KEYS[#KEYS - 1])
if not epoch or epoch ~= ARGV[#ARGV] then
  return 2
end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
if redis.call('ZCOUNT', KEYS[#KEYS], now, '+inf') > 0 then
  return 2
end
`

type imageQuotaCacheMutation struct {
	cacheKey string
	id       int
	kind     string
	owner    string
	receipt  *imageQuotaReceipt
}

const imageQuotaCacheBeginScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', now)
redis.call('ZADD', KEYS[3], now + tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[3], ARGV[2])
redis.call('SET', KEYS[2], ARGV[4], 'PX', ARGV[2])
local ttl = redis.call('PTTL', KEYS[1])
if ttl >= 0 and ttl < tonumber(ARGV[2]) then
  if #KEYS == 4 then
    local fenceTTL = redis.call('PTTL', KEYS[4])
    local protectedTTL = tonumber(ARGV[2]) + tonumber(ARGV[3])
    if fenceTTL >= 0 and fenceTTL < protectedTTL then
      redis.call('PEXPIRE', KEYS[4], protectedTTL)
    end
  end
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 1`

func beginImageQuotaCacheMutation(ctx context.Context, cacheKey string, id int, kind string) (*imageQuotaCacheMutation, error) {
	m := &imageQuotaCacheMutation{cacheKey: cacheKey, id: id, kind: kind, owner: common.GetUUID()}
	if receipt, ok := ctx.Value(imageQuotaReceiptKey{}).(*imageQuotaReceipt); ok {
		m.receipt, m.owner = receipt, receipt.owner
	}
	keys := m.keys()
	if kind == "wallet" {
		// The wallet hash also holds authentication state. Extend a pending
		// auth fence before extending that hash so stale permissions cannot
		// outlive their protection, including after an auth publication fails.
		keys = append(keys, getUserAuthFenceKey(id))
	}
	err := common.RDB.Eval(ctx, imageQuotaCacheBeginScript, keys,
		m.owner, imageQuotaCacheLease.Milliseconds(), int64(userAuthFenceGraceSeconds())*1000, newQuotaCacheEpoch()).Err()
	return m, err
}

func (m *imageQuotaCacheMutation) keys() []string {
	return []string{m.cacheKey, quotaCacheEpochKey(m.cacheKey), quotaCachePendingKey(m.cacheKey)}
}

const imageQuotaCacheReserveScript = walletQuotaSchemaScript + `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
if tonumber(redis.call('ZSCORE', KEYS[3], ARGV[1]) or '0') <= now then
  return -2
end
if tonumber(redis.call('HGET', KEYS[1], 'Id') or '0') ~= tonumber(ARGV[2]) then
  return -1
end
local field = '__image_reservation:' .. ARGV[1]
if redis.call('HEXISTS', KEYS[1], field) == 1 then
  return 1
end
if ARGV[3] == 'token' then
  local remain = tonumber(redis.call('HGET', KEYS[1], 'RemainQuota'))
  if remain == nil or redis.call('HEXISTS', KEYS[1], 'UsedQuota') == 0 then
    return -1
  end
  if ARGV[5] == '0' and remain < tonumber(ARGV[4]) then
    return 0
  end
  if tonumber(ARGV[4]) > 0 then
    redis.call('HINCRBY', KEYS[1], 'RemainQuota', -tonumber(ARGV[4]))
    redis.call('HINCRBY', KEYS[1], 'UsedQuota', ARGV[4])
    redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[6])
  end
else
  if not hasWalletQuotaSchema(KEYS[1], tonumber(ARGV[7]))
    or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
    return -1
  end
  if tonumber(ARGV[4]) > 0 then redis.call('HINCRBY', KEYS[1], 'Quota', -tonumber(ARGV[4])) end
end
redis.call('HSET', KEYS[1], field, ARGV[4])
if ARGV[8] ~= '0' then
  redis.call('HSET', KEYS[1], '__image_task:' .. ARGV[1], ARGV[8])
  redis.call('HINCRBY', KEYS[1], 'ImageQuotaPending', 1)
end
return 1`

func (m *imageQuotaCacheMutation) reserve(ctx context.Context, delta int, unlimited bool) (cacheQuotaResult, error) {
	allowDebt := "0"
	if unlimited {
		allowDebt = "1"
	}
	task := "0"
	if m.receipt != nil {
		task = fmt.Sprintf("%d:%d", m.receipt.taskID, m.receipt.baseVersion)
	}
	result, err := common.RDB.Eval(ctx, imageQuotaCacheReserveScript, m.keys(),
		m.owner, m.id, m.kind, delta, allowDebt, common.GetTimestamp(), userCacheSchemaVersion, task).Int()
	if err == nil && result == -2 {
		return cacheQuotaMiss, fmt.Errorf("image task quota cache lease expired")
	}
	return quotaResultFromLua(result, err)
}

const imageQuotaCacheFinishScript = `
local field = '__image_reservation:' .. ARGV[1]
local amount = tonumber(redis.call('HGET', KEYS[1], field) or '0')
-- A debit is held before commit; a refund becomes spendable only after commit.
if (amount > 0 and ARGV[3] == 'rollback') or (amount < 0 and ARGV[3] == 'commit') then
  amount = math.abs(amount)
  if ARGV[2] == 'token' then
    redis.call('HINCRBY', KEYS[1], 'RemainQuota', amount)
    redis.call('HINCRBY', KEYS[1], 'UsedQuota', -amount)
  else
    redis.call('HINCRBY', KEYS[1], 'Quota', amount)
  end
end
redis.call('HDEL', KEYS[1], field)
if redis.call('HDEL', KEYS[1], '__image_task:' .. ARGV[1]) == 1 then
  if redis.call('HINCRBY', KEYS[1], 'ImageQuotaPending', -1) == 0 then
    redis.call('HDEL', KEYS[1], 'ImageQuotaPending')
  end
end
redis.call('SET', KEYS[2], ARGV[4], 'PX', ARGV[5])
redis.call('ZREM', KEYS[3], ARGV[1])
return 1`

// Cleanup is idempotent, including when Redis applied a script but its reply
// was lost. A rebuilt hash has no operation marker and must never be credited.
func (m *imageQuotaCacheMutation) finish(committed bool) bool {
	if m == nil {
		return true
	}
	outcome := "rollback"
	if committed {
		outcome = "commit"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = common.RDB.Eval(ctx, imageQuotaCacheFinishScript, m.keys(), m.owner, m.kind, outcome, newQuotaCacheEpoch(), imageQuotaCacheLease.Milliseconds()).Err()
		if err == nil {
			return true
		}
		if ctx.Err() != nil {
			break
		}
	}
	common.SysError(fmt.Sprintf("failed to finish image task %s cache reservation: %v", m.kind, err))
	return false
}

const expiredImageQuotaHoldsScript = `
if redis.call('HEXISTS', KEYS[1], 'ImageQuotaPending') == 0 then return {} end
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local result = {}
for _, field in ipairs(redis.call('HKEYS', KEYS[1])) do
  if string.sub(field, 1, 13) == '__image_task:' then
    local owner = string.sub(field, 14)
    if tonumber(redis.call('ZSCORE', KEYS[2], owner) or '0') <= now then
      table.insert(result, owner)
      table.insert(result, redis.call('HGET', KEYS[1], field))
    end
  end
end
return result`

// The task receipt and debit share a SQL transaction. After looking up expired
// holds, callers must re-read the balance: another request may have finished
// recovery between their original Redis read and this one.
func recoverImageQuota(key, kind string) {
	ctx, cancel := context.WithTimeout(DB.Statement.Context, imageQuotaTransactionTimeout)
	defer cancel()
	entries, err := common.RDB.Eval(ctx, expiredImageQuotaHoldsScript, []string{key, quotaCachePendingKey(key)}).StringSlice()
	if err != nil {
		return
	}
	for i := 0; i+1 < len(entries); i += 2 {
		owner := entries[i]
		taskID, version, ok := strings.Cut(entries[i+1], ":")
		id, idErr := strconv.ParseInt(taskID, 10, 64)
		base, verErr := strconv.ParseInt(version, 10, 64)
		if !ok || idErr != nil || verErr != nil || id <= 0 {
			continue
		}
		r := imageQuotaReceipt{owner: owner, taskID: id, baseVersion: base}
		committed, known := r.outcome(ctx)
		if !known {
			continue
		}
		m := imageQuotaCacheMutation{cacheKey: key, owner: owner, kind: kind}
		m.finish(committed)
	}
}

// A locking read on another transaction waits for the original task CAS to
// finish. A lost acknowledgement for this read-only transaction cannot undo
// the observation. If a later writer erased the receipt, never infer a refund.
func (r *imageQuotaReceipt) outcome(ctx context.Context) (committed, known bool) {
	_ = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := lockForUpdate(tx).Select("private_data", "image_base64_version").First(&task, r.taskID).Error; err != nil {
			return err
		}
		ids := task.PrivateData.ImageQuotaReceipts
		committed = ids != nil && slices.Contains(*ids, r.owner)
		known = committed || task.ImageBase64Version == r.baseVersion || (ids != nil && slices.Contains(*ids, "-"+r.owner))
		return nil
	})
	return
}

// Preserve a known rollback only if Redis cleanup failed. Subsequent terminal
// task writes can advance the version, making absence of a commit ambiguous.
func (r *imageQuotaReceipt) recordRollback() {
	ctx, cancel := context.WithTimeout(context.Background(), imageQuotaTransactionTimeout)
	defer cancel()
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := lockForUpdate(tx).Select("private_data", "image_base64_version").First(&task, r.taskID).Error; err != nil {
			return err
		}
		var ids []string
		if task.PrivateData.ImageQuotaReceipts != nil {
			ids = *task.PrivateData.ImageQuotaReceipts
		}
		ids = append(ids, "-"+r.owner)
		task.PrivateData.ImageQuotaReceipts = &ids
		// An in-flight timeout may hold the old private_data. Invalidate
		// that snapshot with the same version used by terminal task writes.
		return tx.Model(&Task{}).Where("id = ? AND image_base64_version = ?", r.taskID, task.ImageBase64Version).
			UpdateColumns(map[string]any{"private_data": task.PrivateData, "image_base64_version": task.ImageBase64Version + 1}).Error
	})
	if err != nil {
		common.SysError(fmt.Sprintf("could not record image task quota rollback: %v", err))
	}
}
