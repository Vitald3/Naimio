package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
local ttl = redis.call('PTTL', KEYS[1])
return {count, ttl}`

type RedisLimiter struct {
	client *redis.Client
	config Config
	prefix string
}

func NewRedis(client *redis.Client, config Config, prefix string) *RedisLimiter {
	return &RedisLimiter{client: client, config: config, prefix: prefix}
}

func (l *RedisLimiter) Allow(ctx context.Context, class Class, key string) (Decision, error) {
	classConfig, ok := l.config[class]
	if !ok || classConfig.Limit <= 0 || classConfig.Window <= 0 {
		return Decision{}, fmt.Errorf("rate limit class %s is not configured", class)
	}
	result, err := l.client.Eval(ctx, redisScript, []string{l.prefix + ":" + string(class) + ":" + key}, classConfig.Window.Milliseconds()).Slice()
	if err != nil {
		return Decision{}, err
	}
	count, okCount := result[0].(int64)
	ttlMilliseconds, okTTL := result[1].(int64)
	if !okCount || !okTTL {
		return Decision{}, fmt.Errorf("unexpected Redis rate-limit response")
	}
	if count > int64(classConfig.Limit) {
		return Decision{RetryAfter: time.Duration(ttlMilliseconds) * time.Millisecond}, nil
	}
	return Decision{Allowed: true}, nil
}
