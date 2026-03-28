package transform

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StreamState tracks reasoning state within a single SSE connection.
type StreamState struct {
	IsReasoning bool
}

// NormalizeThoughtContent extracts <thought> content from a string.
// Handles three cases:
//  1. <thought>...</thought> — normal extraction
//  2. <thought> only (unclosed) — everything after is treated as thought
//  3. </thought> only (orphan close) — removed as dirty data
func NormalizeThoughtContent(content string) (thought, cleaned string, found bool) {
	startIdx := strings.Index(content, "<thought>")
	endIdx := strings.Index(content, "</thought>")

	if startIdx >= 0 {
		prefix := content[:startIdx]
		if endIdx > startIdx {
			thought = content[startIdx+len("<thought>") : endIdx]
			cleaned = prefix + content[endIdx+len("</thought>"):]
			return thought, cleaned, true
		}
		// Unclosed: treat everything after open tag as thought
		thought = content[startIdx+len("<thought>"):]
		cleaned = prefix
		return thought, cleaned, true
	}

	if endIdx >= 0 {
		cleaned = strings.ReplaceAll(content, "</thought>", "")
		return "", cleaned, true
	}

	return "", content, false
}

// TransformDelta converts reasoning_content in a SSE choice delta to
// <thought> tags merged into the content field.
// Shared by all reasoning-capable providers (DeepSeek, Kimi, Zhipu).
func TransformDelta(choice map[string]any, state *StreamState, debug bool) {
	delta, hasDelta := choice["delta"].(map[string]any)
	if !hasDelta {
		delta = map[string]any{}
		choice["delta"] = delta
	}

	finish := choice["finish_reason"] != nil
	if !hasDelta && !finish && !state.IsReasoning {
		return
	}

	rc, hasRC := delta["reasoning_content"]
	content, hasContent := delta["content"]
	hasNonNilContent := hasContent && content != nil

	rcStr, _ := rc.(string)
	contentStr, _ := content.(string)
	hasReasoningChunk := hasRC && rcStr != ""

	var b strings.Builder

	if hasReasoningChunk {
		if !state.IsReasoning {
			if debug {
				fmt.Print("\n--- reasoning start ---\n")
			}
			b.WriteString("<thought>\n")
			state.IsReasoning = true
		}
		b.WriteString(rcStr)
		if debug {
			fmt.Print(rcStr)
		}
	}

	if state.IsReasoning && hasNonNilContent {
		if debug {
			fmt.Print("\n--- reasoning end, content start ---\n")
		}
		b.WriteString("\n</thought>\n\n")
		state.IsReasoning = false
	}

	if hasNonNilContent {
		b.WriteString(contentStr)
		if debug && contentStr != "" {
			fmt.Print(contentStr)
		}
	}

	if state.IsReasoning && finish {
		if debug {
			fmt.Print("\n--- reasoning end (no content) ---\n")
		}
		b.WriteString("\n</thought>\n\n")
		state.IsReasoning = false
	}

	if hasReasoningChunk || hasNonNilContent || b.Len() > 0 {
		delta["content"] = b.String()
	}

	delete(delta, "reasoning_content")
	if v, exists := delta["content"]; exists && v == nil {
		delta["content"] = ""
	}
}

// ClosingTagSSE returns the SSE data line to inject when a stream ends mid-reasoning.
func ClosingTagSSE() string {
	msg := map[string]any{
		"choices": []any{
			map[string]any{
				"delta": map[string]any{
					"content": "\n</thought>\n\n",
				},
			},
		},
	}
	b, _ := json.Marshal(msg)
	return "data: " + string(b) + "\n\n"
}

// TransformFullResponse merges reasoning_content into content for non-streaming responses.
func TransformFullResponse(body []byte) []byte {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	choices, ok := data["choices"].([]any)
	if !ok {
		return body
	}

	changed := false
	for _, c := range choices {
		choice, ok := c.(map[string]any)
		if !ok {
			continue
		}
		msg, ok := choice["message"].(map[string]any)
		if !ok {
			continue
		}

		rc, hasRC := msg["reasoning_content"]
		if !hasRC {
			continue
		}

		rcStr, _ := rc.(string)
		if rcStr == "" {
			delete(msg, "reasoning_content")
			changed = true
			continue
		}

		contentStr, _ := msg["content"].(string)
		msg["content"] = "<thought>\n" + rcStr + "\n</thought>\n\n" + contentStr
		delete(msg, "reasoning_content")
		changed = true
	}

	if changed {
		if newBody, err := json.Marshal(data); err == nil {
			return newBody
		}
	}
	return body
}

// PrepareRequestMessages handles the shared request-side transformation:
//   - Restores reasoning_content from <thought> tags in assistant messages
//   - Optionally cleans reasoning from historical turns (before last user message)
//   - Optionally ensures reasoning_content field exists on all assistant messages
//
// Parameters:
//   - body: raw request JSON
//   - requireField: if true, ensures reasoning_content exists (even empty) on assistant messages (DeepSeek)
//   - cleanHistory: if true, removes reasoning_content from messages before the last user message
func PrepareRequestMessages(body []byte, requireField bool, cleanHistory bool) []byte {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	messages, ok := data["messages"].([]any)
	if !ok {
		return body
	}

	// Find the last user message index as the "current turn" boundary
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if msg, ok := messages[i].(map[string]any); ok && msg["role"] == "user" {
			lastUserIdx = i
			break
		}
	}

	changed := false
	for i, m := range messages {
		msg, ok := m.(map[string]any)
		if !ok || msg["role"] != "assistant" {
			continue
		}

		content, _ := msg["content"].(string)
		thought, cleanedContent, hasThought := NormalizeThoughtContent(content)

		if cleanHistory && i < lastUserIdx {
			// Historical turn: discard reasoning content
			if hasThought {
				msg["content"] = strings.TrimSpace(cleanedContent)
				changed = true
			}
			if _, exists := msg["reasoning_content"]; exists {
				delete(msg, "reasoning_content")
				changed = true
			}
		} else {
			// Current turn: restore reasoning_content from <thought> tags
			if hasThought {
				extracted := strings.TrimSpace(thought)
				existing, _ := msg["reasoning_content"].(string)
				existing = strings.TrimSpace(existing)

				switch {
				case existing == "":
					msg["reasoning_content"] = extracted
				case extracted == "":
					// keep existing
				default:
					msg["reasoning_content"] = existing + "\n" + extracted
				}

				msg["content"] = strings.TrimSpace(cleanedContent)
				changed = true
			}

			// DeepSeek requires reasoning_content field to exist on assistant messages
			if requireField {
				if _, exists := msg["reasoning_content"]; !exists {
					msg["reasoning_content"] = ""
					changed = true
				}
			}
		}
	}

	if changed {
		if newBody, err := json.Marshal(data); err == nil {
			return newBody
		}
	}
	return body
}
