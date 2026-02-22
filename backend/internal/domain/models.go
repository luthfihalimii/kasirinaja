package domain

import "time"

type Product struct {
	SKU        string  `json:"sku"`
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	PriceCents int64   `json:"price_cents"`
	MarginRate float64 `json:"margin_rate"`
	Active     bool    `json:"active"`
}

type ProductCreateRequest struct {
	StoreID      string  `json:"store_id"`
	SKU          string  `json:"sku"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	PriceCents   int64   `json:"price_cents"`
	MarginRate   float64 `json:"margin_rate"`
	InitialStock int     `json:"initial_stock"`
}

type ProductUpdateRequest struct {
	Name       *string  `json:"name,omitempty"`
	Category   *string  `json:"category,omitempty"`
	PriceCents *int64   `json:"price_cents,omitempty"`
	MarginRate *float64 `json:"margin_rate,omitempty"`
	Active     *bool    `json:"active,omitempty"`
}

type ProductPriceHistory struct {
	ID            string    `json:"id"`
	SKU           string    `json:"sku"`
	OldPriceCents int64     `json:"old_price_cents"`
	NewPriceCents int64     `json:"new_price_cents"`
	ChangedBy     string    `json:"changed_by"`
	ChangedAt     time.Time `json:"changed_at"`
}

type CartItem struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

type RecommendationRequest struct {
	StoreID        string     `json:"store_id"`
	TerminalID     string     `json:"terminal_id"`
	Timestamp      *time.Time `json:"timestamp,omitempty"`
	QueueSpeedHint float64    `json:"queue_speed_hint"`
	PromptCount    int        `json:"prompt_count"`
	CartItems      []CartItem `json:"cart_items"`
}

type Recommendation struct {
	SKU                     string  `json:"sku"`
	Name                    string  `json:"name"`
	PriceCents              int64   `json:"price_cents"`
	ExpectedMarginLiftCents int64   `json:"expected_margin_lift_cents"`
	ReasonCode              string  `json:"reason_code"`
	Confidence              float64 `json:"confidence"`
}

type UIPolicy struct {
	Show            bool `json:"show"`
	CooldownSeconds int  `json:"cooldown_seconds"`
}

type RecommendationResponse struct {
	Recommendation *Recommendation `json:"recommendation,omitempty"`
	UIPolicy       UIPolicy        `json:"ui_policy"`
	LatencyMS      int64           `json:"latency_ms"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
	Role        string `json:"role"`
	ExpiresAt   string `json:"expires_at"`
}

type Actor struct {
	Username string
	Role     string
}

type CheckoutRequest struct {
	StoreID            string                     `json:"store_id"`
	TerminalID         string                     `json:"terminal_id"`
	IdempotencyKey     string                     `json:"idempotency_key"`
	PaymentMethod      string                     `json:"payment_method"`
	PaymentReference   string                     `json:"payment_reference,omitempty"`
	PaymentSplits      []PaymentSplit             `json:"payment_splits,omitempty"`
	CashReceivedCents  int64                      `json:"cash_received_cents"`
	DiscountCents      int64                      `json:"discount_cents"`
	TaxRatePercent     float64                    `json:"tax_rate_percent"`
	ManualOverride     bool                       `json:"manual_override"`
	CartItems          []CartItem                 `json:"cart_items"`
	RecommendationInfo CheckoutRecommendationInfo `json:"recommendation_info"`
}

type CheckoutRecommendationInfo struct {
	Shown      bool    `json:"shown"`
	Accepted   bool    `json:"accepted"`
	SKU        string  `json:"sku"`
	ReasonCode string  `json:"reason_code"`
	Confidence float64 `json:"confidence"`
}

