package memory

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kasirinaja/backend/internal/domain"
	"kasirinaja/backend/internal/store"
	"kasirinaja/backend/internal/xid"
)

type Store struct {
	mu                 sync.RWMutex
	products           map[string]domain.Product
	inventory          map[string]map[string]int
	inventoryLots      map[string]map[string][]domain.InventoryLot
	associationPairs   []domain.AssociationPair
	transactionsByID   map[string]*domain.Transaction
	transactionsByIdem map[string]*domain.Transaction
	refundsByID        map[string]domain.Refund
	itemReturnsByID    map[string]domain.ItemReturn
	priceHistoryBySKU  map[string][]domain.ProductPriceHistory
	auditLogs          []domain.AuditLog
	recommendationLog  []domain.RecommendationEvent
	shiftsByID         map[string]domain.Shift
	activeShiftByKey   map[string]string
	promosByID         map[string]domain.PromoRule
	heldCartsByID      map[string]domain.HeldCart
	suppliersByID      map[string]domain.Supplier
	purchaseOrdersByID map[string]domain.PurchaseOrder
	productCosts       map[string]map[string]int64
	usersByUsername    map[string]domain.UserAccount
}

// seedUsers builds the initial in-memory user accounts for dev/demo mode.
// Credentials are read from SEED_ADMIN_PASSWORD and SEED_CASHIER_PASSWORD
// environment variables. If unset, hardcoded dev defaults are used with a
// warning printed to stdout. These credentials are never used in production
// (the backend uses PostgreSQL when DATABASE_URL is set).
func seedUsers() map[string]domain.UserAccount {
	adminPwd := envOr("SEED_ADMIN_PASSWORD", "admin123")
	cashierPwd := envOr("SEED_CASHIER_PASSWORD", "cashier123")
	if os.Getenv("SEED_ADMIN_PASSWORD") == "" || os.Getenv("SEED_CASHIER_PASSWORD") == "" {
		log.Println("[memory-store] WARNING: using default dev credentials. Set SEED_ADMIN_PASSWORD and SEED_CASHIER_PASSWORD to override.")
	}

	now := time.Now().UTC()
	users := map[string]domain.UserAccount{}
	for _, u := range []struct {
		username string
		password string
		role     string
	}{
		{"admin", adminPwd, "admin"},
		{"cashier", cashierPwd, "cashier"},
	} {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("[memory-store] failed to hash seed password for %s: %v", u.username, err)
		}
		users[u.username] = domain.UserAccount{
			Username:  u.username,
			Password:  string(hash),
			Role:      u.role,
			Active:    true,
			CreatedAt: now,
		}
	}
	return users
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func NewSeeded() *Store {
	products := []domain.Product{
		{SKU: "SKU-MIE-01", Name: "Mie Goreng Instan", Category: "grocery", PriceCents: 3500, MarginRate: 0.22, Active: true},
		{SKU: "SKU-TELUR-01", Name: "Telur 10 Butir", Category: "grocery", PriceCents: 26500, MarginRate: 0.13, Active: true},
		{SKU: "SKU-SUSU-01", Name: "Susu UHT 1L", Category: "dairy", PriceCents: 18900, MarginRate: 0.28, Active: true},
		{SKU: "SKU-ROTI-01", Name: "Roti Tawar", Category: "bakery", PriceCents: 17800, MarginRate: 0.30, Active: true},
		{SKU: "SKU-KOPI-01", Name: "Kopi Sachet", Category: "beverage", PriceCents: 2600, MarginRate: 0.34, Active: true},
		{SKU: "SKU-GULA-01", Name: "Gula 1kg", Category: "grocery", PriceCents: 17400, MarginRate: 0.12, Active: true},
		{SKU: "SKU-TEH-01", Name: "Teh Celup", Category: "beverage", PriceCents: 9800, MarginRate: 0.26, Active: true},
		{SKU: "SKU-AIR-01", Name: "Air Mineral 600ml", Category: "beverage", PriceCents: 3900, MarginRate: 0.18, Active: true},
		{SKU: "SKU-KERIPIK-01", Name: "Keripik Singkong", Category: "snack", PriceCents: 12800, MarginRate: 0.37, Active: true},
		{SKU: "SKU-COKLAT-01", Name: "Coklat Batang", Category: "snack", PriceCents: 8600, MarginRate: 0.35, Active: true},
		{SKU: "SKU-SABUN-01", Name: "Sabun Mandi", Category: "household", PriceCents: 7400, MarginRate: 0.32, Active: true},
		{SKU: "SKU-SHAMPOO-01", Name: "Shampoo Sachet", Category: "household", PriceCents: 3200, MarginRate: 0.33, Active: true},
	}

	pairs := []domain.AssociationPair{
		{SourceSKU: "SKU-MIE-01", TargetSKU: "SKU-TELUR-01", Affinity: 0.85},
		{SourceSKU: "SKU-KOPI-01", TargetSKU: "SKU-GULA-01", Affinity: 0.81},
		{SourceSKU: "SKU-ROTI-01", TargetSKU: "SKU-SUSU-01", Affinity: 0.74},
		{SourceSKU: "SKU-AIR-01", TargetSKU: "SKU-KERIPIK-01", Affinity: 0.66},
		{SourceSKU: "SKU-TEH-01", TargetSKU: "SKU-COKLAT-01", Affinity: 0.61},
		{SourceSKU: "SKU-SABUN-01", TargetSKU: "SKU-SHAMPOO-01", Affinity: 0.58},
		{SourceSKU: "SKU-TELUR-01", TargetSKU: "SKU-MIE-01", Affinity: 0.55},
		{SourceSKU: "SKU-SUSU-01", TargetSKU: "SKU-ROTI-01", Affinity: 0.52},
		{SourceSKU: "SKU-KERIPIK-01", TargetSKU: "SKU-AIR-01", Affinity: 0.47},
	}

	productMap := make(map[string]domain.Product, len(products))
	inventory := make(map[string]map[string]int)
	inventory["main-store"] = make(map[string]int)
	for _, p := range products {
		productMap[p.SKU] = p
		inventory["main-store"][p.SKU] = 120
	}

	return &Store{
		products:           productMap,
		inventory:          inventory,
		inventoryLots:      map[string]map[string][]domain.InventoryLot{"main-store": {}},
		associationPairs:   pairs,
		transactionsByID:   make(map[string]*domain.Transaction),
		transactionsByIdem: make(map[string]*domain.Transaction),
		refundsByID:        make(map[string]domain.Refund),
		itemReturnsByID:    make(map[string]domain.ItemReturn),
		priceHistoryBySKU:  make(map[string][]domain.ProductPriceHistory),
		auditLogs:          make([]domain.AuditLog, 0, 128),
		recommendationLog:  make([]domain.RecommendationEvent, 0, 64),
		shiftsByID:         make(map[string]domain.Shift),
		activeShiftByKey:   make(map[string]string),
		promosByID:         make(map[string]domain.PromoRule),
		heldCartsByID:      make(map[string]domain.HeldCart),
		suppliersByID:      make(map[string]domain.Supplier),
		purchaseOrdersByID: make(map[string]domain.PurchaseOrder),
		productCosts:       map[string]map[string]int64{"main-store": {}},
		usersByUsername: seedUsers(),
	}
}

