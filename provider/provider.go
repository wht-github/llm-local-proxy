package provider

import (
	"fmt"

	"llm-local-proxy/config"
	"llm-local-proxy/transform"
)

// Provider defines the adapter interface for upstream LLM services.
type Provider interface {
	// Name returns the provider identifier from config.
	Name() string
	// BaseURL returns the upstream API base URL.
	BaseURL() string
	// APIKey returns the authentication key.
	APIKey() string
	// TransformRequest modifies the request body before forwarding.
	TransformRequest(body []byte) []byte
	// TransformStreamDelta processes a single SSE choice delta.
	TransformStreamDelta(choice map[string]any, state *transform.StreamState)
	// TransformResponse processes a non-streaming response body.
	TransformResponse(body []byte) []byte
}

// Registry maps model names to providers.
type Registry struct {
	byModel         map[string]Provider
	defaultProvider Provider
	debug           bool
}

// NewRegistry builds a provider registry from configuration.
func NewRegistry(cfg *config.Config) (*Registry, error) {
	r := &Registry{
		byModel: make(map[string]Provider),
		debug:   cfg.Debug,
	}

	providers := make(map[string]Provider)
	for _, pc := range cfg.Providers {
		p, err := newProvider(pc, cfg.Debug)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", pc.Name, err)
		}
		providers[pc.Name] = p
		for _, model := range pc.Models {
			r.byModel[model] = p
		}
	}

	if cfg.DefaultProvider != "" {
		dp, ok := providers[cfg.DefaultProvider]
		if !ok {
			return nil, fmt.Errorf("default_provider %q not found", cfg.DefaultProvider)
		}
		r.defaultProvider = dp
	} else {
		// Use first provider as default
		first, _ := newProvider(cfg.Providers[0], cfg.Debug)
		r.defaultProvider = first
	}

	return r, nil
}

// Resolve finds the provider for a given model name.
func (r *Registry) Resolve(model string) Provider {
	if p, ok := r.byModel[model]; ok {
		return p
	}
	if p, ok := r.byModel["*"]; ok {
		return p
	}
	return r.defaultProvider
}

// Default returns the default provider.
func (r *Registry) Default() Provider {
	return r.defaultProvider
}

// Debug returns whether debug mode is enabled.
func (r *Registry) Debug() bool {
	return r.debug
}

func newProvider(pc config.ProviderConfig, debug bool) (Provider, error) {
	switch pc.Type {
	case "deepseek":
		return NewDeepSeek(pc, debug), nil
	case "kimi":
		return NewKimi(pc, debug), nil
	case "zhipu":
		return NewZhipu(pc, debug), nil
	case "passthrough":
		return NewPassthrough(pc), nil
	default:
		return nil, fmt.Errorf("unknown provider type %q", pc.Type)
	}
}
