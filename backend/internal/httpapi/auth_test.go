package httpapi

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"kasirinaja/backend/internal/domain"
)

type userStoreStub struct {
	mu      sync.Mutex
	users   map[string]domain.UserAccount
	updates int
}

func (s *userStoreStub) CreateUser(_ context.Context, user domain.UserAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.users == nil {
		s.users = make(map[string]domain.UserAccount)
	}
	s.users[user.Username] = user
	return nil
}

func (s *userStoreStub) ListUsers(_ context.Context) ([]domain.UserAccount, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.UserAccount, 0, len(s.users))
	for _, user := range s.users {
		out = append(out, user)
	}
	return out, nil
}

func (s *userStoreStub) UpdateUserPassword(_ context.Context, username string, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user := s.users[username]
	user.Password = password
	s.users[username] = user
	s.updates++
	return nil
}

func TestAuthManagerUpgradesLegacyPlainPassword(t *testing.T) {
	store := &userStoreStub{
		users: map[string]domain.UserAccount{
			"admin": {
				Username:  "admin",
				Password:  "admin123",
				Role:      "admin",
				Active:    true,
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	manager := NewAuthManager("test-secret", time.Hour, "123456", store)
	_, err := manager.Login(domain.LoginRequest{
		Username: "admin",
		Password: "admin123",
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	users, err := store.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list users failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Password == "admin123" {
		t.Fatalf("expected password to be upgraded from plain-text")
	}
	if !strings.HasPrefix(users[0].Password, "$2") {
		t.Fatalf("expected bcrypt password hash, got %s", users[0].Password)
	}
}

func TestCreateCashierStoresPasswordHash(t *testing.T) {
	store := &userStoreStub{
		users: map[string]domain.UserAccount{
			"admin": {
				Username:  "admin",
				Password:  "admin123",
				Role:      "admin",
				Active:    true,
				CreatedAt: time.Now().UTC(),
			},
		},
	}

	manager := NewAuthManager("test-secret", time.Hour, "123456", store)
	cashier, err := manager.CreateCashier(domain.CashierCreateRequest{
		Username: "kasirbaru",
		Password: "pass1234",
	})
	if err != nil {
		t.Fatalf("create cashier failed: %v", err)
	}
	if cashier.Username != "kasirbaru" {
		t.Fatalf("unexpected username %s", cashier.Username)
	}

	users, err := store.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list users failed: %v", err)
	}
	var found *domain.UserAccount
	for i := range users {
		if users[i].Username == "kasirbaru" {
			found = &users[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected cashier to be saved")
	}
	if found.Password == "pass1234" {
		t.Fatalf("expected cashier password to be hashed")
	}
	if !strings.HasPrefix(found.Password, "$2") {
		t.Fatalf("expected bcrypt hash prefix, got %s", found.Password)
	}

	_, err = manager.Login(domain.LoginRequest{
		Username: "kasirbaru",
		Password: "pass1234",
	})
	if err != nil {
		t.Fatalf("login with hashed cashier failed: %v", err)
	}
}

func TestManagerPINIsHashedAndStillValidates(t *testing.T) {
	store := &userStoreStub{users: map[string]domain.UserAccount{}}
	manager := NewAuthManager("test-secret", time.Hour, "654321", store)

	if manager.managerPIN == "654321" {
		t.Fatalf("expected manager pin to be stored as hash, got plain-text")
	}

	if !manager.ValidateManagerPIN("654321") {
		t.Fatalf("expected manager pin validation to succeed")
	}

	if manager.ValidateManagerPIN("111111") {
		t.Fatalf("expected wrong manager pin to fail")
	}
}
