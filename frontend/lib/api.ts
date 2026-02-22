import type {
  AuditLog,
  AttachRateMetrics,
  CashDrawerOpenRequest,
  CashDrawerOpenResponse,
  CashierCreateRequest,
  CashierUser,
  DailyReport,
  CheckoutLookupResponse,
  CheckoutRequest,
  CheckoutResponse,
  HardwareReceiptRequest,
  HardwareReceiptResponse,
  HeldCart,
  HoldCartRequest,
  HoldCartResponse,
  InventoryLot,
  InventoryLotReceiveRequest,
  ItemReturnRequest,
  ItemReturnResponse,
  LoginRequest,
  LoginResponse,
  OfflineSyncRequest,
  OfflineSyncResponse,
  OperationalAlertResponse,
  ProductPriceHistory,
  Product,
  ProductCreateRequest,
  PurchaseOrderCreateRequest,
  PurchaseOrderListResponse,
  PurchaseOrderResponse,
  ProductUpdateRequest,
  PromoCreateRequest,
  PromoRule,
  ReorderSuggestionResponse,
  RefundRequest,
  RefundResponse,
  RecommendationRequest,
  RecommendationResponse,
  Supplier,
  SupplierCreateRequest,
  StockOpnameRequest,
  StockOpnameResponse,
  ShiftCloseRequest,
  ShiftOpenRequest,
  ShiftResponse,
  VoidTransactionRequest,
  VoidTransactionResponse,
} from "@/lib/types";

const API_BASE =
  process.env.NEXT_PUBLIC_API_BASE_URL?.replace(/\/$/, "") ??
  "http://127.0.0.1:8080";

const REQUEST_TIMEOUT_MS = 15_000;

export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

function withTimeout(init?: RequestInit): { init: RequestInit; cleanup: () => void } {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);
  const cleanup = () => clearTimeout(timeoutId);
  return {
    init: { ...init, signal: controller.signal },
    cleanup,
  };
}

async function request<T>(
  path: string,
  init?: RequestInit,
  token?: string,
): Promise<T> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const { init: timedInit, cleanup } = withTimeout({ ...init, headers });
  try {
    const response = await fetch(`${API_BASE}${path}`, timedInit);

    if (!response.ok) {
      let errorMessage = `Request failed with status ${response.status}`;
      try {
        const payload = (await response.json()) as { error?: string };
        if (payload.error) {
          errorMessage = payload.error;
        }
      } catch {
        // Ignore non-JSON error payloads.
      }
      throw new ApiError(errorMessage, response.status);
    }

    return (await response.json()) as T;
  } finally {
    cleanup();
  }
}

async function requestText(
  path: string,
  init?: RequestInit,
  token?: string,
): Promise<string> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const { init: timedInit, cleanup } = withTimeout({ ...init, headers });
  try {
    const response = await fetch(`${API_BASE}${path}`, timedInit);
    if (!response.ok) {
      throw new ApiError(`Request failed with status ${response.status}`, response.status);
    }
    return response.text();
  } finally {
    cleanup();
  }
}

