package provider

import (
	"llm-local-proxy/config"
	"llm-local-proxy/transform"
)

// Zhipu (GLM) uses reasoning_content for models with deep thinking capability.
// Historical reasoning is cleaned; field is not strictly required.
type Zhipu struct {
	name    string
	baseURL string
	apiKey  string
	debug   bool
}

func NewZhipu(cfg config.ProviderConfig, debug bool) *Zhipu {
	return &Zhipu{
		name:    cfg.Name,
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		debug:   debug,
	}
}

func (z *Zhipu) Name() string    { return z.name }
func (z *Zhipu) BaseURL() string { return z.baseURL }
func (z *Zhipu) APIKey() string  { return z.apiKey }

func (z *Zhipu) TransformRequest(body []byte) []byte {
	// Zhipu: restore reasoning from <thought> tags, clean history
	return transform.PrepareRequestMessages(body, false, true)
}

func (z *Zhipu) TransformStreamDelta(choice map[string]any, state *transform.StreamState) {
	transform.TransformDelta(choice, state, z.debug)
}

func (z *Zhipu) TransformResponse(body []byte) []byte {
	return transform.TransformFullResponse(body)
}
