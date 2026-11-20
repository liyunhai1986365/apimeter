package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const (
	imageQuotaTransactionTimeout = 30 * time.Second
	// The lease outlives the transaction's database context, including when a
	// worker exits without releasing it. Epochs remain after the lease expires
	// so a reader holding an older DB snapshot still cannot populate the cache.
	imageQuotaCacheLease = 2 * time.Minute
)

func quotaCacheEpochKey(cacheKey string) string   { return "quota:epoch:" + cacheKey }
func quotaCachePendingKey(cacheKey string) string { return "quota:pending:" + cacheKey }

var errQuotaCacheMutationPending = errors.New("quota cache mutation is pending")

// Capture before reading the database, never after obtaining the snapshot.
// An unavailable epoch disables hydration; the caller can still serve the DB.
func quotaCacheHydrationEpoch(cacheKey string) (string, error) {
	// Readers starting during a transaction must remain ineligible to fill
	// the cache even if a worker crashes after COMMIT and its guard expires
	// before their delayed snapshot is published.
	const script = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
if redis.call('ZCOUNT', KEYS[2], now, '+inf') > 0 then
  return false
end
return redis.call('GET', KEYS[1]) or ''`
	epoch, err := common.RDB.Eval(context.Background(), script,
		[]string{quotaCacheEpochKey(cacheKey), quotaCachePendingKey(cacheKey)}).Text()
	if err == redis.Nil {
		return "", errQuotaCacheMutationPending
	}
	return epoch, err
}

// Shared by the user/token hydration scripts. Their last two keys are the
// epoch and pending-operation set, and their last argument is the epoch read
// before the database snapshot. Return 2 means no cache publication is needed.
const quotaCacheHydrationGuardScript = `
local epoch = redis.call('GET', KEYS[#KEYS - 1]) or ''
if epoch ~= ARGV[#ARGV] then
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
}

const imageQuotaCacheBeginScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', now)
redis.call('ZADD', KEYS[3], now + tonumber(ARGV[2]), ARGV[1])
redis.call('PEXPIRE', KEYS[3], ARGV[2])
redis.call('INCR', KEYS[2])
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
	keys := m.keys()
	if kind == "wallet" {
		// The wallet hash also holds authentication state. Extend a pending
		// auth fence before extending that hash so stale permissions cannot
		// outlive their protection, including after an auth publication fails.
		keys = append(keys, getUserAuthFenceKey(id))
	}
	err := common.RDB.Eval(ctx, imageQuotaCacheBeginScript, keys,
		m.owner, imageQuotaCacheLease.Milliseconds(), int64(userAuthFenceGraceSeconds())*1000).Err()
	return m, err
}

func (m *imageQuotaCacheMutation) keys() []string {
	return []string{m.cacheKey, quotaCacheEpochKey(m.cacheKey), quotaCachePendingKey(m.cacheKey)}
}

const imageQuotaCacheReserveScript = `
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
  redis.call('HINCRBY', KEYS[1], 'RemainQuota', -tonumber(ARGV[4]))
  redis.call('HINCRBY', KEYS[1], 'UsedQuota', ARGV[4])
  redis.call('HSET', KEYS[1], 'AccessedTime', ARGV[6])
else
  if tonumber(redis.call('HGET', KEYS[1], 'CacheSchema') or '0') ~= tonumber(ARGV[7])
    or redis.call('HEXISTS', KEYS[1], 'Quota') == 0 then
    return -1
  end
  redis.call('HINCRBY', KEYS[1], 'Quota', -tonumber(ARGV[4]))
end
redis.call('HSET', KEYS[1], field, ARGV[4])
return 1`

func (m *imageQuotaCacheMutation) reserve(ctx context.Context, delta int, unlimited bool) (cacheQuotaResult, error) {
	allowDebt := "0"
	if unlimited {
		allowDebt = "1"
	}
	result, err := common.RDB.Eval(ctx, imageQuotaCacheReserveScript, m.keys(),
		m.owner, m.id, m.kind, delta, allowDebt, common.GetTimestamp(), userCacheSchemaVersion).Int()
	if err == nil && result == -2 {
		return cacheQuotaMiss, fmt.Errorf("image task quota cache lease expired")
	}
	return quotaResultFromLua(result, err)
}

const imageQuotaCacheFinishScript = `
local field = '__image_reservation:' .. ARGV[1]
local amount = tonumber(redis.call('HGET', KEYS[1], field) or '0')
if amount > 0 and ARGV[3] == 'rollback' then
  if ARGV[2] == 'token' then
    redis.call('HINCRBY', KEYS[1], 'RemainQuota', amount)
    redis.call('HINCRBY', KEYS[1], 'UsedQuota', -amount)
  else
    redis.call('HINCRBY', KEYS[1], 'Quota', amount)
  end
end
redis.call('HDEL', KEYS[1], field)
redis.call('INCR', KEYS[2])
redis.call('ZREM', KEYS[3], ARGV[1])
return 1`

// Cleanup is idempotent, including when Redis applied a script but its reply
// was lost. A rebuilt hash has no operation marker and must never be credited.
func (m *imageQuotaCacheMutation) finish(committed bool) {
	if m == nil {
		return
	}
	outcome := "rollback"
	if committed {
		outcome = "commit"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = common.RDB.Eval(ctx, imageQuotaCacheFinishScript, m.keys(), m.owner, m.kind, outcome).Err()
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			break
		}
	}
	common.SysError(fmt.Sprintf("failed to finish image task %s cache reservation: %v", m.kind, err))
}
