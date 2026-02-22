export type Role = "cashier" | "admin";

export type PaymentMethod = "cash" | "card" | "qris" | "ewallet" | "split";

export type PaymentSplit = {
  method: "cash" | "card" | "qris" | "ewallet";
  amount_cents: number;
  reference?: string;
};

export type Product = {
  sku: string;
  name: string;
  category: string;
  price_cents: number;
  margin_rate: number;
  active: boolean;
};

export type ProductCreateRequest = {
  store_id: string;
  sku: string;
  name: string;
  category: string;
  price_cents: number;
  margin_rate: number;
  initial_stock: number;
};

export type ProductUpdateRequest = {
  name?: string;
  category?: string;
  price_cents?: number;
  margin_rate?: number;
  active?: boolean;
};

export type ProductPriceHistory = {
  id: string;
  sku: string;
  old_price_cents: number;
  new_price_cents: number;
  changed_by: string;
  changed_at: string;
};

export type CartItem = {
  sku: string;
  qty: number;
};

export type RecommendationRequest = {
  store_id: string;
  terminal_id: string;
  queue_speed_hint: number;
  prompt_count: number;
  cart_items: CartItem[];
  timestamp: string;
};

export type Recommendation = {
  sku: string;
  name: string;
  price_cents: number;
  expected_margin_lift_cents: number;
  reason_code: string;
  confidence: number;
};

export type RecommendationResponse = {
  recommendation?: Recommendation;
  ui_policy: {
    show: boolean;
    cooldown_seconds: number;
  };
  latency_ms: number;
};

export type LoginRequest = {
  username: string;
  password: string;
};

export type LoginResponse = {
  access_token: string;
  role: Role;
  expires_at: string;
};

export type Shift = {
  id: string;
  store_id: string;
  terminal_id: string;
  cashier_name: string;
  opening_float_cents: number;
  closing_cash_cents?: number;
  status: "open" | "closed";
  opened_at: string;
  closed_at?: string;
};

export type ShiftOpenRequest = {
  store_id: string;
  terminal_id: string;
  cashier_name: string;
  opening_float_cents: number;
};

export type ShiftCloseRequest = {
  store_id: string;
  terminal_id: string;
  closing_cash_cents: number;
  notes: string;
};

export type ShiftResponse = {
  shift: Shift;
};

export type StockOpnameItem = {
  sku: string;
  counted_qty: number;
};

export type StockOpnameRequest = {
  store_id: string;
  notes: string;
  items: StockOpnameItem[];
};

export type StockOpnameResponse = {
  opname_id: string;
  store_id: string;
  notes: string;
  created_at: string;
  adjustments: Array<{
    sku: string;
    system_qty: number;
    counted_qty: number;
    delta_qty: number;
  }>;
};

export type CheckoutRequest = {
  store_id: string;
  terminal_id: string;
  idempotency_key: string;
  payment_method: PaymentMethod;
  payment_reference?: string;
  payment_splits?: PaymentSplit[];
  cash_received_cents: number;
  discount_cents: number;
  tax_rate_percent: number;
  manual_override: boolean;
  cart_items: CartItem[];
  recommendation_info: {
    shown: boolean;
    accepted: boolean;
    sku: string;
    reason_code: string;
    confidence: number;
  };
};

export type CheckoutResponse = {
  transaction_id: string;
  status: string;
  payment_method: PaymentMethod;
  payment_splits?: PaymentSplit[];
  subtotal_cents: number;
  discount_cents: number;
  tax_cents: number;
  total_cents: number;
  cash_received_cents: number;
  change_cents: number;
  item_count: number;
  shift_id?: string;
  recommendation_sku?: string;
  duplicate: boolean;
  created_at: string;
};

export type CheckoutLookupResponse = {
  found: boolean;
  checkout?: CheckoutResponse;
};

export type VoidTransactionRequest = {
  reason: string;
  manager_pin: string;
};

export type VoidTransactionResponse = {
  transaction_id: string;
  status: string;
  voided_at: string;
};

export type RefundRequest = {
  original_transaction_id: string;
  reason: string;
  amount_cents: number;
  manager_pin: string;
};

export type RefundResponse = {
  refund: {
    id: string;
    original_transaction_id: string;
    reason: string;
    amount_cents: number;
    status: string;
    created_at: string;
  };
};

export type ItemReturnLine = {
  sku: string;
  qty: number;
  unit_price_cents?: number;
};

export type ItemReturnRequest = {
  original_transaction_id: string;
  mode: "refund" | "exchange";
  reason: string;
  manager_pin: string;
  store_id?: string;
  terminal_id?: string;
  payment_method?: PaymentMethod;
  payment_reference?: string;
  cash_received_cents?: number;
  return_items: ItemReturnLine[];
  exchange_items?: CartItem[];
};

export type ItemReturnResponse = {
  item_return: {
    id: string;
    store_id: string;
    original_transaction_id: string;
    mode: "refund" | "exchange";
    reason: string;
    refund_amount_cents: number;
    exchange_transaction_id?: string;
    additional_payment_cents: number;
    processed_by: string;
    created_at: string;
    return_items: ItemReturnLine[];
    exchange_items: ItemReturnLine[];
  };
};

