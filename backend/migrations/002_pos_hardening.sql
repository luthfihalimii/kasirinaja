ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS shift_id TEXT,
    ADD COLUMN IF NOT EXISTS payment_reference TEXT,
    ADD COLUMN IF NOT EXISTS subtotal_cents BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS discount_cents BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tax_rate_percent NUMERIC(6,3) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS tax_cents BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'paid',
    ADD COLUMN IF NOT EXISTS void_reason TEXT,
    ADD COLUMN IF NOT EXISTS voided_at TIMESTAMPTZ;

UPDATE transactions
SET subtotal_cents = total_cents
WHERE subtotal_cents = 0;

UPDATE transactions
SET status = 'paid'
WHERE status IS NULL OR status = '';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'transactions_status_check'
    ) THEN
        ALTER TABLE transactions
            ADD CONSTRAINT transactions_status_check
            CHECK (status IN ('paid', 'voided', 'refunded'));
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS shifts (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    terminal_id TEXT NOT NULL,
    cashier_name TEXT NOT NULL,
    opening_float_cents BIGINT NOT NULL DEFAULT 0 CHECK (opening_float_cents >= 0),
    closing_cash_cents BIGINT NOT NULL DEFAULT 0 CHECK (closing_cash_cents >= 0),
    status TEXT NOT NULL CHECK (status IN ('open', 'closed')),
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_shifts_open_per_terminal
    ON shifts (store_id, terminal_id)
    WHERE status = 'open';

CREATE TABLE IF NOT EXISTS drawer_events (
    id TEXT PRIMARY KEY,
    shift_id TEXT NOT NULL REFERENCES shifts(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('open', 'close', 'cash_in', 'cash_out')),
    amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refunds (
    id TEXT PRIMARY KEY,
    original_transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    status TEXT NOT NULL DEFAULT 'refunded' CHECK (status IN ('refunded', 'pending', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS suppliers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    phone TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS purchase_orders (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    supplier_id TEXT NOT NULL REFERENCES suppliers(id),
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'received', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    received_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS purchase_order_items (
    id BIGSERIAL PRIMARY KEY,
    purchase_order_id TEXT NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    sku TEXT NOT NULL REFERENCES products(sku),
    qty INTEGER NOT NULL CHECK (qty > 0),
    cost_cents BIGINT NOT NULL CHECK (cost_cents >= 0)
);

CREATE TABLE IF NOT EXISTS purchase_order_receipts (
    id TEXT PRIMARY KEY,
    purchase_order_id TEXT NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    received_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions (status);
CREATE INDEX IF NOT EXISTS idx_transactions_shift ON transactions (shift_id);
CREATE INDEX IF NOT EXISTS idx_transactions_store_terminal_created_at ON transactions (store_id, terminal_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_refunds_original_transaction ON refunds (original_transaction_id);
CREATE INDEX IF NOT EXISTS idx_drawer_events_shift_created_at ON drawer_events (shift_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_supplier_status ON purchase_orders (supplier_id, status);
CREATE INDEX IF NOT EXISTS idx_purchase_order_items_purchase_order ON purchase_order_items (purchase_order_id);
