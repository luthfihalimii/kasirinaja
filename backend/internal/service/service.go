package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"kasirinaja/backend/internal/domain"
	"kasirinaja/backend/internal/recommendation"
	"kasirinaja/backend/internal/store"
	"kasirinaja/backend/internal/xid"
)

type actorContextKey struct{}

func WithActor(ctx context.Context, actor domain.Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}

func ActorFromContext(ctx context.Context) (domain.Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(domain.Actor)
	return actor, ok
}

type Service struct {
	repo           store.Repository
	recommender    *recommendation.Engine
	defaultStoreID string
}

func New(repo store.Repository, recommender *recommendation.Engine, defaultStoreID string) *Service {
	if defaultStoreID == "" {
		defaultStoreID = "main-store"
	}

	return &Service{
		repo:           repo,
		recommender:    recommender,
		defaultStoreID: defaultStoreID,
	}
}

func (s *Service) ListProducts(ctx context.Context) ([]domain.Product, error) {
	return s.repo.ListProducts(ctx)
}

func (s *Service) CreateProduct(ctx context.Context, req domain.ProductCreateRequest) (domain.Product, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.Product{}, fmt.Errorf("admin role required")
	}

	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}

	req.SKU = strings.ToUpper(strings.TrimSpace(req.SKU))
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)

	if req.SKU == "" || req.Name == "" || req.Category == "" {
		return domain.Product{}, store.ErrInvalidTransaction
	}
	if req.PriceCents < 1 || req.MarginRate < 0 || req.MarginRate > 1 || req.InitialStock < 0 {
		return domain.Product{}, store.ErrInvalidTransaction
	}

	product := domain.Product{
		SKU:        req.SKU,
		Name:       req.Name,
		Category:   req.Category,
		PriceCents: req.PriceCents,
		MarginRate: req.MarginRate,
		Active:     true,
	}

	created, err := s.repo.CreateProduct(ctx, product)
	if err != nil {
		return domain.Product{}, err
	}

	if req.InitialStock > 0 {
		err := s.repo.IncreaseStock(ctx, req.StoreID, []domain.StockAdjustment{{
			SKU: created.SKU,
			Qty: req.InitialStock,
		}})
		if err != nil {
			return domain.Product{}, err
		}
	}

	s.logAudit(ctx, req.StoreID, "product_create", "product", created.SKU, fmt.Sprintf("name=%s,price=%d,stock=%d", created.Name, created.PriceCents, req.InitialStock))
	if err := s.repo.UpsertProductCost(ctx, req.StoreID, created.SKU, deriveUnitCost(*created)); err != nil {
		log.Printf("[service] WARN: failed to upsert product cost sku=%s: %v", created.SKU, err)
	}

	return *created, nil
}

func (s *Service) UpdateProduct(ctx context.Context, sku string, req domain.ProductUpdateRequest) (domain.Product, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.Product{}, fmt.Errorf("admin role required")
	}

	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" {
		return domain.Product{}, store.ErrInvalidTransaction
	}

	existing, err := s.repo.GetProductBySKU(ctx, sku)
	if err != nil {
		return domain.Product{}, err
	}

	updated := *existing
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return domain.Product{}, store.ErrInvalidTransaction
		}
		updated.Name = name
	}
	if req.Category != nil {
		category := strings.TrimSpace(*req.Category)
		if category == "" {
			return domain.Product{}, store.ErrInvalidTransaction
		}
		updated.Category = category
	}
	if req.PriceCents != nil {
		if *req.PriceCents < 1 {
			return domain.Product{}, store.ErrInvalidTransaction
		}
		updated.PriceCents = *req.PriceCents
	}
	if req.MarginRate != nil {
		if *req.MarginRate < 0 || *req.MarginRate > 1 {
			return domain.Product{}, store.ErrInvalidTransaction
		}
		updated.MarginRate = *req.MarginRate
	}
	if req.Active != nil {
		updated.Active = *req.Active
	}

	saved, err := s.repo.UpdateProduct(ctx, updated)
	if err != nil {
		return domain.Product{}, err
	}

	if existing.PriceCents != saved.PriceCents {
		if err := s.repo.CreatePriceHistory(ctx, domain.ProductPriceHistory{
			ID:            xid.New("ph"),
			SKU:           saved.SKU,
			OldPriceCents: existing.PriceCents,
			NewPriceCents: saved.PriceCents,
			ChangedBy:     actor.Username,
			ChangedAt:     time.Now().UTC(),
		}); err != nil {
			log.Printf("[service] WARN: failed to record price history sku=%s: %v", saved.SKU, err)
		}
	}

	s.logAudit(ctx, s.defaultStoreID, "product_update", "product", saved.SKU, fmt.Sprintf("active=%t,price=%d,margin=%.4f", saved.Active, saved.PriceCents, saved.MarginRate))
	if err := s.repo.UpsertProductCost(ctx, s.defaultStoreID, saved.SKU, deriveUnitCost(*saved)); err != nil {
		log.Printf("[service] WARN: failed to upsert product cost sku=%s: %v", saved.SKU, err)
	}

	return *saved, nil
}

func (s *Service) ListProductPriceHistory(ctx context.Context, sku string, limit int) ([]domain.ProductPriceHistory, error) {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" {
		return nil, store.ErrInvalidTransaction
	}
	if limit < 1 {
		limit = 50
	}
	return s.repo.ListPriceHistory(ctx, sku, limit)
}

func (s *Service) Recommend(ctx context.Context, req domain.RecommendationRequest) (domain.RecommendationResponse, error) {
	if len(req.CartItems) == 0 {
		return domain.RecommendationResponse{UIPolicy: domain.UIPolicy{Show: false, CooldownSeconds: 30}}, nil
	}

	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}

	req.CartItems = normalizeItems(req.CartItems)
	if len(req.CartItems) == 0 {
		return domain.RecommendationResponse{UIPolicy: domain.UIPolicy{Show: false, CooldownSeconds: 30}}, nil
	}

	cartSKUs := make([]string, 0, len(req.CartItems))
	for _, item := range req.CartItems {
		cartSKUs = append(cartSKUs, item.SKU)
	}

	pairs, err := s.repo.GetAssociationPairs(ctx, cartSKUs)
	if err != nil {
		return domain.RecommendationResponse{}, err
	}

	productSKUs := make(map[string]struct{}, len(cartSKUs)+len(pairs))
	for _, sku := range cartSKUs {
		productSKUs[sku] = struct{}{}
	}
	for _, pair := range pairs {
		productSKUs[pair.TargetSKU] = struct{}{}
	}

	skus := make([]string, 0, len(productSKUs))
	for sku := range productSKUs {
		skus = append(skus, sku)
	}

	products, err := s.repo.GetProductsBySKUs(ctx, skus)
	if err != nil {
		return domain.RecommendationResponse{}, err
	}

	stockMap, err := s.repo.GetStockMap(ctx, req.StoreID, skus)
	if err != nil {
		return domain.RecommendationResponse{}, err
	}

	resp := s.recommender.Recommend(ctx, req, products, stockMap, pairs)

	if resp.UIPolicy.Show && resp.Recommendation != nil {
		_ = s.repo.CreateRecommendationEvent(ctx, domain.RecommendationEvent{
			StoreID:    req.StoreID,
			TerminalID: req.TerminalID,
			SKU:        resp.Recommendation.SKU,
			Action:     domain.RecommendationShownAction,
			ReasonCode: resp.Recommendation.ReasonCode,
			Confidence: resp.Recommendation.Confidence,
			LatencyMS:  resp.LatencyMS,
			CreatedAt:  time.Now().UTC(),
		})
	}

	return resp, nil
}

