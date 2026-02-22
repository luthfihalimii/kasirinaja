package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"kasirinaja/backend/internal/cache"
	"kasirinaja/backend/internal/domain"
	"kasirinaja/backend/internal/recommendation"
	"kasirinaja/backend/internal/store"
	"kasirinaja/backend/internal/store/memory"
)

func newTestService() *Service {
	repo := memory.NewSeeded()
	recommender := recommendation.NewEngine(cache.NoopRecommendationCache{}, 5*time.Second)
	return New(repo, recommender, "main-store")
}

func TestCheckoutRequiresActiveShift(t *testing.T) {
	svc := newTestService()

	_, err := svc.Checkout(context.Background(), domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-no-shift",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 2},
		},
	})
	if err == nil {
		t.Fatalf("expected checkout to fail when no shift is open")
	}
}

func TestCheckoutWithOpenShiftSupportsNonCash(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	resp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-card",
		PaymentMethod:     "card",
		PaymentReference:  "CARD-REF-001",
		DiscountCents:     1000,
		TaxRatePercent:    11,
		ManualOverride:    true,
		CashReceivedCents: 0,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if resp.PaymentMethod != "card" {
		t.Fatalf("expected card payment method, got %s", resp.PaymentMethod)
	}
	if resp.TotalCents <= 0 {
		t.Fatalf("expected total cents > 0")
	}
}

func TestCheckoutLookupByIdempotency(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	_, err = svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-lookup",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	lookup, err := svc.LookupCheckoutByIdempotency(ctx, "idem-lookup")
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if !lookup.Found || lookup.Checkout == nil {
		t.Fatalf("expected checkout to be found")
	}
}

func TestVoidAndRefundLifecycle(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	checkoutResp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-void",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	_, err = svc.VoidTransaction(ctx, domain.VoidTransactionRequest{
		TransactionID: checkoutResp.TransactionID,
		Reason:        "wrong scan",
	})
	if err != nil {
		t.Fatalf("void failed: %v", err)
	}

	_, err = svc.VoidTransaction(ctx, domain.VoidTransactionRequest{
		TransactionID: checkoutResp.TransactionID,
		Reason:        "duplicate void",
	})
	if err == nil {
		t.Fatalf("expected second void to fail")
	}

	secondCheckoutResp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-refund",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		CartItems: []domain.CartItem{
			{SKU: "SKU-TELUR-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("second checkout failed: %v", err)
	}

	refundResp, err := svc.Refund(ctx, domain.RefundRequest{
		OriginalTransactionID: secondCheckoutResp.TransactionID,
		Reason:                "customer return",
		AmountCents:           3000,
	})
	if err != nil {
		t.Fatalf("refund failed: %v", err)
	}
	if refundResp.Refund.AmountCents != 3000 {
		t.Fatalf("expected refund amount 3000, got %d", refundResp.Refund.AmountCents)
	}
}

func TestCreateProductAdminSuccess(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	product, err := svc.CreateProduct(ctx, domain.ProductCreateRequest{
		StoreID:      "main-store",
		SKU:          "SKU-BARU-01",
		Name:         "Biskuit Coklat",
		Category:     "snack",
		PriceCents:   8500,
		MarginRate:   0.30,
		InitialStock: 40,
	})
	if err != nil {
		t.Fatalf("create product failed: %v", err)
	}
	if product.SKU != "SKU-BARU-01" {
		t.Fatalf("unexpected sku: %s", product.SKU)
	}

	products, err := svc.ListProducts(ctx)
	if err != nil {
		t.Fatalf("list products failed: %v", err)
	}

	found := false
	for _, item := range products {
		if item.SKU == "SKU-BARU-01" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected new product to be listed")
	}
}

func TestCreateProductRequiresAdmin(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "cashier",
		Role:     "cashier",
	})

	_, err := svc.CreateProduct(ctx, domain.ProductCreateRequest{
		StoreID:      "main-store",
		SKU:          "SKU-BARU-02",
		Name:         "Kerupuk Udang",
		Category:     "snack",
		PriceCents:   7000,
		MarginRate:   0.25,
		InitialStock: 30,
	})
	if err == nil {
		t.Fatalf("expected non-admin create product to fail")
	}
}

