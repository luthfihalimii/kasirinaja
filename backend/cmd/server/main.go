package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kasirinaja/backend/internal/cache"
	"kasirinaja/backend/internal/config"
	"kasirinaja/backend/internal/httpapi"
	"kasirinaja/backend/internal/recommendation"
	"kasirinaja/backend/internal/service"
	"kasirinaja/backend/internal/store"
	"kasirinaja/backend/internal/store/memory"
	pgstore "kasirinaja/backend/internal/store/postgres"
)

func main() {
	cfg := config.Load()
	if err := validateSecurityConfig(cfg); err != nil {
		log.Fatalf("invalid security configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var repo store.Repository
	closers := make([]func() error, 0, 2)

	if cfg.DatabaseURL != "" {
		pg, err := pgstore.New(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("postgres unavailable (%v) and DATABASE_URL is set; refusing to start with in-memory fallback", err)
		} else {
			repo = pg
			closers = append(closers, pg.Close)
			log.Println("repository: postgres")
		}
	} else {
		repo = memory.NewSeeded()
		log.Println("repository: in-memory")
	}

	cacheStore := cache.RecommendationCache(cache.NoopRecommendationCache{})
	if cfg.RedisAddr != "" {
		redisCache := cache.NewRedisRecommendationCache(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if err := redisCache.Ping(ctx); err != nil {
			log.Printf("redis unavailable (%v), using noop cache", err)
		} else {
			cacheStore = redisCache
			closers = append(closers, redisCache.Close)
			log.Println("cache: redis")
		}
	} else {
		log.Println("cache: noop")
	}

	recommender := recommendation.NewEngine(cacheStore, time.Duration(cfg.RecommendationTTLSeconds)*time.Second)
	svc := service.New(repo, recommender, cfg.StoreID)
	auth := httpapi.NewAuthManager(cfg.AuthSecret, time.Duration(cfg.AccessTokenTTLMinutes)*time.Minute, cfg.ManagerPIN, repo)
	api := httpapi.New(svc, auth, cfg.AllowedOrigin)

	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("POS backend listening on %s", cfg.Address())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}

	for _, closeFn := range closers {
		if err := closeFn(); err != nil {
			log.Printf("close error: %v", err)
		}
	}

	log.Println("server stopped")
}

func validateSecurityConfig(cfg config.Config) error {
	if len(cfg.AuthSecret) < 32 {
		return fmt.Errorf("AUTH_SECRET must be set and at least 32 characters")
	}
	if len(cfg.ManagerPIN) < 6 {
		return fmt.Errorf("MANAGER_PIN must be set and at least 6 digits")
	}
	if err := validatePINStrength(cfg.ManagerPIN); err != nil {
		return fmt.Errorf("MANAGER_PIN is too weak: %w", err)
	}
	return nil
}

// validatePINStrength rejects PINs that are all the same digit,
// sequential (ascending or descending), or from a known-weak list.
func validatePINStrength(pin string) error {
	known := map[string]bool{
		"123456": true, "654321": true, "000000": true, "111111": true,
		"222222": true, "333333": true, "444444": true, "555555": true,
		"666666": true, "777777": true, "888888": true, "999999": true,
		"121212": true, "112233": true, "123123": true,
	}
	if known[pin] {
		return fmt.Errorf("common PIN not allowed")
	}

	// Reject all-same-digit PINs.
	allSame := true
	for i := 1; i < len(pin); i++ {
		if pin[i] != pin[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return fmt.Errorf("all-same-digit PIN not allowed")
	}

	// Reject ascending or descending sequential PINs (e.g. 123456, 987654).
	ascending, descending := true, true
	for i := 1; i < len(pin); i++ {
		diff := int(pin[i]) - int(pin[i-1])
		if diff != 1 {
			ascending = false
		}
		if diff != -1 {
			descending = false
		}
	}
	if ascending || descending {
		return fmt.Errorf("sequential PIN not allowed")
	}

	return nil
}