func (s *Store) ListProducts(_ context.Context) ([]domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]domain.Product, 0, len(s.products))
	for _, p := range s.products {
		if !p.Active {
			continue
		}
		products = append(products, p)
	}

	slices.SortFunc(products, func(a, b domain.Product) int {
		if a.Category == b.Category {
			return cmpString(a.Name, b.Name)
		}
		return cmpString(a.Category, b.Category)
	})

	return products, nil
}

func (s *Store) CreateProduct(_ context.Context, product domain.Product) (*domain.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if product.SKU == "" || product.Name == "" || product.Category == "" || product.PriceCents < 1 {
		return nil, store.ErrInvalidTransaction
	}
	if product.MarginRate < 0 || product.MarginRate > 1 {
		return nil, store.ErrInvalidTransaction
	}
	if _, exists := s.products[product.SKU]; exists {
		return nil, store.ErrInvalidTransaction
	}

	product.Active = true
	s.products[product.SKU] = product
	created := product
	return &created, nil
}

func (s *Store) GetProductBySKU(_ context.Context, sku string) (*domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	product, exists := s.products[sku]
	if !exists {
		return nil, store.ErrNotFound
	}
	copyProduct := product
	return &copyProduct, nil
}

func (s *Store) UpdateProduct(_ context.Context, product domain.Product) (*domain.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if product.SKU == "" || product.Name == "" || product.Category == "" || product.PriceCents < 1 {
		return nil, store.ErrInvalidTransaction
	}
	if product.MarginRate < 0 || product.MarginRate > 1 {
		return nil, store.ErrInvalidTransaction
	}
	if _, exists := s.products[product.SKU]; !exists {
		return nil, store.ErrNotFound
	}

	s.products[product.SKU] = product
	updated := product
	return &updated, nil
}

func (s *Store) CreatePriceHistory(_ context.Context, entry domain.ProductPriceHistory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = xid.New("ph")
	}
	if entry.ChangedAt.IsZero() {
		entry.ChangedAt = time.Now().UTC()
	}
	s.priceHistoryBySKU[entry.SKU] = append(s.priceHistoryBySKU[entry.SKU], entry)
	return nil
}

func (s *Store) ListPriceHistory(_ context.Context, sku string, limit int) ([]domain.ProductPriceHistory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	history := s.priceHistoryBySKU[sku]
	if len(history) == 0 {
		return []domain.ProductPriceHistory{}, nil
	}

	result := make([]domain.ProductPriceHistory, len(history))
	copy(result, history)
	slices.SortFunc(result, func(a, b domain.ProductPriceHistory) int {
		if a.ChangedAt.Equal(b.ChangedAt) {
			return cmpString(b.ID, a.ID)
		}
		if a.ChangedAt.After(b.ChangedAt) {
			return -1
		}
		return 1
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) GetProductsBySKUs(_ context.Context, skus []string) (map[string]domain.Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]domain.Product, len(skus))
	for _, sku := range skus {
		if p, ok := s.products[sku]; ok && p.Active {
			result[sku] = p
		}
	}
	return result, nil
}

func (s *Store) GetStockMap(_ context.Context, storeID string, skus []string) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stockMap := make(map[string]int, len(skus))
	storeStock := s.inventory[storeID]
	for _, sku := range skus {
		if storeStock == nil {
			stockMap[sku] = 0
			continue
		}
		stockMap[sku] = storeStock[sku]
	}
	return stockMap, nil
}

