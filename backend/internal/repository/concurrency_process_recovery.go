package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/redis/go-redis/v9"
)

var processSlotPrefixPattern = regexp.MustCompile(`^r[a-z0-9]{1,20}$`)
var removeProcessSlotScript = redis.NewScript(`
 local n = 0
 for i = 2, #ARGV do
  if string.sub(ARGV[i], 1, string.len(ARGV[1])) == ARGV[1] then
   n = n + redis.call('ZREM', KEYS[1], ARGV[i])
  end
 end
 return n
`)

// Only an owner proven dead by the caller's OS lock may be reaped. Preserve
// all other owners and aggregate wait counters; bound individual Redis calls.
func (c *concurrencyCache) CleanupProcessSlots(ctx context.Context, owner string) error {
	if !processSlotPrefixPattern.MatchString(owner) {
		return fmt.Errorf("invalid concurrency owner prefix")
	}
	prefix := owner + "-"
	for _, keyPrefix := range []string{accountSlotKeyPrefix, userSlotKeyPrefix, apiKeySlotKeyPrefix, openAIWSIngressLeaseKeyPrefix} {
		var cursor uint64
		var matchedKeys []string
		for {
			keys, next, err := c.rdb.Scan(ctx, cursor, keyPrefix+"*", 100).Result()
			if err != nil {
				return err
			}
			matchedKeys = append(matchedKeys, keys...)
			cursor = next
			if cursor == 0 {
				break
			}
		}
		for _, key := range matchedKeys {
			suffix := strings.TrimPrefix(key, keyPrefix)
			if suffix == "" || strings.Trim(suffix, "0123456789") != "" {
				continue
			}
			var memberCursor uint64
			var owned []any
			for {
				members, more, err := c.rdb.ZScan(ctx, key, memberCursor, prefix+"*", 200).Result()
				if err != nil {
					return err
				}
				for i := 0; i+1 < len(members); i += 2 {
					owned = append(owned, members[i])
				}
				memberCursor = more
				if memberCursor == 0 {
					break
				}
			}
			// Delete after iteration so shrinking collections cannot skip members.
			for i := 0; i < len(owned); i += 200 {
				end := i + 200
				if end > len(owned) {
					end = len(owned)
				}
				args := append([]any{prefix}, owned[i:end]...)
				if err := removeProcessSlotScript.Run(ctx, c.rdb, []string{key}, args...).Err(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
