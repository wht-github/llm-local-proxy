package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llm-local-proxy/provider"
	"llm-local-proxy/transform"
)

var httpClient = &http.Client{
	Timeout: 5 * time.Minute,
}

// Handler routes incoming requests to upstream providers.
type Handler struct {
	registry provider.Registry
}

func NewHandler(registry provider.Registry) *Handler {
	return &Handler{registry: registry}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("[%s] %s %s\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Resolve provider by model in request body
	p := h.resolveProvider(body)
	if p == nil {
		http.Error(w, "no provider matched for requested model", http.StatusBadGateway)
		return
	}
	fmt.Printf("  → provider: %s (%s)\n", p.Name(), p.BaseURL())

	// Transform request body (provider-specific)
	body = p.TransformRequest(body)

	// Build upstream URL
	targetURL := p.BaseURL() + "/chat/completions"
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// Copy and fix headers
	copyHeaders(proxyReq.Header, r.Header)
	if apiKey := p.APIKey(); apiKey != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	proxyReq.Header.Del("Accept-Encoding") // Disable compression for real-time content modification
	proxyReq.Header.Del("Content-Length")  // Let http.Client recalculate
	proxyReq.ContentLength = int64(len(body))

	// Send request upstream
	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		fmt.Printf("  ✗ upstream error: %v\n", err)
		http.Error(w, "Upstream connection failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward response headers (skip conflicting ones)
	for k, vv := range resp.Header {
		if k == "Content-Length" || k == "Content-Encoding" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Route response handling
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	if resp.StatusCode != http.StatusOK || !isSSE {
		// Non-streaming response
		respBody, _ := io.ReadAll(resp.Body)
		w.Write(p.TransformResponse(respBody))
		return
	}

	// SSE streaming response
	h.processSSE(w, resp.Body, p)
}

// resolveProvider parses the model field from the request body and finds the matching provider.
func (h *Handler) resolveProvider(body []byte) provider.Provider {
	var req struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &req) == nil {
		return h.registry.Resolve(req.Model)
	}
	return nil
}

// stripVersionPrefix removes "/v1", "/v2", etc. from the path prefix.
// This allows base_url to include the provider's own version path.
// e.g. "/v1/chat/completions" → "/chat/completions"
func stripVersionPrefix(path string) string {
	if strings.HasPrefix(path, "/v") && len(path) > 2 {
		if idx := strings.Index(path[1:], "/"); idx > 0 {
			return path[idx+1:]
		}
	}
	return path
}

// processSSE handles SSE streaming, applying provider-specific delta transformation.
func (h *Handler) processSSE(w http.ResponseWriter, body io.Reader, p provider.Provider) {
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(body)
	state := &transform.StreamState{}
	debug := h.registry.Debug()

	closeReasoning := func() {
		if !state.IsReasoning {
			return
		}
		w.Write([]byte(transform.ClosingTagSSE()))
		if flusher != nil {
			flusher.Flush()
		}
		state.IsReasoning = false
	}

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			closeReasoning()
			break
		}

		if bytes.HasPrefix(line, []byte("data: ")) {
			dataBytes := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))

			if string(dataBytes) == "[DONE]" {
				closeReasoning()
				if debug {
					fmt.Println("\n[DONE]")
				}
			} else {
				var data map[string]any
				if json.Unmarshal(dataBytes, &data) == nil {
					if choices, ok := data["choices"].([]any); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]any); ok {
							p.TransformStreamDelta(choice, state)
						}
					}
					if newData, err := json.Marshal(data); err == nil {
						line = append([]byte("data: "), newData...)
						line = append(line, '\n')
					}
				}
			}
		}

		w.Write(line)
		if flusher != nil {
			flusher.Flush()
		}

		if err != nil {
			closeReasoning()
			break
		}
	}
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}
