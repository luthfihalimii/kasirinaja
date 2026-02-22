package cache

import (
	"context"
	"time"

	"kasirinaja/backend/internal/domain"
)

type RecommendationCache interface {
	Get(ctx context.Context, key string) (*domain.RecommendationResponse, bool, error)
	Set(ctx context.Context, key string, value *domain.RecommendationResponse, ttl time.Duration) error
}

type NoopRecommendationCache struct{}

func (NoopRecommendationCache) Get(_ context.Context, _ string) (*domain.RecommendationResponse, bool, error) {
	return nil, false, nil
}

func (NoopRecommendationCache) Set(_ context.Context, _ string, _ *domain.RecommendationResponse, _ time.Duration) error {
	return nil
}