type CheckoutResponse struct {
	TransactionID  string         `json:"transaction_id"`
	Status         string         `json:"status"`
	PaymentMethod  string         `json:"payment_method"`
	PaymentSplits  []PaymentSplit `json:"payment_splits,omitempty"`
	SubtotalCents  int64          `json:"subtotal_cents"`
	DiscountCents  int64          `json:"discount_cents"`
	TaxCents       int64          `json:"tax_cents"`
	TotalCents     int64          `json:"total_cents"`
	CashReceived   int64          `json:"cash_received_cents"`
	ChangeCents    int64          `json:"change_cents"`
	ItemCount      int            `json:"item_count"`
	ShiftID        string         `json:"shift_id,omitempty"`
	Recommendation *string        `json:"recommendation_sku,omitempty"`
	Duplicate      bool           `json:"duplicate"`
	CreatedAt      string         `json:"created_at"`
}

type CheckoutLookupResponse struct {
	Found    bool              `json:"found"`
	Checkout *CheckoutResponse `json:"checkout,omitempty"`
}

type OfflineTransaction struct {
	ClientTransactionID string          `json:"client_transaction_id"`
	Checkout            CheckoutRequest `json:"checkout"`
}

type OfflineSyncRequest struct {
	StoreID      string               `json:"store_id"`
	TerminalID   string               `json:"terminal_id"`
	EnvelopeID   string               `json:"envelope_id"`
	Transactions []OfflineTransaction `json:"transactions"`
}

type OfflineSyncStatus struct {
	ClientTransactionID string `json:"client_transaction_id"`
	Status              string `json:"status"`
	Reason              string `json:"reason,omitempty"`
	TransactionID       string `json:"transaction_id,omitempty"`
}

type OfflineSyncResponse struct {
	EnvelopeID string              `json:"envelope_id"`
	Statuses   []OfflineSyncStatus `json:"statuses"`
}