func (s *Service) OpenShift(ctx context.Context, req domain.ShiftOpenRequest) (domain.ShiftResponse, error) {
	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	if req.TerminalID == "" || req.CashierName == "" {
		return domain.ShiftResponse{}, store.ErrInvalidTransaction
	}

	shift := domain.Shift{
		ID:                xid.New("shift"),
		StoreID:           req.StoreID,
		TerminalID:        req.TerminalID,
		CashierName:       req.CashierName,
		OpeningFloatCents: req.OpeningFloatCents,
		Status:            domain.ShiftStatusOpen,
		OpenedAt:          time.Now().UTC(),
	}
	saved, err := s.repo.CreateShift(ctx, shift)
	if err != nil {
		if errors.Is(err, store.ErrInvalidTransaction) {
			return domain.ShiftResponse{}, fmt.Errorf("shift already open")
		}
		return domain.ShiftResponse{}, err
	}

	s.logAudit(ctx, req.StoreID, "shift_open", "shift", saved.ID, req.CashierName)

	return domain.ShiftResponse{Shift: *saved}, nil
}

func (s *Service) CloseShift(ctx context.Context, req domain.ShiftCloseRequest) (domain.ShiftResponse, error) {
	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	if req.TerminalID == "" {
		return domain.ShiftResponse{}, store.ErrInvalidTransaction
	}

	active, err := s.repo.CloseActiveShift(ctx, req.StoreID, req.TerminalID, req.ClosingCashCents, time.Now().UTC())
	if err != nil {
		return domain.ShiftResponse{}, err
	}
	s.logAudit(ctx, req.StoreID, "shift_close", "shift", active.ID, fmt.Sprintf("closing_cash=%d", req.ClosingCashCents))

	return domain.ShiftResponse{Shift: *active}, nil
}

func (s *Service) GetActiveShift(ctx context.Context, storeID string, terminalID string) (domain.ShiftResponse, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}
	if terminalID == "" {
		return domain.ShiftResponse{}, store.ErrInvalidTransaction
	}

	shift, err := s.repo.GetActiveShift(ctx, storeID, terminalID)
	if err != nil {
		return domain.ShiftResponse{}, err
	}

	return domain.ShiftResponse{Shift: *shift}, nil
}

func (s *Service) Checkout(ctx context.Context, req domain.CheckoutRequest) (domain.CheckoutResponse, error) {
	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	req.PaymentSplits = normalizePaymentSplits(req.PaymentSplits)
	if len(req.PaymentSplits) > 0 {
		req.PaymentMethod = "split"
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = "cash"
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = xid.New("idem")
	}

	if !isSupportedPaymentMethod(req.PaymentMethod) {
		return domain.CheckoutResponse{}, store.ErrInvalidTransaction
	}
	if req.TaxRatePercent < 0 || req.TaxRatePercent > 100 {
		return domain.CheckoutResponse{}, store.ErrInvalidTransaction
	}
	if req.DiscountCents < 0 {
		return domain.CheckoutResponse{}, store.ErrInvalidTransaction
	}

	if req.ManualOverride {
		actor, ok := ActorFromContext(ctx)
		if !ok || actor.Role != "admin" {
			return domain.CheckoutResponse{}, fmt.Errorf("manual override requires admin role")
		}
	}

	shift, err := s.GetActiveShift(ctx, req.StoreID, req.TerminalID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.CheckoutResponse{}, fmt.Errorf("active shift required")
		}
		return domain.CheckoutResponse{}, err
	}

	normalized := normalizeItems(req.CartItems)
	if len(normalized) == 0 {
		return domain.CheckoutResponse{}, store.ErrInvalidTransaction
	}

	if existing, err := s.repo.FindTransactionByIdempotency(ctx, req.IdempotencyKey); err == nil {
		return toCheckoutResponse(existing, true), nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.CheckoutResponse{}, err
	}

	skus := make([]string, 0, len(normalized))
	for _, item := range normalized {
		skus = append(skus, item.SKU)
	}
	products, err := s.repo.GetProductsBySKUs(ctx, skus)
	if err != nil {
		return domain.CheckoutResponse{}, err
	}

	subtotal := int64(0)
	for _, item := range normalized {
		product, exists := products[item.SKU]
		if !exists {
			return domain.CheckoutResponse{}, store.ErrInvalidTransaction
		}
		subtotal += int64(item.Qty) * product.PriceCents
	}

	promoDiscount, err := s.calculatePromoDiscount(ctx, subtotal)
	if err != nil {
		return domain.CheckoutResponse{}, err
	}
	req.DiscountCents += promoDiscount
	if req.DiscountCents > subtotal {
		req.DiscountCents = subtotal
	}

	taxBase := subtotal - req.DiscountCents
	taxCents := int64(math.Round(float64(taxBase) * req.TaxRatePercent / 100))
	totalCents := taxBase + taxCents

	switch req.PaymentMethod {
	case "cash":
		if req.CashReceivedCents < totalCents {
			return domain.CheckoutResponse{}, store.ErrInvalidTransaction
		}
	case "split":
		if len(req.PaymentSplits) < 2 {
			return domain.CheckoutResponse{}, store.ErrInvalidTransaction
		}
		splitTotal := int64(0)
		for _, split := range req.PaymentSplits {
			if !isSplitMethodSupported(split.Method) || split.AmountCents < 1 {
				return domain.CheckoutResponse{}, store.ErrInvalidTransaction
			}
			if split.Method != "cash" && strings.TrimSpace(split.Reference) == "" {
				return domain.CheckoutResponse{}, store.ErrInvalidTransaction
			}
			splitTotal += split.AmountCents
		}
		if splitTotal != totalCents {
			return domain.CheckoutResponse{}, store.ErrInvalidTransaction
		}
		req.CashReceivedCents = splitTotal
		req.PaymentReference = encodePaymentSplits(req.PaymentSplits)
	default:
		// Non-cash single payment.
		if strings.TrimSpace(req.PaymentReference) == "" {
			return domain.CheckoutResponse{}, store.ErrInvalidTransaction
		}
	}

	lineItems := make([]domain.TransactionLine, 0, len(normalized))
	for _, item := range normalized {
		lineItems = append(lineItems, domain.TransactionLine{SKU: item.SKU, Qty: item.Qty})
	}

	tx := domain.Transaction{
		ID:                     xid.New("tx"),
		StoreID:                req.StoreID,
		TerminalID:             req.TerminalID,
		ShiftID:                shift.Shift.ID,
		IdempotencyKey:         req.IdempotencyKey,
		PaymentMethod:          req.PaymentMethod,
		PaymentReference:       req.PaymentReference,
		PaymentSplits:          req.PaymentSplits,
		CashReceivedCents:      req.CashReceivedCents,
		DiscountCents:          req.DiscountCents,
		TaxRatePercent:         req.TaxRatePercent,
		Status:                 domain.TxStatusPaid,
		RecommendationShown:    req.RecommendationInfo.Shown,
		RecommendationAccepted: req.RecommendationInfo.Accepted,
		RecommendationSKU:      req.RecommendationInfo.SKU,
		CreatedAt:              time.Now().UTC(),
		Items:                  lineItems,
	}

	created, err := s.repo.CreateCheckout(ctx, tx)
	if err != nil {
		return domain.CheckoutResponse{}, err
	}

	if req.RecommendationInfo.Shown {
		action := domain.RecommendationRejectedAction
		if req.RecommendationInfo.Accepted {
			action = domain.RecommendationAcceptedAction
		}

		_ = s.repo.CreateRecommendationEvent(ctx, domain.RecommendationEvent{
			StoreID:       req.StoreID,
			TerminalID:    req.TerminalID,
			TransactionID: created.ID,
			SKU:           req.RecommendationInfo.SKU,
			Action:        action,
			ReasonCode:    req.RecommendationInfo.ReasonCode,
			Confidence:    req.RecommendationInfo.Confidence,
			CreatedAt:     time.Now().UTC(),
		})
	}

	s.logAudit(
		ctx,
		req.StoreID,
		"checkout",
		"transaction",
		created.ID,
		fmt.Sprintf(
			"total=%d,payment=%s,discount=%d,manual_override=%t,split_count=%d",
			created.TotalCents,
			created.PaymentMethod,
			created.DiscountCents,
			req.ManualOverride,
			len(req.PaymentSplits),
		),
	)

	return toCheckoutResponse(created, false), nil
}

