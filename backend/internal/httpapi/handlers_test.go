package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kasirinaja/backend/internal/domain"
	"kasirinaja/backend/internal/recommendation"
	"kasirinaja/backend/internal/service"
	"kasirinaja/backend/internal/store/memory"
)

// newTestAPI builds a full API with an in-memory store, real AuthManager and
// real Service so handler tests exercise the complete request path.
func newTestAPI(t *testing.T) *API {
	t.Helper()

	repo := memory.NewSeeded()
	engine := recommendation.NewEngine(nil, 0)
	svc := service.New(repo, engine, "test-store")
	auth := NewAuthManager("test-secret-key", time.Hour, "123456", repo)

	return New(svc, auth, "*")
}

// mustHashPassword generates a bcrypt hash of the given password or fails the test.
func mustHashPassword(t *testing.T, plain string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(hash)
}

func TestHandleHealth(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Handler()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok:true, got %v", body["ok"])
	}
}

func TestHandleLogin_Success(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Handler()

	payload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "admin123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["access_token"] == "" || body["access_token"] == nil {
		t.Fatalf("expected access_token in response, got %v", body)
	}
}

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Handler()

	payload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestHandleLogin_RateLimit(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Handler()

	// The loginLimiter allows 5 attempts per minute.
	// Fire 6 requests from the same "IP" (httptest uses RemoteAddr "192.0.2.1:1234").
	payload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "badpass",
	})

	var lastCode int
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		lastCode = rec.Code
	}

	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 6 attempts, got %d", lastCode)
	}
}

func TestHandleProducts_RequiresAuth(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleProducts_WithValidToken(t *testing.T) {
	api := newTestAPI(t)
	handler := api.Handler()

	// First, obtain a valid token by logging in.
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "admin123",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", loginRec.Code, loginRec.Body.String())
	}

	var loginResp domain.LoginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	// Now request products with the token.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["products"] == nil {
		t.Fatalf("expected products key in response, got %v", body)
	}
}

// TestMustHashPassword verifies that the test helper produces valid bcrypt hashes
// (used to confirm test infrastructure is sound).
func TestMustHashPassword(t *testing.T) {
	hash := mustHashPassword(t, "secret")
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("secret")); err != nil {
		t.Fatalf("hash verification failed: %v", err)
	}
}
