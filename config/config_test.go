package config_test

import (
	"os"
	"testing"

	"otapi-hub/config"
)

func TestDefault_Values(t *testing.T) {
	cfg := config.Default()

	if cfg.Server.Port != "5500" {
		t.Errorf("expected port 5500, got %s", cfg.Server.Port)
	}
	if cfg.OTAPI.InstanceKey == "" {
		t.Error("expected non-empty OTAPI InstanceKey")
	}
	if cfg.OTAPI.BaseURL != "https://rest.otapi.net" {
		t.Errorf("unexpected OTAPI BaseURL: %s", cfg.OTAPI.BaseURL)
	}
	if cfg.Pricing.DefaultMarkupPct <= 0 {
		t.Errorf("expected positive markup pct, got %f", cfg.Pricing.DefaultMarkupPct)
	}
	if cfg.Pricing.ExchangeRateCNY <= 0 {
		t.Errorf("expected positive exchange rate, got %f", cfg.Pricing.ExchangeRateCNY)
	}
	if cfg.Database.HubDSN == "" {
		t.Error("expected non-empty HubDSN")
	}
	if cfg.Database.MirrorDSN == "" {
		t.Error("expected non-empty MirrorDSN")
	}
}

func TestLoad_ValidYAML(t *testing.T) {
	yaml := `
server:
  port: "8080"
otapi:
  instance_key: "test-key-123"
  base_url: "https://example.com"
pricing:
  default_markup_pct: 40.0
  exchange_rate_cny: 0.60
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := config.Load(f.Name())
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Server.Port)
	}
	if cfg.OTAPI.InstanceKey != "test-key-123" {
		t.Errorf("expected instance key test-key-123, got %s", cfg.OTAPI.InstanceKey)
	}
	if cfg.Pricing.DefaultMarkupPct != 40.0 {
		t.Errorf("expected markup 40.0, got %f", cfg.Pricing.DefaultMarkupPct)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	f.WriteString("not: valid: yaml: ::::")
	f.Close()

	_, err = config.Load(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
