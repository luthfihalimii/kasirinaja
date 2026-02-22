package httpapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"kasirinaja/backend/internal/domain"
	"kasirinaja/backend/internal/service"
	"kasirinaja/backend/internal/store"
)

type API struct {
	service       *service.Service
	auth          *AuthManager
	allowedOrigin string
	loginLimiter  *attemptLimiter
	pinLimiter    *attemptLimiter
	csrfSecret    []byte
}

func New(svc *service.Service, auth *AuthManager, allowedOrigin string) *API {
	csrfSecret := make([]byte, 32)
	if _, err := rand.Read(csrfSecret); err != nil {
		// Fall back to a deterministic secret if crypto/rand fails (should not happen in practice).
		csrfSecret = []byte("csrf-fallback-secret-change-me!!")
	}
	return &API{
		service:       svc,
		auth:          auth,
		allowedOrigin: allowedOrigin,
		loginLimiter:  newAttemptLimiter(5, time.Minute),
		pinLimiter:    newAttemptLimiter(8, time.Minute),
		csrfSecret:    csrfSecret,
	}
}

// csrfTokenForHour computes an HMAC-SHA256 token for the given hour bucket
// (expressed as Unix time truncated to the hour). The token is hex-encoded.
func (a *API) csrfTokenForHour(hourBucket int64) string {
	h := hmac.New(sha256.New, a.csrfSecret)
	fmt.Fprintf(h, "%d", hourBucket)
	return hex.EncodeToString(h.Sum(nil))
}

// generateCSRFToken returns a token valid for the current hour bucket.
func (a *API) generateCSRFToken() string {
	now := time.Now().UTC()
	bucket := now.Truncate(time.Hour).Unix()
	return a.csrfTokenForHour(bucket)
}

// validateCSRFToken checks whether the provided token matches the current or
// previous hour bucket, giving a 2-hour validity window.
func (a *API) validateCSRFToken(token string) bool {
	if token == "" {
		return false
	}
	now := time.Now().UTC()
	currentBucket := now.Truncate(time.Hour).Unix()
	prevBucket := currentBucket - 3600

	expected1 := a.csrfTokenForHour(currentBucket)
	expected2 := a.csrfTokenForHour(prevBucket)

	return hmac.Equal([]byte(token), []byte(expected1)) ||
		hmac.Equal([]byte(token), []byte(expected2))
}

type attemptLimiter struct {
	mu      sync.Mutex
	max     int
	window  time.Duration
	entries map[string][]time.Time
}

