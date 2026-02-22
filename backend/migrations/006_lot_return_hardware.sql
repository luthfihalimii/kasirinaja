CREATE TABLE IF NOT EXISTS inventory_lots (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    sku TEXT NOT NULL REFERENCES products(sku) ON DELETE CASCADE,
    lot_code TEXT NOT NULL,
    expiry_date DATE NULL,
    qty_received INTEGER NOT NULL CHECK (qty_received > 0),
    qty_available INTEGER NOT NULL CHECK (qty_available >= 0),
    cost_cents BIGINT NOT NULL CHECK (cost_cents > 0),
    source_type TEXT NOT NULL DEFAULT 'manual' CHECK (source_type IN ('manual', 'purchase_order', 'void', 'return')),
    source_id TEXT NULL,
    notes TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_inventory_lots_store_sku_expiry
    ON inventory_lots (store_id, sku, expiry_date, received_at);

CREATE INDEX IF NOT EXISTS idx_inventory_lots_expiry
    ON inventory_lots (expiry_date);

CREATE TABLE IF NOT EXISTS item_returns (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    original_transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('refund', 'exchange')),
    reason TEXT NOT NULL,
    refund_amount_cents BIGINT NOT NULL DEFAULT 0 CHECK (refund_amount_cents >= 0),
    exchange_transaction_id TEXT NULL REFERENCES transactions(id) ON DELETE SET NULL,
    additional_payment_cents BIGINT NOT NULL DEFAULT 0 CHECK (additional_payment_cents >= 0),
    processed_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS item_return_items (
    id BIGSERIAL PRIMARY KEY,
    item_return_id TEXT NOT NULL REFERENCES item_returns(id) ON DELETE CASCADE,
    sku TEXT NOT NULL REFERENCES products(sku) ON DELETE RESTRICT,
    qty INTEGER NOT NULL CHECK (qty > 0),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents > 0),
    kind TEXT NOT NULL CHECK (kind IN ('return', 'exchange'))
);

CREATE INDEX IF NOT EXISTS idx_item_returns_original_tx
    ON item_returns (original_transaction_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_item_return_items_item_return_id
    ON item_return_items (item_return_id);
