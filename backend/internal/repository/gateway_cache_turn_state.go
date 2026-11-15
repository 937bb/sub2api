package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const codexTurnStateOwnerPrefix = "openai:turn_state_owner:v1:"

var _ service.OpenAICodexTurnStateCache = (*gatewayCache)(nil)

func (c *gatewayCache) GetOpenAICodexTurnStateOwner(ctx context.Context, digest string) (string, error) {
	if digest == "" {
		return "", nil
	}
	owner, err := c.rdb.Get(ctx, codexTurnStateOwnerPrefix+digest).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return owner, err
}

func (c *gatewayCache) BindOpenAICodexTurnStateOwner(ctx context.Context, digest, owner string, ttl time.Duration) error {
	if digest == "" || owner == "" || ttl <= 0 {
		return nil
	}
	return c.rdb.Set(ctx, codexTurnStateOwnerPrefix+digest, owner, ttl).Err()
}