func newAttemptLimiter(max int, window time.Duration) *attemptLimiter {
	if max < 1 {
		max = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &attemptLimiter{max: max, window: window, entries: make(map[string][]time.Time)}
}

func (l *attemptLimiter) Allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	history := l.entries[key]
	kept := make([]time.Time, 0, len(history)+1)
	for _, ts := range history {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.max {
		l.entries[key] = kept
		return false
	}
	kept = append(kept, now)
	l.entries[key] = kept
	return true
}

func clientKey(r *http.Request) string {
	host := strings.TrimSpace(r.RemoteAddr)
	if host == "" {
		return "unknown"
	}
	if addr, err := netip.ParseAddrPort(host); err == nil {
		return addr.Addr().String()
	}
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		return host[:idx]
	}
	return host
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", a.handleHealth)
	mux.HandleFunc("/api/v1/auth/login", a.handleLogin)
	mux.HandleFunc("/api/v1/auth/csrf-token", a.handleCSRFToken)

	mux.HandleFunc("/api/v1/products", a.requireAuth(a.handleProducts, "cashier", "admin"))
	mux.HandleFunc("/api/v1/products/", a.requireAuth(a.handleProductActions, "admin"))
	mux.HandleFunc("/api/v1/cart/recommendation", a.requireAuth(a.handleRecommendation, "cashier", "admin"))
	mux.HandleFunc("/api/v1/checkout", a.requireAuth(a.handleCheckout, "cashier", "admin"))
	mux.HandleFunc("/api/v1/checkout/idempotency/", a.requireAuth(a.handleCheckoutLookup, "cashier", "admin"))
	mux.HandleFunc("/api/v1/carts/hold", a.requireAuth(a.handleHeldCarts, "cashier", "admin"))
	mux.HandleFunc("/api/v1/carts/hold/", a.requireAuth(a.handleHeldCartActions, "cashier", "admin"))
	mux.HandleFunc("/api/v1/sync/offline-transactions", a.requireAuth(a.handleOfflineSync, "cashier", "admin"))
	mux.HandleFunc("/api/v1/metrics/attach-rate", a.requireAuth(a.handleAttachMetrics, "cashier", "admin"))

	mux.HandleFunc("/api/v1/shifts/open", a.requireAuth(a.handleShiftOpen, "cashier", "admin"))
	mux.HandleFunc("/api/v1/shifts/close", a.requireAuth(a.handleShiftClose, "cashier", "admin"))
	mux.HandleFunc("/api/v1/shifts/active", a.requireAuth(a.handleShiftActive, "cashier", "admin"))

	mux.HandleFunc("/api/v1/transactions/", a.requireAuth(a.handleTransactionActions, "admin"))
	mux.HandleFunc("/api/v1/refunds", a.requireAuth(a.handleRefunds, "admin"))
	mux.HandleFunc("/api/v1/returns/items", a.requireAuth(a.handleItemReturns, "admin"))
	mux.HandleFunc("/api/v1/stock-opname", a.requireAuth(a.handleStockOpname, "admin"))
	mux.HandleFunc("/api/v1/inventory/lots", a.requireAuth(a.handleInventoryLots, "admin"))
	mux.HandleFunc("/api/v1/audit-logs", a.requireAuth(a.handleAuditLogs, "admin"))
	mux.HandleFunc("/api/v1/reports/daily", a.requireAuth(a.handleDailyReport, "admin"))
	mux.HandleFunc("/api/v1/reorder-suggestions", a.requireAuth(a.handleReorderSuggestions, "admin"))
	mux.HandleFunc("/api/v1/alerts/anomalies", a.requireAuth(a.handleAnomalyAlerts, "admin"))
	mux.HandleFunc("/api/v1/promos", a.requireAuth(a.handlePromos, "admin"))
	mux.HandleFunc("/api/v1/promos/", a.requireAuth(a.handlePromoActions, "admin"))
	mux.HandleFunc("/api/v1/suppliers", a.requireAuth(a.handleSuppliers, "admin"))
	mux.HandleFunc("/api/v1/purchase-orders", a.requireAuth(a.handlePurchaseOrders, "admin"))
	mux.HandleFunc("/api/v1/purchase-orders/", a.requireAuth(a.handlePurchaseOrderActions, "admin"))
	mux.HandleFunc("/api/v1/users/cashiers", a.requireAuth(a.handleCashiers, "admin"))
	mux.HandleFunc("/api/v1/hardware/receipt/escpos", a.requireAuth(a.handleHardwareReceiptEscpos, "cashier", "admin"))
	mux.HandleFunc("/api/v1/hardware/cash-drawer/open", a.requireAuth(a.handleCashDrawerOpen, "cashier", "admin"))
	mux.HandleFunc("/api/v1/recommendation/retrain", a.requireAuth(a.handleRetrain, "admin"))

	return a.withMiddleware(mux)
}

func (a *API) requireAuth(next http.HandlerFunc, roles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			writeError(w, http.StatusUnauthorized, errors.New("missing bearer token"))
			return
		}

		token := strings.TrimSpace(authorization[len("Bearer "):])
		actor, err := a.auth.ParseToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}

		if len(roles) > 0 && !isRoleAllowed(actor.Role, roles) {
			writeError(w, http.StatusForbidden, errors.New("forbidden role"))
			return
		}

		next(w, r.WithContext(service.WithActor(r.Context(), actor)))
	}
}

func isRoleAllowed(role string, allowed []string) bool {
	for _, allow := range allowed {
		if role == allow {
			return true
		}
	}
	return false
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if !a.loginLimiter.Allow(clientKey(r)) {
		writeError(w, http.StatusTooManyRequests, errors.New("too many login attempts"))
		return
	}

	var req domain.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.auth.Login(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCSRFToken returns a stateless CSRF token valid for the current hour bucket.
// Clients must include this token in the X-CSRF-Token header for all mutating requests.
func (a *API) handleCSRFToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"csrf_token": a.generateCSRFToken(),
	})
}

