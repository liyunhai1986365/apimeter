package model

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func getTokenCacheKey(key string) string {
	return fmt.Sprintf("token:%s", common.GenerateHMAC(key))
}

func getTokenCacheFenceKey(key string) string {
	return fmt.Sprintf("token:fence:%s", common.GenerateHMAC(key))
}

func tokenCacheTTLSeconds() int {
	ttl := common.RedisKeyCacheSeconds()
	if ttl <= 0 {
		return 60
	}
	return ttl
}

// tokenCacheFenceSeconds must outlive a token mutation's database write plus
// any in-flight reader's DB-read-to-cache-init gap. The fence is not deleted
// after commit; it expires naturally so a reader holding a pre-mutation
// snapshot cannot publish it right after the mutation cleared the cache.
// While the fence exists readers simply serve the database without caching.
const tokenCacheFenceSeconds = 10

const tokenRoutingCacheVersion = "1"

// invalidateTokenCacheForMutation is called before a token metadata mutation
// writes to the database: it raises the fence and drops the cached hash so no
// reader can act on (or re-publish) the pre-mutation state.
func invalidateTokenCacheForMutation(key string) error {
	if !common.RedisEnabled || key == "" {
		return nil
	}
	ctx := context.Background()
	err := common.RDB.Set(ctx, getTokenCacheFenceKey(key), 1, time.Duration(tokenCacheFenceSeconds)*time.Second).Err()
	if err != nil {
		return err
	}
	return common.RDB.Del(ctx, getTokenCacheKey(key)).Err()
}

// Capture epoch before reading the database. Existing hashes may have pending
// quota debits. Upgrade only routing metadata on a matching token.
func cacheInitTokenAtEpoch(token Token, epoch string) (int, error) {
	allowIps := ""
	if token.AllowIps != nil {
		allowIps = *token.AllowIps
	}
	const script = quotaCacheHydrationGuardScript + `
if redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
local routingVersion = '` + tokenRoutingCacheVersion + `'
if redis.call('EXISTS', KEYS[1]) == 1 then
  if redis.call('HGET', KEYS[1], 'Id') == ARGV[1] and
     (redis.call('HGET', KEYS[1], 'RoutingVersion') ~= routingVersion or
      redis.call('HEXISTS', KEYS[1], 'GroupPolicy') == 0) then
    redis.call('HSET', KEYS[1], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
      'AutoGroups', ARGV[14], 'GroupPolicy', ARGV[17], 'RoutingVersion', routingVersion)
  end
  redis.call('EXPIRE', KEYS[1], ARGV[18])
  return 2
end
redis.call('HSET', KEYS[1],
  'Id', ARGV[1], 'UserId', ARGV[2], 'Status', ARGV[3], 'Name', ARGV[4],
  'CreatedTime', ARGV[5], 'AccessedTime', ARGV[6], 'ExpiredTime', ARGV[7],
  'UnlimitedQuota', ARGV[8], 'ModelLimitsEnabled', ARGV[9], 'ModelLimits', ARGV[10],
  'AllowIps', ARGV[11], 'Group', ARGV[12], 'CrossGroupRetry', ARGV[13],
  'AutoGroups', ARGV[14], 'RemainQuota', ARGV[15], 'UsedQuota', ARGV[16],
  'GroupPolicy', ARGV[17], 'RoutingVersion', routingVersion)
redis.call('EXPIRE', KEYS[1], ARGV[18])
return 1`

	return common.RDB.Eval(context.Background(), script, []string{
		getTokenCacheKey(token.Key), getTokenCacheFenceKey(token.Key),
		quotaCacheEpochKey(getTokenCacheKey(token.Key)), quotaCachePendingKey(getTokenCacheKey(token.Key)),
	},
		token.Id, token.UserId, token.Status, token.Name,
		token.CreatedTime, token.AccessedTime, token.ExpiredTime,
		strconv.FormatBool(token.UnlimitedQuota), strconv.FormatBool(token.ModelLimitsEnabled),
		token.ModelLimits, allowIps, token.Group, strconv.FormatBool(token.CrossGroupRetry),
		token.AutoGroups, token.RemainQuota, token.UsedQuota,
		token.GroupPolicy, tokenCacheTTLSeconds(), epoch,
	).Int()
}

// cacheGetTokenByKey 从缓存读取 token；不完整的哈希（如仅有配额字段）会被拒绝。
func cacheGetTokenByKey(key string) (*Token, error) {
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	cacheKey := getTokenCacheKey(key)
	fields, err := common.RedisHGetObjFields(cacheKey, &token)
	if err != nil {
		return nil, err
	}
	if token.ImageQuotaPending > 0 {
		recoverImageQuota(cacheKey, "token")
		token = Token{}
		fields, err = common.RedisHGetObjFields(cacheKey, &token)
		if err != nil {
			return nil, err
		}
	}
	if _, hasPolicy := fields["GroupPolicy"]; !hasPolicy || fields["RoutingVersion"] != tokenRoutingCacheVersion || token.Id <= 0 {
		return nil, fmt.Errorf("token cache is incomplete")
	}
	token.Key = key
	return &token, nil
}
