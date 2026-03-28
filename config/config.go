package config

import (
	"encoding/json"
	"errors"
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
	Listen    string           `json:"listen"` // e.g. ":12000"
	Debug     bool             `json:"debug"`
	Providers []ProviderConfig `json:"providers"`
}

// Load reads and parses a JSON config file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for required fields.
func (c Config) Validate() error {
	var errs []error

	if len(c.Providers) == 0 {
		errs = append(errs, errors.New("no providers configured"))
	}
	if c.Listen == "" {
		errs = append(errs, errors.New("listen address is required (e.g. \":12000\")"))
	}

	for i, p := range c.Providers {
		if p.Name == "" {
			errs = append(errs, fmt.Errorf("provider[%d]: name is required", i))
			continue
		}
		if p.Type == "" {
			errs = append(errs, fmt.Errorf("provider %q: type is required", p.Name))
		}
		if p.BaseURL == "" {
			errs = append(errs, fmt.Errorf("provider %q: base_url is required", p.Name))
		}
	}

	return errors.Join(errs...)
}
