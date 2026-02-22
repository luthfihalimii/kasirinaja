package config

import "testing"

func TestLoadDoesNotInjectWeakAuthDefaults(t *testing.T) {
	t.Setenv("AUTH_SECRET", "")
	t.Setenv("MANAGER_PIN", "")

	cfg := Load()
	if cfg.AuthSecret != "" {
		t.Fatalf("expected empty AUTH_SECRET when unset, got %q", cfg.AuthSecret)
	}
	if cfg.ManagerPIN != "" {
		t.Fatalf("expected empty MANAGER_PIN when unset, got %q", cfg.ManagerPIN)
	}
}