// csrfExemptPaths lists paths that are exempt from CSRF validation.
// Login and offline-sync are excluded because they are called without a prior CSRF token fetch.
var csrfExemptPaths = []string{
	"/api/v1/auth/login",
	"/api/v1/sync/offline-transactions",
}

// checkCSRF enforces CSRF token validation for state-changing methods (POST/PUT/PATCH).
// Returns false and writes an error response if validation fails.
func (a *API) checkCSRF(w http.ResponseWriter, r *http.Request) bool {
	method := r.Method
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return true
	}
	for _, exempt := range csrfExemptPaths {
		if r.URL.Path == exempt {
			return true
		}
	}
	token := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if !a.validateCSRFToken(token) {
		writeError(w, http.StatusForbidden, errors.New("missing or invalid CSRF token"))
		return false
	}
	return true
}

func (a *API) handleProducts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		products, err := a.service.ListProducts(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"products": products})
	case http.MethodPost:
		actor, ok := service.ActorFromContext(r.Context())
		if !ok || actor.Role != "admin" {
			writeError(w, http.StatusForbidden, errors.New("forbidden role"))
			return
		}

		var req domain.ProductCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		product, err := a.service.CreateProduct(r.Context(), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
				status = http.StatusForbidden
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"product": product})
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) handleProductActions(w http.ResponseWriter, r *http.Request) {
	prefix := "/api/v1/products/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeError(w, http.StatusBadRequest, errors.New("invalid product action path"))
		return
	}

	tail := strings.TrimSpace(strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/"))
	if tail == "" {
		writeError(w, http.StatusBadRequest, errors.New("product sku required"))
		return
	}

	if strings.HasSuffix(tail, "/price-history") {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		sku := strings.TrimSpace(strings.TrimSuffix(tail, "/price-history"))
		sku = strings.Trim(sku, "/")
		if sku == "" {
			writeError(w, http.StatusBadRequest, errors.New("product sku required"))
			return
		}

		limit := parsePositiveLimit(r.URL.Query().Get("limit"), 50, 200)

		history, err := a.service.ListProductPriceHistory(r.Context(), sku, limit)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"history": history})
		return
	}

	if r.Method != http.MethodPatch {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.ProductUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	updated, err := a.service.UpdateProduct(r.Context(), tail, req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
			status = http.StatusForbidden
		}
		writeError(w, status, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"product": updated})
}