func (s *Service) LookupCheckoutByIdempotency(ctx context.Context, idempotencyKey string) (domain.CheckoutLookupResponse, error) {
	if idempotencyKey == "" {
		return domain.CheckoutLookupResponse{}, store.ErrInvalidTransaction
	}

	tx, err := s.repo.FindTransactionByIdempotency(ctx, idempotencyKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.CheckoutLookupResponse{Found: false}, nil
		}
		return domain.CheckoutLookupResponse{}, err
	}
	checkout := toCheckoutResponse(tx, false)
	return domain.CheckoutLookupResponse{Found: true, Checkout: &checkout}, nil
}

func (s *Service) VoidTransaction(ctx context.Context, req domain.VoidTransactionRequest) (domain.VoidTransactionResponse, error) {
	if req.TransactionID == "" {
		return domain.VoidTransactionResponse{}, store.ErrInvalidTransaction
	}
	if req.Reason == "" {
		req.Reason = "unspecified"
	}

	voidedAt := time.Now().UTC()
	tx, err := s.repo.VoidTransaction(ctx, req.TransactionID, req.Reason, voidedAt)
	if err != nil {
		return domain.VoidTransactionResponse{}, err
	}

	s.logAudit(ctx, tx.StoreID, "void_transaction", "transaction", tx.ID, req.Reason)

	return domain.VoidTransactionResponse{
		TransactionID: tx.ID,
		Status:        tx.Status,
		VoidedAt:      voidedAt.Format(time.RFC3339),
	}, nil
}

func (s *Service) Refund(ctx context.Context, req domain.RefundRequest) (domain.RefundResponse, error) {
	if req.OriginalTransactionID == "" || req.AmountCents <= 0 {
		return domain.RefundResponse{}, store.ErrInvalidTransaction
	}

	tx, err := s.repo.FindTransactionByID(ctx, req.OriginalTransactionID)
	if err != nil {
		return domain.RefundResponse{}, err
	}
	if tx.Status == domain.TxStatusVoided {
		return domain.RefundResponse{}, fmt.Errorf("%w: voided transaction cannot be refunded", store.ErrInvalidTransaction)
	}
	if tx.Status == domain.TxStatusRefunded {
		return domain.RefundResponse{}, store.ErrInvalidTransaction
	}
	if req.AmountCents > tx.TotalCents {
		return domain.RefundResponse{}, store.ErrInvalidTransaction
	}

	refund := domain.Refund{
		ID:                    xid.New("refund"),
		OriginalTransactionID: req.OriginalTransactionID,
		Reason:                req.Reason,
		AmountCents:           req.AmountCents,
		Status:                domain.TxStatusRefunded,
		CreatedAt:             time.Now().UTC(),
	}

	created, err := s.repo.CreateRefund(ctx, refund)
	if err != nil {
		return domain.RefundResponse{}, err
	}

	s.logAudit(ctx, tx.StoreID, "refund_transaction", "transaction", tx.ID, fmt.Sprintf("amount=%d,reason=%s", req.AmountCents, req.Reason))

	return domain.RefundResponse{Refund: *created}, nil
}

func (s *Service) SyncOffline(ctx context.Context, req domain.OfflineSyncRequest) (domain.OfflineSyncResponse, error) {
	resp := domain.OfflineSyncResponse{
		EnvelopeID: req.EnvelopeID,
		Statuses:   make([]domain.OfflineSyncStatus, 0, len(req.Transactions)),
	}

	for _, tx := range req.Transactions {
		checkoutReq := tx.Checkout
		if checkoutReq.StoreID == "" {
			checkoutReq.StoreID = req.StoreID
		}
		if checkoutReq.TerminalID == "" {
			checkoutReq.TerminalID = req.TerminalID
		}
		if checkoutReq.IdempotencyKey == "" {
			checkoutReq.IdempotencyKey = tx.ClientTransactionID
		}

		checkoutResp, err := s.Checkout(ctx, checkoutReq)
		status := domain.OfflineSyncStatus{
			ClientTransactionID: tx.ClientTransactionID,
		}
		if err != nil {
			status.Status = "rejected"
			status.Reason = err.Error()
			resp.Statuses = append(resp.Statuses, status)
			continue
		}

		if checkoutResp.Duplicate {
			status.Status = "duplicate"
		} else {
			status.Status = "accepted"
		}
		status.TransactionID = checkoutResp.TransactionID
		resp.Statuses = append(resp.Statuses, status)
	}

	return resp, nil
}

func (s *Service) AttachMetrics(ctx context.Context, storeID string, days int) (domain.AttachMetrics, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}
	if days < 1 {
		days = 30
	}
	to := time.Now().UTC()
	from := to.Add(-time.Duration(days) * 24 * time.Hour)

	metrics, err := s.repo.GetAttachMetrics(ctx, storeID, from, to)
	if err != nil {
		return domain.AttachMetrics{}, err
	}
	return metrics, nil
}

