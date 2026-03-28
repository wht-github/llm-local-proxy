# LLM Local Proxy

一个支持多 Provider 的本地 LLM API 代理，暴露 OpenAI 兼容接口，自动将各 Provider 的思维链（`reasoning_content`）转换为 `<thought>` 标签嵌入 `content` 返回。

## 支持的 Provider

| Provider | Type | 思维链字段 | 特殊处理 |
|----------|------|-----------|---------|
| DeepSeek | `deepseek` | `reasoning_content` | 强制要求字段存在，历史轮次清理 |
| Kimi (Moonshot) | `kimi` | `reasoning_content` | 保留全部历史推理上下文 |
| 智谱 GLM | `zhipu` | `reasoning_content` | 历史轮次清理 |
| 透传 | `passthrough` | - | 不做任何变换 |

## 快速开始

### 1. 配置

```bash
cp config.example.json config.json
```

编辑 `config.json`，填入你的 API Key：

```json
{
  "listen": ":12000",
  "debug": false,
  "providers": [
    {
      "name": "deepseek",
      "type": "deepseek",
      "base_url": "https://api.deepseek.com/v1",
      "api_key": "sk-your-key",
      "models": ["deepseek-chat", "deepseek-reasoner"]
    },
    {
      "name": "kimi",
      "type": "kimi",
      "base_url": "https://api.moonshot.cn/v1",
      "api_key": "sk-your-key",
      "models": ["kimi-k2.5", "kimi-k2-thinking"]
    },
    {
      "name": "zhipu",
      "type": "zhipu",
      "base_url": "https://open.bigmodel.cn/api/paas/v4",
      "api_key": "your-key",
      "models": ["glm-5", "glm-5-turbo", "glm-4.7"]
    }
  ]
}
```

### 2. 运行

```bash
# 直接运行
go run .

# 调试模式
go run . -debug

# 指定配置文件
go run . -config /path/to/config.json
```

### 3. 使用

代理在 `http://127.0.0.1:12000` 启动。在 VS Code Copilot 或其他 OpenAI 兼容客户端中设置 Base URL：

```
http://127.0.0.1:12000/v1
```

代理根据请求中的 `model` 字段自动路由到对应 Provider。

## 路由规则

- 请求体中的 `model` 字段会匹配 Provider 配置中的 `models` 列表
- `models` 中可使用 `"*"` 作为通配符，匹配所有未被其他 Provider 捕获的模型
- 无法匹配任何 Provider 时，返回 502 错误
- 非 chat 请求（如 `/v1/models`）若无法解析 model 字段也会返回 502

## 路径处理

所有请求固定转发到 `base_url + /chat/completions`。`base_url` 须包含版本路径段：

- DeepSeek: `https://api.deepseek.com/v1`
- Kimi: `https://api.moonshot.cn/v1`
- 智谱: `https://open.bigmodel.cn/api/paas/v4`

## 构建

```bash
# 当前平台
just build

# 所有平台
just build-all
```

## 项目结构

```
├── main.go                  # 入口
├── config/
│   └── config.go            # 配置类型与加载
├── proxy/
│   └── handler.go           # HTTP 处理、SSE 流处理
├── provider/
│   ├── provider.go          # Provider 接口 + 注册表
│   ├── deepseek.go          # DeepSeek
│   ├── kimi.go              # Kimi (Moonshot)
│   ├── zhipu.go             # 智谱 GLM
│   └── passthrough.go       # 透传
└── transform/
    └── reasoning.go         # 共享：reasoning_content <-> <thought> 转换
```
