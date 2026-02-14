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
	isSSE := resp.Header.Get("Content-Type") == "text/event-stream"
	if resp.StatusCode != http.StatusOK || !isSSE {
		// 非流式响应 - 记录完整请求和响应（如果启用了调试模式）
		if debugMode {
			debugNonStreaming(r, originalBody, resp, w)
		} else {
			io.Copy(w, resp.Body)
		}
		return
	}

	processSSEResponse(w, resp.Body)
}

// ensureReasoningField 确保 assistant 消息中包含 reasoning_content 字段，避免 400 错误
func ensureReasoningField(body []byte) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	messages, ok := data["messages"].([]interface{})
	if !ok {
		return body
	}

	changed := false
	for _, m := range messages {
		if msg, ok := m.(map[string]any); ok && msg["role"] == "assistant" {
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

// processSSEResponse 处理 SSE 流式响应，清空 reasoning 内容
func processSSEResponse(w http.ResponseWriter, body io.Reader) {
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(body)

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			break
		}

		if bytes.HasPrefix(line, []byte("data: ")) {
			dataBytes := bytes.TrimPrefix(line, []byte("data: "))
			dataBytes = bytes.TrimSpace(dataBytes)

			if string(dataBytes) != "[DONE]" {
				var data map[string]any
				if err := json.Unmarshal(dataBytes, &data); err == nil {
					clearReasoning(data)
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
	}
}

// clearReasoning 清空响应中的推理内容，使 UI 渲染更简洁
func clearReasoning(data map[string]interface{}) {
	choices, ok := data["choices"].([]interface{})
	if !ok {
		return
	}
	for _, c := range choices {
		choice, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		// 同时检查 delta (流式) 和 message (非流式)
		for _, key := range []string{"delta", "message"} {
			if m, ok := choice[key].(map[string]interface{}); ok {
				if _, exists := m["reasoning_content"]; exists {
					m["reasoning_content"] = ""
				}
			}
		}
	}
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

	// 解析并美化打印请求体
	var reqData map[string]interface{}
	if err := json.Unmarshal(requestBody, &reqData); err == nil {
		if pretty, err := json.MarshalIndent(reqData, "", "  "); err == nil {
			fmt.Println("请求体:")
			fmt.Println(string(pretty))
		}
	} else {
		fmt.Printf("请求体解析失败: %v\n", err)
		fmt.Printf("原始请求体: %s\n", string(requestBody))
	}

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
