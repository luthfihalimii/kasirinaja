CREATE TABLE IF NOT EXISTS promo_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('cart_percent', 'flat_cart')),
    min_subtotal_cents BIGINT NOT NULL DEFAULT 0 CHECK (min_subtotal_cents >= 0),
    discount_percent NUMERIC(6,3) NOT NULL DEFAULT 0 CHECK (discount_percent >= 0),
    flat_discount_cents BIGINT NOT NULL DEFAULT 0 CHECK (flat_discount_cents >= 0),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_promo_rules_active ON promo_rules (active);
CREATE INDEX IF NOT EXISTS idx_promo_rules_created_at ON promo_rules (created_at DESC);
