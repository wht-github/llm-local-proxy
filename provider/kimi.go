package provider

import (
	"llm-local-proxy/config"
	"llm-local-proxy/transform"
)

// Kimi (Moonshot) uses reasoning_content identically to DeepSeek.
// Docs recommend preserving all reasoning_content in context (no history cleanup).
// When thinking is enabled, reasoning_content is required on all assistant messages.
type Kimi struct {
	name    string
	baseURL string
	apiKey  string
	debug   bool
}

func NewKimi(cfg config.ProviderConfig, debug bool) *Kimi {
	return &Kimi{
		name:    cfg.Name,
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		debug:   debug,
	}
}

func (k *Kimi) Name() string    { return k.name }
func (k *Kimi) BaseURL() string { return k.baseURL }
func (k *Kimi) APIKey() string  { return k.apiKey }

func (k *Kimi) TransformRequest(body []byte) []byte {
	// Kimi: restore reasoning from <thought> tags, preserve all history reasoning
	return transform.PrepareRequestMessages(body, true, false)
}

func (k *Kimi) TransformStreamDelta(choice map[string]any, state *transform.StreamState) {
	transform.TransformDelta(choice, state, k.debug)
}

func (k *Kimi) TransformResponse(body []byte) []byte {
	return transform.TransformFullResponse(body)
}