func (s *Service) StockOpname(ctx context.Context, req domain.StockOpnameRequest) (domain.StockOpnameResponse, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.StockOpnameResponse{}, fmt.Errorf("admin role required")
	}

	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	if len(req.Items) == 0 {
		return domain.StockOpnameResponse{}, store.ErrInvalidTransaction
	}

	skus := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if item.SKU == "" || item.CountedQty < 0 {
			return domain.StockOpnameResponse{}, store.ErrInvalidTransaction
		}
		skus = append(skus, item.SKU)
	}

	systemStock, err := s.repo.GetStockMap(ctx, req.StoreID, skus)
	if err != nil {
		return domain.StockOpnameResponse{}, err
	}

	adjustments := make([]domain.StockOpnameAdjustment, 0, len(req.Items))
	for _, item := range req.Items {
		systemQty := systemStock[item.SKU]
		if systemQty != item.CountedQty {
			if err := s.repo.SetStock(ctx, req.StoreID, item.SKU, item.CountedQty); err != nil {
				return domain.StockOpnameResponse{}, err
			}
		}
		adjustments = append(adjustments, domain.StockOpnameAdjustment{
			SKU:        item.SKU,
			SystemQty:  systemQty,
			CountedQty: item.CountedQty,
			DeltaQty:   item.CountedQty - systemQty,
		})
	}

	opnameID := xid.New("opname")
	s.logAudit(ctx, req.StoreID, "stock_opname", "inventory", opnameID, fmt.Sprintf("items=%d,notes=%s", len(req.Items), req.Notes))

	return domain.StockOpnameResponse{
		OpnameID:    opnameID,
		StoreID:     req.StoreID,
		Notes:       req.Notes,
		Adjustments: adjustments,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) ReceiveInventoryLot(ctx context.Context, req domain.InventoryLotReceiveRequest) (domain.InventoryLot, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.InventoryLot{}, fmt.Errorf("admin role required")
	}
	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	req.SKU = strings.ToUpper(strings.TrimSpace(req.SKU))
	req.LotCode = strings.TrimSpace(req.LotCode)
	req.Notes = strings.TrimSpace(req.Notes)
	if req.SKU == "" || req.Qty < 1 || req.CostCents < 1 {
		return domain.InventoryLot{}, store.ErrInvalidTransaction
	}

	var expiryDate *time.Time
	if strings.TrimSpace(req.ExpiryDate) != "" {
		parsed, err := time.Parse("2006-01-02", req.ExpiryDate)
		if err != nil {
			return domain.InventoryLot{}, store.ErrInvalidTransaction
		}
		exp := parsed.UTC()
		expiryDate = &exp
	}

	lot, err := s.repo.CreateInventoryLot(ctx, domain.InventoryLot{
		ID:           xid.New("lot"),
		StoreID:      req.StoreID,
		SKU:          req.SKU,
		LotCode:      req.LotCode,
		ExpiryDate:   expiryDate,
		QtyReceived:  req.Qty,
		QtyAvailable: req.Qty,
		CostCents:    req.CostCents,
		SourceType:   "manual",
		Notes:        req.Notes,
		ReceivedAt:   time.Now().UTC(),
	})
	if err != nil {
		return domain.InventoryLot{}, err
	}
	s.logAudit(ctx, req.StoreID, "inventory_lot_receive", "inventory_lot", lot.ID, fmt.Sprintf("sku=%s,qty=%d,expiry=%s", lot.SKU, lot.QtyReceived, req.ExpiryDate))
	return *lot, nil
}

func (s *Service) ListInventoryLots(ctx context.Context, storeID string, sku string, includeExpired bool, limit int) (domain.InventoryLotListResponse, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}
	lots, err := s.repo.ListInventoryLots(ctx, storeID, strings.ToUpper(strings.TrimSpace(sku)), includeExpired, limit)
	if err != nil {
		return domain.InventoryLotListResponse{}, err
	}
	return domain.InventoryLotListResponse{Lots: lots}, nil
}

func (s *Service) ProcessItemReturn(ctx context.Context, req domain.ItemReturnRequest) (domain.ItemReturnResponse, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.ItemReturnResponse{}, fmt.Errorf("admin role required")
	}
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = domain.ItemReturnModeRefund
	}
	if req.Mode != domain.ItemReturnModeRefund && req.Mode != domain.ItemReturnModeExchange {
		return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
	}
	if strings.TrimSpace(req.OriginalTransactionID) == "" || len(req.ReturnItems) == 0 {
		return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
	}

	originalTx, err := s.repo.FindTransactionByID(ctx, req.OriginalTransactionID)
	if err != nil {
		return domain.ItemReturnResponse{}, err
	}
	if originalTx.Status == domain.TxStatusVoided {
		return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
	}
	storeID := strings.TrimSpace(req.StoreID)
	if storeID == "" {
		storeID = originalTx.StoreID
	}

	purchasedBySKU := make(map[string]domain.TransactionLine, len(originalTx.Items))
	for _, line := range originalTx.Items {
		current := purchasedBySKU[line.SKU]
		if current.SKU == "" {
			current = line
		}
		current.Qty += line.Qty
		if current.UnitPriceCents < 1 {
			current.UnitPriceCents = line.UnitPriceCents
		}
		purchasedBySKU[line.SKU] = current
	}

	alreadyReturnedBySKU, err := s.repo.GetReturnedQtyByTransaction(ctx, originalTx.ID)
	if err != nil {
		return domain.ItemReturnResponse{}, err
	}

	returnQtyBySKU := make(map[string]int, len(req.ReturnItems))
	for _, line := range req.ReturnItems {
		sku := strings.ToUpper(strings.TrimSpace(line.SKU))
		if sku == "" || line.Qty < 1 {
			return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
		}
		returnQtyBySKU[sku] += line.Qty
	}

	returnLines := make([]domain.ItemReturnLine, 0, len(returnQtyBySKU))
	returnAmount := int64(0)
	for sku, qty := range returnQtyBySKU {
		purchased, exists := purchasedBySKU[sku]
		if !exists || purchased.UnitPriceCents < 1 {
			return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
		}
		if alreadyReturnedBySKU[sku]+qty > purchased.Qty {
			return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
		}
		returnLines = append(returnLines, domain.ItemReturnLine{
			SKU:            sku,
			Qty:            qty,
			UnitPriceCents: purchased.UnitPriceCents,
		})
		returnAmount += int64(qty) * purchased.UnitPriceCents
	}
	if returnAmount < 1 {
		return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
	}

	refundAmountCents := int64(0)
	exchangeTransactionID := ""
	additionalPaymentCents := int64(0)
	exchangeLines := make([]domain.ItemReturnLine, 0, len(req.ExchangeItems))

	if req.Mode == domain.ItemReturnModeRefund {
		_, err := s.repo.CreateRefund(ctx, domain.Refund{
			ID:                    xid.New("refund"),
			OriginalTransactionID: originalTx.ID,
			Reason:                strings.TrimSpace(req.Reason),
			AmountCents:           returnAmount,
			Status:                domain.TxStatusRefunded,
			CreatedAt:             time.Now().UTC(),
		})
		if err != nil {
			return domain.ItemReturnResponse{}, err
		}
		refundAmountCents = returnAmount
	} else {
		normalizedExchange := normalizeItems(req.ExchangeItems)
		if len(normalizedExchange) == 0 {
			return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
		}
		exchangeSKUs := make([]string, 0, len(normalizedExchange))
		for _, item := range normalizedExchange {
			exchangeSKUs = append(exchangeSKUs, item.SKU)
		}
		exchangeProducts, err := s.repo.GetProductsBySKUs(ctx, exchangeSKUs)
		if err != nil {
			return domain.ItemReturnResponse{}, err
		}

		exchangeSubtotal := int64(0)
		for _, item := range normalizedExchange {
			product, exists := exchangeProducts[item.SKU]
			if !exists || !product.Active {
				return domain.ItemReturnResponse{}, store.ErrInvalidTransaction
			}
			exchangeSubtotal += int64(item.Qty) * product.PriceCents
			exchangeLines = append(exchangeLines, domain.ItemReturnLine{
				SKU:            item.SKU,
				Qty:            item.Qty,
				UnitPriceCents: product.PriceCents,
			})
		}

		creditUsed := returnAmount
		if creditUsed > exchangeSubtotal {
			creditUsed = exchangeSubtotal
		}
		additionalPaymentCents = exchangeSubtotal - creditUsed
		paymentMethod := strings.TrimSpace(req.PaymentMethod)
		if additionalPaymentCents == 0 {
			paymentMethod = "cash"
			req.CashReceivedCents = 0
			req.PaymentReference = ""
		}
		if paymentMethod == "" {
			paymentMethod = "cash"
		}

		checkoutResp, err := s.Checkout(ctx, domain.CheckoutRequest{
			StoreID:           storeID,
			TerminalID:        defaultString(strings.TrimSpace(req.TerminalID), originalTx.TerminalID),
			IdempotencyKey:    xid.New("retx"),
			PaymentMethod:     paymentMethod,
			PaymentReference:  strings.TrimSpace(req.PaymentReference),
			CashReceivedCents: req.CashReceivedCents,
			DiscountCents:     creditUsed,
			TaxRatePercent:    0,
			ManualOverride:    true,
			CartItems:         normalizedExchange,
		})
		if err != nil {
			return domain.ItemReturnResponse{}, err
		}
		exchangeTransactionID = checkoutResp.TransactionID

		remainingCredit := returnAmount - creditUsed
		if remainingCredit > 0 {
			_, err = s.repo.CreateRefund(ctx, domain.Refund{
				ID:                    xid.New("refund"),
				OriginalTransactionID: originalTx.ID,
				Reason:                "remaining credit from exchange",
				AmountCents:           remainingCredit,
				Status:                domain.TxStatusRefunded,
				CreatedAt:             time.Now().UTC(),
			})
			if err != nil {
				return domain.ItemReturnResponse{}, err
			}
			refundAmountCents = remainingCredit
		}
	}

	for _, line := range returnLines {
		_, err := s.repo.CreateInventoryLot(ctx, domain.InventoryLot{
			ID:           xid.New("lot"),
			StoreID:      storeID,
			SKU:          line.SKU,
			LotCode:      "RET-" + originalTx.ID,
			QtyReceived:  line.Qty,
			QtyAvailable: line.Qty,
			CostCents:    maxInt64(line.UnitPriceCents, 1),
			SourceType:   "return",
			SourceID:     originalTx.ID,
			Notes:        "restock from item return",
			ReceivedAt:   time.Now().UTC(),
		})
		if err != nil {
			return domain.ItemReturnResponse{}, err
		}
	}

	itemReturn, err := s.repo.CreateItemReturn(ctx, domain.ItemReturn{
		ID:                     xid.New("ret"),
		StoreID:                storeID,
		OriginalTransactionID:  originalTx.ID,
		Mode:                   req.Mode,
		Reason:                 strings.TrimSpace(req.Reason),
		RefundAmountCents:      refundAmountCents,
		ExchangeTransactionID:  exchangeTransactionID,
		AdditionalPaymentCents: additionalPaymentCents,
		ProcessedBy:            actor.Username,
		CreatedAt:              time.Now().UTC(),
		ReturnItems:            returnLines,
		ExchangeItems:          exchangeLines,
	})
	if err != nil {
		return domain.ItemReturnResponse{}, err
	}

	s.logAudit(ctx, storeID, "item_return_process", "item_return", itemReturn.ID, fmt.Sprintf("mode=%s,refund=%d,exchange_tx=%s", req.Mode, refundAmountCents, exchangeTransactionID))
	return domain.ItemReturnResponse{ItemReturn: *itemReturn}, nil
}

