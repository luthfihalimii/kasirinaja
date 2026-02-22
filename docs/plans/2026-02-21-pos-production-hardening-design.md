# POS Production Hardening Design

## Scope
Implement all identified cashier gaps in one iteration:
- Auth + role-based access
- Multi-payment support
- Void/refund/cancel workflow
- Offline duplicate prevention
- Pricing rules (discount/tax/manual override)
- Shift and cash drawer controls
- Barcode and printable receipt UX
- Supplier purchase + receiving flow
- Recommendation retraining endpoint
- Automated backend tests

## Architecture
- Keep current modular monolith (`service`, `store`, `httpapi`) and extend contracts incrementally.
- Use repository-backed persistence where natural (`transactions`, recommendation pairs, stock movement) and service-managed state for session-like flows (`active shifts`) to avoid destabilizing existing checkout path.
- Add token-based auth middleware in HTTP layer with role guards per route.

## API Extensions
- `POST /api/v1/auth/login`
- `POST /api/v1/shifts/open`, `POST /api/v1/shifts/close`, `GET /api/v1/shifts/active`
- `GET /api/v1/checkout/idempotency/{key}`
- `POST /api/v1/transactions/{id}/void`
- `POST /api/v1/refunds`
- `POST /api/v1/suppliers`
- `GET /api/v1/suppliers`
- `POST /api/v1/purchase-orders`
- `POST /api/v1/purchase-orders/{id}/receive`
- `POST /api/v1/recommendation/retrain`

## Data Model Changes
- Add transaction lifecycle fields (`status`, `voided_at`, `void_reason`)
- Add refund records
- Add pricing artifacts on transaction (`discount_cents`, `tax_cents`, `subtotal_cents`)
- Add supplier and purchase order tables
- Add shift and drawer event tables

## Frontend Changes
- Add cashier login screen with role indicator.
- Add shift open/close controls before checkout.
- Add payment method selector (cash/card/qris/ewallet).
- Add barcode input hot path and print receipt action.
- Add idempotency existence check before pushing failed checkout into offline queue.

## Validation Strategy
- Red/green tests on service layer for auth-protected checkout flow, shift gating, idempotency lookup, void/refund lifecycle, and retrain invocation.
- Existing lint/build/test commands must still pass.
