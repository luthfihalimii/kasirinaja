"use client";

import { useEffect, useMemo, useRef, useState } from "react";

import {
  ApiError,
  checkout,
  closeShift,
  createCashier,
  createProduct,
  createPromo,
  createPurchaseOrder,
  createSupplier,
  discardHeldCart,
  fetchActiveShift,
  fetchAttachRate,
  fetchAuditLogs,
  fetchCashiers,
  fetchDailyReport,
  fetchDailyReportCSV,
  fetchDailyReportPrintableHTML,
  fetchHeldCarts,
  fetchInventoryLots,
  fetchOperationalAlerts,
  fetchProductPriceHistory,
  fetchPromos,
  fetchProducts,
  fetchPurchaseOrders,
  fetchRecommendation,
  fetchReorderSuggestions,
  fetchSuppliers,
  generateEscposReceipt,
  holdCart,
  login,
  lookupCheckoutByIdempotency,
  openCashDrawer,
  openShift,
  processItemReturn,
  receiveInventoryLot,
  receivePurchaseOrder,
  resumeHeldCart,
  refundTransaction,
  setPromoActive,
  stockOpname,
  syncOffline,
  updateProduct,
  voidTransaction,
} from "@/lib/api";
import {
  countOfflineCheckouts,
  enqueueOfflineCheckout,
  listOfflineCheckouts,
  removeOfflineCheckouts,
} from "@/lib/offline-queue";
import {
  clearStoredAuthSession,
  isSessionExpired,
  persistAuthSession,
  restoreAuthSession,
  toAuthSession,
  type AuthSession,
} from "@/lib/auth-session";
import {
  buildReceiptHTML,
  clamp,
  generateId,
  parseNumber,
  parsePositiveInt,
  paymentLabel,
  reasonText,
  type ReceiptCartDetail,
  type ReceiptSnapshot,
} from "@/lib/pos-helpers";
import type {
  AuditLog,
  AttachRateMetrics,
  CashierUser,
  CartItem,
  CheckoutRequest,
  CheckoutResponse,
  DailyReport,
  HeldCart,
  HardwareReceiptResponse,
  InventoryLot,
  OperationalAlert,
  PaymentSplit,
  PaymentMethod,
  Product,
  ProductPriceHistory,
  PurchaseOrder,
  PromoRule,
  Recommendation,
  ReorderSuggestion,
  RefundRequest,
  Shift,
  Supplier,
  VoidTransactionRequest,
} from "@/lib/types";
import { formatCurrency } from "@/lib/utils";
import { LoginCard } from "@/components/pos/login-card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";

const STORE_ID = "main-store";
const TERMINAL_ID = "terminal-a1";

type CartLine = {
  sku: string;
  qty: number;
};

type RecommendationState = {
  shown: boolean;
  accepted: boolean;
  sku: string;
  reason_code: string;
  confidence: number;
};

type CartDetail = ReceiptCartDetail;

type DashboardView =
  | "overview"
  | "cashier"
  | "products"
  | "procurement"
  | "operations"
  | "promos"
  | "reports"
  | "alerts"
  | "team";

type DashboardMenu = {
  key: DashboardView;
  label: string;
  hint: string;
  adminOnly?: boolean;
};

const DASHBOARD_MENUS: DashboardMenu[] = [
  {
    key: "overview",
    label: "Ringkasan",
    hint: "Status terminal dan metrik",
  },
  {
    key: "cashier",
    label: "Kasir",
    hint: "Transaksi cepat",
  },
  {
    key: "products",
    label: "Produk",
    hint: "Tambah dan edit SKU",
    adminOnly: true,
  },
  {
    key: "procurement",
    label: "Procurement",
    hint: "Supplier, PO, reorder",
    adminOnly: true,
  },
  {
    key: "operations",
    label: "Operasional",
    hint: "Opname, void, refund",
    adminOnly: true,
  },
  {
    key: "promos",
    label: "Promo",
    hint: "Rules diskon",
    adminOnly: true,
  },
  {
    key: "reports",
    label: "Laporan",
    hint: "Audit dan export",
    adminOnly: true,
  },
  {
    key: "alerts",
    label: "Alerts",
    hint: "Anomali operasional",
    adminOnly: true,
  },
  {
    key: "team",
    label: "Tim Kasir",
    hint: "Manajemen user kasir",
    adminOnly: true,
  },
];

const PRODUCT_PREVIEW_LIMIT = 6;