func (s *Service) BuildHardwareReceipt(ctx context.Context, req domain.HardwareReceiptRequest) (domain.HardwareReceiptResponse, error) {
	req.TransactionID = strings.TrimSpace(req.TransactionID)
	if req.TransactionID == "" {
		return domain.HardwareReceiptResponse{}, store.ErrInvalidTransaction
	}
	tx, err := s.repo.FindTransactionByID(ctx, req.TransactionID)
	if err != nil {
		return domain.HardwareReceiptResponse{}, err
	}

	lines := []string{
		"KasirinAja POS",
		"========================",
		"TX: " + tx.ID,
		"Store: " + tx.StoreID,
		"Terminal: " + tx.TerminalID,
		"Date: " + tx.CreatedAt.Format("2006-01-02 15:04:05"),
		"------------------------",
	}
	for _, item := range tx.Items {
		lines = append(lines, fmt.Sprintf("%s x%d", item.SKU, item.Qty))
		lines = append(lines, fmt.Sprintf("  %d", item.UnitPriceCents*int64(item.Qty)))
	}
	lines = append(lines,
		"------------------------",
		fmt.Sprintf("Subtotal : %d", tx.SubtotalCents),
		fmt.Sprintf("Diskon   : %d", tx.DiscountCents),
		fmt.Sprintf("Pajak    : %d", tx.TaxCents),
		fmt.Sprintf("Total    : %d", tx.TotalCents),
		fmt.Sprintf("Bayar    : %d", tx.CashReceivedCents),
		fmt.Sprintf("Kembali  : %d", tx.ChangeCents),
		"========================",
		"Terima kasih",
		"",
	)

	escpos := []byte{0x1b, 0x40}
	for _, line := range lines {
		escpos = append(escpos, []byte(line)...)
		escpos = append(escpos, '\n')
	}
	escpos = append(escpos, []byte{0x1d, 0x56, 0x41, 0x10}...)

	return domain.HardwareReceiptResponse{
		TransactionID: tx.ID,
		EscposBase64:  base64.StdEncoding.EncodeToString(escpos),
		PreviewText:   strings.Join(lines, "\n"),
		FileName:      fmt.Sprintf("receipt-%s.bin", tx.ID),
	}, nil
}

func (s *Service) OpenCashDrawer(_ context.Context, req domain.CashDrawerOpenRequest) (domain.CashDrawerOpenResponse, error) {
	terminalID := strings.TrimSpace(req.TerminalID)
	if terminalID == "" {
		terminalID = "main-terminal"
	}
	// Standard ESC/POS pulse command for drawer kick on pin2.
	command := []byte{0x1b, 0x70, 0x00, 0x19, 0xfa}
	return domain.CashDrawerOpenResponse{
		TerminalID:    terminalID,
		CommandBase64: base64.StdEncoding.EncodeToString(command),
		Note:          "Send this ESC/POS pulse command via local printer bridge to open cash drawer.",
	}, nil
}

func (s *Service) DailyReport(ctx context.Context, storeID string, date string) (domain.DailyReport, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}

	var day time.Time
	if strings.TrimSpace(date) == "" {
		now := time.Now().UTC()
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil {
			return domain.DailyReport{}, store.ErrInvalidTransaction
		}
		day = parsed.UTC()
	}
	from := day
	to := from.Add(24 * time.Hour)

	report, err := s.repo.GetDailyReport(ctx, storeID, from, to)
	if err != nil {
		return domain.DailyReport{}, err
	}
	report.StoreID = storeID
	report.Date = from.Format("2006-01-02")
	return report, nil
}