type Shift struct {
	ID                string     `json:"id"`
	StoreID           string     `json:"store_id"`
	TerminalID        string     `json:"terminal_id"`
	CashierName       string     `json:"cashier_name"`
	OpeningFloatCents int64      `json:"opening_float_cents"`
	ClosingCashCents  int64      `json:"closing_cash_cents,omitempty"`
	Status            string     `json:"status"`
	OpenedAt          time.Time  `json:"opened_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
}

type ShiftOpenRequest struct {
	StoreID           string `json:"store_id"`
	TerminalID        string `json:"terminal_id"`
	CashierName       string `json:"cashier_name"`
	OpeningFloatCents int64  `json:"opening_float_cents"`
}

type ShiftCloseRequest struct {
	StoreID          string `json:"store_id"`
	TerminalID       string `json:"terminal_id"`
	ClosingCashCents int64  `json:"closing_cash_cents"`
	Notes            string `json:"notes"`
}

type ShiftResponse struct {
	Shift Shift `json:"shift"`
}

type VoidTransactionRequest struct {
	TransactionID string `json:"transaction_id"`
	Reason        string `json:"reason"`
	ManagerPIN    string `json:"manager_pin"`
}

type VoidTransactionResponse struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	VoidedAt      string `json:"voided_at"`
}

type RefundRequest struct {
	OriginalTransactionID string `json:"original_transaction_id"`
	Reason                string `json:"reason"`
	AmountCents           int64  `json:"amount_cents"`
	ManagerPIN            string `json:"manager_pin"`
}

type Refund struct {
	ID                    string    `json:"id"`
	OriginalTransactionID string    `json:"original_transaction_id"`
	Reason                string    `json:"reason"`
	AmountCents           int64     `json:"amount_cents"`
	Status                string    `json:"status"`
	CreatedAt             time.Time `json:"created_at"`
}

type RefundResponse struct {
	Refund Refund `json:"refund"`
}

type ItemReturnLine struct {
	SKU            string `json:"sku"`
	Qty            int    `json:"qty"`
	UnitPriceCents int64  `json:"unit_price_cents,omitempty"`
}

type ItemReturnRequest struct {
	OriginalTransactionID string           `json:"original_transaction_id"`
	Mode                  string           `json:"mode"`
	Reason                string           `json:"reason"`
	ManagerPIN            string           `json:"manager_pin"`
	StoreID               string           `json:"store_id,omitempty"`
	TerminalID            string           `json:"terminal_id,omitempty"`
	PaymentMethod         string           `json:"payment_method,omitempty"`
	PaymentReference      string           `json:"payment_reference,omitempty"`
	CashReceivedCents     int64            `json:"cash_received_cents,omitempty"`
	ReturnItems           []ItemReturnLine `json:"return_items"`
	ExchangeItems         []CartItem       `json:"exchange_items,omitempty"`
}

type ItemReturn struct {
	ID                     string           `json:"id"`
	StoreID                string           `json:"store_id"`
	OriginalTransactionID  string           `json:"original_transaction_id"`
	Mode                   string           `json:"mode"`
	Reason                 string           `json:"reason"`
	RefundAmountCents      int64            `json:"refund_amount_cents"`
	ExchangeTransactionID  string           `json:"exchange_transaction_id,omitempty"`
	AdditionalPaymentCents int64            `json:"additional_payment_cents"`
	ProcessedBy            string           `json:"processed_by"`
	CreatedAt              time.Time        `json:"created_at"`
	ReturnItems            []ItemReturnLine `json:"return_items"`
	ExchangeItems          []ItemReturnLine `json:"exchange_items,omitempty"`
}

type ItemReturnResponse struct {
	ItemReturn ItemReturn `json:"item_return"`
}

type Supplier struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

type SupplierCreateRequest struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

type PurchaseOrderItem struct {
	SKU       string `json:"sku"`
	Qty       int    `json:"qty"`
	CostCents int64  `json:"cost_cents"`
}

type StockAdjustment struct {
	SKU string `json:"sku"`
	Qty int    `json:"qty"`
}

type InventoryLot struct {
	ID           string     `json:"id"`
	StoreID      string     `json:"store_id"`
	SKU          string     `json:"sku"`
	LotCode      string     `json:"lot_code"`
	ExpiryDate   *time.Time `json:"expiry_date,omitempty"`
	QtyReceived  int        `json:"qty_received"`
	QtyAvailable int        `json:"qty_available"`
	CostCents    int64      `json:"cost_cents"`
	SourceType   string     `json:"source_type"`
	SourceID     string     `json:"source_id,omitempty"`
	Notes        string     `json:"notes,omitempty"`
	ReceivedAt   time.Time  `json:"received_at"`
}

type InventoryLotReceiveRequest struct {
	StoreID    string `json:"store_id"`
	SKU        string `json:"sku"`
	LotCode    string `json:"lot_code"`
	ExpiryDate string `json:"expiry_date,omitempty"`
	Qty        int    `json:"qty"`
	CostCents  int64  `json:"cost_cents"`
	Notes      string `json:"notes"`
}

type InventoryLotListResponse struct {
	Lots []InventoryLot `json:"lots"`
}

type StockOpnameItem struct {
	SKU        string `json:"sku"`
	CountedQty int    `json:"counted_qty"`
}

type StockOpnameRequest struct {
	StoreID string            `json:"store_id"`
	Notes   string            `json:"notes"`
	Items   []StockOpnameItem `json:"items"`
}

type StockOpnameAdjustment struct {
	SKU        string `json:"sku"`
	SystemQty  int    `json:"system_qty"`
	CountedQty int    `json:"counted_qty"`
	DeltaQty   int    `json:"delta_qty"`
}

type StockOpnameResponse struct {
	OpnameID    string                  `json:"opname_id"`
	StoreID     string                  `json:"store_id"`
	Notes       string                  `json:"notes"`
	Adjustments []StockOpnameAdjustment `json:"adjustments"`
	CreatedAt   string                  `json:"created_at"`
}

type PurchaseOrder struct {
	ID         string              `json:"id"`
	StoreID    string              `json:"store_id"`
	SupplierID string              `json:"supplier_id"`
	Status     string              `json:"status"`
	CreatedAt  time.Time           `json:"created_at"`
	ReceivedAt *time.Time          `json:"received_at,omitempty"`
	ReceivedBy string              `json:"received_by,omitempty"`
	Items      []PurchaseOrderItem `json:"items"`
}

type PurchaseOrderCreateRequest struct {
	StoreID    string              `json:"store_id"`
	SupplierID string              `json:"supplier_id"`
	Items      []PurchaseOrderItem `json:"items"`
}

type PurchaseOrderReceiveRequest struct {
	ReceivedBy string `json:"received_by"`
}

type PurchaseOrderResponse struct {
	PurchaseOrder PurchaseOrder `json:"purchase_order"`
}

type PurchaseOrderListResponse struct {
	PurchaseOrders []PurchaseOrder `json:"purchase_orders"`
}

type ReorderSuggestion struct {
	SKU                    string `json:"sku"`
	Name                   string `json:"name"`
	Category               string `json:"category"`
	CurrentStock           int    `json:"current_stock"`
	ReorderPoint           int    `json:"reorder_point"`
	RecommendedQty         int    `json:"recommended_qty"`
	LastCostCents          int64  `json:"last_cost_cents"`
	EstimatedPurchaseCents int64  `json:"estimated_purchase_cents"`
}

type ReorderSuggestionResponse struct {
	StoreID     string              `json:"store_id"`
	GeneratedAt string              `json:"generated_at"`
	Suggestions []ReorderSuggestion `json:"suggestions"`
}

type HoldCartRequest struct {
	StoreID           string         `json:"store_id"`
	TerminalID        string         `json:"terminal_id"`
	Note              string         `json:"note"`
	CartItems         []CartItem     `json:"cart_items"`
	DiscountCents     int64          `json:"discount_cents"`
	TaxRatePercent    float64        `json:"tax_rate_percent"`
	PaymentMethod     string         `json:"payment_method"`
	PaymentReference  string         `json:"payment_reference,omitempty"`
	PaymentSplits     []PaymentSplit `json:"payment_splits,omitempty"`
	CashReceivedCents int64          `json:"cash_received_cents"`
	ManualOverride    bool           `json:"manual_override"`
}

type HeldCart struct {
	ID                string         `json:"id"`
	StoreID           string         `json:"store_id"`
	TerminalID        string         `json:"terminal_id"`
	CashierUsername   string         `json:"cashier_username"`
	Note              string         `json:"note"`
	CartItems         []CartItem     `json:"cart_items"`
	DiscountCents     int64          `json:"discount_cents"`
	TaxRatePercent    float64        `json:"tax_rate_percent"`
	PaymentMethod     string         `json:"payment_method"`
	PaymentReference  string         `json:"payment_reference,omitempty"`
	PaymentSplits     []PaymentSplit `json:"payment_splits,omitempty"`
	CashReceivedCents int64          `json:"cash_received_cents"`
	ManualOverride    bool           `json:"manual_override"`
	HeldAt            time.Time      `json:"held_at"`
}

type HoldCartResponse struct {
	HeldCart HeldCart `json:"held_cart"`
}

type HeldCartListResponse struct {
	Items []HeldCart `json:"items"`
}

type OperationalAlert struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Severity    string  `json:"severity"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	MetricValue float64 `json:"metric_value"`
	Threshold   float64 `json:"threshold"`
	CreatedAt   string  `json:"created_at"`
}

