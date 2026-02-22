CREATE TABLE IF NOT EXISTS products (
    sku TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    category TEXT NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents > 0),
    margin_rate NUMERIC(5,4) NOT NULL CHECK (margin_rate >= 0),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inventory_stocks (
    store_id TEXT NOT NULL,
    sku TEXT NOT NULL REFERENCES products(sku),
    qty INTEGER NOT NULL CHECK (qty >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (store_id, sku)
);

CREATE TABLE IF NOT EXISTS association_item_pairs (
    source_sku TEXT NOT NULL REFERENCES products(sku),
    target_sku TEXT NOT NULL REFERENCES products(sku),
    support NUMERIC(8,6) NOT NULL DEFAULT 0,
    confidence NUMERIC(8,6) NOT NULL DEFAULT 0,
    lift NUMERIC(8,6) NOT NULL DEFAULT 0,
    affinity_score NUMERIC(8,6) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_sku, target_sku)
);

CREATE TABLE IF NOT EXISTS transactions (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    terminal_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    payment_method TEXT NOT NULL,
    total_cents BIGINT NOT NULL CHECK (total_cents >= 0),
    cash_received_cents BIGINT NOT NULL DEFAULT 0,
    change_cents BIGINT NOT NULL DEFAULT 0,
    recommendation_shown BOOLEAN NOT NULL DEFAULT false,
    recommendation_accepted BOOLEAN NOT NULL DEFAULT false,
    recommendation_sku TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transaction_items (
    id BIGSERIAL PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    sku TEXT NOT NULL REFERENCES products(sku),
    qty INTEGER NOT NULL CHECK (qty > 0),
    unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents > 0),
    margin_rate NUMERIC(5,4) NOT NULL CHECK (margin_rate >= 0)
);

CREATE TABLE IF NOT EXISTS recommendation_events (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    terminal_id TEXT NOT NULL,
    transaction_id TEXT NULL,
    sku TEXT NULL,
    action TEXT NOT NULL,
    reason_code TEXT NULL,
    confidence NUMERIC(8,6) NOT NULL DEFAULT 0,
    latency_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_transaction_items_sku ON transaction_items (sku);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions (created_at);
CREATE INDEX IF NOT EXISTS idx_transactions_store_created_at ON transactions (store_id, created_at);
CREATE INDEX IF NOT EXISTS idx_recommendation_events_store_created_at ON recommendation_events (store_id, created_at);

INSERT INTO products (sku, name, category, price_cents, margin_rate, active)
VALUES
('SKU-MIE-01', 'Mie Goreng Instan', 'grocery', 3500, 0.2200, true),
('SKU-TELUR-01', 'Telur 10 Butir', 'grocery', 26500, 0.1300, true),
('SKU-SUSU-01', 'Susu UHT 1L', 'dairy', 18900, 0.2800, true),
('SKU-ROTI-01', 'Roti Tawar', 'bakery', 17800, 0.3000, true),
('SKU-KOPI-01', 'Kopi Sachet', 'beverage', 2600, 0.3400, true),
('SKU-GULA-01', 'Gula 1kg', 'grocery', 17400, 0.1200, true),
('SKU-TEH-01', 'Teh Celup', 'beverage', 9800, 0.2600, true),
('SKU-AIR-01', 'Air Mineral 600ml', 'beverage', 3900, 0.1800, true),
('SKU-KERIPIK-01', 'Keripik Singkong', 'snack', 12800, 0.3700, true),
('SKU-COKLAT-01', 'Coklat Batang', 'snack', 8600, 0.3500, true),
('SKU-SABUN-01', 'Sabun Mandi', 'household', 7400, 0.3200, true),
('SKU-SHAMPOO-01', 'Shampoo Sachet', 'household', 3200, 0.3300, true)
ON CONFLICT (sku) DO NOTHING;

INSERT INTO inventory_stocks (store_id, sku, qty)
SELECT 'main-store', sku, 120
FROM products
ON CONFLICT (store_id, sku) DO NOTHING;

INSERT INTO association_item_pairs (source_sku, target_sku, support, confidence, lift, affinity_score)
VALUES
('SKU-MIE-01', 'SKU-TELUR-01', 0.152000, 0.850000, 2.400000, 0.850000),
('SKU-KOPI-01', 'SKU-GULA-01', 0.143000, 0.810000, 2.120000, 0.810000),
('SKU-ROTI-01', 'SKU-SUSU-01', 0.120000, 0.740000, 1.800000, 0.740000),
('SKU-AIR-01', 'SKU-KERIPIK-01', 0.101000, 0.660000, 1.620000, 0.660000),
('SKU-TEH-01', 'SKU-COKLAT-01', 0.099000, 0.610000, 1.500000, 0.610000),
('SKU-SABUN-01', 'SKU-SHAMPOO-01', 0.083000, 0.580000, 1.470000, 0.580000),
('SKU-TELUR-01', 'SKU-MIE-01', 0.088000, 0.550000, 1.300000, 0.550000),
('SKU-SUSU-01', 'SKU-ROTI-01', 0.084000, 0.520000, 1.310000, 0.520000),
('SKU-KERIPIK-01', 'SKU-AIR-01', 0.076000, 0.470000, 1.250000, 0.470000)
ON CONFLICT (source_sku, target_sku) DO NOTHING;