export function POSTerminal() {
  const [authReady, setAuthReady] = useState(false);
  const [auth, setAuth] = useState<AuthSession | null>(null);
  const [loginUsername, setLoginUsername] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [isLoggingIn, setIsLoggingIn] = useState(false);

  const [products, setProducts] = useState<Product[]>([]);
  const [loadingProducts, setLoadingProducts] = useState(false);
  const [search, setSearch] = useState("");
  const [showAllProducts, setShowAllProducts] = useState(false);
  const [barcodeInput, setBarcodeInput] = useState("");
  const [newProductSKU, setNewProductSKU] = useState("");
  const [newProductName, setNewProductName] = useState("");
  const [newProductCategory, setNewProductCategory] = useState("");
  const [newProductPriceInput, setNewProductPriceInput] = useState("");
  const [newProductMarginInput, setNewProductMarginInput] = useState("25");
  const [newProductInitialStockInput, setNewProductInitialStockInput] = useState("0");
  const [isCreatingProduct, setIsCreatingProduct] = useState(false);
  const [manageProductSKU, setManageProductSKU] = useState("");
  const [manageProductName, setManageProductName] = useState("");
  const [manageProductCategory, setManageProductCategory] = useState("");
  const [manageProductPriceInput, setManageProductPriceInput] = useState("");
  const [manageProductMarginInput, setManageProductMarginInput] = useState("");
  const [manageProductActive, setManageProductActive] = useState(true);
  const [priceHistory, setPriceHistory] = useState<ProductPriceHistory[]>([]);
  const [isUpdatingProduct, setIsUpdatingProduct] = useState(false);

  const [stockOpnameRaw, setStockOpnameRaw] = useState("");
  const [stockOpnameNotes, setStockOpnameNotes] = useState("");
  const [isRunningStockOpname, setIsRunningStockOpname] = useState(false);

  const [voidTransactionID, setVoidTransactionID] = useState("");
  const [voidReason, setVoidReason] = useState("");
  const [refundTransactionID, setRefundTransactionID] = useState("");
  const [refundAmountInput, setRefundAmountInput] = useState("");
  const [refundReason, setRefundReason] = useState("");
  const [managerPinInput, setManagerPinInput] = useState("");

  const [reportDate, setReportDate] = useState(new Date().toISOString().slice(0, 10));
  const [dailyReport, setDailyReport] = useState<DailyReport | null>(null);
  const [isLoadingReport, setIsLoadingReport] = useState(false);

  const [promoName, setPromoName] = useState("");
  const [promoType, setPromoType] = useState<"cart_percent" | "flat_cart">("cart_percent");
  const [promoMinSubtotalInput, setPromoMinSubtotalInput] = useState("0");
  const [promoDiscountPercentInput, setPromoDiscountPercentInput] = useState("10");
  const [promoFlatDiscountInput, setPromoFlatDiscountInput] = useState("0");
  const [promos, setPromos] = useState<PromoRule[]>([]);
  const [isSavingPromo, setIsSavingPromo] = useState(false);

  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([]);

  const [cart, setCart] = useState<CartLine[]>([]);
  const [cashInput, setCashInput] = useState("");
  const [paymentMethod, setPaymentMethod] = useState<PaymentMethod>("cash");
  const [paymentReference, setPaymentReference] = useState("");
  const [splitCashInput, setSplitCashInput] = useState("");
  const [splitCardInput, setSplitCardInput] = useState("");
  const [splitQrisInput, setSplitQrisInput] = useState("");
  const [splitQrisReference, setSplitQrisReference] = useState("");
  const [splitEwalletInput, setSplitEwalletInput] = useState("");
  const [splitEwalletReference, setSplitEwalletReference] = useState("");
  const [discountInput, setDiscountInput] = useState("0");
  const [taxRateInput, setTaxRateInput] = useState("11");
  const [manualOverride, setManualOverride] = useState(false);

  const [holdNote, setHoldNote] = useState("");
  const [heldCarts, setHeldCarts] = useState<HeldCart[]>([]);
  const [isHoldingCart, setIsHoldingCart] = useState(false);
  const [isLoadingHeldCarts, setIsLoadingHeldCarts] = useState(false);

  const [supplierNameInput, setSupplierNameInput] = useState("");
  const [supplierPhoneInput, setSupplierPhoneInput] = useState("");
  const [suppliers, setSuppliers] = useState<Supplier[]>([]);
  const [purchaseOrders, setPurchaseOrders] = useState<PurchaseOrder[]>([]);
  const [selectedSupplierID, setSelectedSupplierID] = useState("");
  const [poItemsRaw, setPoItemsRaw] = useState("");
  const [poReceivedByInput, setPoReceivedByInput] = useState("");
  const [lotSKUInput, setLotSKUInput] = useState("");
  const [lotCodeInput, setLotCodeInput] = useState("");
  const [lotExpiryInput, setLotExpiryInput] = useState("");
  const [lotQtyInput, setLotQtyInput] = useState("");
  const [lotCostInput, setLotCostInput] = useState("");
  const [lotNotesInput, setLotNotesInput] = useState("");
  const [lotFilterSKUInput, setLotFilterSKUInput] = useState("");
  const [inventoryLots, setInventoryLots] = useState<InventoryLot[]>([]);
  const [reorderSuggestions, setReorderSuggestions] = useState<ReorderSuggestion[]>([]);
  const [isSavingSupplier, setIsSavingSupplier] = useState(false);
  const [isSavingPO, setIsSavingPO] = useState(false);
  const [isSavingLot, setIsSavingLot] = useState(false);

  const [itemReturnTxIDInput, setItemReturnTxIDInput] = useState("");
  const [itemReturnMode, setItemReturnMode] = useState<"refund" | "exchange">("refund");
  const [itemReturnReasonInput, setItemReturnReasonInput] = useState("");
  const [itemReturnItemsRaw, setItemReturnItemsRaw] = useState("");
  const [itemExchangeItemsRaw, setItemExchangeItemsRaw] = useState("");
  const [itemReturnPaymentMethod, setItemReturnPaymentMethod] = useState<PaymentMethod>("cash");
  const [itemReturnPaymentReference, setItemReturnPaymentReference] = useState("");
  const [itemReturnCashReceivedInput, setItemReturnCashReceivedInput] = useState("");
  const [isProcessingItemReturn, setIsProcessingItemReturn] = useState(false);

  const [hardwareTxIDInput, setHardwareTxIDInput] = useState("");
  const [escposPayload, setEscposPayload] = useState<HardwareReceiptResponse | null>(null);
  const [isGeneratingEscpos, setIsGeneratingEscpos] = useState(false);
  const [isOpeningDrawer, setIsOpeningDrawer] = useState(false);

  const [alerts, setAlerts] = useState<OperationalAlert[]>([]);
  const [isLoadingAlerts, setIsLoadingAlerts] = useState(false);

  const [cashierUsers, setCashierUsers] = useState<CashierUser[]>([]);
  const [newCashierUsername, setNewCashierUsername] = useState("");
  const [newCashierPassword, setNewCashierPassword] = useState("");
  const [isSavingCashier, setIsSavingCashier] = useState(false);

  const [recommendation, setRecommendation] = useState<Recommendation | null>(null);
  const [recommendationState, setRecommendationState] = useState<RecommendationState>({
    shown: false,
    accepted: false,
    sku: "",
    reason_code: "",
    confidence: 0,
  });

  const [queueSpeedHint, setQueueSpeedHint] = useState(0);
  const [promptCount, setPromptCount] = useState(0);
  const [cooldownUntil, setCooldownUntil] = useState(0);
  const [latencyMS, setLatencyMS] = useState<number | null>(null);

  const [isOnline, setIsOnline] = useState(true);
  const [offlineCount, setOfflineCount] = useState(0);
  const [isSyncing, setIsSyncing] = useState(false);

  const [metrics, setMetrics] = useState<AttachRateMetrics | null>(null);
  const [lastCheckout, setLastCheckout] = useState<CheckoutResponse | null>(null);
  const [lastReceipt, setLastReceipt] = useState<ReceiptSnapshot | null>(null);
  const [notice, setNotice] = useState<string>("");
  const [isSubmittingCheckout, setIsSubmittingCheckout] = useState(false);
  const [activeView, setActiveView] = useState<DashboardView>("cashier");

  const [activeShift, setActiveShift] = useState<Shift | null>(null);
  const [shiftCashierName, setShiftCashierName] = useState("Kasir A");
  const [openingFloatInput, setOpeningFloatInput] = useState("200000");
  const [closingCashInput, setClosingCashInput] = useState("");
  const [isShiftLoading, setIsShiftLoading] = useState(false);

  const recommendationTimeoutRef = useRef<number | undefined>(undefined);
  const scanTimesRef = useRef<number[]>([]);

  const authToken = auth?.accessToken ?? "";

  const dashboardMenus = useMemo(() => {
    if (!auth) {
      return [];
    }
    return DASHBOARD_MENUS.filter((menu) => !menu.adminOnly || auth.role === "admin");
  }, [auth]);

  const currentMenu = useMemo(() => {
    return dashboardMenus.find((menu) => menu.key === activeView) ?? dashboardMenus[0] ?? null;
  }, [dashboardMenus, activeView]);

  const productMap = useMemo(() => {
    const map = new Map<string, Product>();
    products.forEach((product) => map.set(product.sku, product));
    return map;
  }, [products]);

  const searchQuery = search.trim().toLowerCase();

  useEffect(() => {
    if (searchQuery) {
      setShowAllProducts(false);
    }
  }, [searchQuery]);

  const filteredProducts = useMemo(() => {
    if (!searchQuery) {
      return products;
    }

    return products.filter(
      (product) =>
        product.name.toLowerCase().includes(searchQuery) ||
        product.sku.toLowerCase().includes(searchQuery) ||
        product.category.toLowerCase().includes(searchQuery),
    );
  }, [products, searchQuery]);

  const visibleProducts = useMemo(() => {
    if (searchQuery || showAllProducts) {
      return filteredProducts;
    }
    return filteredProducts.slice(0, PRODUCT_PREVIEW_LIMIT);
  }, [filteredProducts, searchQuery, showAllProducts]);

  const cartDetails = useMemo(() => {
    return cart
      .map((line) => {
        const product = productMap.get(line.sku);
        if (!product) {
          return null;
        }

        return {
          ...line,
          name: product.name,
          price_cents: product.price_cents,
          subtotal_cents: product.price_cents * line.qty,
        };
      })
      .filter((item): item is CartDetail => item !== null);
  }, [cart, productMap]);

  const pricing = useMemo(() => {
    let subtotalCents = 0;
    let itemCount = 0;

    for (const item of cartDetails) {
      subtotalCents += item.subtotal_cents;
      itemCount += item.qty;
    }

    const discountCents = clamp(parsePositiveInt(discountInput), 0, subtotalCents);
    const taxRatePercent = clamp(parseNumber(taxRateInput, 0), 0, 100);
    const taxableBase = subtotalCents - discountCents;
    const taxCents = Math.round((taxableBase * taxRatePercent) / 100);
    const totalCents = taxableBase + taxCents;

    return {
      subtotalCents,
      itemCount,
      discountCents,
      taxRatePercent,
      taxCents,
      totalCents,
    };
  }, [cartDetails, discountInput, taxRateInput]);

  const splitPayments = useMemo<PaymentSplit[]>(() => {
    const cardAmount = parsePositiveInt(splitCardInput);
    const cashAmount = parsePositiveInt(splitCashInput);
    const qrisAmount = parsePositiveInt(splitQrisInput);
    const ewalletAmount = parsePositiveInt(splitEwalletInput);

    const splits: PaymentSplit[] = [];
    if (cashAmount > 0) {
      splits.push({ method: "cash", amount_cents: cashAmount });
    }
    if (cardAmount > 0) {
      splits.push({ method: "card", amount_cents: cardAmount, reference: "CARD-SPLIT" });
    }
    if (qrisAmount > 0) {
      splits.push({
        method: "qris",
        amount_cents: qrisAmount,
        reference: splitQrisReference.trim() || "QRIS-SPLIT",
      });
    }
    if (ewalletAmount > 0) {
      splits.push({
        method: "ewallet",
        amount_cents: ewalletAmount,
        reference: splitEwalletReference.trim() || "EWALLET-SPLIT",
      });
    }
    return splits;
  }, [
    splitCardInput,
    splitCashInput,
    splitQrisInput,
    splitQrisReference,
    splitEwalletInput,
    splitEwalletReference,
  ]);

  useEffect(() => {
    try {
      setIsOnline(typeof navigator !== "undefined" ? navigator.onLine : true);
      void countOfflineCheckouts().then(setOfflineCount).catch((error) => {
        console.error("[pos] failed to count offline checkouts:", error);
      });
    } catch (error) {
      console.error("[pos] startup error:", error);
    }

    function onOnline() {
      setIsOnline(true);
    }

    function onOffline() {
      setIsOnline(false);
    }

    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);

    return () => {
      window.removeEventListener("online", onOnline);
      window.removeEventListener("offline", onOffline);
    };
  }, []);

  useEffect(() => {
    const restored = restoreAuthSession();
    if (restored && !isSessionExpired(restored.expiresAt)) {
      setAuth(restored);
      setLoginUsername(restored.username);
    } else {
      clearStoredAuthSession();
    }

    setAuthReady(true);
  }, []);

  useEffect(() => {
    if (!auth || dashboardMenus.length === 0) {
      return;
    }
    if (!dashboardMenus.some((menu) => menu.key === activeView)) {
      setActiveView(dashboardMenus[0].key);
    }
  }, [activeView, auth, dashboardMenus]);

  useEffect(() => {
    if (!authToken) {
      setProducts([]);
      setMetrics(null);
      setActiveShift(null);
      setLoadingProducts(false);
      return;
    }

    let mounted = true;
    setLoadingProducts(true);

    async function hydrate() {
      try {
        const [productList, attachRate, shift] = await Promise.all([
          fetchProducts(authToken),
          fetchAttachRate(authToken, STORE_ID, 30),
          fetchActiveShift(authToken, STORE_ID, TERMINAL_ID),
        ]);

        if (!mounted) {
          return;
        }

        setProducts(productList);
        setMetrics(attachRate);
        setActiveShift(shift?.shift ?? null);
        try {
          const holds = await fetchHeldCarts(authToken, STORE_ID, TERMINAL_ID);
          if (mounted) {
            setHeldCarts(holds);
          }
        } catch (error) {
          console.error("[pos] held cart hydration failed:", error);
        }

        if (auth?.role === "admin") {
          const [promoList, logs, supplierList, poList, reorder, anomaly, cashierList, lots] = await Promise.all([
            fetchPromos(authToken),
            fetchAuditLogs(authToken, STORE_ID, reportDate, 50),
            fetchSuppliers(authToken),
            fetchPurchaseOrders(authToken).then((payload) => payload.purchase_orders),
            fetchReorderSuggestions(authToken, STORE_ID).then((payload) => payload.suggestions),
            fetchOperationalAlerts(authToken, STORE_ID, reportDate).then((payload) => payload.alerts),
            fetchCashiers(authToken),
            fetchInventoryLots(authToken, STORE_ID, "", true, 120),
          ]);
          setPromos(promoList);
          setAuditLogs(logs);
          setSuppliers(supplierList);
          setPurchaseOrders(poList);
          setReorderSuggestions(reorder);
          setAlerts(anomaly);
          setCashierUsers(cashierList);
          setInventoryLots(lots);
          if (supplierList.length > 0) {
            setSelectedSupplierID((prev) => prev || supplierList[0].id);
          }
        }
      } catch (error) {
        if (!mounted) {
          return;
        }

        if (error instanceof ApiError && error.status === 401) {
          setNotice("Sesi login berakhir. Silakan login ulang.");
          clearStoredAuthSession();
          setAuth(null);
          return;
        }

        const message = error instanceof Error ? error.message : "Gagal memuat data awal";
        setNotice(message);
      } finally {
        if (mounted) {
          setLoadingProducts(false);
        }
      }
    }

    void hydrate();

    return () => {
      mounted = false;
      if (recommendationTimeoutRef.current) {
        window.clearTimeout(recommendationTimeoutRef.current);
      }
    };
  }, [auth?.role, authToken, reportDate]);

  useEffect(() => {
    if (!authToken || cart.length === 0) {
      setRecommendation(null);
      setLatencyMS(null);
      return;
    }

    if (Date.now() < cooldownUntil) {
      return;
    }

    if (recommendationTimeoutRef.current) {
      window.clearTimeout(recommendationTimeoutRef.current);
    }

    recommendationTimeoutRef.current = window.setTimeout(async () => {
      try {
        const response = await fetchRecommendation(authToken, {
          store_id: STORE_ID,
          terminal_id: TERMINAL_ID,
          queue_speed_hint: queueSpeedHint,
          prompt_count: promptCount,
          timestamp: new Date().toISOString(),
          cart_items: cart as CartItem[],
        });

        setLatencyMS(response.latency_ms ?? null);

        if (response.ui_policy.show && response.recommendation) {
          setRecommendation(response.recommendation);
          setPromptCount((count) => count + 1);
          setCooldownUntil(Date.now() + response.ui_policy.cooldown_seconds * 1000);
          setRecommendationState({
            shown: true,
            accepted: false,
            sku: response.recommendation.sku,
            reason_code: response.recommendation.reason_code,
            confidence: response.recommendation.confidence,
          });
          return;
        }

        setRecommendation(null);
      } catch (error) {
        console.error("[pos] recommendation fetch failed:", error);
        setRecommendation(null);
      }
    }, 250);
  }, [authToken, cart, queueSpeedHint, promptCount, cooldownUntil]);

  function resetRecommendation() {
    setRecommendation(null);
    setRecommendationState({
      shown: false,
      accepted: false,
      sku: "",
      reason_code: "",
      confidence: 0,
    });
    setPromptCount(0);
    setCooldownUntil(0);
  }

  function clearCheckoutForm() {
    setCart([]);
    setCashInput("");
    setPaymentReference("");
    setSplitCashInput("");
    setSplitCardInput("");
    setSplitQrisInput("");
    setSplitQrisReference("");
    setSplitEwalletInput("");
    setSplitEwalletReference("");
    resetRecommendation();
  }

  function markScanSpeed() {
    const now = Date.now();
    const history = scanTimesRef.current;
    history.push(now);
    if (history.length > 8) {
      history.shift();
    }

    if (history.length > 1) {
      const elapsedMS = history[history.length - 1] - history[0];
      if (elapsedMS > 0) {
        const speedPerMinute = ((history.length - 1) / elapsedMS) * 60000;
        setQueueSpeedHint(Math.max(0, Number(speedPerMinute.toFixed(1))));
      }
    }
  }

  function addToCart(sku: string, qty = 1) {
    if (!activeShift) {
      setNotice("Buka shift terlebih dahulu sebelum menambah item.");
      return;
    }

    setCart((prev) => {
      const next = [...prev];
      const index = next.findIndex((line) => line.sku === sku);
      if (index >= 0) {
        next[index] = { ...next[index], qty: next[index].qty + qty };
      } else {
        next.push({ sku, qty });
      }
      return next;
    });

    markScanSpeed();
  }

  function decreaseItem(sku: string) {
    setCart((prev) => {
      const next = [...prev];
      const index = next.findIndex((line) => line.sku === sku);
      if (index < 0) {
        return prev;
      }

      const nextQty = next[index].qty - 1;
      if (nextQty <= 0) {
        next.splice(index, 1);
      } else {
        next[index] = { ...next[index], qty: nextQty };
      }
      return next;
    });
  }

  function addFromBarcode() {
    const code = barcodeInput.trim().toUpperCase();
    if (!code) {
      return;
    }

    const match = products.find((product) => product.sku.toUpperCase() === code);
    if (!match) {
      setNotice(`SKU ${code} tidak ditemukan.`);
      return;
    }

    addToCart(match.sku, 1);
    setBarcodeInput("");
  }

  async function handleHoldCurrentCart() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }
    if (cart.length === 0) {
      setNotice("Keranjang kosong, tidak ada yang bisa di-hold.");
      return;
    }

    setIsHoldingCart(true);
    try {
      await holdCart(auth.accessToken, {
        store_id: STORE_ID,
        terminal_id: TERMINAL_ID,
        note: holdNote.trim(),
        cart_items: cart,
        discount_cents: pricing.discountCents,
        tax_rate_percent: pricing.taxRatePercent,
        payment_method: paymentMethod,
        payment_reference: paymentReference.trim(),
        payment_splits: paymentMethod === "split" ? splitPayments : undefined,
        cash_received_cents: parsePositiveInt(cashInput),
        manual_override: manualOverride,
      });

      const holds = await fetchHeldCarts(auth.accessToken, STORE_ID, TERMINAL_ID);
      setHeldCarts(holds);
      clearCheckoutForm();
      setHoldNote("");
      setNotice("Keranjang berhasil di-hold.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal hold keranjang";
      setNotice(message);
    } finally {
      setIsHoldingCart(false);
    }
  }

  async function refreshHeldCartsSilently() {
    if (!auth) {
      return;
    }
    setIsLoadingHeldCarts(true);
    try {
      const holds = await fetchHeldCarts(auth.accessToken, STORE_ID, TERMINAL_ID);
      setHeldCarts(holds);
    } catch (error) {
      console.error("[pos] held carts refresh failed:", error);
    } finally {
      setIsLoadingHeldCarts(false);
    }
  }

  async function handleResumeHeldCart(holdID: string) {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }
    try {
      const response = await resumeHeldCart(auth.accessToken, holdID);
      const held = response.held_cart;
      setCart(held.cart_items);
      setDiscountInput(String(held.discount_cents));
      setTaxRateInput(String(held.tax_rate_percent));
      setPaymentMethod(held.payment_method);
      setPaymentReference(held.payment_reference ?? "");
      setCashInput(String(held.cash_received_cents || 0));
      setManualOverride(Boolean(held.manual_override));
      if (held.payment_method === "split" && held.payment_splits) {
        const cashSplit = held.payment_splits.find((item) => item.method === "cash");
        const cardSplit = held.payment_splits.find((item) => item.method === "card");
        const qrisSplit = held.payment_splits.find((item) => item.method === "qris");
        const ewalletSplit = held.payment_splits.find((item) => item.method === "ewallet");
        setSplitCashInput(cashSplit ? String(cashSplit.amount_cents) : "");
        setSplitCardInput(cardSplit ? String(cardSplit.amount_cents) : "");
        setSplitQrisInput(qrisSplit ? String(qrisSplit.amount_cents) : "");
        setSplitQrisReference(qrisSplit?.reference ?? "");
        setSplitEwalletInput(ewalletSplit ? String(ewalletSplit.amount_cents) : "");
        setSplitEwalletReference(ewalletSplit?.reference ?? "");
      } else {
        setSplitCashInput("");
        setSplitCardInput("");
        setSplitQrisInput("");
        setSplitQrisReference("");
        setSplitEwalletInput("");
        setSplitEwalletReference("");
      }
      await refreshHeldCartsSilently();
      setNotice("Keranjang hold berhasil dipanggil kembali.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal memanggil keranjang hold";
      setNotice(message);
    }
  }

  async function handleDiscardHeldCart(holdID: string) {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }
    if (!window.confirm("Buang keranjang hold ini? Tindakan tidak dapat dibatalkan.")) {
      return;
    }
    try {
      await discardHeldCart(auth.accessToken, holdID);
      await refreshHeldCartsSilently();
      setNotice("Keranjang hold dibuang.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal membuang keranjang hold";
      setNotice(message);
    }
  }

  function resetNewProductForm() {
    setNewProductSKU("");
    setNewProductName("");
    setNewProductCategory("");
    setNewProductPriceInput("");
    setNewProductMarginInput("25");
    setNewProductInitialStockInput("0");
  }

  async function handleCreateProduct() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa menambah produk.");
      return;
    }

    const sku = newProductSKU.trim().toUpperCase();
    const name = newProductName.trim();
    const category = newProductCategory.trim().toLowerCase();
    const priceCents = parsePositiveInt(newProductPriceInput);
    const marginPercent = clamp(parseNumber(newProductMarginInput, 0), 0, 100);
    const marginRate = marginPercent / 100;
    const initialStock = parsePositiveInt(newProductInitialStockInput);

    if (!sku || !name || !category) {
      setNotice("SKU, nama, dan kategori produk wajib diisi.");
      return;
    }
    if (priceCents < 1) {
      setNotice("Harga produk harus lebih dari 0.");
      return;
    }

    setIsCreatingProduct(true);
    try {
      const created = await createProduct(auth.accessToken, {
        store_id: STORE_ID,
        sku,
        name,
        category,
        price_cents: priceCents,
        margin_rate: marginRate,
        initial_stock: initialStock,
      });
      const latestProducts = await fetchProducts(auth.accessToken);
      setProducts(latestProducts);
      resetNewProductForm();
      setNotice(`Produk ${created.name} (${created.sku}) berhasil ditambahkan.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menambah produk";
      setNotice(message);
    } finally {
      setIsCreatingProduct(false);
    }
  }

  async function handleUpdateProduct() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa update produk.");
      return;
    }

    const sku = manageProductSKU.trim().toUpperCase();
    if (!sku) {
      setNotice("SKU produk wajib diisi untuk update.");
      return;
    }

    const payload: {
      name?: string;
      category?: string;
      price_cents?: number;
      margin_rate?: number;
      active?: boolean;
    } = {
      active: manageProductActive,
    };

    if (manageProductName.trim()) {
      payload.name = manageProductName.trim();
    }
    if (manageProductCategory.trim()) {
      payload.category = manageProductCategory.trim().toLowerCase();
    }
    if (manageProductPriceInput.trim()) {
      payload.price_cents = parsePositiveInt(manageProductPriceInput);
    }
    if (manageProductMarginInput.trim()) {
      payload.margin_rate = clamp(parseNumber(manageProductMarginInput, 0), 0, 100) / 100;
    }

    setIsUpdatingProduct(true);
    try {
      const updated = await updateProduct(auth.accessToken, sku, payload);
      const [latestProducts, history] = await Promise.all([
        fetchProducts(auth.accessToken),
        fetchProductPriceHistory(auth.accessToken, sku, 20),
      ]);
      setProducts(latestProducts);
      setPriceHistory(history);
      setNotice(`Produk ${updated.sku} berhasil diperbarui.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal update produk";
      setNotice(message);
    } finally {
      setIsUpdatingProduct(false);
    }
  }

  function parseStockOpnameItems(): { sku: string; counted_qty: number }[] {
    return stockOpnameRaw
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0)
      .map((line) => {
        const [skuPart, qtyPart] = line.split(",");
        return {
          sku: (skuPart ?? "").trim().toUpperCase(),
          counted_qty: parsePositiveInt((qtyPart ?? "").trim()),
        };
      })
      .filter((item) => item.sku !== "");
  }

  function parsePurchaseOrderItems(): { sku: string; qty: number; cost_cents: number }[] {
    return poItemsRaw
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0)
      .map((line) => {
        const [skuPart, qtyPart, costPart] = line.split(",");
        return {
          sku: (skuPart ?? "").trim().toUpperCase(),
          qty: parsePositiveInt((qtyPart ?? "").trim()),
          cost_cents: parsePositiveInt((costPart ?? "").trim()),
        };
      })
      .filter((item) => item.sku !== "" && item.qty > 0 && item.cost_cents > 0);
  }

  function parseReturnItemsRaw(raw: string): { sku: string; qty: number }[] {
    return raw
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0)
      .map((line) => {
        const [skuPart, qtyPart] = line.split(",");
        return {
          sku: (skuPart ?? "").trim().toUpperCase(),
          qty: parsePositiveInt((qtyPart ?? "").trim()),
        };
      })
      .filter((item) => item.sku !== "" && item.qty > 0);
  }

  async function refreshProcurementData() {
    if (!auth || auth.role !== "admin") {
      return;
    }
    try {
      const [supplierList, poList, reorder, lots] = await Promise.all([
        fetchSuppliers(auth.accessToken),
        fetchPurchaseOrders(auth.accessToken).then((payload) => payload.purchase_orders),
        fetchReorderSuggestions(auth.accessToken, STORE_ID).then((payload) => payload.suggestions),
        fetchInventoryLots(auth.accessToken, STORE_ID, lotFilterSKUInput.trim().toUpperCase(), true, 120),
      ]);
      setSuppliers(supplierList);
      setPurchaseOrders(poList);
      setReorderSuggestions(reorder);
      setInventoryLots(lots);
      if (supplierList.length > 0) {
        setSelectedSupplierID((prev) => prev || supplierList[0].id);
      }
    } catch (error) {
      console.error("[pos] procurement data refresh failed:", error);
    }
  }

  async function handleCreateInventoryLot() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa menerima batch stok.");
      return;
    }
    const sku = lotSKUInput.trim().toUpperCase();
    const qty = parsePositiveInt(lotQtyInput);
    const cost = parsePositiveInt(lotCostInput);
    if (!sku || qty < 1 || cost < 1) {
      setNotice("Isi SKU, qty, dan cost per unit batch dengan benar.");
      return;
    }

    setIsSavingLot(true);
    try {
      const lot = await receiveInventoryLot(auth.accessToken, {
        store_id: STORE_ID,
        sku,
        lot_code: lotCodeInput.trim(),
        expiry_date: lotExpiryInput.trim() || undefined,
        qty,
        cost_cents: cost,
        notes: lotNotesInput.trim(),
      });
      setLotSKUInput("");
      setLotCodeInput("");
      setLotExpiryInput("");
      setLotQtyInput("");
      setLotCostInput("");
      setLotNotesInput("");
      await refreshProcurementData();
      setNotice(`Batch ${lot.lot_code} untuk ${lot.sku} berhasil diterima.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menerima batch stok";
      setNotice(message);
    } finally {
      setIsSavingLot(false);
    }
  }

  async function handleCreateSupplier() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa menambah supplier.");
      return;
    }
    if (!supplierNameInput.trim()) {
      setNotice("Nama supplier wajib diisi.");
      return;
    }

    setIsSavingSupplier(true);
    try {
      const supplier = await createSupplier(auth.accessToken, {
        name: supplierNameInput.trim(),
        phone: supplierPhoneInput.trim(),
      });
      setSupplierNameInput("");
      setSupplierPhoneInput("");
      setSelectedSupplierID(supplier.id);
      await refreshProcurementData();
      setNotice(`Supplier ${supplier.name} berhasil ditambahkan.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menambah supplier";
      setNotice(message);
    } finally {
      setIsSavingSupplier(false);
    }
  }

  async function handleCreatePurchaseOrder() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa membuat PO.");
      return;
    }
    if (!selectedSupplierID) {
      setNotice("Pilih supplier terlebih dahulu.");
      return;
    }

    const items = parsePurchaseOrderItems();
    if (items.length === 0) {
      setNotice("Isi item PO dengan format SKU,QTY,COST per baris.");
      return;
    }

    setIsSavingPO(true);
    try {
      const response = await createPurchaseOrder(auth.accessToken, {
        store_id: STORE_ID,
        supplier_id: selectedSupplierID,
        items,
      });
      setPoItemsRaw("");
      await refreshProcurementData();
      setNotice(`PO ${response.purchase_order.id} berhasil dibuat.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal membuat PO";
      setNotice(message);
    } finally {
      setIsSavingPO(false);
    }
  }

  async function handleReceivePurchaseOrder(purchaseOrderID: string) {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa receive PO.");
      return;
    }
    try {
      const response = await receivePurchaseOrder(
        auth.accessToken,
        purchaseOrderID,
        poReceivedByInput.trim() || auth.username,
      );
      await refreshProcurementData();
      setNotice(`PO ${response.purchase_order.id} sudah diterima.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal receive PO";
      setNotice(message);
    }
  }

  async function handleRunStockOpname() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa menjalankan stock opname.");
      return;
    }

    const items = parseStockOpnameItems();
    if (items.length === 0) {
      setNotice("Isi data stock opname dengan format SKU,QTY per baris.");
      return;
    }

    setIsRunningStockOpname(true);
    try {
      const result = await stockOpname(auth.accessToken, {
        store_id: STORE_ID,
        notes: stockOpnameNotes.trim(),
        items,
      });
      setNotice(`Stock opname selesai. ${result.adjustments.length} SKU diproses.`);
      setStockOpnameRaw("");
      const latestProducts = await fetchProducts(auth.accessToken);
      setProducts(latestProducts);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Stock opname gagal";
      setNotice(message);
    } finally {
      setIsRunningStockOpname(false);
    }
  }

  async function handleVoidTransaction() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa void transaksi.");
      return;
    }
    const transactionID = voidTransactionID.trim();
    if (!transactionID || !voidReason.trim() || !managerPinInput.trim()) {
      setNotice("Isi transaction ID, alasan, dan manager PIN.");
      return;
    }

    const payload: VoidTransactionRequest = {
      reason: voidReason.trim(),
      manager_pin: managerPinInput.trim(),
    };

    try {
      const result = await voidTransaction(auth.accessToken, transactionID, payload);
      setNotice(`Transaksi ${result.transaction_id} berhasil di-void.`);
      setVoidTransactionID("");
      setVoidReason("");
      setManagerPinInput("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Void gagal";
      setNotice(message);
      setManagerPinInput("");
    }
  }

  async function handleRefundTransaction() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa refund transaksi.");
      return;
    }
    const transactionID = refundTransactionID.trim();
    const amountCents = parsePositiveInt(refundAmountInput);
    if (!transactionID || amountCents < 1 || !refundReason.trim() || !managerPinInput.trim()) {
      setNotice("Isi transaction ID, nominal refund, alasan, dan manager PIN.");
      return;
    }

    const payload: RefundRequest = {
      original_transaction_id: transactionID,
      amount_cents: amountCents,
      reason: refundReason.trim(),
      manager_pin: managerPinInput.trim(),
    };

    try {
      const result = await refundTransaction(auth.accessToken, payload);
      setNotice(`Refund ${result.refund.id} berhasil dibuat.`);
      setRefundTransactionID("");
      setRefundAmountInput("");
      setRefundReason("");
      setManagerPinInput("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Refund gagal";
      setNotice(message);
      setManagerPinInput("");
    }
  }

  async function handleProcessItemReturn() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa proses return/exchange item.");
      return;
    }
    const originalTransactionID = itemReturnTxIDInput.trim();
    const returnItems = parseReturnItemsRaw(itemReturnItemsRaw);
    if (!originalTransactionID || returnItems.length === 0 || !itemReturnReasonInput.trim() || !managerPinInput.trim()) {
      setNotice("Isi transaction ID, item return, alasan, dan manager PIN.");
      return;
    }

    const exchangeItems = parseReturnItemsRaw(itemExchangeItemsRaw).map((item) => ({
      sku: item.sku,
      qty: item.qty,
    }));
    if (itemReturnMode === "exchange" && exchangeItems.length === 0) {
      setNotice("Untuk mode exchange, isi item pengganti minimal 1 baris.");
      return;
    }

    setIsProcessingItemReturn(true);
    try {
      const result = await processItemReturn(auth.accessToken, {
        original_transaction_id: originalTransactionID,
        mode: itemReturnMode,
        reason: itemReturnReasonInput.trim(),
        manager_pin: managerPinInput.trim(),
        store_id: STORE_ID,
        terminal_id: TERMINAL_ID,
        payment_method: itemReturnMode === "exchange" ? itemReturnPaymentMethod : undefined,
        payment_reference: itemReturnMode === "exchange" ? itemReturnPaymentReference.trim() : undefined,
        cash_received_cents: itemReturnMode === "exchange" ? parsePositiveInt(itemReturnCashReceivedInput) : undefined,
        return_items: returnItems,
        exchange_items: itemReturnMode === "exchange" ? exchangeItems : [],
      });
      setItemReturnTxIDInput("");
      setItemReturnReasonInput("");
      setItemReturnItemsRaw("");
      setItemExchangeItemsRaw("");
      setItemReturnPaymentReference("");
      setItemReturnCashReceivedInput("");
      setManagerPinInput("");
      setNotice(
        `Return ${result.item_return.id} berhasil diproses. Refund: ${formatCurrency(result.item_return.refund_amount_cents)}${result.item_return.exchange_transaction_id ? ` | Exchange TX: ${result.item_return.exchange_transaction_id}` : ""}`,
      );
      await refreshProcurementData();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal memproses return/exchange item";
      setNotice(message);
      setManagerPinInput("");
    } finally {
      setIsProcessingItemReturn(false);
    }
  }

  async function handleGenerateEscposReceipt() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }
    const transactionID = hardwareTxIDInput.trim() || lastCheckout?.transaction_id || "";
    if (!transactionID) {
      setNotice("Isi transaction ID untuk generate ESC/POS.");
      return;
    }

    setIsGeneratingEscpos(true);
    try {
      const payload = await generateEscposReceipt(auth.accessToken, {
        transaction_id: transactionID,
      });
      setEscposPayload(payload);
      setNotice(`Payload ESC/POS untuk ${payload.transaction_id} berhasil dibuat.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal generate ESC/POS";
      setNotice(message);
    } finally {
      setIsGeneratingEscpos(false);
    }
  }

  function handleDownloadEscposPayload() {
    if (!escposPayload) {
      setNotice("Belum ada payload ESC/POS.");
      return;
    }
    const binary = atob(escposPayload.escpos_base64);
    const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0));
    const blob = new Blob([bytes], { type: "application/octet-stream" });
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = escposPayload.file_name;
    link.click();
    URL.revokeObjectURL(link.href);
  }

  async function handleOpenCashDrawerHardware() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }
    setIsOpeningDrawer(true);
    try {
      const payload = await openCashDrawer(auth.accessToken, {
        terminal_id: TERMINAL_ID,
      });
      await navigator.clipboard.writeText(payload.command_base64);
      setNotice("Command cash drawer berhasil dibuat dan disalin ke clipboard.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal membuat command cash drawer";
      setNotice(message);
    } finally {
      setIsOpeningDrawer(false);
    }
  }

  async function handleLoadDailyReport() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }

    setIsLoadingReport(true);
    try {
      const [report, logs] = await Promise.all([
        fetchDailyReport(auth.accessToken, STORE_ID, reportDate),
        fetchAuditLogs(auth.accessToken, STORE_ID, reportDate, 100),
      ]);
      setDailyReport(report);
      setAuditLogs(logs);
      if (auth.role === "admin") {
        const list = await fetchPromos(auth.accessToken);
        setPromos(list);
      }
      setNotice(`Laporan harian ${report.date} berhasil dimuat.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal memuat laporan";
      setNotice(message);
    } finally {
      setIsLoadingReport(false);
    }
  }

  async function handleLoadOperationalAlerts() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa melihat anomaly alerts.");
      return;
    }
    setIsLoadingAlerts(true);
    try {
      const response = await fetchOperationalAlerts(auth.accessToken, STORE_ID, reportDate);
      setAlerts(response.alerts);
      setNotice(`Anomaly alerts tanggal ${response.date} berhasil dimuat.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal memuat anomaly alerts";
      setNotice(message);
    } finally {
      setIsLoadingAlerts(false);
    }
  }

  async function refreshCashierUsers() {
    if (!auth || auth.role !== "admin") {
      return;
    }
    try {
      const users = await fetchCashiers(auth.accessToken);
      setCashierUsers(users);
    } catch (error) {
      console.error("[pos] cashier list refresh failed:", error);
    }
  }

  async function handleCreateCashier() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa menambah kasir.");
      return;
    }

    const username = newCashierUsername.trim().toLowerCase();
    if (!username || !newCashierPassword) {
      setNotice("Username dan password kasir wajib diisi.");
      return;
    }

    setIsSavingCashier(true);
    try {
      const cashier = await createCashier(auth.accessToken, {
        username,
        password: newCashierPassword,
      });
      setNewCashierUsername("");
      setNewCashierPassword("");
      await refreshCashierUsers();
      setNotice(`Kasir ${cashier.username} berhasil dibuat.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menambah kasir";
      setNotice(message);
    } finally {
      setIsSavingCashier(false);
    }
  }

  async function handleExportDailyCSV() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }

    try {
      const csv = await fetchDailyReportCSV(auth.accessToken, STORE_ID, reportDate);
      const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `daily-report-${reportDate}.csv`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      setNotice("Export CSV berhasil.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Export CSV gagal";
      setNotice(message);
    }
  }

  async function handlePrintDailyReport() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }

    try {
      const html = await fetchDailyReportPrintableHTML(auth.accessToken, STORE_ID, reportDate);
      const popup = window.open("", "_blank", "width=960,height=780");
      if (!popup) {
        setNotice("Popup diblokir browser.");
        return;
      }
      popup.document.write(html);
      popup.document.close();
      popup.focus();
      popup.print();
      setNotice("Preview PDF siap dicetak.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Export PDF gagal";
      setNotice(message);
    }
  }

  async function handleCreatePromo() {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa membuat promo.");
      return;
    }

    if (!promoName.trim()) {
      setNotice("Nama promo wajib diisi.");
      return;
    }

    setIsSavingPromo(true);
    try {
      await createPromo(auth.accessToken, {
        name: promoName.trim(),
        type: promoType,
        min_subtotal_cents: parsePositiveInt(promoMinSubtotalInput),
        discount_percent: promoType === "cart_percent" ? clamp(parseNumber(promoDiscountPercentInput, 0), 0, 100) : 0,
        flat_discount_cents: promoType === "flat_cart" ? parsePositiveInt(promoFlatDiscountInput) : 0,
      });
      const latestPromos = await fetchPromos(auth.accessToken);
      setPromos(latestPromos);
      setPromoName("");
      setPromoMinSubtotalInput("0");
      setPromoDiscountPercentInput("10");
      setPromoFlatDiscountInput("0");
      setNotice("Promo berhasil ditambahkan.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal membuat promo";
      setNotice(message);
    } finally {
      setIsSavingPromo(false);
    }
  }

  async function handleTogglePromo(promoID: string, active: boolean) {
    if (!auth || auth.role !== "admin") {
      setNotice("Hanya admin yang bisa mengubah promo.");
      return;
    }

    try {
      await setPromoActive(auth.accessToken, promoID, active);
      const latestPromos = await fetchPromos(auth.accessToken);
      setPromos(latestPromos);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal update promo";
      setNotice(message);
    }
  }

  async function handleLoginSubmit() {
    if (!loginUsername.trim() || !loginPassword) {
      setNotice("Username dan password wajib diisi.");
      return;
    }

    setIsLoggingIn(true);
    try {
      const payload = await login({
        username: loginUsername.trim(),
        password: loginPassword,
      });

      const session = toAuthSession(loginUsername.trim(), payload);
      persistAuthSession(session);
      setAuth(session);
      setNotice(`Login berhasil sebagai ${session.role}.`);
      setLoginPassword("");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Login gagal";
      setNotice(message);
    } finally {
      setIsLoggingIn(false);
    }
  }

  function handleLogout() {
    clearStoredAuthSession();
    setAuth(null);
    setCart([]);
    resetRecommendation();
    setNotice("Anda sudah logout.");
  }

  async function handleOpenShift() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }

    if (activeShift) {
      setNotice("Shift sudah aktif.");
      return;
    }

    const openingFloatCents = parsePositiveInt(openingFloatInput);
    if (!shiftCashierName.trim()) {
      setNotice("Nama kasir wajib diisi.");
      return;
    }

    setIsShiftLoading(true);
    try {
      const response = await openShift(auth.accessToken, {
        store_id: STORE_ID,
        terminal_id: TERMINAL_ID,
        cashier_name: shiftCashierName.trim(),
        opening_float_cents: openingFloatCents,
      });

      setActiveShift(response.shift);
      setNotice(`Shift dibuka untuk ${response.shift.cashier_name}.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal membuka shift";
      setNotice(message);
    } finally {
      setIsShiftLoading(false);
    }
  }

  async function handleCloseShift() {
    if (!auth || !activeShift) {
      setNotice("Tidak ada shift aktif.");
      return;
    }

    const closingCashCents = parsePositiveInt(closingCashInput || cashInput);

    setIsShiftLoading(true);
    try {
      await closeShift(auth.accessToken, {
        store_id: STORE_ID,
        terminal_id: TERMINAL_ID,
        closing_cash_cents: closingCashCents,
        notes: "close from terminal",
      });

      setActiveShift(null);
      setClosingCashInput("");
      clearCheckoutForm();
      setNotice("Shift berhasil ditutup.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menutup shift";
      setNotice(message);
    } finally {
      setIsShiftLoading(false);
    }
  }

  async function runOfflineSync() {
    if (!auth) {
      setNotice("Login diperlukan untuk sinkronisasi offline.");
      return;
    }

    if (isSyncing) {
      return;
    }

    setIsSyncing(true);
    try {
      const queue = await listOfflineCheckouts();
      if (queue.length === 0) {
        setNotice("Tidak ada transaksi offline untuk disinkronkan.");
        return;
      }

      const response = await syncOffline(auth.accessToken, {
        store_id: STORE_ID,
        terminal_id: TERMINAL_ID,
        envelope_id: generateId("sync"),
        transactions: queue,
      });

      const completedIDs = response.statuses
        .filter((status) => status.status === "accepted" || status.status === "duplicate")
        .map((status) => status.client_transaction_id);

      await removeOfflineCheckouts(completedIDs);
      const pending = await countOfflineCheckouts();
      setOfflineCount(pending);

      const rejectedCount = response.statuses.length - completedIDs.length;
      setNotice(`Sinkronisasi selesai. ${completedIDs.length} sukses, ${rejectedCount} gagal.`);

      const nextMetrics = await fetchAttachRate(auth.accessToken, STORE_ID, 30);
      setMetrics(nextMetrics);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Sinkronisasi gagal";
      setNotice(message);
    } finally {
      setIsSyncing(false);
    }
  }

  function buildReceiptSnapshot(checkoutResponse: CheckoutResponse): ReceiptSnapshot {
    return {
      checkout: checkoutResponse,
      cartDetails: cartDetails.map((line) => ({ ...line })),
      cashierName: activeShift?.cashier_name ?? auth?.username ?? "-",
      discountCents: pricing.discountCents,
      taxRatePercent: pricing.taxRatePercent,
    };
  }

  async function submitCheckout() {
    if (!auth) {
      setNotice("Silakan login dulu.");
      return;
    }

    if (!activeShift) {
      setNotice("Shift belum dibuka. Buka shift sebelum checkout.");
      return;
    }

    if (cart.length === 0) {
      setNotice("Keranjang masih kosong.");
      return;
    }

    if (paymentMethod !== "cash" && paymentMethod !== "split" && !paymentReference.trim()) {
      setNotice("Referensi pembayaran wajib diisi untuk metode non-cash.");
      return;
    }

    let cashReceivedCents = paymentMethod === "cash" ? parsePositiveInt(cashInput) : 0;
    let checkoutPaymentMethod: PaymentMethod = paymentMethod;
    let checkoutPaymentReference = paymentReference.trim() || undefined;
    let checkoutPaymentSplits: PaymentSplit[] | undefined;

    if (paymentMethod === "cash" && cashReceivedCents < pricing.totalCents) {
      setNotice("Nominal tunai kurang dari total transaksi.");
      return;
    }
    if (paymentMethod === "split") {
      if (splitPayments.length < 2) {
        setNotice("Split payment minimal 2 metode pembayaran.");
        return;
      }
      const splitTotal = splitPayments.reduce((sum, item) => sum + item.amount_cents, 0);
      if (splitTotal !== pricing.totalCents) {
        setNotice("Total split payment harus sama dengan total transaksi.");
        return;
      }
      checkoutPaymentMethod = "split";
      checkoutPaymentSplits = splitPayments;
      checkoutPaymentReference = undefined;
      cashReceivedCents = splitTotal;
    }

    const idempotencyKey = generateId("checkout");
    const payload: CheckoutRequest = {
      store_id: STORE_ID,
      terminal_id: TERMINAL_ID,
      idempotency_key: idempotencyKey,
      payment_method: checkoutPaymentMethod,
      payment_reference: checkoutPaymentReference,
      payment_splits: checkoutPaymentSplits,
      cash_received_cents: cashReceivedCents,
      discount_cents: pricing.discountCents,
      tax_rate_percent: pricing.taxRatePercent,
      manual_override: manualOverride,
      cart_items: cart,
      recommendation_info: recommendationState,
    };

    setIsSubmittingCheckout(true);
    try {
      if (!isOnline) {
        await enqueueOfflineCheckout({
          client_transaction_id: payload.idempotency_key,
          checkout: payload,
        });
        const pending = await countOfflineCheckouts();
        setOfflineCount(pending);
        clearCheckoutForm();
        setNotice("Offline mode aktif. Transaksi masuk queue sinkronisasi.");
        return;
      }

      const response = await checkout(auth.accessToken, payload);
      setLastCheckout(response);
      setLastReceipt(buildReceiptSnapshot(response));
      setNotice(response.duplicate ? "Transaksi duplikat terdeteksi, data lama ditampilkan." : "Checkout berhasil.");

      clearCheckoutForm();

      const nextMetrics = await fetchAttachRate(auth.accessToken, STORE_ID, 30);
      setMetrics(nextMetrics);
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        handleLogout();
        setNotice("Sesi login berakhir. Silakan login ulang.");
        return;
      }

      try {
        const lookup = await lookupCheckoutByIdempotency(auth.accessToken, payload.idempotency_key);
        if (lookup.found && lookup.checkout) {
          setLastCheckout(lookup.checkout);
          setLastReceipt(buildReceiptSnapshot(lookup.checkout));
          clearCheckoutForm();
          setNotice("Transaksi sudah tercatat sebelumnya (idempotency hit).");
          return;
        }
      } catch (error) {
        console.error("[pos] idempotency lookup failed, continuing with offline fallback:", error);
      }

      try {
        await enqueueOfflineCheckout({
          client_transaction_id: payload.idempotency_key,
          checkout: payload,
        });
        const pending = await countOfflineCheckouts();
        setOfflineCount(pending);
        clearCheckoutForm();
        setNotice("Checkout gagal ke backend. Transaksi dipindah ke offline queue.");
      } catch (queueError) {
        console.error("[pos] offline queue enqueue failed:", queueError);
        const message = error instanceof Error ? error.message : "Checkout gagal";
        setNotice(message);
      }
    } finally {
      setIsSubmittingCheckout(false);
    }
  }

  function acceptRecommendation() {
    if (!recommendation) {
      return;
    }

    addToCart(recommendation.sku, 1);
    setRecommendationState((prev) => ({ ...prev, accepted: true }));
    setNotice(`Item ${recommendation.name} ditambahkan dari Smart Basket AI.`);
  }

  function printLatestReceipt() {
    if (!lastReceipt) {
      setNotice("Belum ada struk untuk dicetak.");
      return;
    }

    const popup = window.open("", "_blank", "width=420,height=760");
    if (!popup) {
      setNotice("Popup diblokir browser. Izinkan pop-up untuk cetak struk.");
      return;
    }

    popup.document.write(buildReceiptHTML(lastReceipt, STORE_ID, TERMINAL_ID));
    popup.document.close();
    popup.focus();
    popup.print();
  }

  if (!authReady) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[var(--c-bg)]">
        <p className="text-sm text-[var(--c-text-muted)]">Menyiapkan terminal...</p>
      </div>
    );
  }

  if (!auth) {
    return (
      <div className="relative min-h-screen overflow-hidden bg-[var(--c-bg)]">
        <div className="pointer-events-none absolute inset-0">
          <div className="absolute -left-28 top-[-3rem] h-72 w-72 rounded-full bg-[var(--c-orb-a)] blur-3xl" />
          <div className="absolute right-[-4rem] top-[7rem] h-72 w-72 rounded-full bg-[var(--c-orb-b)] blur-3xl" />
        </div>

        <div className="relative mx-auto flex min-h-screen max-w-xl items-center px-4 py-8 sm:px-6">
          <LoginCard
            username={loginUsername}
            password={loginPassword}
            isLoggingIn={isLoggingIn}
            notice={notice}
            onUsernameChange={setLoginUsername}
            onPasswordChange={setLoginPassword}
            onSubmit={() => {
              void handleLoginSubmit();
            }}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="relative min-h-screen overflow-hidden bg-[var(--c-bg)] pb-8">
      <div className="pointer-events-none absolute inset-0">
        <div className="absolute -left-28 top-[-3rem] h-72 w-72 rounded-full bg-[var(--c-orb-a)] blur-3xl" />
        <div className="absolute right-[-4rem] top-[7rem] h-72 w-72 rounded-full bg-[var(--c-orb-b)] blur-3xl" />
      </div>

      <div className="relative mx-auto max-w-[1580px] px-4 py-5 sm:px-6 lg:px-8">
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-[290px_minmax(0,1fr)]">
          <aside className="rounded-2xl border border-[var(--c-border)] bg-[var(--c-panel)] p-4 shadow-[0_20px_40px_rgba(0,0,0,0.08)] lg:sticky lg:top-5 lg:h-fit">
            <div className="mb-4">
              <p className="font-display text-sm uppercase tracking-[0.18em] text-[var(--c-text-muted)]">
                KasirinAja
              </p>
              <h1 className="font-display text-3xl uppercase tracking-[0.06em] text-[var(--c-title)]">
                POS Dashboard
              </h1>
              <p className="text-xs text-[var(--c-text-muted)]">Store {STORE_ID} - {TERMINAL_ID}</p>
            </div>

            <div className="mb-4 flex flex-wrap gap-2">
              <Badge variant={isOnline ? "success" : "warning"}>{isOnline ? "Online" : "Offline"}</Badge>
              <Badge variant="muted">Role: {auth.role}</Badge>
              <Badge variant={activeShift ? "success" : "warning"}>
                Shift: {activeShift ? "Open" : "Closed"}
              </Badge>
              <Badge variant="muted">Queue: {offlineCount}</Badge>
            </div>

            <nav className="space-y-2">
              {dashboardMenus.map((menu) => {
                const isActive = currentMenu?.key === menu.key;
                return (
                  <button
                    key={menu.key}
                    type="button"
                    onClick={() => setActiveView(menu.key)}
                    className={
                      "w-full rounded-xl border px-3 py-2 text-left transition-colors " +
                      (isActive
                        ? "border-[var(--c-ink)] bg-[var(--c-panel-soft)]"
                        : "border-[var(--c-border)] bg-transparent hover:bg-[var(--c-panel-soft)]")
                    }
                  >
                    <p className="font-display text-lg uppercase tracking-[0.05em] text-[var(--c-title)]">
                      {menu.label}
                    </p>
                    <p className="text-xs text-[var(--c-text-muted)]">{menu.hint}</p>
                  </button>
                );
              })}
            </nav>

            <div className="mt-4 space-y-2 border-t border-[var(--c-border)] pt-4">
              <Button className="w-full justify-center" variant="outline" onClick={runOfflineSync} disabled={isSyncing}>
                {isSyncing ? "Sync..." : "Sinkron Offline"}
              </Button>
              <Button className="w-full justify-center" variant="ghost" onClick={handleLogout}>
                Logout
              </Button>
            </div>
          </aside>

          <main className="min-w-0 space-y-4">
            <Card>
              <CardContent className="flex flex-col gap-3 pt-5 md:flex-row md:items-end md:justify-between">
                <div>
                  <p className="font-display text-xs uppercase tracking-[0.2em] text-[var(--c-text-muted)]">
                    Menu Aktif
                  </p>
                  <h2 className="font-display text-3xl uppercase tracking-[0.05em] text-[var(--c-title)]">
                    {currentMenu?.label ?? "Dashboard"}
                  </h2>
                  <p className="text-sm text-[var(--c-text-muted)]">
                    {currentMenu?.hint ?? "Ringkasan operasional terminal."}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="accent">
                    Attach Rate 30H: {metrics ? `${metrics.attach_rate.toFixed(1)}%` : "-"}
                  </Badge>
                  <Badge variant="muted">
                    {new Date().toLocaleDateString("id-ID", {
                      day: "2-digit",
                      month: "long",
                      year: "numeric",
                    })}
                  </Badge>
                </div>
              </CardContent>
            </Card>

            {notice ? (
              <p className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-4 py-3 text-sm text-[var(--c-text)]">
                {notice}
              </p>
            ) : null}

            {currentMenu?.key === "overview" ? (
              <>
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
                  <Card>
                    <CardHeader>
                      <CardTitle>Total SKU</CardTitle>
                    </CardHeader>
                    <CardContent>
                      <p className="font-display text-4xl text-[var(--c-title)]">{products.length}</p>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>Item Keranjang</CardTitle>
                    </CardHeader>
                    <CardContent>
                      <p className="font-display text-4xl text-[var(--c-title)]">{pricing.itemCount}</p>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>Offline Queue</CardTitle>
                    </CardHeader>
                    <CardContent>
                      <p className="font-display text-4xl text-[var(--c-title)]">{offlineCount}</p>
                    </CardContent>
                  </Card>
                  <Card>
                    <CardHeader>
                      <CardTitle>Shift</CardTitle>
                    </CardHeader>
                    <CardContent>
                      <p className="font-display text-4xl text-[var(--c-title)]">
                        {activeShift ? "OPEN" : "CLOSED"}
                      </p>
                    </CardContent>
                  </Card>
                </div>

                <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1.2fr_1fr]">
                  <Card>
                    <CardHeader>
                      <CardTitle>Kontrol Shift Cepat</CardTitle>
                      <CardDescription>Akses cepat buka/tutup shift dari dashboard.</CardDescription>
                    </CardHeader>
                    <CardContent className="grid grid-cols-1 gap-3 md:grid-cols-3">
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Kasir</p>
                        <Input
                          value={shiftCashierName}
                          onChange={(event) => setShiftCashierName(event.target.value)}
                          placeholder="Nama kasir"
                          disabled={Boolean(activeShift)}
                        />
                      </div>
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Modal Awal</p>
                        <Input
                          inputMode="numeric"
                          value={openingFloatInput}
                          onChange={(event) => setOpeningFloatInput(event.target.value)}
                          placeholder="200000"
                          disabled={Boolean(activeShift)}
                        />
                      </div>
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Kas Akhir</p>
                        <Input
                          inputMode="numeric"
                          value={closingCashInput}
                          onChange={(event) => setClosingCashInput(event.target.value)}
                          placeholder="Isi saat tutup shift"
                          disabled={!activeShift}
                        />
                      </div>
                    </CardContent>
                    <CardFooter className="justify-between">
                      <p className="text-xs text-[var(--c-text-muted)]">
                        {activeShift
                          ? `Shift aktif oleh ${activeShift.cashier_name}`
                          : "Belum ada shift aktif"}
                      </p>
                      <div className="flex items-center gap-2">
                        <Button onClick={handleOpenShift} disabled={Boolean(activeShift) || isShiftLoading}>
                          Buka Shift
                        </Button>
                        <Button variant="outline" onClick={handleCloseShift} disabled={!activeShift || isShiftLoading}>
                          Tutup Shift
                        </Button>
                      </div>
                    </CardFooter>
                  </Card>

                  <Card>
                    <CardHeader>
                      <CardTitle>Transaksi Terakhir</CardTitle>
                      <CardDescription>Snapshot checkout terbaru dari terminal ini.</CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-2 text-sm text-[var(--c-text-muted)]">
                      {lastCheckout ? (
                        <>
                          <p>ID: {lastCheckout.transaction_id}</p>
                          <p>Status: {lastCheckout.status}</p>
                          <p>Total: {formatCurrency(lastCheckout.total_cents)}</p>
                          <p>Waktu: {new Date(lastCheckout.created_at).toLocaleString("id-ID")}</p>
                          <Button size="sm" variant="outline" onClick={printLatestReceipt}>
                            Cetak Struk
                          </Button>
                        </>
                      ) : (
                        <p>Belum ada transaksi yang tercatat pada sesi ini.</p>
                      )}
                    </CardContent>
                    <CardFooter className="justify-end">
                      <Button variant="outline" onClick={() => setActiveView("cashier")}>
                        Buka Menu Kasir
                      </Button>
                    </CardFooter>
                  </Card>
                </div>
              </>
            ) : null}

            {currentMenu?.key === "cashier" ? (
              <>
                <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1.2fr_1fr]">
                  <Card>
                    <CardHeader>
                      <CardTitle>Kontrol Shift</CardTitle>
                      <CardDescription>Kasir wajib membuka shift sebelum transaksi.</CardDescription>
                    </CardHeader>
                    <CardContent className="grid grid-cols-1 gap-3 md:grid-cols-3">
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Kasir</p>
                        <Input
                          value={shiftCashierName}
                          onChange={(event) => setShiftCashierName(event.target.value)}
                          placeholder="Nama kasir"
                          disabled={Boolean(activeShift)}
                        />
                      </div>
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Modal Awal</p>
                        <Input
                          inputMode="numeric"
                          value={openingFloatInput}
                          onChange={(event) => setOpeningFloatInput(event.target.value)}
                          placeholder="200000"
                          disabled={Boolean(activeShift)}
                        />
                      </div>
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Kas Akhir</p>
                        <Input
                          inputMode="numeric"
                          value={closingCashInput}
                          onChange={(event) => setClosingCashInput(event.target.value)}
                          placeholder="Isi saat tutup shift"
                          disabled={!activeShift}
                        />
                      </div>
                    </CardContent>
                    <CardFooter className="justify-between">
                      <div className="text-xs text-[var(--c-text-muted)]">
                        {activeShift ? (
                          <>
                            Shift aktif: <span className="font-semibold text-[var(--c-title)]">{activeShift.cashier_name}</span> sejak{" "}
                            {new Date(activeShift.opened_at).toLocaleTimeString("id-ID")}
                          </>
                        ) : (
                          "Belum ada shift aktif di terminal ini."
                        )}
                      </div>
                      <div className="flex items-center gap-2">
                        <Button onClick={handleOpenShift} disabled={Boolean(activeShift) || isShiftLoading}>
                          Buka Shift
                        </Button>
                        <Button variant="outline" onClick={handleCloseShift} disabled={!activeShift || isShiftLoading}>
                          Tutup Shift
                        </Button>
                      </div>
                    </CardFooter>
                  </Card>

                  <Card>
                    <CardHeader>
                      <CardTitle>Pembayaran</CardTitle>
                      <CardDescription>Atur metode bayar, diskon, pajak, dan override.</CardDescription>
                    </CardHeader>
                    <CardContent className="grid grid-cols-1 gap-3 md:grid-cols-2">
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Metode</p>
                        <select
                          className="h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none"
                          value={paymentMethod}
                          onChange={(event) => setPaymentMethod(event.target.value as PaymentMethod)}
                        >
                          <option value="cash">Tunai</option>
                          <option value="card">Kartu</option>
                          <option value="qris">QRIS</option>
                          <option value="ewallet">E-Wallet</option>
                          <option value="split">Split Payment</option>
                        </select>
                      </div>

                      <div className={paymentMethod === "split" ? "md:col-span-2" : ""}>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Referensi</p>
                        <Input
                          value={paymentReference}
                          onChange={(event) => setPaymentReference(event.target.value)}
                          placeholder={
                            paymentMethod === "cash"
                              ? "Opsional"
                              : paymentMethod === "split"
                                ? "Tidak wajib untuk split"
                                : "No. referensi transaksi"
                          }
                          disabled={paymentMethod === "split"}
                        />
                      </div>

                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Nominal Tunai</p>
                        <Input
                          inputMode="numeric"
                          value={cashInput}
                          onChange={(event) => setCashInput(event.target.value)}
                          placeholder="Masukkan nominal cash"
                          disabled={paymentMethod !== "cash" && paymentMethod !== "split"}
                        />
                      </div>

                      {paymentMethod === "split" ? (
                        <>
                          <div>
                            <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Split Tunai</p>
                            <Input inputMode="numeric" value={splitCashInput} onChange={(event) => setSplitCashInput(event.target.value)} placeholder="0" />
                          </div>
                          <div>
                            <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Split Kartu</p>
                            <Input inputMode="numeric" value={splitCardInput} onChange={(event) => setSplitCardInput(event.target.value)} placeholder="0" />
                          </div>
                          <div>
                            <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Split QRIS</p>
                            <Input inputMode="numeric" value={splitQrisInput} onChange={(event) => setSplitQrisInput(event.target.value)} placeholder="0" />
                          </div>
                          <div>
                            <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Ref QRIS</p>
                            <Input value={splitQrisReference} onChange={(event) => setSplitQrisReference(event.target.value)} placeholder="Opsional" />
                          </div>
                          <div>
                            <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Split E-Wallet</p>
                            <Input inputMode="numeric" value={splitEwalletInput} onChange={(event) => setSplitEwalletInput(event.target.value)} placeholder="0" />
                          </div>
                          <div>
                            <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Ref E-Wallet</p>
                            <Input value={splitEwalletReference} onChange={(event) => setSplitEwalletReference(event.target.value)} placeholder="Opsional" />
                          </div>
                        </>
                      ) : null}

                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Diskon (Rp)</p>
                        <Input
                          inputMode="numeric"
                          value={discountInput}
                          onChange={(event) => setDiscountInput(event.target.value)}
                          placeholder="0"
                        />
                      </div>

                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Pajak (%)</p>
                        <Input
                          inputMode="decimal"
                          value={taxRateInput}
                          onChange={(event) => setTaxRateInput(event.target.value)}
                          placeholder="11"
                        />
                      </div>

                      <label className="flex items-center gap-2 rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)]">
                        <input
                          type="checkbox"
                          checked={manualOverride}
                          onChange={(event) => setManualOverride(event.target.checked)}
                          disabled={auth.role !== "admin"}
                        />
                        Manual override {auth.role === "admin" ? "(admin)" : "(admin only)"}
                      </label>
                    </CardContent>
                  </Card>
                </div>

                <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1.2fr_1fr_0.9fr]">
                  <Card>
                    <CardHeader>
                      <CardTitle>Katalog Produk</CardTitle>
                      <CardDescription>Scan barcode/SKU cepat lalu tambahkan ke keranjang.</CardDescription>
                      <div className="flex gap-2">
                        <Input
                          value={barcodeInput}
                          onChange={(event) => setBarcodeInput(event.target.value)}
                          placeholder="Scan barcode / input SKU lalu Enter"
                          onKeyDown={(event) => {
                            if (event.key === "Enter") {
                              addFromBarcode();
                            }
                          }}
                        />
                        <Button variant="secondary" onClick={addFromBarcode}>
                          Scan
                        </Button>
                      </div>
                      <Input
                        value={search}
                        onChange={(event) => setSearch(event.target.value)}
                        placeholder="Cari SKU, nama produk, atau kategori"
                      />
                    </CardHeader>
                    <CardContent>
                      {loadingProducts ? (
                        <p className="text-sm text-[var(--c-text-muted)]">Memuat produk...</p>
                      ) : filteredProducts.length === 0 ? (
                        <p className="text-sm text-[var(--c-text-muted)]">Produk tidak ditemukan. Coba kata kunci lain.</p>
                      ) : (
                        <>
                          <p className="mb-3 text-xs text-[var(--c-text-muted)]">
                            {searchQuery
                              ? `Menampilkan ${visibleProducts.length} hasil pencarian.`
                              : showAllProducts
                                ? `Menampilkan semua ${filteredProducts.length} produk.`
                                : `Menampilkan ${visibleProducts.length} dari ${filteredProducts.length} produk. Gunakan pencarian untuk produk lain.`}
                          </p>
                          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                            {visibleProducts.map((product) => (
                              <article
                                key={product.sku}
                                className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-3"
                              >
                                <div className="mb-2 flex items-start justify-between gap-2">
                                  <div>
                                    <h3 className="text-sm font-semibold text-[var(--c-title)]">{product.name}</h3>
                                    <p className="text-xs text-[var(--c-text-muted)]">{product.sku}</p>
                                  </div>
                                  <Badge variant="muted">{product.category}</Badge>
                                </div>
                                <div className="flex items-end justify-between">
                                  <div>
                                    <p className="font-display text-xl tracking-[0.04em] text-[var(--c-title)]">
                                      {formatCurrency(product.price_cents)}
                                    </p>
                                    <p className="text-xs text-[var(--c-text-muted)]">
                                      Margin {(product.margin_rate * 100).toFixed(0)}%
                                    </p>
                                  </div>
                                  <Button size="sm" onClick={() => addToCart(product.sku)} disabled={!activeShift}>
                                    Tambah
                                  </Button>
                                </div>
                              </article>
                            ))}
                          </div>
                          {!searchQuery && filteredProducts.length > PRODUCT_PREVIEW_LIMIT ? (
                            <div className="mt-3 flex justify-center">
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => setShowAllProducts((current) => !current)}
                              >
                                {showAllProducts ? `Tampilkan ${PRODUCT_PREVIEW_LIMIT} saja` : "Lihat semua produk"}
                              </Button>
                            </div>
                          ) : null}
                        </>
                      )}
                    </CardContent>
                  </Card>

                  <Card>
                    <CardHeader>
                      <CardTitle>Keranjang</CardTitle>
                      <CardDescription>
                        {pricing.itemCount} item, throughput {queueSpeedHint.toFixed(1)} scan/menit
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-2">
                      {cartDetails.length === 0 ? (
                        <p className="rounded-lg border border-dashed border-[var(--c-border)] p-4 text-sm text-[var(--c-text-muted)]">
                          Belum ada item dalam keranjang.
                        </p>
                      ) : (
                        cartDetails.map((item) => (
                          <div
                            key={item.sku}
                            className="flex items-center justify-between rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 py-2"
                          >
                            <div>
                              <p className="text-sm font-semibold text-[var(--c-title)]">{item.name}</p>
                              <p className="text-xs text-[var(--c-text-muted)]">
                                {item.qty} x {formatCurrency(item.price_cents)}
                              </p>
                            </div>
                            <div className="flex items-center gap-1">
                              <Button size="sm" variant="outline" onClick={() => decreaseItem(item.sku)}>
                                -
                              </Button>
                              <Badge variant="muted">{item.qty}</Badge>
                              <Button size="sm" onClick={() => addToCart(item.sku, 1)}>
                                +
                              </Button>
                            </div>
                          </div>
                        ))
                      )}

                      <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-3">
                        <p className="mb-2 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
                          Hold Cart
                        </p>
                        <div className="mb-2 flex gap-2">
                          <Input
                            value={holdNote}
                            onChange={(event) => setHoldNote(event.target.value)}
                            placeholder="Catatan hold (opsional)"
                          />
                          <Button variant="outline" onClick={handleHoldCurrentCart} disabled={isHoldingCart || cart.length === 0}>
                            {isHoldingCart ? "Menyimpan..." : "Hold"}
                          </Button>
                        </div>
                        <div className="max-h-28 space-y-1 overflow-auto">
                          {heldCarts.length === 0 ? (
                            <p className="text-xs text-[var(--c-text-muted)]">
                              {isLoadingHeldCarts ? "Memuat hold cart..." : "Belum ada keranjang yang di-hold."}
                            </p>
                          ) : (
                            heldCarts.map((held) => (
                              <div key={held.id} className="flex items-center justify-between gap-2 rounded border border-[var(--c-border)] px-2 py-1 text-xs text-[var(--c-text-muted)]">
                                <span>{held.id} ({held.cart_items.length} item)</span>
                                <div className="flex items-center gap-1">
                                  <Button size="sm" variant="outline" onClick={() => void handleResumeHeldCart(held.id)}>
                                    Ambil
                                  </Button>
                                  <Button size="sm" variant="ghost" onClick={() => void handleDiscardHeldCart(held.id)}>
                                    Hapus
                                  </Button>
                                </div>
                              </div>
                            ))
                          )}
                        </div>
                      </div>
                    </CardContent>
                    <CardFooter className="justify-between">
                      <div className="space-y-1 text-xs text-[var(--c-text-muted)]">
                        <p className="flex items-center justify-between gap-8">
                          <span>Subtotal</span>
                          <span>{formatCurrency(pricing.subtotalCents)}</span>
                        </p>
                        <p className="flex items-center justify-between gap-8">
                          <span>Diskon</span>
                          <span>{formatCurrency(pricing.discountCents)}</span>
                        </p>
                        <p className="flex items-center justify-between gap-8">
                          <span>Pajak</span>
                          <span>{formatCurrency(pricing.taxCents)}</span>
                        </p>
                        <p className="font-display text-xl text-[var(--c-title)]">Total {formatCurrency(pricing.totalCents)}</p>
                      </div>
                      <Button variant="ghost" onClick={clearCheckoutForm}>
                        Kosongkan
                      </Button>
                    </CardFooter>
                  </Card>

                  <Card>
                    <CardHeader>
                      <CardTitle>Smart Basket AI</CardTitle>
                      <CardDescription>
                        Satu rekomendasi terbaik, satu klik untuk menambah ke keranjang.
                      </CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-4">
                      <div className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-3">
                        {recommendation ? (
                          <div className="space-y-3">
                            <div className="flex items-start justify-between gap-2">
                              <div>
                                <p className="text-sm font-semibold text-[var(--c-title)]">{recommendation.name}</p>
                                <p className="text-xs text-[var(--c-text-muted)]">{recommendation.sku}</p>
                              </div>
                              <Badge variant="accent">{(recommendation.confidence * 100).toFixed(0)}%</Badge>
                            </div>
                            <p className="text-xs text-[var(--c-text-muted)]">
                              Alasan: {reasonText(recommendation.reason_code)}
                            </p>
                            <p className="text-xs text-[var(--c-text-muted)]">
                              Potensi margin: {formatCurrency(recommendation.expected_margin_lift_cents)}
                            </p>
                            <div className="flex items-center gap-2">
                              <Button className="flex-1" onClick={acceptRecommendation}>
                                Tambah 1 Item
                              </Button>
                              <Button
                                variant="outline"
                                onClick={() => {
                                  setRecommendation(null);
                                  setNotice("Rekomendasi dilewati.");
                                }}
                              >
                                Lewati
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <p className="text-sm text-[var(--c-text-muted)]">
                            Belum ada rekomendasi aktif. Tambahkan item untuk memicu Smart Basket AI.
                          </p>
                        )}
                      </div>

                      <div className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-3">
                        <div className="mb-2 flex items-center justify-between text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
                          <span>Ringkasan Checkout</span>
                          <span>{paymentLabel(paymentMethod)}</span>
                        </div>
                        <div className="space-y-1 text-xs text-[var(--c-text-muted)]">
                          <p className="flex items-center justify-between">
                            <span>Total</span>
                            <span>{formatCurrency(pricing.totalCents)}</span>
                          </p>
                          <p className="flex items-center justify-between">
                            <span>Pembayaran Tercatat</span>
                            <span>
                              {formatCurrency(
                                paymentMethod === "cash"
                                  ? parsePositiveInt(cashInput)
                                  : paymentMethod === "split"
                                    ? splitPayments.reduce((sum, item) => sum + item.amount_cents, 0)
                                  : pricing.totalCents,
                              )}
                            </span>
                          </p>
                          <p className="flex items-center justify-between">
                            <span>Estimasi Kembalian</span>
                            <span>
                              {formatCurrency(
                                paymentMethod === "cash"
                                  ? Math.max(0, parsePositiveInt(cashInput) - pricing.totalCents)
                                  : paymentMethod === "split"
                                    ? Math.max(
                                        0,
                                        splitPayments.reduce((sum, item) => sum + item.amount_cents, 0) -
                                          pricing.totalCents,
                                      )
                                  : 0,
                              )}
                            </span>
                          </p>
                        </div>
                        <div className="mt-2 flex items-center justify-between text-xs text-[var(--c-text-muted)]">
                          <span>Latency rekomendasi</span>
                          <span>{latencyMS !== null ? `${latencyMS} ms` : "-"}</span>
                        </div>
                      </div>

                      {lastCheckout ? (
                        <div className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-3 text-xs text-[var(--c-text-muted)]">
                          <p className="mb-1 font-semibold text-[var(--c-title)]">Transaksi terakhir</p>
                          <p>ID: {lastCheckout.transaction_id}</p>
                          <p>Status: {lastCheckout.status}</p>
                          <p>Total: {formatCurrency(lastCheckout.total_cents)}</p>
                          <p>Kembalian: {formatCurrency(lastCheckout.change_cents)}</p>
                          <p>Waktu: {new Date(lastCheckout.created_at).toLocaleString("id-ID")}</p>
                          <Button size="sm" variant="outline" className="mt-2" onClick={printLatestReceipt}>
                            Cetak Struk
                          </Button>
                        </div>
                      ) : null}
                    </CardContent>
                    <CardFooter>
                      <Button
                        size="lg"
                        className="w-full"
                        onClick={submitCheckout}
                        disabled={isSubmittingCheckout || !activeShift}
                      >
                        {isSubmittingCheckout ? "Memproses..." : "Checkout Sekarang"}
                      </Button>
                    </CardFooter>
                  </Card>
                </div>

                <Card>
                  <CardHeader>
                    <CardTitle>Hardware Printer dan Cash Drawer</CardTitle>
                    <CardDescription>Generate payload ESC/POS dan command drawer untuk bridge lokal.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-[1fr_auto_auto]">
                      <Input
                        value={hardwareTxIDInput}
                        onChange={(event) => setHardwareTxIDInput(event.target.value)}
                        placeholder="Transaction ID (kosongkan untuk transaksi terakhir)"
                      />
                      <Button variant="outline" onClick={handleGenerateEscposReceipt} disabled={isGeneratingEscpos}>
                        {isGeneratingEscpos ? "Generate..." : "Generate ESC/POS"}
                      </Button>
                      <Button variant="outline" onClick={handleDownloadEscposPayload} disabled={!escposPayload}>
                        Download BIN
                      </Button>
                    </div>
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-[auto_1fr]">
                      <Button variant="secondary" onClick={handleOpenCashDrawerHardware} disabled={isOpeningDrawer}>
                        {isOpeningDrawer ? "Membuka..." : "Open Cash Drawer"}
                      </Button>
                      <p className="self-center text-xs text-[var(--c-text-muted)]">
                        Command drawer otomatis disalin ke clipboard dalam format base64.
                      </p>
                    </div>
                    <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-3 text-xs text-[var(--c-text-muted)]">
                      {escposPayload ? (
                        <div className="space-y-2">
                          <p className="font-semibold text-[var(--c-title)]">Payload siap untuk bridge printer</p>
                          <p>TX: {escposPayload.transaction_id}</p>
                          <p>File: {escposPayload.file_name}</p>
                          <textarea
                            className="min-h-24 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel)] px-2 py-2 text-xs text-[var(--c-text)] outline-none"
                            value={escposPayload.preview_text}
                            readOnly
                          />
                        </div>
                      ) : (
                        <p>Belum ada payload ESC/POS yang di-generate.</p>
                      )}
                    </div>
                  </CardContent>
                </Card>
              </>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "products" ? (
              <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Tambah Produk Baru</CardTitle>
                    <CardDescription>Form cepat admin untuk menambah SKU baru ke katalog.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid grid-cols-1 gap-3 md:grid-cols-3">
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">SKU</p>
                      <Input value={newProductSKU} onChange={(event) => setNewProductSKU(event.target.value)} placeholder="SKU-BARU-01" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Nama Produk</p>
                      <Input value={newProductName} onChange={(event) => setNewProductName(event.target.value)} placeholder="Biskuit Coklat" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Kategori</p>
                      <Input value={newProductCategory} onChange={(event) => setNewProductCategory(event.target.value)} placeholder="snack" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Harga</p>
                      <Input inputMode="numeric" value={newProductPriceInput} onChange={(event) => setNewProductPriceInput(event.target.value)} placeholder="8500" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Margin (%)</p>
                      <Input inputMode="decimal" value={newProductMarginInput} onChange={(event) => setNewProductMarginInput(event.target.value)} placeholder="25" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Stok Awal</p>
                      <Input inputMode="numeric" value={newProductInitialStockInput} onChange={(event) => setNewProductInitialStockInput(event.target.value)} placeholder="0" />
                    </div>
                  </CardContent>
                  <CardFooter className="justify-end gap-2">
                    <Button variant="outline" onClick={resetNewProductForm} disabled={isCreatingProduct}>Reset</Button>
                    <Button onClick={handleCreateProduct} disabled={isCreatingProduct}>{isCreatingProduct ? "Menyimpan..." : "Simpan Produk"}</Button>
                  </CardFooter>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Edit / Nonaktifkan Produk</CardTitle>
                    <CardDescription>Update nama, kategori, harga, margin, dan status aktif.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid grid-cols-1 gap-3 md:grid-cols-2">
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">SKU Target</p>
                      <Input value={manageProductSKU} onChange={(event) => setManageProductSKU(event.target.value)} placeholder="SKU-MIE-01" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Nama (opsional)</p>
                      <Input value={manageProductName} onChange={(event) => setManageProductName(event.target.value)} placeholder="Mie Goreng Instan" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Kategori (opsional)</p>
                      <Input value={manageProductCategory} onChange={(event) => setManageProductCategory(event.target.value)} placeholder="grocery" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Harga Baru (opsional)</p>
                      <Input inputMode="numeric" value={manageProductPriceInput} onChange={(event) => setManageProductPriceInput(event.target.value)} placeholder="3600" />
                    </div>
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Margin Baru % (opsional)</p>
                      <Input inputMode="decimal" value={manageProductMarginInput} onChange={(event) => setManageProductMarginInput(event.target.value)} placeholder="22" />
                    </div>
                    <label className="flex items-center gap-2 rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)]">
                      <input type="checkbox" checked={manageProductActive} onChange={(event) => setManageProductActive(event.target.checked)} />
                      Produk Aktif
                    </label>
                    <div className="md:col-span-2">
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Riwayat Harga</p>
                      <div className="max-h-40 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                        {priceHistory.length === 0 ? (
                          <p>Belum ada riwayat harga ditampilkan.</p>
                        ) : (
                          priceHistory.map((row) => (
                            <p key={row.id}>
                              {new Date(row.changed_at).toLocaleString("id-ID")} | {row.old_price_cents} -&gt; {row.new_price_cents} ({row.changed_by})
                            </p>
                          ))
                        )}
                      </div>
                    </div>
                  </CardContent>
                  <CardFooter className="justify-end">
                    <Button onClick={handleUpdateProduct} disabled={isUpdatingProduct}>{isUpdatingProduct ? "Menyimpan..." : "Update Produk"}</Button>
                  </CardFooter>
                </Card>
              </div>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "procurement" ? (
              <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Supplier</CardTitle>
                    <CardDescription>Tambah supplier untuk alur procurement.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <Input value={supplierNameInput} onChange={(event) => setSupplierNameInput(event.target.value)} placeholder="Nama supplier" />
                    <Input value={supplierPhoneInput} onChange={(event) => setSupplierPhoneInput(event.target.value)} placeholder="No. telepon" />
                    <Button onClick={handleCreateSupplier} disabled={isSavingSupplier}>
                      {isSavingSupplier ? "Menyimpan..." : "Tambah Supplier"}
                    </Button>
                    <div className="max-h-36 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                      {suppliers.length === 0 ? (
                        <p>Belum ada supplier.</p>
                      ) : (
                        suppliers.map((supplier) => (
                          <p key={supplier.id}>
                            {supplier.name} ({supplier.phone || "-"})
                          </p>
                        ))
                      )}
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Reorder Suggestions</CardTitle>
                    <CardDescription>Saran pembelian berdasarkan stok minimum.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <Button variant="outline" onClick={() => void refreshProcurementData()}>
                      Refresh Suggestions
                    </Button>
                    <div className="max-h-48 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                      {reorderSuggestions.length === 0 ? (
                        <p>Tidak ada SKU yang butuh reorder saat ini.</p>
                      ) : (
                        reorderSuggestions.map((item) => (
                          <p key={item.sku}>
                            {item.sku} | stok {item.current_stock} / min {item.reorder_point} | rekom {item.recommended_qty} | HPP {formatCurrency(item.last_cost_cents)}
                          </p>
                        ))
                      )}
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Batch Lot dan Expiry</CardTitle>
                    <CardDescription>Terima stok per batch dan pantau FEFO per tanggal kedaluwarsa.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                      <Input value={lotSKUInput} onChange={(event) => setLotSKUInput(event.target.value)} placeholder="SKU" />
                      <Input value={lotCodeInput} onChange={(event) => setLotCodeInput(event.target.value)} placeholder="Kode lot (opsional)" />
                      <Input type="date" value={lotExpiryInput} onChange={(event) => setLotExpiryInput(event.target.value)} />
                      <Input inputMode="numeric" value={lotQtyInput} onChange={(event) => setLotQtyInput(event.target.value)} placeholder="Qty batch" />
                      <Input inputMode="numeric" value={lotCostInput} onChange={(event) => setLotCostInput(event.target.value)} placeholder="Cost / unit" />
                      <Input value={lotNotesInput} onChange={(event) => setLotNotesInput(event.target.value)} placeholder="Catatan batch" />
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Button onClick={handleCreateInventoryLot} disabled={isSavingLot}>
                        {isSavingLot ? "Menyimpan..." : "Terima Batch"}
                      </Button>
                      <Button variant="outline" onClick={() => void refreshProcurementData()}>
                        Refresh Lot
                      </Button>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Input
                        value={lotFilterSKUInput}
                        onChange={(event) => setLotFilterSKUInput(event.target.value)}
                        placeholder="Filter SKU lot"
                        onKeyDown={(event) => {
                          if (event.key === "Enter") {
                            void refreshProcurementData();
                          }
                        }}
                      />
                    </div>
                    <div className="max-h-52 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                      {inventoryLots.length === 0 ? (
                        <p>Belum ada data lot.</p>
                      ) : (
                        inventoryLots.map((lot) => (
                          <div key={lot.id} className="mb-2 rounded border border-[var(--c-border)] px-2 py-1">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-semibold text-[var(--c-title)]">{lot.sku}</span>
                              <Badge variant={lot.qty_available > 0 ? "success" : "muted"}>stok {lot.qty_available}</Badge>
                            </div>
                            <p>
                              Lot {lot.lot_code} | terima {lot.qty_received} | sisa {lot.qty_available} | HPP{" "}
                              {formatCurrency(lot.cost_cents)}
                            </p>
                            <p>
                              Exp: {lot.expiry_date ? new Date(lot.expiry_date).toLocaleDateString("id-ID") : "-"} | sumber:{" "}
                              {lot.source_type}
                            </p>
                          </div>
                        ))
                      )}
                    </div>
                  </CardContent>
                </Card>

                <Card className="xl:col-span-2">
                  <CardHeader>
                    <CardTitle>Purchase Order</CardTitle>
                    <CardDescription>Buat PO dan receive barang untuk update stok + HPP.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
                      <select
                        className="h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none"
                        value={selectedSupplierID}
                        onChange={(event) => setSelectedSupplierID(event.target.value)}
                      >
                        <option value="">Pilih supplier</option>
                        {suppliers.map((supplier) => (
                          <option key={supplier.id} value={supplier.id}>
                            {supplier.name}
                          </option>
                        ))}
                      </select>
                      <Input value={poReceivedByInput} onChange={(event) => setPoReceivedByInput(event.target.value)} placeholder="Nama penerima barang" />
                      <Button onClick={handleCreatePurchaseOrder} disabled={isSavingPO}>
                        {isSavingPO ? "Menyimpan..." : "Buat PO"}
                      </Button>
                    </div>
                    <textarea
                      className="min-h-24 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 py-2 text-sm text-[var(--c-text)] outline-none"
                      value={poItemsRaw}
                      onChange={(event) => setPoItemsRaw(event.target.value)}
                      placeholder={"SKU-MIE-01,20,2000\nSKU-TELUR-01,10,21000"}
                    />
                    <div className="max-h-48 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                      {purchaseOrders.length === 0 ? (
                        <p>Belum ada purchase order.</p>
                      ) : (
                        purchaseOrders.map((po) => (
                          <div key={po.id} className="mb-2 flex items-center justify-between gap-2">
                            <span>
                              {po.id} | {po.status} | supplier {po.supplier_id} | item {po.items.length}
                            </span>
                            {po.status !== "received" ? (
                              <Button size="sm" variant="outline" onClick={() => void handleReceivePurchaseOrder(po.id)}>
                                Receive
                              </Button>
                            ) : (
                              <Badge variant="success">Received</Badge>
                            )}
                          </div>
                        ))
                      )}
                    </div>
                  </CardContent>
                </Card>
              </div>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "operations" ? (
              <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Stock Opname</CardTitle>
                    <CardDescription>Set stok fisik per SKU dengan format SKU,QTY per baris.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Data Opname</p>
                      <textarea
                        className="min-h-32 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 py-2 text-sm text-[var(--c-text)] outline-none"
                        value={stockOpnameRaw}
                        onChange={(event) => setStockOpnameRaw(event.target.value)}
                        placeholder={"SKU-MIE-01,120\nSKU-TELUR-01,80"}
                      />
                    </div>
                    <Input value={stockOpnameNotes} onChange={(event) => setStockOpnameNotes(event.target.value)} placeholder="Catatan opname" />
                    <Button variant="outline" onClick={handleRunStockOpname} disabled={isRunningStockOpname}>
                      {isRunningStockOpname ? "Memproses..." : "Jalankan Stock Opname"}
                    </Button>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Void dan Refund</CardTitle>
                    <CardDescription>Semua aksi membutuhkan manager PIN.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <Input type="password" value={managerPinInput} onChange={(event) => setManagerPinInput(event.target.value)} placeholder="Manager PIN" />
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                      <Input value={voidTransactionID} onChange={(event) => setVoidTransactionID(event.target.value)} placeholder="Transaction ID untuk void" />
                      <Input value={voidReason} onChange={(event) => setVoidReason(event.target.value)} placeholder="Alasan void" />
                    </div>
                    <Button variant="outline" onClick={handleVoidTransaction}>Void Transaksi</Button>
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
                      <Input value={refundTransactionID} onChange={(event) => setRefundTransactionID(event.target.value)} placeholder="Transaction ID refund" />
                      <Input inputMode="numeric" value={refundAmountInput} onChange={(event) => setRefundAmountInput(event.target.value)} placeholder="Nominal refund" />
                      <Input value={refundReason} onChange={(event) => setRefundReason(event.target.value)} placeholder="Alasan refund" />
                    </div>
                    <Button variant="outline" onClick={handleRefundTransaction}>Refund Transaksi</Button>
                  </CardContent>
                </Card>

                <Card className="xl:col-span-2">
                  <CardHeader>
                    <CardTitle>Item Return dan Exchange</CardTitle>
                    <CardDescription>Validasi return per item berdasarkan transaksi asli.</CardDescription>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <Input
                      type="password"
                      value={managerPinInput}
                      onChange={(event) => setManagerPinInput(event.target.value)}
                      placeholder="Manager PIN"
                    />
                    <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                      <Input
                        value={itemReturnTxIDInput}
                        onChange={(event) => setItemReturnTxIDInput(event.target.value)}
                        placeholder="Transaction ID asal"
                      />
                      <select
                        className="h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none"
                        value={itemReturnMode}
                        onChange={(event) => setItemReturnMode(event.target.value as "refund" | "exchange")}
                      >
                        <option value="refund">Refund Item</option>
                        <option value="exchange">Exchange Item</option>
                      </select>
                    </div>
                    <Input
                      value={itemReturnReasonInput}
                      onChange={(event) => setItemReturnReasonInput(event.target.value)}
                      placeholder="Alasan return/exchange"
                    />
                    <div>
                      <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Item Return (SKU,QTY)</p>
                      <textarea
                        className="min-h-20 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 py-2 text-sm text-[var(--c-text)] outline-none"
                        value={itemReturnItemsRaw}
                        onChange={(event) => setItemReturnItemsRaw(event.target.value)}
                        placeholder={"SKU-MIE-01,1\nSKU-MINUM-01,2"}
                      />
                    </div>
                    {itemReturnMode === "exchange" ? (
                      <>
                        <div>
                          <p className="mb-1 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">Item Exchange (SKU,QTY)</p>
                          <textarea
                            className="min-h-20 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 py-2 text-sm text-[var(--c-text)] outline-none"
                            value={itemExchangeItemsRaw}
                            onChange={(event) => setItemExchangeItemsRaw(event.target.value)}
                            placeholder={"SKU-BARU-01,1"}
                          />
                        </div>
                        <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
                          <select
                            className="h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none"
                            value={itemReturnPaymentMethod}
                            onChange={(event) => setItemReturnPaymentMethod(event.target.value as PaymentMethod)}
                          >
                            <option value="cash">Tunai</option>
                            <option value="card">Kartu</option>
                            <option value="qris">QRIS</option>
                            <option value="ewallet">E-Wallet</option>
                          </select>
                          <Input
                            value={itemReturnPaymentReference}
                            onChange={(event) => setItemReturnPaymentReference(event.target.value)}
                            placeholder="Referensi (jika non-tunai)"
                          />
                          <Input
                            inputMode="numeric"
                            value={itemReturnCashReceivedInput}
                            onChange={(event) => setItemReturnCashReceivedInput(event.target.value)}
                            placeholder="Tunai diterima (jika kurang bayar)"
                          />
                        </div>
                      </>
                    ) : null}
                    <Button variant="outline" onClick={handleProcessItemReturn} disabled={isProcessingItemReturn}>
                      {isProcessingItemReturn ? "Memproses..." : "Proses Return / Exchange"}
                    </Button>
                  </CardContent>
                </Card>
              </div>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "promos" ? (
              <Card>
                <CardHeader>
                  <CardTitle>Promo Engine</CardTitle>
                  <CardDescription>Buat promo cart percentage atau flat cart discount.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <Input value={promoName} onChange={(event) => setPromoName(event.target.value)} placeholder="Nama promo" />
                  <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
                    <select
                      className="h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none"
                      value={promoType}
                      onChange={(event) => setPromoType(event.target.value as "cart_percent" | "flat_cart")}
                    >
                      <option value="cart_percent">cart_percent</option>
                      <option value="flat_cart">flat_cart</option>
                    </select>
                    <Input inputMode="numeric" value={promoMinSubtotalInput} onChange={(event) => setPromoMinSubtotalInput(event.target.value)} placeholder="Min subtotal" />
                  </div>
                  {promoType === "cart_percent" ? (
                    <Input inputMode="decimal" value={promoDiscountPercentInput} onChange={(event) => setPromoDiscountPercentInput(event.target.value)} placeholder="Discount %" />
                  ) : (
                    <Input inputMode="numeric" value={promoFlatDiscountInput} onChange={(event) => setPromoFlatDiscountInput(event.target.value)} placeholder="Flat discount cents" />
                  )}
                  <Button onClick={handleCreatePromo} disabled={isSavingPromo}>{isSavingPromo ? "Menyimpan..." : "Simpan Promo"}</Button>
                  <div className="max-h-48 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                    {promos.length === 0 ? <p>Belum ada promo.</p> : promos.map((promo) => (
                      <div key={promo.id} className="mb-2 flex items-center justify-between gap-2">
                        <span>{promo.name} ({promo.type})</span>
                        <Button size="sm" variant="outline" onClick={() => void handleTogglePromo(promo.id, !promo.active)}>
                          {promo.active ? "Nonaktifkan" : "Aktifkan"}
                        </Button>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "reports" ? (
              <Card>
                <CardHeader>
                  <CardTitle>Laporan Harian + Audit Log</CardTitle>
                  <CardDescription>Ringkasan omzet, margin, metode bayar, export CSV/PDF, dan jejak audit.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Input type="date" value={reportDate} onChange={(event) => setReportDate(event.target.value)} className="max-w-56" />
                    <Button variant="outline" onClick={handleLoadDailyReport} disabled={isLoadingReport}>{isLoadingReport ? "Memuat..." : "Muat Laporan"}</Button>
                    <Button variant="outline" onClick={handleExportDailyCSV}>Export CSV</Button>
                    <Button variant="outline" onClick={handlePrintDailyReport}>Export PDF</Button>
                  </div>
                  {dailyReport ? (
                    <div className="grid grid-cols-1 gap-2 text-xs text-[var(--c-text-muted)] md:grid-cols-3">
                      <p>Transaksi: {dailyReport.transactions}</p>
                      <p>Omzet Bersih: {formatCurrency(dailyReport.net_sales_cents)}</p>
                      <p>Margin Estimasi: {formatCurrency(dailyReport.estimated_margin_cents)}</p>
                    </div>
                  ) : null}
                  <div className="max-h-52 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                    {auditLogs.length === 0 ? (
                      <p>Audit log belum ada untuk tanggal ini.</p>
                    ) : (
                      auditLogs.map((row) => (
                        <p key={row.id}>
                          {new Date(row.created_at).toLocaleTimeString("id-ID")} | {row.actor_username} | {row.action} | {row.entity_type}:{row.entity_id}
                        </p>
                      ))
                    )}
                  </div>
                </CardContent>
              </Card>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "alerts" ? (
              <Card>
                <CardHeader>
                  <CardTitle>Operational Anomaly Alerts</CardTitle>
                  <CardDescription>Deteksi void/refund spike, override berlebih, dan frekuensi opname.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Input type="date" value={reportDate} onChange={(event) => setReportDate(event.target.value)} className="max-w-56" />
                    <Button variant="outline" onClick={handleLoadOperationalAlerts} disabled={isLoadingAlerts}>
                      {isLoadingAlerts ? "Memuat..." : "Muat Alerts"}
                    </Button>
                  </div>
                  <div className="max-h-64 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                    {alerts.length === 0 ? (
                      <p>Tidak ada alert untuk tanggal ini.</p>
                    ) : (
                      alerts.map((alert) => (
                        <p key={alert.id}>
                          [{alert.severity}] {alert.title} | {alert.description}
                        </p>
                      ))
                    )}
                  </div>
                </CardContent>
              </Card>
            ) : null}

            {auth.role === "admin" && currentMenu?.key === "team" ? (
              <Card>
                <CardHeader>
                  <CardTitle>Manajemen Kasir</CardTitle>
                  <CardDescription>Tambah akun kasir baru untuk login terminal.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
                    <Input value={newCashierUsername} onChange={(event) => setNewCashierUsername(event.target.value)} placeholder="username kasir" />
                    <Input type="password" value={newCashierPassword} onChange={(event) => setNewCashierPassword(event.target.value)} placeholder="password (min 6)" />
                    <Button onClick={handleCreateCashier} disabled={isSavingCashier}>
                      {isSavingCashier ? "Menyimpan..." : "Tambah Kasir"}
                    </Button>
                  </div>
                  <div className="max-h-56 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
                    {cashierUsers.length === 0 ? (
                      <p>Belum ada user kasir tambahan.</p>
                    ) : (
                      cashierUsers.map((user) => (
                        <p key={user.username}>
                          {user.username} | aktif: {user.active ? "ya" : "tidak"} | dibuat: {new Date(user.created_at).toLocaleString("id-ID")}
                        </p>
                      ))
                    )}
                  </div>
                </CardContent>
              </Card>
            ) : null}
          </main>
        </div>
      </div>
    </div>
  );
}
