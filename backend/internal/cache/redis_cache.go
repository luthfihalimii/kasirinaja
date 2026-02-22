package cache

import (
	"context"
	"encoding/json"
	"time"

	redis "github.com/redis/go-redis/v9"

	"kasirinaja/backend/internal/domain"
)

type RedisRecommendationCache struct {
	client *redis.Client
}

func NewRedisRecommendationCache(addr string, password string, db int) *RedisRecommendationCache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisRecommendationCache{client: client}
}

func (c *RedisRecommendationCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisRecommendationCache) Close() error {
	return c.client.Close()
}

func (c *RedisRecommendationCache) Get(ctx context.Context, key string) (*domain.RecommendationResponse, bool, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	var resp domain.RecommendationResponse
	if err := json.Unmarshal([]byte(val), &resp); err != nil {
		return nil, false, err
	}
	return &resp, true, nil
}

func (c *RedisRecommendationCache) Set(ctx context.Context, key string, value *domain.RecommendationResponse, ttl time.Duration) error {
	if value == nil {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, payload, ttl).Err()
}