func TestCheckoutSplitPayment(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir Split",
		OpeningFloatCents: 200000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	resp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-split",
		PaymentMethod:     "split",
		TaxRatePercent:    0,
		CashReceivedCents: 0,
		PaymentSplits: []domain.PaymentSplit{
			{Method: "cash", AmountCents: 3000},
			{Method: "qris", AmountCents: 4000, Reference: "TRX-QRIS-001"},
		},
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 2},
		},
	})
	if err != nil {
		t.Fatalf("split checkout failed: %v", err)
	}
	if resp.PaymentMethod != "split" {
		t.Fatalf("expected split payment, got %s", resp.PaymentMethod)
	}
	if len(resp.PaymentSplits) != 2 {
		t.Fatalf("expected 2 payment splits, got %d", len(resp.PaymentSplits))
	}
}

func TestHoldAndResumeCart(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "cashier",
		Role:     "cashier",
	})

	held, err := svc.HoldCart(ctx, domain.HoldCartRequest{
		StoreID:        "main-store",
		TerminalID:     "terminal-a1",
		Note:           "customer ambil dompet",
		PaymentMethod:  "cash",
		DiscountCents:  0,
		TaxRatePercent: 11,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
			{SKU: "SKU-TELUR-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("hold cart failed: %v", err)
	}

	list, err := svc.ListHeldCarts(ctx, "main-store", "terminal-a1")
	if err != nil {
		t.Fatalf("list held carts failed: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 held cart, got %d", len(list.Items))
	}

	resumed, err := svc.ResumeHeldCart(ctx, held.HeldCart.ID)
	if err != nil {
		t.Fatalf("resume held cart failed: %v", err)
	}
	if len(resumed.HeldCart.CartItems) != 2 {
		t.Fatalf("expected resumed cart with 2 items")
	}

	afterResume, err := svc.ListHeldCarts(ctx, "main-store", "terminal-a1")
	if err != nil {
		t.Fatalf("list after resume failed: %v", err)
	}
	if len(afterResume.Items) != 0 {
		t.Fatalf("expected held carts to be empty after resume")
	}
}

func TestProcurementReceiveAndReorderSuggestions(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.StockOpname(ctx, domain.StockOpnameRequest{
		StoreID: "main-store",
		Notes:   "set low stock",
		Items: []domain.StockOpnameItem{
			{SKU: "SKU-MIE-01", CountedQty: 5},
		},
	})
	if err != nil {
		t.Fatalf("stock opname failed: %v", err)
	}

	supplier, err := svc.CreateSupplier(ctx, domain.SupplierCreateRequest{
		Name:  "Supplier Test",
		Phone: "08123456789",
	})
	if err != nil {
		t.Fatalf("create supplier failed: %v", err)
	}

	poResp, err := svc.CreatePurchaseOrder(ctx, domain.PurchaseOrderCreateRequest{
		StoreID:    "main-store",
		SupplierID: supplier.ID,
		Items: []domain.PurchaseOrderItem{
			{SKU: "SKU-MIE-01", Qty: 20, CostCents: 2000},
		},
	})
	if err != nil {
		t.Fatalf("create purchase order failed: %v", err)
	}

	received, err := svc.ReceivePurchaseOrder(ctx, poResp.PurchaseOrder.ID, domain.PurchaseOrderReceiveRequest{
		ReceivedBy: "manager-a",
	})
	if err != nil {
		t.Fatalf("receive purchase order failed: %v", err)
	}
	if received.PurchaseOrder.Status != "received" {
		t.Fatalf("expected PO status received, got %s", received.PurchaseOrder.Status)
	}
	if received.PurchaseOrder.ReceivedBy != "manager-a" {
		t.Fatalf("expected received_by manager-a, got %s", received.PurchaseOrder.ReceivedBy)
	}

	suggestions, err := svc.ReorderSuggestions(ctx, "main-store")
	if err != nil {
		t.Fatalf("reorder suggestions failed: %v", err)
	}

	found := false
	for _, item := range suggestions.Suggestions {
		if item.SKU == "SKU-MIE-01" {
			found = true
			if item.CurrentStock != 25 {
				t.Fatalf("expected current stock 25 after receive, got %d", item.CurrentStock)
			}
			if item.LastCostCents != 2000 {
				t.Fatalf("expected last cost 2000, got %d", item.LastCostCents)
			}
		}
	}
	if !found {
		t.Fatalf("expected reorder suggestion for SKU-MIE-01")
	}
}

func TestDetectOperationalAnomalies(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		resp, checkoutErr := svc.Checkout(ctx, domain.CheckoutRequest{
			StoreID:           "main-store",
			TerminalID:        "terminal-a1",
			IdempotencyKey:    "idem-anom-" + strconv.Itoa(i),
			PaymentMethod:     "cash",
			CashReceivedCents: 100000,
			CartItems: []domain.CartItem{
				{SKU: "SKU-MIE-01", Qty: 1},
			},
		})
		if checkoutErr != nil {
			t.Fatalf("checkout #%d failed: %v", i, checkoutErr)
		}
		_, voidErr := svc.VoidTransaction(ctx, domain.VoidTransactionRequest{
			TransactionID: resp.TransactionID,
			Reason:        "void test",
		})
		if voidErr != nil {
			t.Fatalf("void #%d failed: %v", i, voidErr)
		}
	}

	alerts, err := svc.DetectOperationalAnomalies(ctx, "main-store", time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		t.Fatalf("detect anomalies failed: %v", err)
	}

	foundVoidSpike := false
	for _, alert := range alerts.Alerts {
		if alert.Code == "void_spike" {
			foundVoidSpike = true
			break
		}
	}
	if !foundVoidSpike {
		t.Fatalf("expected void_spike anomaly to be detected")
	}
}

