package recommendation

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"kasirinaja/backend/internal/cache"
	"kasirinaja/backend/internal/domain"
)

type Engine struct {
	cache         cache.RecommendationCache
	cacheTTL      time.Duration
	minConfidence float64
}

func NewEngine(cacheStore cache.RecommendationCache, cacheTTL time.Duration) *Engine {
	if cacheStore == nil {
		cacheStore = cache.NoopRecommendationCache{}
	}
	if cacheTTL <= 0 {
		cacheTTL = 20 * time.Second
	}

	return &Engine{
		cache:         cacheStore,
		cacheTTL:      cacheTTL,
		minConfidence: 0.35,
	}
}

func (e *Engine) Recommend(
	ctx context.Context,
	req domain.RecommendationRequest,
	products map[string]domain.Product,
	stockMap map[string]int,
	pairs []domain.AssociationPair,
) domain.RecommendationResponse {
	startedAt := time.Now()

	if len(req.CartItems) == 0 {
		return domain.RecommendationResponse{
			UIPolicy:  domain.UIPolicy{Show: false, CooldownSeconds: 30},
			LatencyMS: time.Since(startedAt).Milliseconds(),
		}
	}

	if req.QueueSpeedHint >= 28 {
		return domain.RecommendationResponse{
			UIPolicy:  domain.UIPolicy{Show: false, CooldownSeconds: 90},
			LatencyMS: time.Since(startedAt).Milliseconds(),
		}
	}

	cacheKey := buildCacheKey(req)
	if cached, ok, err := e.cache.Get(ctx, cacheKey); err == nil && ok {
		cached.LatencyMS = time.Since(startedAt).Milliseconds()
		return *cached
	}

	normalizedItems := normalizeCartItems(req.CartItems)
	cartSet := make(map[string]struct{}, len(normalizedItems))
	for _, item := range normalizedItems {
		cartSet[item.SKU] = struct{}{}
	}

	pairSignal := make(map[string]float64)
	for _, pair := range pairs {
		if _, exists := cartSet[pair.TargetSKU]; exists {
			continue
		}
		pairSignal[pair.TargetSKU] += pair.Affinity
	}

	hour := time.Now().Hour()
	if req.Timestamp != nil {
		hour = req.Timestamp.Hour()
	}

	bestSKU := ""
	bestScore := 0.0
	bestReason := ""
	bestConfidence := 0.0
	bestMarginLift := int64(0)

	for sku, pairAffinityRaw := range pairSignal {
		product, ok := products[sku]
		if !ok || !product.Active {
			continue
		}

		stock := stockMap[sku]
		if stock <= 0 {
			continue
		}

		pairAffinity := clamp(pairAffinityRaw/float64(max(1, len(normalizedItems))), 0, 1)
		marginScore := clamp(product.MarginRate/0.40, 0, 1)
		stockScore := clamp(float64(stock)/90.0, 0, 1)
		timeRelevance := categoryHourRelevance(product.Category, hour)
		promptFatigue := clamp(float64(req.PromptCount)/4.0, 0, 1)

		score :=
			0.40*pairAffinity +
				0.25*marginScore +
				0.20*stockScore +
				0.10*timeRelevance -
				0.05*promptFatigue

		confidence := clamp(score, 0, 1)
		if confidence < e.minConfidence {
			continue
		}

		reasonCode := deriveReason(pairAffinity, marginScore, stockScore, timeRelevance)
		expectedMarginLift := int64(math.Round(float64(product.PriceCents) * product.MarginRate))

		if confidence > bestScore {
			bestScore = confidence
			bestSKU = sku
			bestReason = reasonCode
			bestConfidence = confidence
			bestMarginLift = expectedMarginLift
		}
	}

	resp := domain.RecommendationResponse{
		UIPolicy: domain.UIPolicy{Show: false, CooldownSeconds: 45},
	}

	if bestSKU != "" {
		product := products[bestSKU]
		resp.Recommendation = &domain.Recommendation{
			SKU:                     product.SKU,
			Name:                    product.Name,
			PriceCents:              product.PriceCents,
			ExpectedMarginLiftCents: bestMarginLift,
			ReasonCode:              bestReason,
			Confidence:              round2(bestConfidence),
		}

		cooldown := 45
		if req.QueueSpeedHint > 18 {
			cooldown = 70
		}
		resp.UIPolicy = domain.UIPolicy{Show: true, CooldownSeconds: cooldown}
	}

	resp.LatencyMS = time.Since(startedAt).Milliseconds()
	_ = e.cache.Set(ctx, cacheKey, &resp, e.cacheTTL)
	return resp
}

func normalizeCartItems(items []domain.CartItem) []domain.CartItem {
	aggregated := make(map[string]int, len(items))
	for _, item := range items {
		if item.SKU == "" || item.Qty < 1 {
			continue
		}
		aggregated[item.SKU] += item.Qty
	}

	result := make([]domain.CartItem, 0, len(aggregated))
	for sku, qty := range aggregated {
		result = append(result, domain.CartItem{SKU: sku, Qty: qty})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SKU < result[j].SKU })
	return result
}

func deriveReason(pairAffinity float64, marginScore float64, stockScore float64, timeRelevance float64) string {
	type reasonWeight struct {
		code  string
		value float64
	}

	reasons := []reasonWeight{
		{code: "often_bought_together", value: pairAffinity},
		{code: "high_margin_boost", value: marginScore},
		{code: "healthy_stock", value: stockScore},
		{code: "time_slot_match", value: timeRelevance},
	}

	sort.Slice(reasons, func(i, j int) bool {
		return reasons[i].value > reasons[j].value
	})
	return reasons[0].code
}

func categoryHourRelevance(category string, hour int) float64 {
	switch category {
	case "snack", "beverage":
		if hour >= 16 && hour <= 21 {
			return 0.95
		}
		if hour >= 8 && hour < 16 {
			return 0.75
		}
	case "dairy", "bakery", "grocery":
		if hour >= 6 && hour <= 11 {
			return 0.90
		}
		if hour >= 17 && hour <= 21 {
			return 0.70
		}
	}
	return 0.55
}

func buildCacheKey(req domain.RecommendationRequest) string {
	parts := make([]string, 0, len(req.CartItems)+2)
	parts = append(parts, req.StoreID)
	for _, item := range normalizeCartItems(req.CartItems) {
		parts = append(parts, fmt.Sprintf("%s:%d", item.SKU, item.Qty))
	}
	parts = append(parts, fmt.Sprintf("q:%d", int(req.QueueSpeedHint)))
	parts = append(parts, fmt.Sprintf("p:%d", req.PromptCount))

	hash := sha1.Sum([]byte(strings.Join(parts, "|")))
	return "pos:recommendation:" + hex.EncodeToString(hash[:])
}

func clamp(val float64, minVal float64, maxVal float64) float64 {
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func round2(val float64) float64 {
	return math.Round(val*100) / 100
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