func (a *API) handleRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.RecommendationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if strings.TrimSpace(req.TerminalID) == "" {
		req.TerminalID = "terminal-1"
	}

	resp, err := a.service.Recommend(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.CheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.Checkout(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInsufficientStock):
			writeError(w, http.StatusConflict, err)
		case errors.Is(err, store.ErrInvalidTransaction):
			writeError(w, http.StatusBadRequest, err)
		case strings.Contains(strings.ToLower(err.Error()), "manual override"):
			writeError(w, http.StatusForbidden, err)
		default:
			writeError(w, http.StatusUnprocessableEntity, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleCheckoutLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	prefix := "/api/v1/checkout/idempotency/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeError(w, http.StatusBadRequest, errors.New("invalid path"))
		return
	}
	idempotencyKey := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))
	if idempotencyKey == "" {
		writeError(w, http.StatusBadRequest, errors.New("idempotency key required"))
		return
	}

	resp, err := a.service.LookupCheckoutByIdempotency(r.Context(), idempotencyKey)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleHeldCarts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		storeID := r.URL.Query().Get("store_id")
		terminalID := r.URL.Query().Get("terminal_id")
		resp, err := a.service.ListHeldCarts(r.Context(), storeID, terminalID)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req domain.HoldCartRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		resp, err := a.service.HoldCart(r.Context(), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) handleHeldCartActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	prefix := "/api/v1/carts/hold/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		writeError(w, http.StatusBadRequest, errors.New("invalid held cart action path"))
		return
	}

	tail := strings.TrimSpace(strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/"))
	if tail == "" {
		writeError(w, http.StatusBadRequest, errors.New("held cart action path required"))
		return
	}

	if strings.HasSuffix(tail, "/resume") {
		holdID := strings.Trim(strings.TrimSuffix(tail, "/resume"), "/")
		resp, err := a.service.ResumeHeldCart(r.Context(), holdID)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	if strings.HasSuffix(tail, "/discard") {
		holdID := strings.Trim(strings.TrimSuffix(tail, "/discard"), "/")
		err := a.service.DiscardHeldCart(r.Context(), holdID)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	writeError(w, http.StatusBadRequest, errors.New("unknown held cart action"))
}

func (a *API) handleOfflineSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.OfflineSyncRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.SyncOffline(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleAttachMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	storeID := r.URL.Query().Get("store_id")
	days := 30
	if dayParam := r.URL.Query().Get("days"); dayParam != "" {
		parsed, err := strconv.Atoi(dayParam)
		if err == nil {
			days = parsed
		}
	}

	metrics, err := a.service.AttachMetrics(r.Context(), storeID, days)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}

	writeJSON(w, http.StatusOK, metrics)
}

func (a *API) handleShiftOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.ShiftOpenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.OpenShift(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleShiftClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.ShiftCloseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.CloseShift(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleShiftActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	storeID := r.URL.Query().Get("store_id")
	terminalID := r.URL.Query().Get("terminal_id")
	resp, err := a.service.GetActiveShift(r.Context(), storeID, terminalID)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleTransactionActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	prefix := "/api/v1/transactions/"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/void") {
		writeError(w, http.StatusBadRequest, errors.New("invalid transaction action path"))
		return
	}
	transactionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/void")
	transactionID = strings.TrimSpace(strings.Trim(transactionID, "/"))
	if transactionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("transaction id required"))
		return
	}

	var req domain.VoidTransactionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !a.pinLimiter.Allow("pin:void:" + clientKey(r)) {
		writeError(w, http.StatusTooManyRequests, errors.New("too many manager pin attempts"))
		return
	}
	if !a.auth.ValidateManagerPIN(req.ManagerPIN) {
		writeError(w, http.StatusForbidden, errors.New("invalid manager pin"))
		return
	}
	req.TransactionID = transactionID

	resp, err := a.service.VoidTransaction(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleRefunds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.RefundRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !a.pinLimiter.Allow("pin:refund:" + clientKey(r)) {
		writeError(w, http.StatusTooManyRequests, errors.New("too many manager pin attempts"))
		return
	}
	if !a.auth.ValidateManagerPIN(req.ManagerPIN) {
		writeError(w, http.StatusForbidden, errors.New("invalid manager pin"))
		return
	}

	resp, err := a.service.Refund(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleItemReturns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.ItemReturnRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !a.pinLimiter.Allow("pin:return:" + clientKey(r)) {
		writeError(w, http.StatusTooManyRequests, errors.New("too many manager pin attempts"))
		return
	}
	if !a.auth.ValidateManagerPIN(req.ManagerPIN) {
		writeError(w, http.StatusForbidden, errors.New("invalid manager pin"))
		return
	}

	resp, err := a.service.ProcessItemReturn(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleInventoryLots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		storeID := strings.TrimSpace(r.URL.Query().Get("store_id"))
		sku := strings.TrimSpace(r.URL.Query().Get("sku"))
		includeExpired := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_expired")), "true")
		limit := parsePositiveLimit(r.URL.Query().Get("limit"), 200, 500)

		resp, err := a.service.ListInventoryLots(r.Context(), storeID, sku, includeExpired, limit)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req domain.InventoryLotReceiveRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		lot, err := a.service.ReceiveInventoryLot(r.Context(), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
				status = http.StatusForbidden
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"lot": lot})
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) handleStockOpname(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.StockOpnameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.StockOpname(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
			status = http.StatusForbidden
		}
		writeError(w, status, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	storeID := r.URL.Query().Get("store_id")
	date := r.URL.Query().Get("date")
	limit := parsePositiveLimit(r.URL.Query().Get("limit"), 100, 500)

	logs, err := a.service.ListAuditLogs(r.Context(), storeID, date, limit)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (a *API) handleDailyReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	storeID := r.URL.Query().Get("store_id")
	date := r.URL.Query().Get("date")
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))

	report, err := a.service.DailyReport(r.Context(), storeID, date)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}

	switch format {
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"daily-report-%s.csv\"", report.Date))
		_, _ = w.Write([]byte(dailyReportToCSV(report)))
	case "pdf":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dailyReportToPrintableHTML(report)))
	default:
		writeJSON(w, http.StatusOK, report)
	}
}

