package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

// Response values are partitioned by tenant/key/account in the digest; capacity and hit targets are per group.
// Policy changes create a fresh namespace. Statistics expire hourly, never pretending a miss is a hit.
func kiroCacheKeys(id int64, digest string, p service.KiroCachePolicy) (string, string, string) {
	data, _ := json.Marshal(p)
	hash := sha256.Sum256(data)
	base := fmt.Sprintf("kiro:group-response:{%d}:%s", id, hex.EncodeToString(hash[:8]))
	return base + ":body:" + digest, base + ":entries", base + ":stats:" + time.Now().UTC().Format("2006010215")
}

var kiroGetScript = redis.NewScript(`
local requests=redis.call('HINCRBY',KEYS[2],'requests',1)
redis.call('EXPIRE',KEYS[2],7200)
local body=redis.call('GET',KEYS[1])
if not body then return '' end
redis.call('HINCRBY',KEYS[2],'available',1)
local hits=tonumber(redis.call('HGET',KEYS[2],'hits') or '0')
if (hits+1)*100>requests*tonumber(ARGV[1]) then
 redis.call('HINCRBY',KEYS[2],'bypassed',1)
 return ''
end
redis.call('HINCRBY',KEYS[2],'hits',1)
return body
`)
var kiroPutScript = redis.NewScript(`
local now=redis.call('TIME')
local timestamp=tonumber(now[1])
local expired=redis.call('ZRANGEBYSCORE',KEYS[2],'-inf',timestamp)
for _,key in ipairs(expired) do redis.call('DEL',key) end
redis.call('ZREMRANGEBYSCORE',KEYS[2],'-inf',timestamp)
redis.call('SET',KEYS[1],ARGV[1],'EX',ARGV[2])
redis.call('ZADD',KEYS[2],timestamp+tonumber(ARGV[2]),KEYS[1])
local excess=redis.call('ZCARD',KEYS[2])-tonumber(ARGV[3])
if excess>0 then
 local victims=redis.call('ZRANGE',KEYS[2],0,excess-1)
 for _,key in ipairs(victims) do redis.call('DEL',key);redis.call('ZREM',KEYS[2],key) end
end
redis.call('EXPIRE',KEYS[2],tonumber(ARGV[2])+60)
return 1
`)

func (c *gatewayCache) GetKiroResponse(ctx context.Context, id int64, digest string, p service.KiroCachePolicy) ([]byte, error) {
	body, _, stats := kiroCacheKeys(id, digest, p)
	result, err := kiroGetScript.Run(ctx, c.rdb, []string{body, stats}, p.TargetPercent).Text()
	return []byte(result), err
}
func (c *gatewayCache) PutKiroResponse(ctx context.Context, id int64, digest string, data []byte, p service.KiroCachePolicy) error {
	if len(data) > 1<<20 {
		return nil
	}
	body, index, _ := kiroCacheKeys(id, digest, p)
	return kiroPutScript.Run(ctx, c.rdb, []string{body, index}, data, p.TTLSeconds, p.MaxEntries).Err()
}