export type InventoryLot = {
  id: string;
  store_id: string;
  sku: string;
  lot_code: string;
  expiry_date?: string;
  qty_received: number;
  qty_available: number;
  cost_cents: number;
  source_type: string;
  source_id?: string;
  notes?: string;
  received_at: string;
};

export type InventoryLotReceiveRequest = {
  store_id: string;
  sku: string;
  lot_code: string;
  expiry_date?: string;
  qty: number;
  cost_cents: number;
  notes: string;
};

export type InventoryLotListResponse = {
  lots: InventoryLot[];
};

export type HardwareReceiptRequest = {
  transaction_id: string;
};

export type HardwareReceiptResponse = {
  transaction_id: string;
  escpos_base64: string;
  preview_text: string;
  file_name: string;
};

export type CashDrawerOpenRequest = {
  terminal_id?: string;
};

export type CashDrawerOpenResponse = {
  terminal_id: string;
  command_base64: string;
  note: string;
};

export type OfflineTransaction = {
  client_transaction_id: string;
  checkout: CheckoutRequest;
};

export type OfflineSyncRequest = {
  store_id: string;
  terminal_id: string;
  envelope_id: string;
  transactions: OfflineTransaction[];
};

export type OfflineSyncResponse = {
  envelope_id: string;
  statuses: Array<{
    client_transaction_id: string;
    status: "accepted" | "duplicate" | "rejected";
    reason?: string;
    transaction_id?: string;
  }>;
};

export type AttachRateMetrics = {
  transactions: number;
  accepted: number;
  attach_rate: number;
};

export type DailyReport = {
  store_id: string;
  date: string;
  transactions: number;
  gross_sales_cents: number;
  discount_cents: number;
  tax_cents: number;
  net_sales_cents: number;
  estimated_margin_cents: number;
  by_payment: Array<{
    payment_method: string;
    transactions: number;
    total_cents: number;
  }>;
  by_terminal: Array<{
    terminal_id: string;
    transactions: number;
    total_cents: number;
  }>;
};

export type AuditLog = {
  id: string;
  store_id: string;
  actor_username: string;
  actor_role: string;
  action: string;
  entity_type: string;
  entity_id: string;
  detail: string;
  created_at: string;
};

export type PromoRule = {
  id: string;
  name: string;
  type: "cart_percent" | "flat_cart";
  min_subtotal_cents: number;
  discount_percent: number;
  flat_discount_cents: number;
  active: boolean;
  created_at: string;
};

export type PromoCreateRequest = {
  name: string;
  type: "cart_percent" | "flat_cart";
  min_subtotal_cents: number;
  discount_percent: number;
  flat_discount_cents: number;
};

export type HoldCartRequest = {
  store_id: string;
  terminal_id: string;
  note: string;
  cart_items: CartItem[];
  discount_cents: number;
  tax_rate_percent: number;
  payment_method: PaymentMethod;
  payment_reference?: string;
  payment_splits?: PaymentSplit[];
  cash_received_cents: number;
  manual_override: boolean;
};

export type HeldCart = {
  id: string;
  store_id: string;
  terminal_id: string;
  cashier_username: string;
  note: string;
  cart_items: CartItem[];
  discount_cents: number;
  tax_rate_percent: number;
  payment_method: PaymentMethod;
  payment_reference?: string;
  payment_splits?: PaymentSplit[];
  cash_received_cents: number;
  manual_override: boolean;
  held_at: string;
};

export type HoldCartResponse = {
  held_cart: HeldCart;
};

export type HeldCartListResponse = {
  items: HeldCart[];
};

export type Supplier = {
  id: string;
  name: string;
  phone: string;
  created_at: string;
};

export type SupplierCreateRequest = {
  name: string;
  phone: string;
};

export type PurchaseOrderItem = {
  sku: string;
  qty: number;
  cost_cents: number;
};

export type PurchaseOrder = {
  id: string;
  store_id: string;
  supplier_id: string;
  status: "draft" | "received" | "cancelled";
  created_at: string;
  received_at?: string;
  received_by?: string;
  items: PurchaseOrderItem[];
};

export type PurchaseOrderCreateRequest = {
  store_id: string;
  supplier_id: string;
  items: PurchaseOrderItem[];
};

export type PurchaseOrderResponse = {
  purchase_order: PurchaseOrder;
};

export type PurchaseOrderListResponse = {
  purchase_orders: PurchaseOrder[];
};

export type ReorderSuggestion = {
  sku: string;
  name: string;
  category: string;
  current_stock: number;
  reorder_point: number;
  recommended_qty: number;
  last_cost_cents: number;
  estimated_purchase_cents: number;
};

export type ReorderSuggestionResponse = {
  store_id: string;
  generated_at: string;
  suggestions: ReorderSuggestion[];
};

export type OperationalAlert = {
  id: string;
  code: string;
  severity: "low" | "medium" | "high";
  title: string;
  description: string;
  metric_value: number;
  threshold: number;
  created_at: string;
};

export type OperationalAlertResponse = {
  store_id: string;
  date: string;
  alerts: OperationalAlert[];
};

export type CashierCreateRequest = {
  username: string;
  password: string;
};

export type CashierUser = {
  username: string;
  role: "cashier";
  active: boolean;
  created_at: string;
};