func (a *API) handleReorderSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	storeID := r.URL.Query().Get("store_id")
	resp, err := a.service.ReorderSuggestions(r.Context(), storeID)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleAnomalyAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	storeID := r.URL.Query().Get("store_id")
	date := r.URL.Query().Get("date")
	resp, err := a.service.DetectOperationalAnomalies(r.Context(), storeID, date)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handlePromos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		promos, err := a.service.ListPromos(r.Context())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"promos": promos})
	case http.MethodPost:
		var req domain.PromoCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		promo, err := a.service.CreatePromo(r.Context(), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
				status = http.StatusForbidden
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"promo": promo})
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) handlePromoActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	prefix := "/api/v1/promos/"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/toggle") {
		writeError(w, http.StatusBadRequest, errors.New("invalid promo action path"))
		return
	}
	promoID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/toggle")
	promoID = strings.TrimSpace(strings.Trim(promoID, "/"))
	if promoID == "" {
		writeError(w, http.StatusBadRequest, errors.New("promo id required"))
		return
	}

	var req domain.PromoToggleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	promo, err := a.service.SetPromoActive(r.Context(), promoID, req.Active)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
			status = http.StatusForbidden
		}
		writeError(w, status, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"promo": promo})
}

func (a *API) handleSuppliers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req domain.SupplierCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		supplier, err := a.service.CreateSupplier(r.Context(), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			if strings.Contains(strings.ToLower(err.Error()), "admin role required") {
				status = http.StatusForbidden
			}
			writeError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"supplier": supplier})
	case http.MethodGet:
		suppliers, err := a.service.ListSuppliers(r.Context())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"suppliers": suppliers})
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) handlePurchaseOrders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := r.URL.Query().Get("status")
		resp, err := a.service.ListPurchaseOrders(r.Context(), status)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		var req domain.PurchaseOrderCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		resp, err := a.service.CreatePurchaseOrder(r.Context(), req)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, store.ErrNotFound) {
				status = http.StatusNotFound
			}
			if errors.Is(err, store.ErrInvalidTransaction) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err)
			return
		}

		writeJSON(w, http.StatusOK, resp)
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) handlePurchaseOrderActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	prefix := "/api/v1/purchase-orders/"
	if !strings.HasPrefix(r.URL.Path, prefix) || !strings.HasSuffix(r.URL.Path, "/receive") {
		writeError(w, http.StatusBadRequest, errors.New("invalid purchase order action path"))
		return
	}
	purchaseOrderID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), "/receive")
	purchaseOrderID = strings.TrimSpace(strings.Trim(purchaseOrderID, "/"))
	if purchaseOrderID == "" {
		writeError(w, http.StatusBadRequest, errors.New("purchase order id required"))
		return
	}

	var req domain.PurchaseOrderReceiveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.ReceivePurchaseOrder(r.Context(), purchaseOrderID, req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleHardwareReceiptEscpos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.HardwareReceiptRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.BuildHardwareReceipt(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleCashDrawerOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.CashDrawerOpenRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.OpenCashDrawer(r.Context(), req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, store.ErrInvalidTransaction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleRetrain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req domain.RetrainRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := a.service.RetrainAssociations(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) handleCashiers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cashiers := a.auth.ListCashiers()
		writeJSON(w, http.StatusOK, map[string]any{"cashiers": cashiers})
	case http.MethodPost:
		var req domain.CashierCreateRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		cashier, err := a.auth.CreateCashier(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"cashier": cashier})
	default:
		writeMethodNotAllowed(w)
	}
}

func (a *API) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Access-Control-Allow-Origin", a.allowedOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,OPTIONS")
		w.Header().Set("Vary", "Origin")

		if (r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut) && strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Enforce CSRF protection for all state-changing requests.
		if !a.checkCSRF(w, r) {
			return
		}

		startedAt := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(startedAt))
	})
}

