package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ProviderConfig defines a single upstream LLM provider.
type ProviderConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`     // "deepseek", "kimi", "zhipu", "passthrough"
	BaseURL string   `json:"base_url"` // Full base URL including version path (e.g. "https://api.moonshot.cn/v1")
	APIKey  string   `json:"api_key"`
	Models  []string `json:"models"` // Model names to route to this provider; "*" = catch-all
}

// Config is the top-level configuration.
type Config struct {
	Listen          string           `json:"listen"` // e.g. ":12000"
	Debug           bool             `json:"debug"`
	Providers       []ProviderConfig `json:"providers"`
	DefaultProvider string           `json:"default_provider"` // Name of the default provider
}

// Load reads and parses a JSON config file.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}
	if cfg.Listen == "" {
		cfg.Listen = ":12000"
	}

	for i, p := range cfg.Providers {
		if p.Name == "" {
			return nil, fmt.Errorf("provider[%d]: name is required", i)
		}
		if p.Type == "" {
			return nil, fmt.Errorf("provider %q: type is required", p.Name)
		}
		if p.BaseURL == "" {
			return nil, fmt.Errorf("provider %q: base_url is required", p.Name)
		}
	}

	return &cfg, nil
}