func (s *Service) ListAuditLogs(ctx context.Context, storeID string, date string, limit int) ([]domain.AuditLog, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}
	if limit < 1 {
		limit = 100
	}

	var from time.Time
	if strings.TrimSpace(date) == "" {
		from = time.Now().UTC().Add(-24 * time.Hour)
	} else {
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil {
			return nil, store.ErrInvalidTransaction
		}
		from = parsed.UTC()
	}
	to := from.Add(24 * time.Hour)

	return s.repo.ListAuditLogs(ctx, storeID, from, to, limit)
}

func (s *Service) HoldCart(ctx context.Context, req domain.HoldCartRequest) (domain.HoldCartResponse, error) {
	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	req.TerminalID = strings.TrimSpace(req.TerminalID)
	req.Note = strings.TrimSpace(req.Note)
	req.PaymentMethod = strings.TrimSpace(req.PaymentMethod)
	req.PaymentSplits = normalizePaymentSplits(req.PaymentSplits)
	if len(req.PaymentSplits) > 0 {
		req.PaymentMethod = "split"
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = "cash"
	}
	if req.TerminalID == "" || !isSupportedPaymentMethod(req.PaymentMethod) {
		return domain.HoldCartResponse{}, store.ErrInvalidTransaction
	}
	normalizedItems := normalizeItems(req.CartItems)
	if len(normalizedItems) == 0 {
		return domain.HoldCartResponse{}, store.ErrInvalidTransaction
	}

	actor, _ := ActorFromContext(ctx)
	held := domain.HeldCart{
		ID:                xid.New("hold"),
		StoreID:           req.StoreID,
		TerminalID:        req.TerminalID,
		CashierUsername:   actor.Username,
		Note:              req.Note,
		CartItems:         normalizedItems,
		DiscountCents:     req.DiscountCents,
		TaxRatePercent:    req.TaxRatePercent,
		PaymentMethod:     req.PaymentMethod,
		PaymentReference:  req.PaymentReference,
		PaymentSplits:     req.PaymentSplits,
		CashReceivedCents: req.CashReceivedCents,
		ManualOverride:    req.ManualOverride,
		HeldAt:            time.Now().UTC(),
	}

	if held.PaymentMethod == "split" {
		held.PaymentReference = encodePaymentSplits(held.PaymentSplits)
	}

	saved, err := s.repo.CreateHeldCart(ctx, held)
	if err != nil {
		return domain.HoldCartResponse{}, err
	}
	s.logAudit(ctx, req.StoreID, "cart_hold", "held_cart", held.ID, fmt.Sprintf("items=%d", len(held.CartItems)))
	return domain.HoldCartResponse{HeldCart: *saved}, nil
}

func (s *Service) ListHeldCarts(ctx context.Context, storeID string, terminalID string) (domain.HeldCartListResponse, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return domain.HeldCartListResponse{}, store.ErrInvalidTransaction
	}

	items, err := s.repo.ListHeldCarts(ctx, storeID, terminalID, 200)
	if err != nil {
		return domain.HeldCartListResponse{}, err
	}
	return domain.HeldCartListResponse{Items: items}, nil
}

func (s *Service) ResumeHeldCart(ctx context.Context, holdID string) (domain.HoldCartResponse, error) {
	holdID = strings.TrimSpace(holdID)
	if holdID == "" {
		return domain.HoldCartResponse{}, store.ErrInvalidTransaction
	}

	held, err := s.repo.PopHeldCart(ctx, holdID)
	if err != nil {
		return domain.HoldCartResponse{}, err
	}

	s.logAudit(ctx, held.StoreID, "cart_resume", "held_cart", held.ID, fmt.Sprintf("items=%d", len(held.CartItems)))
	return domain.HoldCartResponse{HeldCart: *held}, nil
}

func (s *Service) DiscardHeldCart(ctx context.Context, holdID string) error {
	holdID = strings.TrimSpace(holdID)
	if holdID == "" {
		return store.ErrInvalidTransaction
	}

	held, err := s.repo.PopHeldCart(ctx, holdID)
	if err != nil {
		return err
	}

	s.logAudit(ctx, held.StoreID, "cart_discard", "held_cart", held.ID, "discarded")
	return nil
}