func dailyReportToCSV(report domain.DailyReport) string {
	lines := []string{
		"section,key,value",
		fmt.Sprintf("summary,date,%s", report.Date),
		fmt.Sprintf("summary,store_id,%s", report.StoreID),
		fmt.Sprintf("summary,transactions,%d", report.Transactions),
		fmt.Sprintf("summary,gross_sales_cents,%d", report.GrossSalesCents),
		fmt.Sprintf("summary,discount_cents,%d", report.DiscountCents),
		fmt.Sprintf("summary,tax_cents,%d", report.TaxCents),
		fmt.Sprintf("summary,net_sales_cents,%d", report.NetSalesCents),
		fmt.Sprintf("summary,estimated_margin_cents,%d", report.EstimatedMarginCents),
	}
	for _, payment := range report.ByPayment {
		lines = append(lines, fmt.Sprintf("payment,%s_transactions,%d", payment.PaymentMethod, payment.Transactions))
		lines = append(lines, fmt.Sprintf("payment,%s_total_cents,%d", payment.PaymentMethod, payment.TotalCents))
	}
	for _, terminal := range report.ByTerminal {
		lines = append(lines, fmt.Sprintf("terminal,%s_transactions,%d", terminal.TerminalID, terminal.Transactions))
		lines = append(lines, fmt.Sprintf("terminal,%s_total_cents,%d", terminal.TerminalID, terminal.TotalCents))
	}
	return strings.Join(lines, "\n") + "\n"
}

// dailyReportHTMLTmpl is the html/template used to render printable daily reports.
// All user-controlled fields are auto-escaped by html/template to prevent XSS.
var dailyReportHTMLTmpl = template.Must(template.New("daily-report").Parse(`<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Daily Report {{.Date}}</title>
  <style>
    body { font-family: sans-serif; margin: 24px; }
    table { width: 100%; border-collapse: collapse; margin-top: 8px; }
    th, td { border: 1px solid #ddd; padding: 6px; font-size: 13px; }
    h2, h3 { margin-bottom: 4px; }
  </style>
</head>
<body>
  <h2>Daily Report {{.Date}}</h2>
  <p>Store: {{.StoreID}}</p>
  <p>Transactions: {{.Transactions}}</p>
  <p>Gross: {{.GrossSalesCents}} | Discount: {{.DiscountCents}} | Tax: {{.TaxCents}} | Net: {{.NetSalesCents}} | Margin: {{.EstimatedMarginCents}}</p>

  <h3>By Payment</h3>
  <table>
    <thead><tr><th>Payment</th><th>Transactions</th><th>Total Cents</th></tr></thead>
    <tbody>{{range .ByPayment}}<tr><td>{{.PaymentMethod}}</td><td style="text-align:right;">{{.Transactions}}</td><td style="text-align:right;">{{.TotalCents}}</td></tr>{{end}}</tbody>
  </table>

  <h3>By Terminal</h3>
  <table>
    <thead><tr><th>Terminal</th><th>Transactions</th><th>Total Cents</th></tr></thead>
    <tbody>{{range .ByTerminal}}<tr><td>{{.TerminalID}}</td><td style="text-align:right;">{{.Transactions}}</td><td style="text-align:right;">{{.TotalCents}}</td></tr>{{end}}</tbody>
  </table>
</body>
</html>
`))

func dailyReportToPrintableHTML(report domain.DailyReport) string {
	var buf bytes.Buffer
	if err := dailyReportHTMLTmpl.Execute(&buf, report); err != nil {
		// Fallback: return a plain-text error page rather than leaking internal details.
		return "<!doctype html><html><body><p>Report rendering error.</p></body></html>"
	}
	return buf.String()
}

func decodeJSON(r *http.Request, dest any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	return nil
}

func parsePositiveLimit(raw string, fallback int, max int) int {
	limit := fallback
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		if parsed, err := strconv.Atoi(trimmed); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if max > 0 && limit > max {
		return max
	}
	return limit
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func writeError(w http.ResponseWriter, status int, err error) {
	// For 5xx responses, return a generic message to avoid leaking internal
	// implementation details (stack traces, SQL errors, file paths, etc.).
	// 4xx responses are user-facing so we return the original error message.
	msg := err.Error()
	if status >= 500 {
		log.Printf("internal error (status %d): %v", status, err)
		msg = "internal server error"
	}
	writeJSON(w, status, map[string]any{
		"error": msg,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
