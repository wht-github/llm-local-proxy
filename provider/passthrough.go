package provider

import (
	"llm-local-proxy/config"
	"llm-local-proxy/transform"
)

// Passthrough forwards requests and responses without any transformation.
// Use for models/providers that don't have reasoning_content or need no processing.
type Passthrough struct {
	name    string
	baseURL string
	apiKey  string
}

func NewPassthrough(cfg config.ProviderConfig) *Passthrough {
	return &Passthrough{
		name:    cfg.Name,
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
	}
}

func (p *Passthrough) Name() string    { return p.name }
func (p *Passthrough) BaseURL() string { return p.baseURL }
func (p *Passthrough) APIKey() string  { return p.apiKey }

func (p *Passthrough) TransformRequest(body []byte) []byte  { return body }
func (p *Passthrough) TransformStreamDelta(_ map[string]any, _ *transform.StreamState) {}
func (p *Passthrough) TransformResponse(body []byte) []byte { return body }
