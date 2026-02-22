package main

import (
	"testing"

	"kasirinaja/backend/internal/config"
)

func TestValidateSecurityConfigRejectsWeakValues(t *testing.T) {
	err := validateSecurityConfig(config.Config{AuthSecret: "short", ManagerPIN: "123456"})
	if err == nil {
		t.Fatalf("expected weak security config to be rejected")
	}
}

func TestValidateSecurityConfigAcceptsStrongValues(t *testing.T) {
	err := validateSecurityConfig(config.Config{AuthSecret: "0123456789abcdef0123456789abcdef", ManagerPIN: "739154"})
	if err != nil {
		t.Fatalf("expected strong config to pass, got %v", err)
	}
}