func TestRefundRejectsCumulativeOverRefund(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	resp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-refund-cumulative",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		TaxRatePercent:    0,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	_, err = svc.Refund(ctx, domain.RefundRequest{
		OriginalTransactionID: resp.TransactionID,
		Reason:                "partial refund",
		AmountCents:           2000,
	})
	if err != nil {
		t.Fatalf("first refund failed: %v", err)
	}

	_, err = svc.Refund(ctx, domain.RefundRequest{
		OriginalTransactionID: resp.TransactionID,
		Reason:                "over refund",
		AmountCents:           2000,
	})
	if err == nil {
		t.Fatalf("expected second refund to fail due cumulative over-refund")
	}
}

func TestVoidRejectedForRefundedTransaction(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	resp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-refund-before-void",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		TaxRatePercent:    0,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	_, err = svc.Refund(ctx, domain.RefundRequest{
		OriginalTransactionID: resp.TransactionID,
		Reason:                "full refund",
		AmountCents:           resp.TotalCents,
	})
	if err != nil {
		t.Fatalf("refund failed: %v", err)
	}

	_, err = svc.VoidTransaction(ctx, domain.VoidTransactionRequest{
		TransactionID: resp.TransactionID,
		Reason:        "void after refund",
	})
	if err == nil {
		t.Fatalf("expected void to fail for refunded transaction")
	}
}

func TestRefundRejectsVoidedTransactionWithInvalidTransactionError(t *testing.T) {
	svc := newTestService()
	ctx := WithActor(context.Background(), domain.Actor{
		Username: "admin",
		Role:     "admin",
	})

	_, err := svc.OpenShift(ctx, domain.ShiftOpenRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		CashierName:       "Kasir A",
		OpeningFloatCents: 250000,
	})
	if err != nil {
		t.Fatalf("open shift failed: %v", err)
	}

	resp, err := svc.Checkout(ctx, domain.CheckoutRequest{
		StoreID:           "main-store",
		TerminalID:        "terminal-a1",
		IdempotencyKey:    "idem-void-before-refund",
		PaymentMethod:     "cash",
		CashReceivedCents: 100000,
		TaxRatePercent:    0,
		CartItems: []domain.CartItem{
			{SKU: "SKU-MIE-01", Qty: 1},
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	_, err = svc.VoidTransaction(ctx, domain.VoidTransactionRequest{
		TransactionID: resp.TransactionID,
		Reason:        "void before refund",
	})
	if err != nil {
		t.Fatalf("void failed: %v", err)
	}

	_, err = svc.Refund(ctx, domain.RefundRequest{
		OriginalTransactionID: resp.TransactionID,
		Reason:                "refund should be rejected",
		AmountCents:           1000,
	})
	if !errors.Is(err, store.ErrInvalidTransaction) {
		t.Fatalf("expected ErrInvalidTransaction for refund on voided transaction, got %v", err)
	}
}
