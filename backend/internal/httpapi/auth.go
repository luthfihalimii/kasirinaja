package httpapi

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"kasirinaja/backend/internal/domain"
)

type AuthManager struct {
	mu         sync.RWMutex
	secret     []byte
	tokenTTL   time.Duration
	managerPIN string
	userStore  UserStore
	users      map[string]credential
}

type UserStore interface {
	CreateUser(ctx context.Context, user domain.UserAccount) error
	ListUsers(ctx context.Context) ([]domain.UserAccount, error)
	UpdateUserPassword(ctx context.Context, username string, password string) error
}

type credential struct {
	password string
	role     string
	active   bool
	created  time.Time
}

type posCustomClaims struct {
	jwtlib.RegisteredClaims
	Role string `json:"role"`
}

func NewAuthManager(secret string, tokenTTL time.Duration, managerPIN string, userStore UserStore) *AuthManager {
	if secret == "" {
		secret = "dev-change-me"
	}
	if tokenTTL <= 0 {
		tokenTTL = 8 * time.Hour
	}
	managerPIN = strings.TrimSpace(managerPIN)
	if managerPIN == "" {
		managerPIN = "disabled"
	}
	hashedPIN, err := hashPassword(managerPIN)
	if err == nil {
		managerPIN = hashedPIN
	}

	manager := &AuthManager{
		secret:     []byte(secret),
		tokenTTL:   tokenTTL,
		managerPIN: managerPIN,
		userStore:  userStore,
		users:      make(map[string]credential),
	}
	// context.Background() is appropriate here because this is a startup operation
	// that runs before any request context exists.
	manager.bootstrapUsers(context.Background())
	return manager
}

func (a *AuthManager) Login(req domain.LoginRequest) (domain.LoginResponse, error) {
	// TODO: bootstrapUsers is called on every login to pick up users added outside this
	// process. This is acceptable for low-traffic POS deployments but should use a
	// bounded context (e.g. with a timeout) rather than context.Background() to avoid
	// hanging indefinitely if the user store is slow.
	a.bootstrapUsers(context.Background())
	username := strings.TrimSpace(req.Username)
	a.mu.RLock()
	cred, ok := a.users[username]
	a.mu.RUnlock()
	if !ok {
		return domain.LoginResponse{}, errors.New("invalid credentials")
	}

	valid := verifyPassword(cred.password, req.Password)
	if !valid {
		return domain.LoginResponse{}, errors.New("invalid credentials")
	}
	if !cred.active {
		return domain.LoginResponse{}, errors.New("account is inactive")
	}

	expiresAt := time.Now().UTC().Add(a.tokenTTL)
	token, err := a.sign(username, cred.role, expiresAt)
	if err != nil {
		return domain.LoginResponse{}, err
	}

	return domain.LoginResponse{
		AccessToken: token,
		Role:        cred.role,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	}, nil
}

func (a *AuthManager) ParseToken(tokenStr string) (domain.Actor, error) {
	claims := &posCustomClaims{}
	token, err := jwtlib.ParseWithClaims(tokenStr, claims, func(t *jwtlib.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.secret, nil
	}, jwtlib.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return domain.Actor{}, errors.New("invalid or expired token")
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return domain.Actor{}, errors.New("invalid token subject")
	}
	return domain.Actor{Username: sub, Role: claims.Role}, nil
}

func (a *AuthManager) sign(username, role string, expiresAt time.Time) (string, error) {
	claims := posCustomClaims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwtlib.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
			Issuer:    "kasirinaja",
		},
		Role: role,
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims)
	return token.SignedString(a.secret)
}

func (a *AuthManager) ValidateManagerPIN(pin string) bool {
	input := strings.TrimSpace(pin)
	if input == "" || !isPasswordHash(a.managerPIN) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(a.managerPIN), []byte(input)) == nil
}

func (a *AuthManager) CreateCashier(req domain.CashierCreateRequest) (domain.CashierUser, error) {
	// context.Background() is correct here: CreateCashier is an admin operation that
	// does not carry a request context through the AuthManager API.
	a.bootstrapUsers(context.Background())
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" || len(username) < 4 {
		return domain.CashierUser{}, fmt.Errorf("username must be at least 4 characters")
	}
	if strings.ContainsAny(username, " \t\r\n") {
		return domain.CashierUser{}, fmt.Errorf("username must not contain spaces")
	}
	if strings.TrimSpace(req.Password) == "" || len(req.Password) < 6 {
		return domain.CashierUser{}, fmt.Errorf("password must be at least 6 characters")
	}

	a.mu.RLock()
	_, exists := a.users[username]
	a.mu.RUnlock()
	if exists {
		return domain.CashierUser{}, fmt.Errorf("username already exists")
	}

	now := time.Now().UTC()
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return domain.CashierUser{}, fmt.Errorf("failed to hash password")
	}

	if a.userStore != nil {
		err := a.userStore.CreateUser(context.Background(), domain.UserAccount{
			Username:  username,
			Password:  passwordHash,
			Role:      "cashier",
			Active:    true,
			CreatedAt: now,
		})
		if err != nil {
			return domain.CashierUser{}, err
		}
	}

	a.mu.Lock()
	a.users[username] = credential{
		password: passwordHash,
		role:     "cashier",
		active:   true,
		created:  now,
	}
	a.mu.Unlock()

	return domain.CashierUser{
		Username:  username,
		Role:      "cashier",
		Active:    true,
		CreatedAt: now,
	}, nil
}

func (a *AuthManager) ListCashiers() []domain.CashierUser {
	// context.Background() is correct here: ListCashiers is an admin operation that
	// does not carry a request context through the AuthManager API.
	a.bootstrapUsers(context.Background())
	a.mu.RLock()
	result := make([]domain.CashierUser, 0, len(a.users))
	for username, user := range a.users {
		if user.role != "cashier" {
			continue
		}
		result = append(result, domain.CashierUser{
			Username:  username,
			Role:      user.role,
			Active:    user.active,
			CreatedAt: user.created,
		})
	}
	a.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool {
		return result[i].Username < result[j].Username
	})
	return result
}

// bootstrapUsers loads user accounts from the user store into the in-memory
// credential cache. It also upgrades any legacy plain-text passwords to bcrypt
// hashes in the store. The provided ctx is passed through to all store calls.
func (a *AuthManager) bootstrapUsers(ctx context.Context) {
	if a.userStore == nil {
		return
	}

	users, err := a.userStore.ListUsers(ctx)
	if err != nil || len(users) == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, user := range users {
		username := strings.ToLower(strings.TrimSpace(user.Username))
		if username == "" {
			continue
		}
		password := user.Password
		if !isPasswordHash(password) {
			hashed, err := hashPassword(password)
			if err == nil {
				password = hashed
				_ = a.userStore.UpdateUserPassword(ctx, username, hashed)
			}
		}
		a.users[username] = credential{
			password: password,
			role:     user.Role,
			active:   user.Active,
			created:  user.CreatedAt,
		}
	}
}

func verifyPassword(stored string, input string) bool {
	if stored == "" || strings.TrimSpace(input) == "" || !isPasswordHash(stored) {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(input)) == nil
}

func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func isPasswordHash(value string) bool {
	return strings.HasPrefix(value, "$2a$") || strings.HasPrefix(value, "$2b$") || strings.HasPrefix(value, "$2y$")
}

