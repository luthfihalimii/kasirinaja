package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestVoidTransactionRestocksInventory(t *testing.T) {
	databaseURL := os.Getenv("KASIRINAJA_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KASIRINAJA_TEST_DATABASE_URL to run postgres integration test")
	}

	ctx := context.Background()
	s, err := New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})

	stamp := time.Now().UnixNano()
	sku := fmt.Sprintf("SKU-VOID-IT-%d", stamp)
	txID := fmt.Sprintf("tx-void-it-%d", stamp)
	idempotencyKey := fmt.Sprintf("idem-void-it-%d", stamp)
	storeID := "main-store"

	t.Cleanup(func() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM transaction_items WHERE transaction_id = $1`, txID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM transactions WHERE id = $1`, txID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM inventory_stocks WHERE store_id = $1 AND sku = $2`, storeID, sku)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM products WHERE sku = $1`, sku)
	})

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO products (sku, name, category, price_cents, margin_rate, active, created_at, updated_at)
		VALUES ($1, 'Produk Void IT', 'snack', 12000, 0.2, true, now(), now())
	`, sku); err != nil {
		t.Fatalf("insert product: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO inventory_stocks (store_id, sku, qty, updated_at)
		VALUES ($1, $2, 10, now())
		ON CONFLICT (store_id, sku)
		DO UPDATE SET qty = 10, updated_at = now()
	`, storeID, sku); err != nil {
		t.Fatalf("seed stock: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO transactions (
			id, store_id, terminal_id, idempotency_key, payment_method,
			total_cents, cash_received_cents, change_cents,
			recommendation_shown, recommendation_accepted, recommendation_sku,
			created_at, subtotal_cents, discount_cents, tax_rate_percent, tax_cents, status
		)
		VALUES (
			$1, $2, 'T-VOID-IT', $3, 'cash',
			12000, 15000, 3000,
			false, false, null,
			now(), 12000, 0, 0, 0, 'paid'
		)
	`, txID, storeID, idempotencyKey); err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO transaction_items (transaction_id, sku, qty, unit_price_cents, margin_rate)
		VALUES ($1, $2, 2, 6000, 0.2)
	`, txID, sku); err != nil {
		t.Fatalf("insert transaction item: %v", err)
	}

	at := time.Now().UTC()
	if _, err := s.VoidTransaction(ctx, txID, "integration test void", at); err != nil {
		t.Fatalf("void transaction: %v", err)
	}

	var qty int
	if err := s.db.QueryRowContext(ctx, `
		SELECT qty
		FROM inventory_stocks
		WHERE store_id = $1 AND sku = $2
	`, storeID, sku).Scan(&qty); err != nil {
		t.Fatalf("query stock: %v", err)
	}
	if qty != 12 {
		t.Fatalf("expected stock 12 after void restock, got %d", qty)
	}

	var status string
	if err := s.db.QueryRowContext(ctx, `
		SELECT status
		FROM transactions
		WHERE id = $1
	`, txID).Scan(&status); err != nil {
		t.Fatalf("query transaction status: %v", err)
	}
	if status != "voided" {
		t.Fatalf("expected transaction status voided, got %s", status)
	}
}
