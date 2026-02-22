# POS Production Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Upgrade current POS MVP into production-ready cashier flow with auth, payment, lifecycle controls, shift management, purchasing, and retraining operations.

**Architecture:** Extend existing Go backend contracts and Next.js terminal UI in-place while preserving current checkout and recommendation behavior. Use additive APIs and schema changes to avoid regressions.

**Tech Stack:** Go 1.25, Next.js 16, PostgreSQL, Redis, Bun, Tailwind.

---

### Task 1: Extend Domain Models and Repository Contracts

**Files:**
- Modify: `backend/internal/domain/models.go`
- Modify: `backend/internal/store/store.go`

1. Add auth, shift, refund, supplier/PO, and retrain request/response models.
2. Extend checkout with `payment_method`, `discount_cents`, `tax_rate_percent`, `manual_override`.
3. Add repository methods for transaction lookup/void/refund/stock receiving/retrain.

### Task 2: Add Failing Service Tests (TDD RED)

**Files:**
- Create: `backend/internal/service/service_test.go`

1. Write tests for:
- checkout blocked when shift not open
- non-cash checkout accepted with active shift
- idempotency lookup returns existing transaction
- void transaction prevents duplicate void
- refund stores reference to original transaction
2. Run `go test ./backend/internal/service -run Test -v` and confirm failures.

### Task 3: Implement Service Layer Features (TDD GREEN)

**Files:**
- Modify: `backend/internal/service/service.go`

1. Add shift lifecycle state and methods.
2. Add checkout pricing/tax calculations.
3. Add idempotency query, void, refund, supplier/PO/receive, retrain methods.
4. Re-run service tests until all pass.

### Task 4: Implement Store Backends

**Files:**
- Modify: `backend/internal/store/memory/memory.go`
- Modify: `backend/internal/store/postgres/postgres.go`

1. Implement new repository methods for both memory and postgres.
2. Ensure transactional stock updates on receive/refund/void where relevant.

### Task 5: Add DB Migration for New Capabilities

**Files:**
- Create: `backend/migrations/002_pos_hardening.sql`

1. Add tables/columns for shifts, drawer events, refunds, suppliers, purchase orders, receive events, transaction status/pricing.
2. Add indexes and constraints.

### Task 6: Add Auth Middleware + New HTTP Routes

**Files:**
- Modify: `backend/internal/httpapi/httpapi.go`

1. Add login endpoint.
2. Add auth middleware and route guards.
3. Add handlers for new APIs and idempotency lookup path parsing.

### Task 7: Frontend Cashier Workflow Upgrade

**Files:**
- Modify: `frontend/lib/types.ts`
- Modify: `frontend/lib/api.ts`
- Modify: `frontend/components/pos/pos-terminal.tsx`

1. Add login + token storage.
2. Add shift open/close UI.
3. Add payment method selector and pricing/tax display.
4. Add barcode quick input and print receipt.
5. Add idempotency lookup before offline enqueue on checkout error.

### Task 8: Verification and Regression

1. Run backend tests: `cd backend && GOCACHE=/tmp/go-build GOPATH=/tmp/go-path go test ./...`
2. Run frontend checks: `cd frontend && bun run lint && bun run build --webpack`
3. Run lightweight API smoke tests if socket permissions allow.
