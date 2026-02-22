package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                     string
	AllowedOrigin            string
	DatabaseURL              string
	RedisAddr                string
	RedisPassword            string
	RedisDB                  int
	StoreID                  string
	RecommendationTTLSeconds int
	AuthSecret               string
	AccessTokenTTLMinutes    int
	ManagerPIN               string
}

func Load() Config {
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	ttl, err := strconv.Atoi(getEnv("RECOMMENDATION_TTL_SECONDS", "20"))
	if err != nil || ttl < 1 {
		ttl = 20
	}
	tokenTTL, err := strconv.Atoi(getEnv("ACCESS_TOKEN_TTL_MINUTES", "480"))
	if err != nil || tokenTTL < 1 {
		tokenTTL = 480
	}

	cfg := Config{
		Port:                     getEnv("PORT", "8080"),
		AllowedOrigin:            getEnv("ALLOWED_ORIGIN", "http://127.0.0.1:3000"),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		RedisAddr:                os.Getenv("REDIS_ADDR"),
		RedisPassword:            os.Getenv("REDIS_PASSWORD"),
		RedisDB:                  redisDB,
		StoreID:                  getEnv("DEFAULT_STORE_ID", "main-store"),
		RecommendationTTLSeconds: ttl,
		AuthSecret:               strings.TrimSpace(os.Getenv("AUTH_SECRET")),
		AccessTokenTTLMinutes:    tokenTTL,
		ManagerPIN:               strings.TrimSpace(os.Getenv("MANAGER_PIN")),
	}

	return cfg
}

func (c Config) Address() string {
	return fmt.Sprintf(":%s", c.Port)
}

func getEnv(key string, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}