func (s *Service) DetectOperationalAnomalies(ctx context.Context, storeID string, date string) (domain.OperationalAlertResponse, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}

	logs, err := s.ListAuditLogs(ctx, storeID, date, 500)
	if err != nil {
		return domain.OperationalAlertResponse{}, err
	}

	voidByActor := map[string]int{}
	refundByActor := map[string]int{}
	checkoutManualOverrideCount := 0
	opnameBatchCount := 0

	for _, log := range logs {
		switch log.Action {
		case "void_transaction":
			voidByActor[log.ActorUsername]++
		case "refund_transaction":
			refundByActor[log.ActorUsername]++
		case "stock_opname":
			opnameBatchCount++
		case "checkout":
			if strings.Contains(log.Detail, "manual_override=true") {
				checkoutManualOverrideCount++
			}
		}
	}

	alerts := make([]domain.OperationalAlert, 0, 16)
	for actor, count := range voidByActor {
		if count >= 3 {
			alerts = append(alerts, domain.OperationalAlert{
				ID:          xid.New("alert"),
				Code:        "void_spike",
				Severity:    "high",
				Title:       "Void transaksi meningkat",
				Description: fmt.Sprintf("Actor %s melakukan %d void transaksi dalam 1 hari.", actor, count),
				MetricValue: float64(count),
				Threshold:   3,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
	for actor, count := range refundByActor {
		if count >= 2 {
			alerts = append(alerts, domain.OperationalAlert{
				ID:          xid.New("alert"),
				Code:        "refund_spike",
				Severity:    "high",
				Title:       "Refund transaksi meningkat",
				Description: fmt.Sprintf("Actor %s melakukan %d refund dalam 1 hari.", actor, count),
				MetricValue: float64(count),
				Threshold:   2,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
	if checkoutManualOverrideCount >= 5 {
		alerts = append(alerts, domain.OperationalAlert{
			ID:          xid.New("alert"),
			Code:        "manual_override_spike",
			Severity:    "medium",
			Title:       "Manual override tinggi",
			Description: fmt.Sprintf("Terdapat %d checkout dengan manual override.", checkoutManualOverrideCount),
			MetricValue: float64(checkoutManualOverrideCount),
			Threshold:   5,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}
	if opnameBatchCount >= 3 {
		alerts = append(alerts, domain.OperationalAlert{
			ID:          xid.New("alert"),
			Code:        "stock_opname_frequency",
			Severity:    "medium",
			Title:       "Frekuensi stock opname tinggi",
			Description: fmt.Sprintf("Stock opname dijalankan %d kali hari ini.", opnameBatchCount),
			MetricValue: float64(opnameBatchCount),
			Threshold:   3,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Severity == alerts[j].Severity {
			return alerts[i].MetricValue > alerts[j].MetricValue
		}
		return severityRank(alerts[i].Severity) < severityRank(alerts[j].Severity)
	})

	reportDate := strings.TrimSpace(date)
	if reportDate == "" {
		reportDate = time.Now().UTC().Format("2006-01-02")
	}

	return domain.OperationalAlertResponse{
		StoreID: storeID,
		Date:    reportDate,
		Alerts:  alerts,
	}, nil
}

func (s *Service) CreatePromo(ctx context.Context, req domain.PromoCreateRequest) (domain.PromoRule, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.PromoRule{}, fmt.Errorf("admin role required")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.TrimSpace(req.Type)
	if req.Name == "" {
		return domain.PromoRule{}, store.ErrInvalidTransaction
	}
	if req.MinSubtotalCents < 0 || req.DiscountPercent < 0 || req.DiscountPercent > 100 || req.FlatDiscountCents < 0 {
		return domain.PromoRule{}, store.ErrInvalidTransaction
	}
	if req.Type != "cart_percent" && req.Type != "flat_cart" {
		return domain.PromoRule{}, store.ErrInvalidTransaction
	}
	if req.Type == "cart_percent" && req.DiscountPercent <= 0 {
		return domain.PromoRule{}, store.ErrInvalidTransaction
	}
	if req.Type == "flat_cart" && req.FlatDiscountCents <= 0 {
		return domain.PromoRule{}, store.ErrInvalidTransaction
	}

	rule := domain.PromoRule{
		ID:                xid.New("promo"),
		Name:              req.Name,
		Type:              req.Type,
		MinSubtotalCents:  req.MinSubtotalCents,
		DiscountPercent:   req.DiscountPercent,
		FlatDiscountCents: req.FlatDiscountCents,
		Active:            true,
		CreatedAt:         time.Now().UTC(),
	}
	saved, err := s.repo.CreatePromo(ctx, rule)
	if err != nil {
		return domain.PromoRule{}, err
	}

	s.logAudit(ctx, s.defaultStoreID, "promo_create", "promo", saved.ID, fmt.Sprintf("type=%s,name=%s", saved.Type, saved.Name))

	return *saved, nil
}

func (s *Service) ListPromos(ctx context.Context) ([]domain.PromoRule, error) {
	return s.repo.ListPromos(ctx)
}

func (s *Service) SetPromoActive(ctx context.Context, promoID string, active bool) (domain.PromoRule, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.PromoRule{}, fmt.Errorf("admin role required")
	}

	rule, err := s.repo.UpdatePromoActive(ctx, promoID, active)
	if err != nil {
		return domain.PromoRule{}, err
	}

	s.logAudit(ctx, s.defaultStoreID, "promo_toggle", "promo", promoID, fmt.Sprintf("active=%t", active))
	return *rule, nil
}

func (s *Service) CreateSupplier(ctx context.Context, req domain.SupplierCreateRequest) (domain.Supplier, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.Supplier{}, fmt.Errorf("admin role required")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Phone = strings.TrimSpace(req.Phone)
	if req.Name == "" {
		return domain.Supplier{}, store.ErrInvalidTransaction
	}

	now := time.Now().UTC()
	supplier := domain.Supplier{
		ID:        xid.New("sup"),
		Name:      req.Name,
		Phone:     req.Phone,
		CreatedAt: now,
	}

	saved, err := s.repo.CreateSupplier(ctx, supplier)
	if err != nil {
		return domain.Supplier{}, err
	}

	s.logAudit(ctx, s.defaultStoreID, "supplier_create", "supplier", saved.ID, fmt.Sprintf("name=%s", saved.Name))
	return *saved, nil
}

func (s *Service) ListSuppliers(ctx context.Context) ([]domain.Supplier, error) {
	return s.repo.ListSuppliers(ctx)
}

func (s *Service) CreatePurchaseOrder(ctx context.Context, req domain.PurchaseOrderCreateRequest) (domain.PurchaseOrderResponse, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.PurchaseOrderResponse{}, fmt.Errorf("admin role required")
	}

	if req.StoreID == "" {
		req.StoreID = s.defaultStoreID
	}
	if req.SupplierID == "" || len(req.Items) == 0 {
		return domain.PurchaseOrderResponse{}, store.ErrInvalidTransaction
	}

	normalizedItems := make([]domain.PurchaseOrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if item.SKU == "" || item.Qty < 1 || item.CostCents < 1 {
			return domain.PurchaseOrderResponse{}, store.ErrInvalidTransaction
		}
		normalizedItems = append(normalizedItems, item)
	}

	po := domain.PurchaseOrder{
		ID:         xid.New("po"),
		StoreID:    req.StoreID,
		SupplierID: req.SupplierID,
		Status:     "draft",
		CreatedAt:  time.Now().UTC(),
		Items:      normalizedItems,
	}

	saved, err := s.repo.CreatePurchaseOrder(ctx, po)
	if err != nil {
		return domain.PurchaseOrderResponse{}, err
	}
	s.logAudit(ctx, req.StoreID, "purchase_order_create", "purchase_order", saved.ID, fmt.Sprintf("items=%d", len(saved.Items)))
	return domain.PurchaseOrderResponse{PurchaseOrder: *saved}, nil
}

func (s *Service) ListPurchaseOrders(ctx context.Context, status string) (domain.PurchaseOrderListResponse, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	pos, err := s.repo.ListPurchaseOrders(ctx, s.defaultStoreID, status, 200)
	if err != nil {
		return domain.PurchaseOrderListResponse{}, err
	}
	return domain.PurchaseOrderListResponse{PurchaseOrders: pos}, nil
}

func (s *Service) ReceivePurchaseOrder(ctx context.Context, purchaseOrderID string, req domain.PurchaseOrderReceiveRequest) (domain.PurchaseOrderResponse, error) {
	actor, ok := ActorFromContext(ctx)
	if !ok || actor.Role != "admin" {
		return domain.PurchaseOrderResponse{}, fmt.Errorf("admin role required")
	}

	if purchaseOrderID == "" {
		return domain.PurchaseOrderResponse{}, store.ErrInvalidTransaction
	}
	req.ReceivedBy = strings.TrimSpace(req.ReceivedBy)
	if req.ReceivedBy == "" {
		req.ReceivedBy = actor.Username
	}

	po, err := s.repo.GetPurchaseOrderByID(ctx, purchaseOrderID)
	if err != nil {
		return domain.PurchaseOrderResponse{}, err
	}
	if po.Status == "received" {
		return domain.PurchaseOrderResponse{}, store.ErrInvalidTransaction
	}

	received, err := s.repo.ReceivePurchaseOrder(ctx, purchaseOrderID, req.ReceivedBy, time.Now().UTC())
	if err != nil {
		return domain.PurchaseOrderResponse{}, err
	}
	s.logAudit(ctx, received.StoreID, "purchase_order_receive", "purchase_order", received.ID, fmt.Sprintf("received_by=%s", req.ReceivedBy))
	return domain.PurchaseOrderResponse{PurchaseOrder: *received}, nil
}

func (s *Service) ReorderSuggestions(ctx context.Context, storeID string) (domain.ReorderSuggestionResponse, error) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}

	products, err := s.repo.ListProducts(ctx)
	if err != nil {
		return domain.ReorderSuggestionResponse{}, err
	}

	skus := make([]string, 0, len(products))
	for _, product := range products {
		skus = append(skus, product.SKU)
	}
	stockMap, err := s.repo.GetStockMap(ctx, storeID, skus)
	if err != nil {
		return domain.ReorderSuggestionResponse{}, err
	}
	costs, err := s.repo.GetProductCosts(ctx, storeID, skus)
	if err != nil {
		return domain.ReorderSuggestionResponse{}, err
	}

	suggestions := make([]domain.ReorderSuggestion, 0, 24)
	for _, product := range products {
		if !product.Active {
			continue
		}
		current := stockMap[product.SKU]
		reorderPoint := defaultReorderPoint(product)
		if current > reorderPoint {
			continue
		}
		targetStock := reorderPoint * 2
		recommendedQty := targetStock - current
		if recommendedQty < 1 {
			continue
		}
		cost := costs[product.SKU]
		if cost < 1 {
			cost = deriveUnitCost(product)
		}
		suggestions = append(suggestions, domain.ReorderSuggestion{
			SKU:                    product.SKU,
			Name:                   product.Name,
			Category:               product.Category,
			CurrentStock:           current,
			ReorderPoint:           reorderPoint,
			RecommendedQty:         recommendedQty,
			LastCostCents:          cost,
			EstimatedPurchaseCents: int64(recommendedQty) * cost,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].CurrentStock == suggestions[j].CurrentStock {
			return suggestions[i].EstimatedPurchaseCents > suggestions[j].EstimatedPurchaseCents
		}
		return suggestions[i].CurrentStock < suggestions[j].CurrentStock
	})

	return domain.ReorderSuggestionResponse{
		StoreID:     storeID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Suggestions: suggestions,
	}, nil
}

func (s *Service) RetrainAssociations(ctx context.Context, req domain.RetrainRequest) (domain.RetrainResponse, error) {
	storeID := req.StoreID
	if storeID == "" {
		storeID = s.defaultStoreID
	}

	updatedPairs, err := s.repo.RebuildAssociationPairs(ctx, storeID)
	if err != nil {
		return domain.RetrainResponse{}, err
	}

	return domain.RetrainResponse{
		UpdatedPairs: updatedPairs,
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func toCheckoutResponse(tx *domain.Transaction, duplicate bool) domain.CheckoutResponse {
	itemCount := 0
	for _, item := range tx.Items {
		itemCount += item.Qty
	}

	var recommendation *string
	if tx.RecommendationSKU != "" {
		recommendation = &tx.RecommendationSKU
	}

	paymentSplits := tx.PaymentSplits
	if len(paymentSplits) == 0 && tx.PaymentMethod == "split" {
		paymentSplits = decodePaymentSplits(tx.PaymentReference)
	}

	return domain.CheckoutResponse{
		TransactionID:  tx.ID,
		Status:         tx.Status,
		PaymentMethod:  tx.PaymentMethod,
		PaymentSplits:  paymentSplits,
		SubtotalCents:  tx.SubtotalCents,
		DiscountCents:  tx.DiscountCents,
		TaxCents:       tx.TaxCents,
		TotalCents:     tx.TotalCents,
		CashReceived:   tx.CashReceivedCents,
		ChangeCents:    tx.ChangeCents,
		ItemCount:      itemCount,
		ShiftID:        tx.ShiftID,
		Recommendation: recommendation,
		Duplicate:      duplicate,
		CreatedAt:      tx.CreatedAt.Format(time.RFC3339),
	}
}

func normalizeItems(items []domain.CartItem) []domain.CartItem {
	agg := make(map[string]int, len(items))
	for _, item := range items {
		if item.SKU == "" || item.Qty < 1 {
			continue
		}
		agg[item.SKU] += item.Qty
	}

	normalized := make([]domain.CartItem, 0, len(agg))
	for sku, qty := range agg {
		normalized = append(normalized, domain.CartItem{SKU: sku, Qty: qty})
	}
	return normalized
}

func (s *Service) calculatePromoDiscount(ctx context.Context, subtotalCents int64) (int64, error) {
	if subtotalCents < 1 {
		return 0, nil
	}

	promos, err := s.repo.ListPromos(ctx)
	if err != nil {
		return 0, err
	}

	var best int64
	for _, rule := range promos {
		if !rule.Active || subtotalCents < rule.MinSubtotalCents {
			continue
		}

		discount := int64(0)
		switch rule.Type {
		case "cart_percent":
			discount = int64(math.Round(float64(subtotalCents) * rule.DiscountPercent / 100))
		case "flat_cart":
			discount = rule.FlatDiscountCents
		}

		if discount > best {
			best = discount
		}
	}
	if best > subtotalCents {
		return subtotalCents, nil
	}
	return best, nil
}

func (s *Service) logAudit(ctx context.Context, storeID string, action string, entityType string, entityID string, detail string) {
	if storeID == "" {
		storeID = s.defaultStoreID
	}

	actor, ok := ActorFromContext(ctx)
	if !ok {
		actor = domain.Actor{Username: "system", Role: "system"}
	}

	if err := s.repo.CreateAuditLog(ctx, domain.AuditLog{
		ID:            xid.New("audit"),
		StoreID:       storeID,
		ActorUsername: actor.Username,
		ActorRole:     actor.Role,
		Action:        action,
		EntityType:    entityType,
		EntityID:      entityID,
		Detail:        detail,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		log.Printf("[audit] WARN: failed to write audit log action=%s entity=%s/%s: %v", action, entityType, entityID, err)
	}
}

func deriveUnitCost(product domain.Product) int64 {
	if product.PriceCents < 1 {
		return 0
	}
	estimated := int64(math.Round(float64(product.PriceCents) * (1 - product.MarginRate)))
	if estimated < 1 {
		return 1
	}
	return estimated
}

func defaultReorderPoint(product domain.Product) int {
	point := 30
	switch strings.ToLower(product.Category) {
	case "grocery", "beverage":
		point = 40
	case "dairy", "snack":
		point = 35
	}
	if product.MarginRate < 0.15 {
		point += 10
	}
	return point
}

func normalizePaymentSplits(splits []domain.PaymentSplit) []domain.PaymentSplit {
	normalized := make([]domain.PaymentSplit, 0, len(splits))
	for _, split := range splits {
		method := strings.ToLower(strings.TrimSpace(split.Method))
		if method == "" || split.AmountCents < 1 {
			continue
		}
		normalized = append(normalized, domain.PaymentSplit{
			Method:      method,
			AmountCents: split.AmountCents,
			Reference:   strings.TrimSpace(split.Reference),
		})
	}
	return normalized
}

func encodePaymentSplits(splits []domain.PaymentSplit) string {
	payload, err := json.Marshal(splits)
	if err != nil {
		return ""
	}
	return string(payload)
}

func decodePaymentSplits(raw string) []domain.PaymentSplit {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.HasPrefix(trimmed, "[") {
		return nil
	}
	var splits []domain.PaymentSplit
	if err := json.Unmarshal([]byte(trimmed), &splits); err != nil {
		return nil
	}
	return normalizePaymentSplits(splits)
}

func isSplitMethodSupported(method string) bool {
	switch method {
	case "cash", "card", "qris", "ewallet":
		return true
	default:
		return false
	}
}

func severityRank(severity string) int {
	switch severity {
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func ValidateStoreID(storeID string) error {
	if storeID == "" {
		return fmt.Errorf("store_id is required")
	}
	return nil
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func isSupportedPaymentMethod(method string) bool {
	switch method {
	case "cash", "card", "qris", "ewallet", "split":
		return true
	default:
		return false
	}
}
