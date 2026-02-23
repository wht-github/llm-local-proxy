package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// 配置结构
type Config struct {
	APIKey        string `json:"api_key"`
	ProxyPort     string `json:"proxy_port"`
	TargetBaseURL string `json:"target_base_url"`
}

// 全局配置变量
var config Config
var debugMode bool

// 复用连接池
var httpClient = &http.Client{
	Timeout: 5 * time.Minute,
}

// loadConfig 从 JSON 文件加载配置
func loadConfig(configFile string) error {
	file, err := os.Open(configFile)
	if err != nil {
		return fmt.Errorf("无法打开配置文件 %s: %v", configFile, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 验证必要字段
	if config.APIKey == "" {
		return fmt.Errorf("配置文件中缺少 api_key")
	}
	if config.ProxyPort == "" {
		config.ProxyPort = "12000" // 默认端口
	}
	if config.TargetBaseURL == "" {
		config.TargetBaseURL = "https://api.deepseek.com" // 默认地址
	}

	return nil
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "config.json", "配置文件路径")
	flag.BoolVar(&debugMode, "debug", false, "启用调试模式，打印非流式请求和响应详情")
	flag.Parse()

	// 加载配置文件
	if err := loadConfig(configFile); err != nil {
		fmt.Printf("❌ 加载配置失败: %v\n", err)
		fmt.Println("请创建 config.json 文件，格式如下:")
		fmt.Println(`{
  "api_key": "your-deepseek-api-key-here",
  "proxy_port": "12000",
  "target_base_url": "https://api.deepseek.com"
}`)
		os.Exit(1)
	}

	if debugMode {
		fmt.Println("🔧 调试模式已启用 - 将打印非流式请求和响应详情")
	}

	// 注册路由 - 保留所有原始路由，转发到对应路径
	http.HandleFunc("/", handleProxy)

	fmt.Printf("🚀 LLM Proxy 已就绪: http://127.0.0.1:%s\n", config.ProxyPort)
	fmt.Printf("📡 目标服务器: %s\n", config.TargetBaseURL)
	if err := http.ListenAndServe(":"+config.ProxyPort, nil); err != nil {
		fmt.Printf("服务器启动失败: %v\n", err)
	}
}

// handleProxy 处理请求转发与响应拦截
func handleProxy(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("[%s] %s %s\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path)

	// 1. 读取并修复请求体 (解决 DeepSeek 必须回传 reasoning_content 的限制)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// 保存原始请求体用于调试
	originalBody := make([]byte, len(body))
	copy(originalBody, body)

	body = ensureReasoningField(body)

	// 2. 构造转发请求 - 保留原始路由路径
	targetURL := config.TargetBaseURL + r.URL.Path
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, bytes.NewBuffer(body))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// 3. 设置 Headers
	copyHeader(proxyReq.Header, r.Header)
	proxyReq.Header.Set("Authorization", "Bearer "+config.APIKey)

	// 修正转发必要的 Header
	proxyReq.Header.Del("Accept-Encoding") // 禁用压缩以便进行实时修改内容
	proxyReq.Header.Del("Content-Length")  // 由 http.Client 自动计算
	proxyReq.Host = "api.deepseek.com"
	proxyReq.ContentLength = int64(len(body))

	// 4. 发送请求
	resp, err := httpClient.Do(proxyReq)
	if err != nil {
		fmt.Printf("Upstream error: %v\n", err)
		http.Error(w, "DeepSeek connection failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 5. 转发响应头 (剔除可能导致冲突的字段)
	for k, vv := range resp.Header {
		if k == "Content-Length" || k == "Content-Encoding" {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 6. 处理响应内容
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	if resp.StatusCode != http.StatusOK || !isSSE {
		// 非流式响应 - 记录完整请求和响应（如果启用了调试模式）
		if debugMode {
			debugNonStreaming(r, originalBody, resp, w)
		} else {
			io.Copy(w, resp.Body)
		}
		return
	}

	if debugMode {
		fmt.Println("\n" + strings.Repeat("=", 80))
		fmt.Println("📤 流式请求详情")
		fmt.Println(strings.Repeat("-", 40))
		fmt.Println("请求体 (简化):")
		fmt.Println(getDebugRequestBody(originalBody))
		fmt.Println("\n📥 流式响应内容:")
		fmt.Println(strings.Repeat("-", 40))
	}

	processSSEResponse(w, resp.Body)

	if debugMode {
		fmt.Println("\n" + strings.Repeat("=", 80) + "\n")
	}
}

// ensureReasoningField 确保 assistant 消息中包含 reasoning_content 字段。
// 遵循 DeepSeek 最佳实践：
// 1. 在当前轮对话（如工具调用过程中）还原 <thought> 为 reasoning_content，防止 400 错误。
// 2. 在开启新一轮对话时，丢弃之前轮次的 reasoning_content 以节省带宽。
//
// 多步工具调用场景：
// - 在工具调用过程中，每个 assistant 响应都可能包含独立的推理过程
// - 必须保留当前轮次的所有 reasoning_content，否则 API 会返回 400 错误
// - 只有在开启新一轮对话（新的 user 消息）后，才丢弃历史 reasoning_content
func ensureReasoningField(body []byte) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	messages, ok := data["messages"].([]interface{})
	if !ok {
		return body
	}

	// 找到最后一个用户消息的索引，作为“当前轮次”的界限
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
		hasThought := strings.Contains(content, "<thought>") && strings.Contains(content, "</thought>")

		if i < lastUserIdx {
			// 情况 A: 历史轮次的思考内容，根据文档建议予以丢弃
			if hasThought {
				startIdx := strings.Index(content, "<thought>")
				endIdx := strings.Index(content, "</thought>")
				newContent := content[:startIdx] + content[endIdx+len("</thought>"):]
				msg["content"] = strings.TrimSpace(newContent)
				changed = true
			}
			if _, exists := msg["reasoning_content"]; exists {
				delete(msg, "reasoning_content")
				changed = true
			}
		} else {
			// 情况 B: 当前轮次（可能是工具调用），必须还原/保留 reasoning_content
			if hasThought {
				startIdx := strings.Index(content, "<thought>")
				endIdx := strings.Index(content, "</thought>")
				thought := content[startIdx+len("<thought>") : endIdx]
				msg["reasoning_content"] = strings.TrimSpace(thought)

				newContent := content[:startIdx] + content[endIdx+len("</thought>"):]
				msg["content"] = strings.TrimSpace(newContent)
				changed = true
			}

			// API 要求 assistant 角色的 reasoning_content 字段必须存在（即使为空）
			if _, exists := msg["reasoning_content"]; !exists {
				msg["reasoning_content"] = ""
				changed = true
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

// processSSEResponse 处理 SSE 流式响应。
// 按照 DeepSeek 文档，正确处理 reasoning_content，并将其合并到 content 中显示以保证兼容性。
//
// 关键要点：
// 1. reasoning_content 一定先于 content 出现（流式输出特性）
// 2. 当 content 字段首次出现时，标志着推理阶段结束
// 3. reasoning_content 和 content 的 tokens 总数受 max_tokens 限制
// 4. 支持多步工具调用场景，每次工具调用可能都有独立的推理过程
func processSSEResponse(w http.ResponseWriter, body io.Reader) {
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(body)

	// 状态追踪（按流顺序处理，确保每段推理都成对闭合）
	isReasoning := false

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if isReasoning {
				injectClosingTag(w, flusher)
				isReasoning = false
				if debugMode {
					fmt.Print("\n--- 推理意外结束（流提前关闭）---\n")
				}
			}
			break
		}

		if bytes.HasPrefix(line, []byte("data: ")) {
			dataBytes := bytes.TrimPrefix(line, []byte("data: "))
			dataBytes = bytes.TrimSpace(dataBytes)

			if string(dataBytes) == "[DONE]" {
				// 如果流结束时还在推理状态（如 max_tokens 耗尽），强行闭合标签
				if isReasoning {
					injectClosingTag(w, flusher)
					isReasoning = false
					if debugMode {
						fmt.Print("\n--- 推理意外结束（tokens 耗尽）---\n")
					}
				}
				if debugMode {
					fmt.Println("\n[DONE]")
				}
			} else {
				var data map[string]any
				if err := json.Unmarshal(dataBytes, &data); err == nil {
					if choices, ok := data["choices"].([]any); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]any); ok {
							delta, hasDelta := choice["delta"].(map[string]any)
							if !hasDelta {
								delta = map[string]any{}
								choice["delta"] = delta
							}

							if hasDelta || choice["finish_reason"] != nil || isReasoning {
								// 检查是否存在 reasoning_content 和 content 字段
								// 注意：某些情况下字段可能为 nil，需要区分"不存在"和"存在但为空/null"
								rc, hasRC := delta["reasoning_content"]
								content, hasContent := delta["content"]
								hasNonNilContent := hasContent && content != nil

								// 提取字符串值（如果不是字符串类型则视为空）
								rcStr := ""
								if rcVal, ok := rc.(string); ok {
									rcStr = rcVal
								}
								contentStr := ""
								if contentVal, ok := content.(string); ok {
									contentStr = contentVal
								}

								// === 核心逻辑：根据字段出现情况判断推理阶段 ===
								newContent := ""

								// 情况 1：reasoning_content 存在且非空 => 进入/持续推理阶段
								if hasRC && rcStr != "" {
									if !isReasoning {
										if debugMode {
											fmt.Print("\n--- 推理开始 ---\n")
										}
										newContent += "<thought>\n"
										isReasoning = true
									}
									newContent += rcStr
									if debugMode {
										fmt.Print(rcStr)
									}
								}

								// 情况 2：推理阶段中遇到 content => 立即闭合后输出正文
								if isReasoning && hasNonNilContent {
									if debugMode {
										fmt.Print("\n--- 推理结束，正文开始 ---\n")
									}
									newContent += "\n</thought>\n\n"
									isReasoning = false
								}

								// 情况 3：正文输出（含无推理或刚闭合后的正文）
								if hasNonNilContent {
									newContent += contentStr
									if debugMode && contentStr != "" {
										fmt.Print(contentStr)
									}
								}

								// 情况 4：推理中直接结束（例如工具调用，无后续 content）
								if isReasoning && choice["finish_reason"] != nil {
									if debugMode {
										fmt.Print("\n--- 推理结束（无后续内容）---\n")
									}
									newContent += "\n</thought>\n\n"
									isReasoning = false
								}

								// 对于 finish_reason 收口，newContent 可能仅包含闭合标签，需确保写回。
								if hasRC || hasNonNilContent || newContent != "" {
									delta["content"] = newContent
								}

								// 清理原始 reasoning_content 字段（客户端不需要看到）
								delete(delta, "reasoning_content")

								// 修复 content: null 问题（某些客户端库不支持 null 字符串）
								if v, exists := delta["content"]; exists && v == nil {
									delta["content"] = ""
								}
							}

							// finish_reason 收口已在当前 chunk 内完成，避免额外补发导致客户端漏收。
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
			if isReasoning {
				injectClosingTag(w, flusher)
				isReasoning = false
				if debugMode {
					fmt.Print("\n--- 推理意外结束（无换行结尾）---\n")
				}
			}
			break
		}
	}
}

// injectClosingTag 在流意外结束时注入闭合标签
func injectClosingTag(w http.ResponseWriter, flusher http.Flusher) {
	msg := map[string]any{
		"choices": []any{
			map[string]any{
				"delta": map[string]any{
					"content": "\n</thought>\n\n",
				},
			},
		},
	}
	if b, err := json.Marshal(msg); err == nil {
		w.Write([]byte("data: "))
		w.Write(b)
		w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// getDebugRequestBody 简化并转义请求体用于调试打印
func getDebugRequestBody(body []byte) string {
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return string(body)
	}

	if messages, ok := data["messages"].([]any); ok {
		simplified := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			if msg, ok := m.(map[string]any); ok {
				newMsg := make(map[string]any)
				for _, field := range []string{"role", "content", "reasoning_content"} {
					if val, ok := msg[field]; ok {
						newMsg[field] = val
					}
				}
				simplified = append(simplified, newMsg)
			}
		}
		data["messages"] = simplified
	}

	// 仅保留 messages 和 model 字段以便调试，其余字段（如 tools, max_tokens 等）忽略
	cleanData := map[string]any{
		"messages": data["messages"],
	}
	if model, ok := data["model"]; ok {
		cleanData["model"] = model
	}

	// 使用 Marshal 而不使用 MarshalIndent，实现“转义”效果（所有内容在一行，字符串中的特殊字符会被转义）
	b, _ := json.Marshal(cleanData)
	return string(b)
}

// copyHeader 复制完整的 Header
func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// debugNonStreaming 调试非流式请求和响应，打印详细信息
func debugNonStreaming(r *http.Request, requestBody []byte, resp *http.Response, w http.ResponseWriter) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("📤 请求详情")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("请求体 (简化):")
	fmt.Println(getDebugRequestBody(requestBody))

	// 读取响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("读取响应体失败: %v\n", err)
		return
	}

	fmt.Println("\n📥 响应详情")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("状态码: %d\n", resp.StatusCode)

	// 解析并美化打印响应体
	var respData map[string]interface{}
	if err := json.Unmarshal(respBody, &respData); err == nil {
		if pretty, err := json.MarshalIndent(respData, "", "  "); err == nil {
			fmt.Println("响应体:")
			fmt.Println(string(pretty))
		}
	} else {
		fmt.Printf("响应体解析失败: %v\n", err)
		fmt.Printf("原始响应体: %s\n", string(respBody))
	}

	fmt.Println(strings.Repeat("=", 80) + "\n")

	// 将响应体写回给客户端
	w.Write(respBody)
}
