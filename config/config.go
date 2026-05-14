package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	OTAPI    OTAPIConfig    `yaml:"otapi"`
	CSCart   CSCartConfig   `yaml:"cscart"`
	DeepSeek DeepSeekConfig `yaml:"deepseek"`
	Pricing  PricingConfig  `yaml:"pricing"`
}

type CSCartConfig struct {
	BaseURL   string `yaml:"base_url"`
	Email     string `yaml:"email"`
	APIKey    string `yaml:"api_key"`
	CompanyID int    `yaml:"company_id"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type DatabaseConfig struct {
	HubDSN    string `yaml:"hub_dsn"`
	MirrorDSN string `yaml:"mirror_dsn"`
}

type OTAPIConfig struct {
	InstanceKey string `yaml:"instance_key"`
	BaseURL     string `yaml:"base_url"`
	LegacyURL   string `yaml:"legacy_url"`
}

type DeepSeekConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

type PricingConfig struct {
	DefaultMarkupPct float64 `yaml:"default_markup_pct"`
	ExchangeRateCNY  float64 `yaml:"exchange_rate_cny"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{Port: "5500"},
		Database: DatabaseConfig{
			HubDSN:    "otapi:otapi_pass@tcp(127.0.0.1:3360)/otapi_hub?collation=utf8mb4_unicode_ci&parseTime=true&tls=skip-verify",
			MirrorDSN: "otapi:otapi_pass@tcp(127.0.0.1:3360)/wabrum_mv?collation=utf8mb4_unicode_ci&parseTime=true&tls=skip-verify",
		},
		OTAPI: OTAPIConfig{
			InstanceKey: "REDACTED",
			BaseURL:     "https://rest.otapi.net",
			LegacyURL:   "https://otapi.net/service-json",
		},
		CSCart: CSCartConfig{
			BaseURL:   "https://wabrum.com",
			Email:     "api@wabrum.com",
			APIKey:    "REDACTED",
			CompanyID: 376,
		},
		DeepSeek: DeepSeekConfig{
			APIKey:  "sk-REDACTED",
			BaseURL: "https://api.deepseek.com",
		},
		Pricing: PricingConfig{
			DefaultMarkupPct: 35.0,
			ExchangeRateCNY:  0.57,
		},
	}
}
