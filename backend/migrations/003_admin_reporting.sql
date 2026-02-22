CREATE TABLE IF NOT EXISTS product_price_history (
    id TEXT PRIMARY KEY,
    sku TEXT NOT NULL REFERENCES products(sku) ON DELETE CASCADE,
    old_price_cents BIGINT NOT NULL CHECK (old_price_cents >= 0),
    new_price_cents BIGINT NOT NULL CHECK (new_price_cents >= 0),
    changed_by TEXT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    store_id TEXT NOT NULL,
    actor_username TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_product_price_history_sku_changed_at
    ON product_price_history (sku, changed_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_store_created_at
    ON audit_logs (store_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action
    ON audit_logs (action);
