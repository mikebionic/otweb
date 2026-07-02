package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Database DatabaseConfig `yaml:"database"`
	OTAPI    OTAPIConfig    `yaml:"otapi"`
	CSCart   CSCartConfig   `yaml:"cscart"`
	DeepSeek DeepSeekConfig `yaml:"deepseek"`
	Pricing  PricingConfig  `yaml:"pricing"`
	Images   ImagesConfig   `yaml:"images"`
}

type ImagesConfig struct {
	// Локальный каталог на сервере магазина, куда скачиваются фото перед пушем
	// (CS-Cart забирает их по публичному URL и сохраняет у себя).
	LocalDir string `yaml:"local_dir"`
	// Публичный путь (относительно cscart.base_url), по которому отдаются скачанные фото.
	PublicPath string `yaml:"public_path"`
}

type AuthConfig struct {
	Username string     `yaml:"username"`
	Password string     `yaml:"password"`
	Users    []AuthUser `yaml:"users"`
}

type AuthUser struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Check reports whether the given credentials match the primary admin
// account (username/password) or any of the additional users listed under
// auth.users. Add/change logins by editing config.yaml and restarting.
func (a AuthConfig) Check(username, password string) bool {
	if username == "" || password == "" {
		return false
	}
	if username == a.Username && password == a.Password {
		return true
	}
	for _, u := range a.Users {
		if username == u.Username && password == u.Password {
			return true
		}
	}
	return false
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
		Auth:   AuthConfig{Username: "admin", Password: "admin"},
		Database: DatabaseConfig{
			HubDSN:    "user:pass@tcp(127.0.0.1:3306)/otapi_hub?collation=utf8mb4_unicode_ci&parseTime=true&tls=skip-verify",
			MirrorDSN: "user:pass@tcp(127.0.0.1:3306)/shop_mirror?collation=utf8mb4_unicode_ci&parseTime=true&tls=skip-verify",
		},
		OTAPI: OTAPIConfig{
			BaseURL:   "https://rest.otapi.net",
			LegacyURL: "https://otapi.net/service-json",
		},
		CSCart: CSCartConfig{
			BaseURL: "https://example.com",
		},
		DeepSeek: DeepSeekConfig{
			BaseURL: "https://api.deepseek.com",
		},
		Pricing: PricingConfig{
			DefaultMarkupPct: 35.0,
			ExchangeRateCNY:  0.57,
		},
		Images: ImagesConfig{
			PublicPath: "/images/otapi",
		},
	}
}