func (s *Store) SetStock(_ context.Context, storeID string, sku string, qty int) error {
	if sku == "" || qty < 0 {
		return store.ErrInvalidTransaction
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.products[sku]; !exists {
		return fmt.Errorf("sku %s unavailable", sku)
	}
	storeStock, ok := s.inventory[storeID]
	if !ok {
		storeStock = make(map[string]int)
		s.inventory[storeID] = storeStock
	}
	storeStock[sku] = qty
	return nil
}

func (s *Store) CreateInventoryLot(_ context.Context, lot domain.InventoryLot) (*domain.InventoryLot, error) {
	if lot.StoreID == "" || lot.SKU == "" || lot.QtyReceived < 1 || lot.CostCents < 1 {
		return nil, store.ErrInvalidTransaction
	}
	if lot.ID == "" {
		lot.ID = xid.New("lot")
	}
	if strings.TrimSpace(lot.LotCode) == "" {
		lot.LotCode = "MANUAL-" + lot.ID
	}
	if lot.QtyAvailable < 0 || lot.QtyAvailable > lot.QtyReceived {
		return nil, store.ErrInvalidTransaction
	}
	if lot.QtyAvailable == 0 {
		lot.QtyAvailable = lot.QtyReceived
	}
	if lot.SourceType == "" {
		lot.SourceType = "manual"
	}
	if lot.ReceivedAt.IsZero() {
		lot.ReceivedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.products[lot.SKU]; !exists {
		return nil, store.ErrNotFound
	}
	if _, ok := s.inventory[lot.StoreID]; !ok {
		s.inventory[lot.StoreID] = map[string]int{}
	}
	if _, ok := s.inventoryLots[lot.StoreID]; !ok {
		s.inventoryLots[lot.StoreID] = map[string][]domain.InventoryLot{}
	}

	s.inventoryLots[lot.StoreID][lot.SKU] = append(s.inventoryLots[lot.StoreID][lot.SKU], lot)
	s.inventory[lot.StoreID][lot.SKU] += lot.QtyAvailable
	created := cloneInventoryLot(lot)
	return &created, nil
}

func (s *Store) ListInventoryLots(_ context.Context, storeID string, sku string, includeExpired bool, limit int) ([]domain.InventoryLot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit < 1 {
		limit = 200
	}
	today := nowDateUTC(time.Now().UTC())
	result := make([]domain.InventoryLot, 0, limit)

	appendLot := func(lot domain.InventoryLot) {
		if !includeExpired && lot.ExpiryDate != nil && lot.ExpiryDate.Before(today) {
			return
		}
		result = append(result, cloneInventoryLot(lot))
	}

	if storeID != "" && sku != "" {
		for _, lot := range s.inventoryLots[storeID][sku] {
			appendLot(lot)
		}
	} else if storeID != "" {
		for _, lots := range s.inventoryLots[storeID] {
			for _, lot := range lots {
				appendLot(lot)
			}
		}
	} else {
		for _, bySKU := range s.inventoryLots {
			for _, lots := range bySKU {
				for _, lot := range lots {
					appendLot(lot)
				}
			}
		}
	}

	slices.SortFunc(result, compareLotForFEFO)
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) IncreaseStock(_ context.Context, storeID string, adjustments []domain.StockAdjustment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	storeStock, ok := s.inventory[storeID]
	if !ok {
		storeStock = make(map[string]int)
		s.inventory[storeID] = storeStock
	}

	for _, adj := range adjustments {
		if adj.Qty < 1 {
			continue
		}
		if _, exists := s.products[adj.SKU]; !exists {
			return fmt.Errorf("sku %s unavailable", adj.SKU)
		}
		storeStock[adj.SKU] += adj.Qty
	}

	return nil
}

func (s *Store) GetAssociationPairs(_ context.Context, sourceSKUs []string) ([]domain.AssociationPair, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sourceSet := make(map[string]struct{}, len(sourceSKUs))
	for _, sku := range sourceSKUs {
		sourceSet[sku] = struct{}{}
	}

	pairs := make([]domain.AssociationPair, 0, len(s.associationPairs))
	for _, pair := range s.associationPairs {
		if _, ok := sourceSet[pair.SourceSKU]; ok {
			pairs = append(pairs, pair)
		}
	}

	return pairs, nil
}

func (s *Store) FindTransactionByIdempotency(_ context.Context, key string) (*domain.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tx, ok := s.transactionsByIdem[key]
	if !ok {
		return nil, store.ErrNotFound
	}
	return cloneTransaction(tx), nil
}

func (s *Store) FindTransactionByID(_ context.Context, id string) (*domain.Transaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tx, ok := s.transactionsByID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return cloneTransaction(tx), nil
}

func (s *Store) CreateCheckout(_ context.Context, tx domain.Transaction) (*domain.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tx.IdempotencyKey == "" {
		return nil, store.ErrInvalidTransaction
	}

	if existing, ok := s.transactionsByIdem[tx.IdempotencyKey]; ok {
		return cloneTransaction(existing), nil
	}

	if len(tx.Items) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	storeStock, ok := s.inventory[tx.StoreID]
	if !ok {
		return nil, fmt.Errorf("store %s unavailable", tx.StoreID)
	}
	if _, ok := s.inventoryLots[tx.StoreID]; !ok {
		s.inventoryLots[tx.StoreID] = map[string][]domain.InventoryLot{}
	}
	today := nowDateUTC(time.Now().UTC())

	subtotal := int64(0)
	recomputedItems := make([]domain.TransactionLine, 0, len(tx.Items))
	for _, item := range tx.Items {
		if item.Qty < 1 {
			return nil, store.ErrInvalidTransaction
		}
		product, exists := s.products[item.SKU]
		if !exists || !product.Active {
			return nil, fmt.Errorf("sku %s unavailable", item.SKU)
		}
		remaining := storeStock[item.SKU] - item.Qty
		if remaining < 0 {
			return nil, store.ErrInsufficientStock
		}
		lots := s.inventoryLots[tx.StoreID][item.SKU]
		if len(lots) > 0 {
			availableByLot := 0
			for _, lot := range lots {
				if lot.ExpiryDate != nil && lot.ExpiryDate.Before(today) {
					continue
				}
				availableByLot += lot.QtyAvailable
			}
			if availableByLot < item.Qty {
				return nil, store.ErrInsufficientStock
			}
		}
		recomputedItems = append(recomputedItems, domain.TransactionLine{
			SKU:            item.SKU,
			Qty:            item.Qty,
			UnitPriceCents: product.PriceCents,
			MarginRate:     product.MarginRate,
		})
		subtotal += int64(item.Qty) * product.PriceCents
	}

	if tx.DiscountCents < 0 || tx.DiscountCents > subtotal {
		return nil, store.ErrInvalidTransaction
	}
	if tx.TaxRatePercent < 0 || tx.TaxRatePercent > 100 {
		return nil, store.ErrInvalidTransaction
	}

	taxBase := subtotal - tx.DiscountCents
	taxCents := int64(math.Round(float64(taxBase) * tx.TaxRatePercent / 100))
	total := taxBase + taxCents

	if tx.ID == "" {
		tx.ID = xid.New("tx")
	}
	if tx.CreatedAt.IsZero() {
		tx.CreatedAt = time.Now().UTC()
	}
	tx.Items = recomputedItems
	tx.SubtotalCents = subtotal
	tx.TaxCents = taxCents
	tx.TotalCents = total
	if tx.Status == "" {
		tx.Status = domain.TxStatusPaid
	}

	if tx.PaymentMethod == "cash" {
		if tx.CashReceivedCents < tx.TotalCents {
			return nil, store.ErrInvalidTransaction
		}
		tx.ChangeCents = tx.CashReceivedCents - tx.TotalCents
	} else {
		tx.ChangeCents = 0
	}

	for _, item := range tx.Items {
		storeStock[item.SKU] -= item.Qty
		lots := s.inventoryLots[tx.StoreID][item.SKU]
		if len(lots) == 0 {
			continue
		}
		slices.SortFunc(lots, compareLotForFEFO)
		remaining := item.Qty
		for i := range lots {
			if remaining == 0 {
				break
			}
			if lots[i].QtyAvailable < 1 {
				continue
			}
			if lots[i].ExpiryDate != nil && lots[i].ExpiryDate.Before(today) {
				continue
			}
			used := remaining
			if used > lots[i].QtyAvailable {
				used = lots[i].QtyAvailable
			}
			lots[i].QtyAvailable -= used
			remaining -= used
		}
		s.inventoryLots[tx.StoreID][item.SKU] = lots
	}

	txCopy := cloneTransaction(&tx)
	s.transactionsByID[tx.ID] = txCopy
	s.transactionsByIdem[tx.IdempotencyKey] = txCopy

	return cloneTransaction(txCopy), nil
}

func (s *Store) VoidTransaction(_ context.Context, id string, reason string, at time.Time) (*domain.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, ok := s.transactionsByID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	if tx.Status != domain.TxStatusPaid {
		return nil, store.ErrInvalidTransaction
	}

	storeStock := s.inventory[tx.StoreID]
	if _, ok := s.inventoryLots[tx.StoreID]; !ok {
		s.inventoryLots[tx.StoreID] = map[string][]domain.InventoryLot{}
	}
	for _, item := range tx.Items {
		storeStock[item.SKU] += item.Qty
		lot := domain.InventoryLot{
			ID:           xid.New("lot"),
			StoreID:      tx.StoreID,
			SKU:          item.SKU,
			LotCode:      "VOID-" + tx.ID,
			QtyReceived:  item.Qty,
			QtyAvailable: item.Qty,
			CostCents:    maxInt64(1, item.UnitPriceCents),
			SourceType:   "void",
			SourceID:     tx.ID,
			Notes:        "auto restock from void",
			ReceivedAt:   at,
		}
		s.inventoryLots[tx.StoreID][item.SKU] = append(s.inventoryLots[tx.StoreID][item.SKU], lot)
	}

	tx.Status = domain.TxStatusVoided
	tx.VoidReason = reason
	tx.VoidedAt = &at

	return cloneTransaction(tx), nil
}

func (s *Store) CreateRefund(_ context.Context, refund domain.Refund) (*domain.Refund, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if refund.ID == "" {
		refund.ID = xid.New("refund")
	}
	if refund.CreatedAt.IsZero() {
		refund.CreatedAt = time.Now().UTC()
	}
	if refund.Status == "" {
		refund.Status = domain.TxStatusRefunded
	}

	tx, ok := s.transactionsByID[refund.OriginalTransactionID]
	if !ok {
		return nil, store.ErrNotFound
	}
	if tx.Status == domain.TxStatusVoided || tx.Status == domain.TxStatusRefunded {
		return nil, store.ErrInvalidTransaction
	}
	refundedSoFar := int64(0)
	for _, item := range s.refundsByID {
		if item.OriginalTransactionID != refund.OriginalTransactionID || item.Status != domain.TxStatusRefunded {
			continue
		}
		refundedSoFar += item.AmountCents
	}
	remaining := tx.TotalCents - refundedSoFar
	if refund.AmountCents > remaining {
		return nil, store.ErrInvalidTransaction
	}
	if refundedSoFar+refund.AmountCents >= tx.TotalCents {
		tx.Status = domain.TxStatusRefunded
	}

	s.refundsByID[refund.ID] = refund
	return &refund, nil
}

func (s *Store) GetReturnedQtyByTransaction(_ context.Context, transactionID string) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]int)
	for _, itemReturn := range s.itemReturnsByID {
		if itemReturn.OriginalTransactionID != transactionID {
			continue
		}
		for _, line := range itemReturn.ReturnItems {
			result[line.SKU] += line.Qty
		}
	}
	return result, nil
}

