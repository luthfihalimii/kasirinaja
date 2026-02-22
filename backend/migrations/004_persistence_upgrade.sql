ALTER TABLE purchase_orders
    ADD COLUMN IF NOT EXISTS received_by TEXT;

CREATE TABLE IF NOT EXISTS held_carts (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    terminal_id TEXT NOT NULL,
    cashier_username TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    cart_items JSONB NOT NULL,
    discount_cents BIGINT NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
    tax_rate_percent NUMERIC(6,3) NOT NULL DEFAULT 0,
    payment_method TEXT NOT NULL,
    payment_reference TEXT,
    payment_splits JSONB NOT NULL DEFAULT '[]'::jsonb,
    cash_received_cents BIGINT NOT NULL DEFAULT 0 CHECK (cash_received_cents >= 0),
    manual_override BOOLEAN NOT NULL DEFAULT false,
    held_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_held_carts_store_terminal_held_at
    ON held_carts (store_id, terminal_id, held_at DESC);

CREATE TABLE IF NOT EXISTS product_costs (
    store_id TEXT NOT NULL,
    sku TEXT NOT NULL REFERENCES products(sku) ON DELETE CASCADE,
    cost_cents BIGINT NOT NULL CHECK (cost_cents > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (store_id, sku)
);

CREATE TABLE IF NOT EXISTS app_users (
    username TEXT PRIMARY KEY,
    password TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'cashier')),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