type OperationalAlertResponse struct {
	StoreID string             `json:"store_id"`
	Date    string             `json:"date"`
	Alerts  []OperationalAlert `json:"alerts"`
}

type CashierCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CashierUser struct {
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// UserAccount is an internal persistence model for auth credentials.
type UserAccount struct {
	Username  string
	Password  string
	Role      string
	Active    bool
	CreatedAt time.Time
}

type RetrainRequest struct {
	StoreID string `json:"store_id"`
}

type RetrainResponse struct {
	UpdatedPairs int    `json:"updated_pairs"`
	UpdatedAt    string `json:"updated_at"`
}

type RecommendationEvent struct {
	StoreID       string
	TerminalID    string
	TransactionID string
	SKU           string
	Action        string
	ReasonCode    string
	Confidence    float64
	LatencyMS     int64
	CreatedAt     time.Time
}

type AssociationPair struct {
	SourceSKU string
	TargetSKU string
	Affinity  float64
}

type TransactionLine struct {
	SKU            string
	Qty            int
	UnitPriceCents int64
	MarginRate     float64
}

type Transaction struct {
	ID                     string
	StoreID                string
	TerminalID             string
	ShiftID                string
	IdempotencyKey         string
	PaymentMethod          string
	PaymentReference       string
	PaymentSplits          []PaymentSplit
	SubtotalCents          int64
	DiscountCents          int64
	TaxRatePercent         float64
	TaxCents               int64
	TotalCents             int64
	CashReceivedCents      int64
	ChangeCents            int64
	Status                 string
	VoidReason             string
	VoidedAt               *time.Time
	RecommendationShown    bool
	RecommendationAccepted bool
	RecommendationSKU      string
	CreatedAt              time.Time
	Items                  []TransactionLine
}

type AttachMetrics struct {
	Transactions int64   `json:"transactions"`
	Accepted     int64   `json:"accepted"`
	AttachRate   float64 `json:"attach_rate"`
}

type DailyReportPayment struct {
	PaymentMethod string `json:"payment_method"`
	Transactions  int64  `json:"transactions"`
	TotalCents    int64  `json:"total_cents"`
}

type DailyReportTerminal struct {
	TerminalID   string `json:"terminal_id"`
	Transactions int64  `json:"transactions"`
	TotalCents   int64  `json:"total_cents"`
}

type DailyReport struct {
	StoreID              string                `json:"store_id"`
	Date                 string                `json:"date"`
	Transactions         int64                 `json:"transactions"`
	GrossSalesCents      int64                 `json:"gross_sales_cents"`
	DiscountCents        int64                 `json:"discount_cents"`
	TaxCents             int64                 `json:"tax_cents"`
	NetSalesCents        int64                 `json:"net_sales_cents"`
	EstimatedMarginCents int64                 `json:"estimated_margin_cents"`
	ByPayment            []DailyReportPayment  `json:"by_payment"`
	ByTerminal           []DailyReportTerminal `json:"by_terminal"`
}

type AuditLog struct {
	ID            string    `json:"id"`
	StoreID       string    `json:"store_id"`
	ActorUsername string    `json:"actor_username"`
	ActorRole     string    `json:"actor_role"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	Detail        string    `json:"detail"`
	CreatedAt     time.Time `json:"created_at"`
}

type PromoRule struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	MinSubtotalCents  int64     `json:"min_subtotal_cents"`
	DiscountPercent   float64   `json:"discount_percent"`
	FlatDiscountCents int64     `json:"flat_discount_cents"`
	Active            bool      `json:"active"`
	CreatedAt         time.Time `json:"created_at"`
}

type PromoCreateRequest struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	MinSubtotalCents  int64   `json:"min_subtotal_cents"`
	DiscountPercent   float64 `json:"discount_percent"`
	FlatDiscountCents int64   `json:"flat_discount_cents"`
}

type PromoToggleRequest struct {
	Active bool `json:"active"`
}

type HardwareReceiptRequest struct {
	TransactionID string `json:"transaction_id"`
}

type HardwareReceiptResponse struct {
	TransactionID string `json:"transaction_id"`
	EscposBase64  string `json:"escpos_base64"`
	PreviewText   string `json:"preview_text"`
	FileName      string `json:"file_name"`
}

type CashDrawerOpenRequest struct {
	TerminalID string `json:"terminal_id"`
}

type CashDrawerOpenResponse struct {
	TerminalID    string `json:"terminal_id"`
	CommandBase64 string `json:"command_base64"`
	Note          string `json:"note"`
}

type PaymentSplit struct {
	Method      string `json:"method"`
	AmountCents int64  `json:"amount_cents"`
	Reference   string `json:"reference,omitempty"`
}

const (
	RecommendationShownAction    = "shown"
	RecommendationAcceptedAction = "accepted"
	RecommendationRejectedAction = "rejected"
)

const (
	TxStatusPaid     = "paid"
	TxStatusVoided   = "voided"
	TxStatusRefunded = "refunded"
)

const (
	ItemReturnModeRefund   = "refund"
	ItemReturnModeExchange = "exchange"
)

const (
	ShiftStatusOpen   = "open"
	ShiftStatusClosed = "closed"
)