func (s *Store) CreateItemReturn(_ context.Context, itemReturn domain.ItemReturn) (*domain.ItemReturn, error) {
	if itemReturn.ID == "" {
		itemReturn.ID = xid.New("ret")
	}
	if itemReturn.CreatedAt.IsZero() {
		itemReturn.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(itemReturn.OriginalTransactionID) == "" || len(itemReturn.ReturnItems) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.itemReturnsByID[itemReturn.ID] = cloneItemReturn(itemReturn)
	created := cloneItemReturn(itemReturn)
	return &created, nil
}

func (s *Store) CreateRecommendationEvent(_ context.Context, event domain.RecommendationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.recommendationLog = append(s.recommendationLog, event)
	return nil
}

func (s *Store) GetAttachMetrics(_ context.Context, storeID string, from time.Time, to time.Time) (domain.AttachMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := domain.AttachMetrics{}
	for _, tx := range s.transactionsByID {
		if tx.StoreID != storeID {
			continue
		}
		if tx.CreatedAt.Before(from) || tx.CreatedAt.After(to) {
			continue
		}
		if tx.Status != domain.TxStatusPaid && tx.Status != domain.TxStatusRefunded {
			continue
		}
		metrics.Transactions++
		if tx.RecommendationAccepted {
			metrics.Accepted++
		}
	}

	if metrics.Transactions > 0 {
		metrics.AttachRate = (float64(metrics.Accepted) / float64(metrics.Transactions)) * 100
	}

	return metrics, nil
}

func (s *Store) GetDailyReport(_ context.Context, storeID string, from time.Time, to time.Time) (domain.DailyReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report := domain.DailyReport{
		StoreID:    storeID,
		ByPayment:  make([]domain.DailyReportPayment, 0, 4),
		ByTerminal: make([]domain.DailyReportTerminal, 0, 8),
	}
	byPayment := map[string]*domain.DailyReportPayment{}
	byTerminal := map[string]*domain.DailyReportTerminal{}

	for _, tx := range s.transactionsByID {
		if tx.StoreID != storeID {
			continue
		}
		if tx.CreatedAt.Before(from) || !tx.CreatedAt.Before(to) {
			continue
		}
		if tx.Status == domain.TxStatusVoided {
			continue
		}

		report.Transactions++
		report.GrossSalesCents += tx.SubtotalCents
		report.DiscountCents += tx.DiscountCents
		report.TaxCents += tx.TaxCents
		report.NetSalesCents += tx.TotalCents
		for _, item := range tx.Items {
			margin := int64(math.Round(float64(item.UnitPriceCents*int64(item.Qty)) * item.MarginRate))
			report.EstimatedMarginCents += margin
		}

		payment := byPayment[tx.PaymentMethod]
		if payment == nil {
			payment = &domain.DailyReportPayment{PaymentMethod: tx.PaymentMethod}
			byPayment[tx.PaymentMethod] = payment
		}
		payment.Transactions++
		payment.TotalCents += tx.TotalCents

		terminal := byTerminal[tx.TerminalID]
		if terminal == nil {
			terminal = &domain.DailyReportTerminal{TerminalID: tx.TerminalID}
			byTerminal[tx.TerminalID] = terminal
		}
		terminal.Transactions++
		terminal.TotalCents += tx.TotalCents
	}

	for _, entry := range byPayment {
		report.ByPayment = append(report.ByPayment, *entry)
	}
	for _, entry := range byTerminal {
		report.ByTerminal = append(report.ByTerminal, *entry)
	}

	slices.SortFunc(report.ByPayment, func(a, b domain.DailyReportPayment) int {
		return cmpString(a.PaymentMethod, b.PaymentMethod)
	})
	slices.SortFunc(report.ByTerminal, func(a, b domain.DailyReportTerminal) int {
		return cmpString(a.TerminalID, b.TerminalID)
	})

	return report, nil
}

func (s *Store) CreateAuditLog(_ context.Context, entry domain.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry.ID == "" {
		entry.ID = xid.New("audit")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.auditLogs = append(s.auditLogs, entry)
	return nil
}

func (s *Store) ListAuditLogs(_ context.Context, storeID string, from time.Time, to time.Time, limit int) ([]domain.AuditLog, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.AuditLog, 0, 64)
	for _, entry := range s.auditLogs {
		if storeID != "" && entry.StoreID != storeID {
			continue
		}
		if entry.CreatedAt.Before(from) || !entry.CreatedAt.Before(to) {
			continue
		}
		result = append(result, entry)
	}

	slices.SortFunc(result, func(a, b domain.AuditLog) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return cmpString(b.ID, a.ID)
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) CreateShift(_ context.Context, shift domain.Shift) (*domain.Shift, error) {
	if strings.TrimSpace(shift.StoreID) == "" || strings.TrimSpace(shift.TerminalID) == "" {
		return nil, store.ErrInvalidTransaction
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := shiftMapKey(shift.StoreID, shift.TerminalID)
	if _, exists := s.activeShiftByKey[key]; exists {
		return nil, store.ErrInvalidTransaction
	}
	if shift.ID == "" {
		shift.ID = xid.New("shift")
	}
	if shift.OpenedAt.IsZero() {
		shift.OpenedAt = time.Now().UTC()
	}
	shift.Status = domain.ShiftStatusOpen
	shift.ClosedAt = nil
	shift.ClosingCashCents = 0

	s.shiftsByID[shift.ID] = shift
	s.activeShiftByKey[key] = shift.ID
	copyShift := shift
	return &copyShift, nil
}

func (s *Store) CloseActiveShift(_ context.Context, storeID string, terminalID string, closingCashCents int64, closedAt time.Time) (*domain.Shift, error) {
	if strings.TrimSpace(storeID) == "" || strings.TrimSpace(terminalID) == "" {
		return nil, store.ErrInvalidTransaction
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := shiftMapKey(storeID, terminalID)
	shiftID, exists := s.activeShiftByKey[key]
	if !exists {
		return nil, store.ErrNotFound
	}
	shift, exists := s.shiftsByID[shiftID]
	if !exists || shift.Status != domain.ShiftStatusOpen {
		return nil, store.ErrNotFound
	}
	if closedAt.IsZero() {
		closedAt = time.Now().UTC()
	}
	shift.Status = domain.ShiftStatusClosed
	shift.ClosingCashCents = closingCashCents
	shift.ClosedAt = &closedAt

	delete(s.activeShiftByKey, key)
	s.shiftsByID[shiftID] = shift
	copyShift := shift
	return &copyShift, nil
}

func (s *Store) GetActiveShift(_ context.Context, storeID string, terminalID string) (*domain.Shift, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := shiftMapKey(storeID, terminalID)
	shiftID, exists := s.activeShiftByKey[key]
	if !exists {
		return nil, store.ErrNotFound
	}
	shift, exists := s.shiftsByID[shiftID]
	if !exists || shift.Status != domain.ShiftStatusOpen {
		return nil, store.ErrNotFound
	}
	copyShift := shift
	return &copyShift, nil
}

func (s *Store) CreatePromo(_ context.Context, promo domain.PromoRule) (*domain.PromoRule, error) {
	if strings.TrimSpace(promo.Name) == "" {
		return nil, store.ErrInvalidTransaction
	}
	if promo.Type != "cart_percent" && promo.Type != "flat_cart" {
		return nil, store.ErrInvalidTransaction
	}
	if promo.Type == "cart_percent" && promo.DiscountPercent <= 0 {
		return nil, store.ErrInvalidTransaction
	}
	if promo.Type == "flat_cart" && promo.FlatDiscountCents <= 0 {
		return nil, store.ErrInvalidTransaction
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if promo.ID == "" {
		promo.ID = xid.New("promo")
	}
	if promo.CreatedAt.IsZero() {
		promo.CreatedAt = time.Now().UTC()
	}
	promo.Active = true
	s.promosByID[promo.ID] = promo
	copyPromo := promo
	return &copyPromo, nil
}

func (s *Store) ListPromos(_ context.Context) ([]domain.PromoRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	promos := make([]domain.PromoRule, 0, len(s.promosByID))
	for _, promo := range s.promosByID {
		promos = append(promos, promo)
	}
	slices.SortFunc(promos, func(a, b domain.PromoRule) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return cmpString(a.ID, b.ID)
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return promos, nil
}

func (s *Store) UpdatePromoActive(_ context.Context, promoID string, active bool) (*domain.PromoRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	promo, exists := s.promosByID[promoID]
	if !exists {
		return nil, store.ErrNotFound
	}
	promo.Active = active
	s.promosByID[promoID] = promo
	copyPromo := promo
	return &copyPromo, nil
}

func (s *Store) RebuildAssociationPairs(_ context.Context, storeID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sourceCount := map[string]int{}
	pairCount := map[string]int{}

	for _, tx := range s.transactionsByID {
		if tx.StoreID != storeID || tx.Status != domain.TxStatusPaid {
			continue
		}
		seen := map[string]struct{}{}
		for _, item := range tx.Items {
			seen[item.SKU] = struct{}{}
		}
		skus := make([]string, 0, len(seen))
		for sku := range seen {
			skus = append(skus, sku)
			sourceCount[sku]++
		}
		for _, source := range skus {
			for _, target := range skus {
				if source == target {
					continue
				}
				key := source + "->" + target
				pairCount[key]++
			}
		}
	}

	nextPairs := make([]domain.AssociationPair, 0, len(pairCount))
	for key, cnt := range pairCount {
		arrow := -1
		for i := 0; i+1 < len(key); i++ {
			if key[i] == '-' && key[i+1] == '>' {
				arrow = i
				break
			}
		}
		if arrow < 1 {
			continue
		}
		source := key[:arrow]
		target := key[arrow+2:]
		srcCnt := sourceCount[source]
		if srcCnt < 1 {
			continue
		}
		affinity := float64(cnt) / float64(srcCnt)
		if affinity < 0.2 {
			continue
		}
		nextPairs = append(nextPairs, domain.AssociationPair{
			SourceSKU: source,
			TargetSKU: target,
			Affinity:  affinity,
		})
	}

	slices.SortFunc(nextPairs, func(a, b domain.AssociationPair) int {
		if a.SourceSKU == b.SourceSKU {
			if a.Affinity == b.Affinity {
				return cmpString(a.TargetSKU, b.TargetSKU)
			}
			if a.Affinity > b.Affinity {
				return -1
			}
			return 1
		}
		return cmpString(a.SourceSKU, b.SourceSKU)
	})

	if len(nextPairs) > 250 {
		nextPairs = nextPairs[:250]
	}
	if len(nextPairs) > 0 {
		s.associationPairs = nextPairs
	}

	return len(nextPairs), nil
}

func (s *Store) CreateHeldCart(_ context.Context, held domain.HeldCart) (*domain.HeldCart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if held.ID == "" {
		held.ID = xid.New("hold")
	}
	if held.HeldAt.IsZero() {
		held.HeldAt = time.Now().UTC()
	}
	if held.StoreID == "" || held.TerminalID == "" || len(held.CartItems) == 0 {
		return nil, store.ErrInvalidTransaction
	}

	s.heldCartsByID[held.ID] = cloneHeldCart(held)
	saved := cloneHeldCart(s.heldCartsByID[held.ID])
	return &saved, nil
}

func (s *Store) ListHeldCarts(_ context.Context, storeID string, terminalID string, limit int) ([]domain.HeldCart, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]domain.HeldCart, 0, 64)
	for _, held := range s.heldCartsByID {
		if storeID != "" && held.StoreID != storeID {
			continue
		}
		if terminalID != "" && held.TerminalID != terminalID {
			continue
		}
		result = append(result, cloneHeldCart(held))
	}
	slices.SortFunc(result, func(a, b domain.HeldCart) int {
		if a.HeldAt.Equal(b.HeldAt) {
			return cmpString(b.ID, a.ID)
		}
		if a.HeldAt.After(b.HeldAt) {
			return -1
		}
		return 1
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) PopHeldCart(_ context.Context, holdID string) (*domain.HeldCart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	held, exists := s.heldCartsByID[holdID]
	if !exists {
		return nil, store.ErrNotFound
	}
	delete(s.heldCartsByID, holdID)
	result := cloneHeldCart(held)
	return &result, nil
}

func (s *Store) DeleteHeldCart(_ context.Context, holdID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.heldCartsByID[holdID]; !exists {
		return store.ErrNotFound
	}
	delete(s.heldCartsByID, holdID)
	return nil
}

func (s *Store) CreateSupplier(_ context.Context, supplier domain.Supplier) (*domain.Supplier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	supplier.Name = strings.TrimSpace(supplier.Name)
	if supplier.Name == "" {
		return nil, store.ErrInvalidTransaction
	}
	if supplier.ID == "" {
		supplier.ID = xid.New("sup")
	}
	if supplier.CreatedAt.IsZero() {
		supplier.CreatedAt = time.Now().UTC()
	}

	s.suppliersByID[supplier.ID] = supplier
	copySupplier := supplier
	return &copySupplier, nil
}

func (s *Store) ListSuppliers(_ context.Context) ([]domain.Supplier, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	suppliers := make([]domain.Supplier, 0, len(s.suppliersByID))
	for _, supplier := range s.suppliersByID {
		suppliers = append(suppliers, supplier)
	}
	slices.SortFunc(suppliers, func(a, b domain.Supplier) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return cmpString(a.Name, b.Name)
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return suppliers, nil
}

func (s *Store) CreatePurchaseOrder(_ context.Context, po domain.PurchaseOrder) (*domain.PurchaseOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if po.StoreID == "" || po.SupplierID == "" || len(po.Items) == 0 {
		return nil, store.ErrInvalidTransaction
	}
	if _, exists := s.suppliersByID[po.SupplierID]; !exists {
		return nil, store.ErrNotFound
	}
	if po.ID == "" {
		po.ID = xid.New("po")
	}
	if po.CreatedAt.IsZero() {
		po.CreatedAt = time.Now().UTC()
	}
	if po.Status == "" {
		po.Status = "draft"
	}

	items := make([]domain.PurchaseOrderItem, 0, len(po.Items))
	for _, item := range po.Items {
		item.SKU = strings.ToUpper(strings.TrimSpace(item.SKU))
		if item.SKU == "" || item.Qty < 1 || item.CostCents < 1 {
			return nil, store.ErrInvalidTransaction
		}
		items = append(items, item)
	}
	po.Items = items

	s.purchaseOrdersByID[po.ID] = clonePurchaseOrder(po)
	saved := clonePurchaseOrder(s.purchaseOrdersByID[po.ID])
	return &saved, nil
}

func (s *Store) GetPurchaseOrderByID(_ context.Context, purchaseOrderID string) (*domain.PurchaseOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	po, exists := s.purchaseOrdersByID[purchaseOrderID]
	if !exists {
		return nil, store.ErrNotFound
	}
	copyPO := clonePurchaseOrder(po)
	return &copyPO, nil
}

func (s *Store) ListPurchaseOrders(_ context.Context, storeID string, status string, limit int) ([]domain.PurchaseOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status = strings.ToLower(strings.TrimSpace(status))
	result := make([]domain.PurchaseOrder, 0, len(s.purchaseOrdersByID))
	for _, po := range s.purchaseOrdersByID {
		if storeID != "" && po.StoreID != storeID {
			continue
		}
		if status != "" && po.Status != status {
			continue
		}
		result = append(result, clonePurchaseOrder(po))
	}
	slices.SortFunc(result, func(a, b domain.PurchaseOrder) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			return cmpString(b.ID, a.ID)
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return -1
		}
		return 1
	})
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *Store) ReceivePurchaseOrder(_ context.Context, purchaseOrderID string, receivedBy string, receivedAt time.Time) (*domain.PurchaseOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	po, exists := s.purchaseOrdersByID[purchaseOrderID]
	if !exists {
		return nil, store.ErrNotFound
	}
	if po.Status == "received" {
		return nil, store.ErrInvalidTransaction
	}
	if po.Status == "cancelled" {
		return nil, store.ErrInvalidTransaction
	}
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	storeStock, ok := s.inventory[po.StoreID]
	if !ok {
		storeStock = make(map[string]int)
		s.inventory[po.StoreID] = storeStock
	}
	if _, ok := s.productCosts[po.StoreID]; !ok {
		s.productCosts[po.StoreID] = make(map[string]int64)
	}
	storeCosts := s.productCosts[po.StoreID]
	if _, ok := s.inventoryLots[po.StoreID]; !ok {
		s.inventoryLots[po.StoreID] = map[string][]domain.InventoryLot{}
	}

	for idx, item := range po.Items {
		if item.Qty < 1 || item.CostCents < 1 {
			return nil, store.ErrInvalidTransaction
		}
		currentQty := storeStock[item.SKU]
		prevCost := storeCosts[item.SKU]
		if prevCost < 1 {
			prevCost = item.CostCents
		}
		storeStock[item.SKU] = currentQty + item.Qty
		storeCosts[item.SKU] = weightedCostCents(prevCost, currentQty, item.CostCents, item.Qty)
		lot := domain.InventoryLot{
			ID:           xid.New("lot"),
			StoreID:      po.StoreID,
			SKU:          item.SKU,
			LotCode:      fmt.Sprintf("PO-%s-%02d", po.ID, idx+1),
			QtyReceived:  item.Qty,
			QtyAvailable: item.Qty,
			CostCents:    item.CostCents,
			SourceType:   "purchase_order",
			SourceID:     po.ID,
			Notes:        "auto lot from purchase order receive",
			ReceivedAt:   receivedAt,
		}
		s.inventoryLots[po.StoreID][item.SKU] = append(s.inventoryLots[po.StoreID][item.SKU], lot)
	}

	po.Status = "received"
	po.ReceivedBy = strings.TrimSpace(receivedBy)
	if po.ReceivedBy == "" {
		po.ReceivedBy = "system"
	}
	po.ReceivedAt = &receivedAt
	s.purchaseOrdersByID[purchaseOrderID] = po
	updated := clonePurchaseOrder(po)
	return &updated, nil
}

func (s *Store) GetProductCosts(_ context.Context, storeID string, skus []string) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]int64, len(skus))
	storeCosts := s.productCosts[storeID]
	for _, sku := range skus {
		if storeCosts == nil {
			result[sku] = 0
			continue
		}
		result[sku] = storeCosts[sku]
	}
	return result, nil
}

func (s *Store) UpsertProductCost(_ context.Context, storeID string, sku string, costCents int64) error {
	if sku == "" || costCents < 1 {
		return store.ErrInvalidTransaction
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.products[sku]; !exists {
		return store.ErrNotFound
	}
	if _, ok := s.productCosts[storeID]; !ok {
		s.productCosts[storeID] = make(map[string]int64)
	}
	s.productCosts[storeID][sku] = costCents
	return nil
}

func (s *Store) CreateUser(_ context.Context, user domain.UserAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	username := strings.ToLower(strings.TrimSpace(user.Username))
	if username == "" || strings.TrimSpace(user.Password) == "" {
		return store.ErrInvalidTransaction
	}
	if _, exists := s.usersByUsername[username]; exists {
		return store.ErrInvalidTransaction
	}
	user.Username = username
	if user.Role == "" {
		user.Role = "cashier"
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	user.Active = true
	s.usersByUsername[user.Username] = user
	return nil
}

func (s *Store) ListUsers(_ context.Context) ([]domain.UserAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]domain.UserAccount, 0, len(s.usersByUsername))
	for _, user := range s.usersByUsername {
		users = append(users, user)
	}
	slices.SortFunc(users, func(a, b domain.UserAccount) int {
		return cmpString(a.Username, b.Username)
	})
	return users, nil
}

func (s *Store) UpdateUserPassword(_ context.Context, username string, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || strings.TrimSpace(password) == "" {
		return store.ErrInvalidTransaction
	}
	user, exists := s.usersByUsername[username]
	if !exists {
		return store.ErrNotFound
	}
	user.Password = password
	s.usersByUsername[username] = user
	return nil
}

func weightedCostCents(oldCost int64, oldQty int, incomingCost int64, incomingQty int) int64 {
	if incomingQty <= 0 || incomingCost <= 0 {
		return oldCost
	}
	if oldQty <= 0 || oldCost <= 0 {
		return incomingCost
	}
	totalQty := oldQty + incomingQty
	if totalQty <= 0 {
		return incomingCost
	}
	totalValue := oldCost*int64(oldQty) + incomingCost*int64(incomingQty)
	weighted := int64(math.Round(float64(totalValue) / float64(totalQty)))
	if weighted < 1 {
		return 1
	}
	return weighted
}

func shiftMapKey(storeID string, terminalID string) string {
	return storeID + "::" + terminalID
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func nowDateUTC(t time.Time) time.Time {
	return time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC)
}

func compareLotForFEFO(a domain.InventoryLot, b domain.InventoryLot) int {
	if a.ExpiryDate == nil && b.ExpiryDate != nil {
		return 1
	}
	if a.ExpiryDate != nil && b.ExpiryDate == nil {
		return -1
	}
	if a.ExpiryDate != nil && b.ExpiryDate != nil {
		if a.ExpiryDate.Before(*b.ExpiryDate) {
			return -1
		}
		if a.ExpiryDate.After(*b.ExpiryDate) {
			return 1
		}
	}
	if a.ReceivedAt.Before(b.ReceivedAt) {
		return -1
	}
	if a.ReceivedAt.After(b.ReceivedAt) {
		return 1
	}
	return cmpString(a.ID, b.ID)
}

func cmpString(a string, b string) int {
	if a == b {
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}

func cloneTransaction(src *domain.Transaction) *domain.Transaction {
	if src == nil {
		return nil
	}
	dup := *src
	dupItems := make([]domain.TransactionLine, len(src.Items))
	copy(dupItems, src.Items)
	dup.Items = dupItems
	dupSplits := make([]domain.PaymentSplit, len(src.PaymentSplits))
	copy(dupSplits, src.PaymentSplits)
	dup.PaymentSplits = dupSplits
	return &dup
}

func cloneHeldCart(src domain.HeldCart) domain.HeldCart {
	dup := src
	items := make([]domain.CartItem, len(src.CartItems))
	copy(items, src.CartItems)
	dup.CartItems = items
	splits := make([]domain.PaymentSplit, len(src.PaymentSplits))
	copy(splits, src.PaymentSplits)
	dup.PaymentSplits = splits
	return dup
}

func clonePurchaseOrder(src domain.PurchaseOrder) domain.PurchaseOrder {
	dup := src
	items := make([]domain.PurchaseOrderItem, len(src.Items))
	copy(items, src.Items)
	dup.Items = items
	return dup
}

func cloneInventoryLot(src domain.InventoryLot) domain.InventoryLot {
	dup := src
	if src.ExpiryDate != nil {
		expiry := src.ExpiryDate.UTC()
		dup.ExpiryDate = &expiry
	}
	return dup
}

func cloneItemReturn(src domain.ItemReturn) domain.ItemReturn {
	dup := src
	returnLines := make([]domain.ItemReturnLine, len(src.ReturnItems))
	copy(returnLines, src.ReturnItems)
	dup.ReturnItems = returnLines
	exchangeLines := make([]domain.ItemReturnLine, len(src.ExchangeItems))
	copy(exchangeLines, src.ExchangeItems)
	dup.ExchangeItems = exchangeLines
	return dup
}