export async function login(body: LoginRequest): Promise<LoginResponse> {
  return request<LoginResponse>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export async function fetchProducts(token: string): Promise<Product[]> {
  const payload = await request<{ products: Product[] }>(
    "/api/v1/products",
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.products;
}

export async function createProduct(
  token: string,
  body: ProductCreateRequest,
): Promise<Product> {
  const payload = await request<{ product: Product }>(
    "/api/v1/products",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
  return payload.product;
}

export async function updateProduct(
  token: string,
  sku: string,
  body: ProductUpdateRequest,
): Promise<Product> {
  const encodedSKU = encodeURIComponent(sku);
  const payload = await request<{ product: Product }>(
    `/api/v1/products/${encodedSKU}`,
    {
      method: "PATCH",
      body: JSON.stringify(body),
    },
    token,
  );
  return payload.product;
}

export async function fetchProductPriceHistory(
  token: string,
  sku: string,
  limit = 20,
): Promise<ProductPriceHistory[]> {
  const encodedSKU = encodeURIComponent(sku);
  const payload = await request<{ history: ProductPriceHistory[] }>(
    `/api/v1/products/${encodedSKU}/price-history?limit=${limit}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.history;
}

export async function fetchRecommendation(
  token: string,
  body: RecommendationRequest,
): Promise<RecommendationResponse> {
  return request<RecommendationResponse>(
    "/api/v1/cart/recommendation",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function checkout(
  token: string,
  body: CheckoutRequest,
): Promise<CheckoutResponse> {
  return request<CheckoutResponse>(
    "/api/v1/checkout",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function lookupCheckoutByIdempotency(
  token: string,
  idempotencyKey: string,
): Promise<CheckoutLookupResponse> {
  const encodedID = encodeURIComponent(idempotencyKey);
  return request<CheckoutLookupResponse>(
    `/api/v1/checkout/idempotency/${encodedID}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function openShift(
  token: string,
  body: ShiftOpenRequest,
): Promise<ShiftResponse> {
  return request<ShiftResponse>(
    "/api/v1/shifts/open",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function closeShift(
  token: string,
  body: ShiftCloseRequest,
): Promise<ShiftResponse> {
  return request<ShiftResponse>(
    "/api/v1/shifts/close",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function fetchActiveShift(
  token: string,
  storeID: string,
  terminalID: string,
): Promise<ShiftResponse | null> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedTerminalID = encodeURIComponent(terminalID);

  try {
    return await request<ShiftResponse>(
      `/api/v1/shifts/active?store_id=${encodedStoreID}&terminal_id=${encodedTerminalID}`,
      {
        method: "GET",
        cache: "no-store",
      },
      token,
    );
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) {
      return null;
    }
    throw error;
  }
}

export async function syncOffline(
  token: string,
  body: OfflineSyncRequest,
): Promise<OfflineSyncResponse> {
  return request<OfflineSyncResponse>(
    "/api/v1/sync/offline-transactions",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function voidTransaction(
  token: string,
  transactionID: string,
  body: VoidTransactionRequest,
): Promise<VoidTransactionResponse> {
  const encodedID = encodeURIComponent(transactionID);
  return request<VoidTransactionResponse>(
    `/api/v1/transactions/${encodedID}/void`,
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function refundTransaction(
  token: string,
  body: RefundRequest,
): Promise<RefundResponse> {
  return request<RefundResponse>(
    "/api/v1/refunds",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function processItemReturn(
  token: string,
  body: ItemReturnRequest,
): Promise<ItemReturnResponse> {
  return request<ItemReturnResponse>(
    "/api/v1/returns/items",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function stockOpname(
  token: string,
  body: StockOpnameRequest,
): Promise<StockOpnameResponse> {
  return request<StockOpnameResponse>(
    "/api/v1/stock-opname",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function fetchAttachRate(
  token: string,
  storeID: string,
  days = 30,
): Promise<AttachRateMetrics> {
  const encodedStoreID = encodeURIComponent(storeID);
  return request<AttachRateMetrics>(
    `/api/v1/metrics/attach-rate?store_id=${encodedStoreID}&days=${days}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function fetchDailyReport(
  token: string,
  storeID: string,
  date: string,
): Promise<DailyReport> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedDate = encodeURIComponent(date);
  return request<DailyReport>(
    `/api/v1/reports/daily?store_id=${encodedStoreID}&date=${encodedDate}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function fetchDailyReportCSV(
  token: string,
  storeID: string,
  date: string,
): Promise<string> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedDate = encodeURIComponent(date);
  return requestText(
    `/api/v1/reports/daily?store_id=${encodedStoreID}&date=${encodedDate}&format=csv`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function fetchDailyReportPrintableHTML(
  token: string,
  storeID: string,
  date: string,
): Promise<string> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedDate = encodeURIComponent(date);
  return requestText(
    `/api/v1/reports/daily?store_id=${encodedStoreID}&date=${encodedDate}&format=pdf`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function fetchAuditLogs(
  token: string,
  storeID: string,
  date: string,
  limit = 100,
): Promise<AuditLog[]> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedDate = encodeURIComponent(date);
  const payload = await request<{ logs: AuditLog[] }>(
    `/api/v1/audit-logs?store_id=${encodedStoreID}&date=${encodedDate}&limit=${limit}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.logs;
}

export async function fetchPromos(token: string): Promise<PromoRule[]> {
  const payload = await request<{ promos: PromoRule[] }>(
    "/api/v1/promos",
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.promos;
}

export async function createPromo(
  token: string,
  body: PromoCreateRequest,
): Promise<PromoRule> {
  const payload = await request<{ promo: PromoRule }>(
    "/api/v1/promos",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
  return payload.promo;
}

export async function setPromoActive(
  token: string,
  promoID: string,
  active: boolean,
): Promise<PromoRule> {
  const encodedID = encodeURIComponent(promoID);
  const payload = await request<{ promo: PromoRule }>(
    `/api/v1/promos/${encodedID}/toggle`,
    {
      method: "POST",
      body: JSON.stringify({ active }),
    },
    token,
  );
  return payload.promo;
}

export async function holdCart(
  token: string,
  body: HoldCartRequest,
): Promise<HoldCartResponse> {
  return request<HoldCartResponse>(
    "/api/v1/carts/hold",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function fetchHeldCarts(
  token: string,
  storeID: string,
  terminalID: string,
): Promise<HeldCart[]> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedTerminalID = encodeURIComponent(terminalID);
  const payload = await request<{ items: HeldCart[] }>(
    `/api/v1/carts/hold?store_id=${encodedStoreID}&terminal_id=${encodedTerminalID}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.items;
}

export async function resumeHeldCart(
  token: string,
  holdID: string,
): Promise<HoldCartResponse> {
  const encodedID = encodeURIComponent(holdID);
  return request<HoldCartResponse>(
    `/api/v1/carts/hold/${encodedID}/resume`,
    {
      method: "POST",
    },
    token,
  );
}

export async function discardHeldCart(
  token: string,
  holdID: string,
): Promise<void> {
  const encodedID = encodeURIComponent(holdID);
  await request<{ ok: boolean }>(
    `/api/v1/carts/hold/${encodedID}/discard`,
    {
      method: "POST",
    },
    token,
  );
}

export async function fetchReorderSuggestions(
  token: string,
  storeID: string,
): Promise<ReorderSuggestionResponse> {
  const encodedStoreID = encodeURIComponent(storeID);
  return request<ReorderSuggestionResponse>(
    `/api/v1/reorder-suggestions?store_id=${encodedStoreID}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function fetchInventoryLots(
  token: string,
  storeID: string,
  sku = "",
  includeExpired = false,
  limit = 100,
): Promise<InventoryLot[]> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedSKU = encodeURIComponent(sku);
  const payload = await request<{ lots: InventoryLot[] }>(
    `/api/v1/inventory/lots?store_id=${encodedStoreID}&sku=${encodedSKU}&include_expired=${includeExpired ? "true" : "false"}&limit=${limit}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.lots;
}

export async function receiveInventoryLot(
  token: string,
  body: InventoryLotReceiveRequest,
): Promise<InventoryLot> {
  const payload = await request<{ lot: InventoryLot }>(
    "/api/v1/inventory/lots",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
  return payload.lot;
}

export async function fetchSuppliers(token: string): Promise<Supplier[]> {
  const payload = await request<{ suppliers: Supplier[] }>(
    "/api/v1/suppliers",
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.suppliers;
}

export async function createSupplier(
  token: string,
  body: SupplierCreateRequest,
): Promise<Supplier> {
  const payload = await request<{ supplier: Supplier }>(
    "/api/v1/suppliers",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
  return payload.supplier;
}

export async function fetchPurchaseOrders(
  token: string,
  status?: string,
): Promise<PurchaseOrderListResponse> {
  const suffix = status ? `?status=${encodeURIComponent(status)}` : "";
  return request<PurchaseOrderListResponse>(
    `/api/v1/purchase-orders${suffix}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function createPurchaseOrder(
  token: string,
  body: PurchaseOrderCreateRequest,
): Promise<PurchaseOrderResponse> {
  return request<PurchaseOrderResponse>(
    "/api/v1/purchase-orders",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function receivePurchaseOrder(
  token: string,
  purchaseOrderID: string,
  receivedBy: string,
): Promise<PurchaseOrderResponse> {
  const encodedID = encodeURIComponent(purchaseOrderID);
  return request<PurchaseOrderResponse>(
    `/api/v1/purchase-orders/${encodedID}/receive`,
    {
      method: "POST",
      body: JSON.stringify({ received_by: receivedBy }),
    },
    token,
  );
}

export async function fetchOperationalAlerts(
  token: string,
  storeID: string,
  date: string,
): Promise<OperationalAlertResponse> {
  const encodedStoreID = encodeURIComponent(storeID);
  const encodedDate = encodeURIComponent(date);
  return request<OperationalAlertResponse>(
    `/api/v1/alerts/anomalies?store_id=${encodedStoreID}&date=${encodedDate}`,
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
}

export async function fetchCashiers(token: string): Promise<CashierUser[]> {
  const payload = await request<{ cashiers: CashierUser[] }>(
    "/api/v1/users/cashiers",
    {
      method: "GET",
      cache: "no-store",
    },
    token,
  );
  return payload.cashiers;
}

export async function createCashier(
  token: string,
  body: CashierCreateRequest,
): Promise<CashierUser> {
  const payload = await request<{ cashier: CashierUser }>(
    "/api/v1/users/cashiers",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
  return payload.cashier;
}

export async function generateEscposReceipt(
  token: string,
  body: HardwareReceiptRequest,
): Promise<HardwareReceiptResponse> {
  return request<HardwareReceiptResponse>(
    "/api/v1/hardware/receipt/escpos",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}

export async function openCashDrawer(
  token: string,
  body: CashDrawerOpenRequest,
): Promise<CashDrawerOpenResponse> {
  return request<CashDrawerOpenResponse>(
    "/api/v1/hardware/cash-drawer/open",
    {
      method: "POST",
      body: JSON.stringify(body),
    },
    token,
  );
}
