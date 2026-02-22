package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kasirinaja/backend/internal/domain"
)

func TestMiddlewareSetsSecurityHeaders(t *testing.T) {
	api := newTestAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()

	api.Handler().ServeHTTP(res, req)

	if got := res.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := res.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := res.Header().Get("Referrer-Policy"); got == "" {
		t.Fatalf("expected Referrer-Policy to be set")
	}
}

func TestLoginRateLimitReturns429(t *testing.T) {
	api := newTestAPI(t)
	body, _ := json.Marshal(domain.LoginRequest{Username: "admin", Password: "wrong-pass"})

	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:5000"
		res := httptest.NewRecorder()

		api.Handler().ServeHTTP(res, req)

		if i < 5 && res.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d expected 401 before limit, got %d", i+1, res.Code)
		}
		if i == 5 && res.Code != http.StatusTooManyRequests {
			t.Fatalf("attempt 6 expected 429, got %d", res.Code)
		}
	}
}

func TestJSONBodyTooLargeRejected(t *testing.T) {
	api := newTestAPI(t)
	veryLong := strings.Repeat("a", (1<<20)+1024)
	body := fmt.Sprintf(`{"username":"%s","password":"x"}`, veryLong)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	api.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too large body, got %d", res.Code)
	}
}

func TestManagerPINRateLimitReturns429(t *testing.T) {
	api := newTestAPI(t)
	token := loginAsAdmin(t, api)
	csrf := fetchCSRFToken(t, api)

	body, _ := json.Marshal(map[string]string{
		"reason":      "test",
		"manager_pin": "000000",
	})

	for i := 0; i < 9; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/transactions/tx-nonexistent/void", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-CSRF-Token", csrf)
		req.RemoteAddr = "127.0.0.1:5001"
		res := httptest.NewRecorder()

		api.Handler().ServeHTTP(res, req)

		if i < 8 && res.Code != http.StatusForbidden {
			t.Fatalf("attempt %d expected 403 before pin limit, got %d", i+1, res.Code)
		}
		if i == 8 && res.Code != http.StatusTooManyRequests {
			t.Fatalf("attempt 9 expected 429, got %d", res.Code)
		}
	}
}

func TestParsePositiveLimitCaps(t *testing.T) {
	if got := parsePositiveLimit("9999", 50, 200); got != 200 {
		t.Fatalf("expected capped limit 200, got %d", got)
	}
	if got := parsePositiveLimit("", 50, 200); got != 50 {
		t.Fatalf("expected fallback limit 50, got %d", got)
	}
	if got := parsePositiveLimit("invalid", 50, 200); got != 50 {
		t.Fatalf("expected fallback on invalid input, got %d", got)
	}
}

// fetchCSRFToken calls the CSRF token endpoint and returns the token string.
func fetchCSRFToken(t *testing.T, api *API) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/csrf-token", nil)
	res := httptest.NewRecorder()
	api.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("csrf-token endpoint returned status %d", res.Code)
	}
	var payload map[string]string
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode csrf-token response failed: %v", err)
	}
	tok := payload["csrf_token"]
	if strings.TrimSpace(tok) == "" {
		t.Fatalf("expected non-empty csrf_token in response")
	}
	return tok
}

func loginAsAdmin(t *testing.T, api *API) string {
	t.Helper()

	body, _ := json.Marshal(domain.LoginRequest{Username: "admin", Password: "admin123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	api.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("admin login failed, status %d", res.Code)
	}

	var payload domain.LoginResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode login response failed: %v", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		t.Fatalf("expected access token in login response")
	}
	return payload.AccessToken
}
