package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func kiroPrefixKeys(groupID int64, scope string, policy service.KiroCachePolicy) (string, string) {
	encoded, _ := json.Marshal(policy)
	settings := sha256.Sum256(encoded)
	identity := sha256.Sum256([]byte(scope))
	base := fmt.Sprintf("kiro:group-prefix:v1:{%d}:%x", groupID, settings[:8])
	return base + ":" + hex.EncodeToString(identity[:]) + ":", base + ":entries"
}

func (c *gatewayCache) LookupKiroPrefix(ctx context.Context, groupID int64, scope string, plan *kiro.PrefixPlan, policy service.KiroCachePolicy) (*kiro.PrefixPoint, error) {
	if plan == nil || len(plan.Lookups) == 0 {
		return nil, nil
	}
	prefix, _ := kiroPrefixKeys(groupID, scope, policy)
	keys := make([]string, len(plan.Lookups))
	for i, point := range plan.Lookups {
		keys[i] = prefix + point.Digest
	}
	values, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	var best *kiro.PrefixPoint
	for i, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		var entry kiro.PrefixPoint
		if json.Unmarshal([]byte(text), &entry) != nil || entry.Digest != plan.Lookups[i].Digest || entry.Tokens != plan.Lookups[i].Tokens || entry.TTLSeconds < 1 || entry.TTLSeconds > 3600 {
			continue
		}
		if best == nil || entry.Tokens > best.Tokens {
			copy := entry
			best = &copy
		}
	}
	return best, nil
}

// Entries are group-bounded and expire independently. Cache reads are renewed
// only after successful inference, with the TTL stored on the original entry.
var kiroPrefixCommitScript = redis.NewScript(`
local now=tonumber(redis.call('TIME')[1])
local expired=redis.call('ZRANGEBYSCORE',KEYS[1],'-inf',now)
for _,key in ipairs(expired) do redis.call('DEL',key) end
redis.call('ZREMRANGEBYSCORE',KEYS[1],'-inf',now)
local writes=cjson.decode(ARGV[1])
local hit=cjson.decode(ARGV[2])
if hit.key and redis.call('EXISTS',hit.key)==1 then
 redis.call('EXPIRE',hit.key,hit.ttl)
 redis.call('ZADD',KEYS[1],now+hit.ttl,hit.key)
end
for _,entry in ipairs(writes) do
 redis.call('SET',entry.key,entry.value,'EX',entry.ttl)
 redis.call('ZADD',KEYS[1],now+entry.ttl,entry.key)
end
local excess=redis.call('ZCARD',KEYS[1])-tonumber(ARGV[3])
if excess>0 then
 local victims=redis.call('ZRANGE',KEYS[1],0,excess-1)
 for _,key in ipairs(victims) do redis.call('DEL',key);redis.call('ZREM',KEYS[1],key) end
end
redis.call('EXPIRE',KEYS[1],3660)
return 1
`)

func (c *gatewayCache) CommitKiroPrefix(ctx context.Context, groupID int64, scope string, plan *kiro.PrefixPlan, hit *kiro.PrefixPoint, policy service.KiroCachePolicy) error {
	prefix, index := kiroPrefixKeys(groupID, scope, policy)
	writes := make([]map[string]any, 0, len(plan.Writes))
	for _, point := range plan.Writes {
		value, _ := json.Marshal(point)
		writes = append(writes, map[string]any{"key": prefix + point.Digest, "value": string(value), "ttl": point.TTLSeconds})
	}
	renew := map[string]any{}
	if hit != nil {
		renew = map[string]any{"key": prefix + hit.Digest, "ttl": hit.TTLSeconds}
	}
	encoded, _ := json.Marshal(writes)
	matched, _ := json.Marshal(renew)
	return kiroPrefixCommitScript.Run(ctx, c.rdb, []string{index}, string(encoded), string(matched), policy.MaxEntries).Err()
}
