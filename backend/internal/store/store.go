package store

import (
	"context"
	"errors"
	"time"

	"kasirinaja/backend/internal/domain"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrInsufficientStock  = errors.New("insufficient stock")
	ErrInvalidTransaction = errors.New("invalid transaction")
)

type Repository interface {
	ListProducts(ctx context.Context) ([]domain.Product, error)
	CreateProduct(ctx context.Context, product domain.Product) (*domain.Product, error)
	GetProductBySKU(ctx context.Context, sku string) (*domain.Product, error)
	UpdateProduct(ctx context.Context, product domain.Product) (*domain.Product, error)
	CreatePriceHistory(ctx context.Context, entry domain.ProductPriceHistory) error
	ListPriceHistory(ctx context.Context, sku string, limit int) ([]domain.ProductPriceHistory, error)
	GetProductsBySKUs(ctx context.Context, skus []string) (map[string]domain.Product, error)
	GetStockMap(ctx context.Context, storeID string, skus []string) (map[string]int, error)
	SetStock(ctx context.Context, storeID string, sku string, qty int) error
	CreateInventoryLot(ctx context.Context, lot domain.InventoryLot) (*domain.InventoryLot, error)
	ListInventoryLots(ctx context.Context, storeID string, sku string, includeExpired bool, limit int) ([]domain.InventoryLot, error)
	GetAssociationPairs(ctx context.Context, sourceSKUs []string) ([]domain.AssociationPair, error)
	IncreaseStock(ctx context.Context, storeID string, adjustments []domain.StockAdjustment) error
	FindTransactionByIdempotency(ctx context.Context, key string) (*domain.Transaction, error)
	FindTransactionByID(ctx context.Context, id string) (*domain.Transaction, error)
	CreateCheckout(ctx context.Context, tx domain.Transaction) (*domain.Transaction, error)
	VoidTransaction(ctx context.Context, id string, reason string, at time.Time) (*domain.Transaction, error)
	CreateRefund(ctx context.Context, refund domain.Refund) (*domain.Refund, error)
	GetReturnedQtyByTransaction(ctx context.Context, transactionID string) (map[string]int, error)
	CreateItemReturn(ctx context.Context, itemReturn domain.ItemReturn) (*domain.ItemReturn, error)
	CreateRecommendationEvent(ctx context.Context, event domain.RecommendationEvent) error
	GetAttachMetrics(ctx context.Context, storeID string, from time.Time, to time.Time) (domain.AttachMetrics, error)
	GetDailyReport(ctx context.Context, storeID string, from time.Time, to time.Time) (domain.DailyReport, error)
	CreateAuditLog(ctx context.Context, entry domain.AuditLog) error
	ListAuditLogs(ctx context.Context, storeID string, from time.Time, to time.Time, limit int) ([]domain.AuditLog, error)
	RebuildAssociationPairs(ctx context.Context, storeID string) (int, error)
	CreateShift(ctx context.Context, shift domain.Shift) (*domain.Shift, error)
	CloseActiveShift(ctx context.Context, storeID string, terminalID string, closingCashCents int64, closedAt time.Time) (*domain.Shift, error)
	GetActiveShift(ctx context.Context, storeID string, terminalID string) (*domain.Shift, error)
	CreatePromo(ctx context.Context, promo domain.PromoRule) (*domain.PromoRule, error)
	ListPromos(ctx context.Context) ([]domain.PromoRule, error)
	UpdatePromoActive(ctx context.Context, promoID string, active bool) (*domain.PromoRule, error)
	CreateHeldCart(ctx context.Context, held domain.HeldCart) (*domain.HeldCart, error)
	ListHeldCarts(ctx context.Context, storeID string, terminalID string, limit int) ([]domain.HeldCart, error)
	PopHeldCart(ctx context.Context, holdID string) (*domain.HeldCart, error)
	DeleteHeldCart(ctx context.Context, holdID string) error
	CreateSupplier(ctx context.Context, supplier domain.Supplier) (*domain.Supplier, error)
	ListSuppliers(ctx context.Context) ([]domain.Supplier, error)
	CreatePurchaseOrder(ctx context.Context, po domain.PurchaseOrder) (*domain.PurchaseOrder, error)
	GetPurchaseOrderByID(ctx context.Context, purchaseOrderID string) (*domain.PurchaseOrder, error)
	ListPurchaseOrders(ctx context.Context, storeID string, status string, limit int) ([]domain.PurchaseOrder, error)
	ReceivePurchaseOrder(ctx context.Context, purchaseOrderID string, receivedBy string, receivedAt time.Time) (*domain.PurchaseOrder, error)
	GetProductCosts(ctx context.Context, storeID string, skus []string) (map[string]int64, error)
	UpsertProductCost(ctx context.Context, storeID string, sku string, costCents int64) error
	CreateUser(ctx context.Context, user domain.UserAccount) error
	ListUsers(ctx context.Context) ([]domain.UserAccount, error)
	UpdateUserPassword(ctx context.Context, username string, password string) error
}
