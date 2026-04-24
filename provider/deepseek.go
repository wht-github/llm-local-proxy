package provider

import (
	"llm-local-proxy/config"
	"llm-local-proxy/transform"
)

// DeepSeek requires reasoning_content field on all assistant messages (even empty).
// Historical reasoning is cleaned to save bandwidth per their docs.
// reasoning_effort is injected from config if the client doesn't send it.
type DeepSeek struct {
	name            string
	baseURL         string
	apiKey          string
	reasoningEffort string // from config: "high" or "max"
	debug           bool
}

func NewDeepSeek(cfg config.ProviderConfig, debug bool) *DeepSeek {
	return &DeepSeek{
		name:            cfg.Name,
		baseURL:         cfg.BaseURL,
		apiKey:          cfg.APIKey,
		reasoningEffort: cfg.ReasoningEffort,
		debug:           debug,
	}
}

func (d *DeepSeek) Name() string    { return d.name }
func (d *DeepSeek) BaseURL() string { return d.baseURL }
func (d *DeepSeek) APIKey() string  { return d.apiKey }

func (d *DeepSeek) TransformRequest(body []byte) []byte {
	body = transform.PrepareRequestMessages(body, true, true)
	return transform.InjectReasoningEffort(body, d.reasoningEffort, d.debug)
}

func (d *DeepSeek) TransformStreamDelta(choice map[string]any, state *transform.StreamState) {
	transform.TransformDelta(choice, state, d.debug)
}

func (d *DeepSeek) TransformResponse(body []byte) []byte {
	return transform.TransformFullResponse(body)
}
