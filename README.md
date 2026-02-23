# DeepSeek API Proxy

一个用于 DeepSeek API 的 Go 语言代理服务器，支持流式响应和推理内容过滤。

## 功能特性

- 🔄 **透明代理**：转发所有 DeepSeek API 请求，保留原始路由路径
- 🚀 **流式响应**：完整支持 Server-Sent Events (SSE) 流式传输
- 🧠 **推理内容处理**：智能转换 `reasoning_content` 为 `<thought></thought>` 标签，完美兼容 OpenAI 客户端
- 🛠️ **多步工具调用**：正确处理工具调用场景中的推理内容，避免 400 错误
- ⚙️ **配置驱动**：通过 JSON 配置文件管理 API 密钥和端口
- 🐞 **调试模式**：详细记录请求响应和推理过程详情
- 🔧 **跨平台**：支持 Windows、Linux、macOS 多个平台

## 快速开始

### 1. 安装

**方式一：下载预编译二进制**
从 [Releases](https://github.com/wht-github/llm-local-proxy/releases) 下载对应平台的二进制文件。

**方式二：从源码构建**
```bash
# 克隆仓库
git clone https://github.com/wht-github/llm-local-proxy.git
cd llm-local-proxy

# 构建
just build
# 或构建所有平台
just build-all
```

### 2. 配置

复制配置文件模板：
```bash
cp config.example.json config.json
```

编辑 `config.json`：
```json
{
  "api_key": "your-actual-deepseek-api-key",
  "proxy_port": "12000",
  "target_base_url": "https://api.deepseek.com"
}
```

### 3. 运行

```bash
# 直接运行
./ds-proxy

# 或使用调试模式
./ds-proxy -debug

# 或指定配置文件
./ds-proxy -config /path/to/config.json
```

### 4. 使用

代理服务器将在 `http://127.0.0.1:12000` 启动。

在 VS Code Copilot 或其他 AI 工具中，将 Base URL 设置为：
```
http://127.0.0.1:12000
```

## 构建说明

### 使用 justfile
```bash
# 查看所有可用任务
just

# 构建当前平台
just build

# 构建所有平台
just build-all

# 清理构建产物
just clean
```

### 手动构建
```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o ds-proxy.exe main.go

# Linux
GOOS=linux GOARCH=amd64 go build -o ds-proxy main.go

# macOS
GOOS=darwin GOARCH=arm64 go build -o ds-proxy main.go
```

## 发布管理

### 创建新版本

1. **更新版本号**（可选，在代码中标记版本）
2. **构建发布包**：
   ```bash
   just release
   ```
3. **创建 GitHub Release**：
   - 在 GitHub 仓库页面点击 "Draft a new release"
   - 输入版本号（如 v1.0.0）
   - 添加发布说明
   - 上传 `dist/` 目录下的所有打包文件

### 发布文件说明

`just release` 会在 `dist/` 目录生成以下二进制文件：
- `ds-proxy-windows-amd64.exe` - Windows 64位
- `ds-proxy-windows-386.exe` - Windows 32位  
- `ds-proxy-linux-amd64` - Linux 64位
- `ds-proxy-linux-arm64` - Linux ARM64（树莓派等）
- `ds-proxy-macos-amd64` - Intel Mac
- `ds-proxy-macos-arm64` - Apple Silicon Mac

## 配置说明

### 配置文件参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `api_key` | string | 必填 | DeepSeek API 密钥 |
| `proxy_port` | string | `12000` | 代理服务器监听端口 |
| `target_base_url` | string | `https://api.deepseek.com` | DeepSeek API 基础 URL |

### 命令行参数

| 参数 | 说明 |
|------|------|
| `-config` | 配置文件路径（默认: config.json） |
| `-debug` | 启用调试模式，打印请求/响应详情 |

## 工作原理

### 请求处理流程
1. **接收请求**：接收来自客户端的 API 请求
2. **修复请求体**：
   - 当前轮次：还原 `<thought>` 为 `reasoning_content`，避免 400 错误
   - 历史轮次：丢弃 `reasoning_content`，节省带宽
3. **转发请求**：添加 API 密钥后转发到 DeepSeek
4. **处理响应**（流式）：
   - 检测 `reasoning_content` 出现，注入 `<thought>` 开始标签
   - 检测 `content` 出现（推理结束标志），注入 `</thought>` 闭合标签
   - 处理边界情况（tokens 耗尽、工具调用等）

### reasoning_content 处理详解

本代理实现了完整的 `reasoning_content` 字段处理逻辑：

#### 流式输出转换
```
DeepSeek API 流式响应：
  {"delta": {"reasoning_content": "首先分析..."}}
  {"delta": {"reasoning_content": "然后考虑..."}}
  {"delta": {"content": "答案是..."}}

↓ 自动转换为 ↓

客户端接收：
  {"delta": {"content": "<thought>\n首先分析..."}}
  {"delta": {"content": "然后考虑..."}}
  {"delta": {"content": "\n</thought>\n\n答案是..."}}
```

#### 关键特性
- **顺序识别**：`reasoning_content` 始终先于 `content` 出现
- **智能切换**：通过 `content` 首次出现判断推理结束
- **多步工具调用**：保留当前轮次所有推理内容
- **边界处理**：tokens 耗尽时自动闭合标签

详细文档请参阅：
- [reasoning_content 处理说明](REASONING_CONTENT.md)
- [快速参考指南](REASONING_CONTENT_QUICK_REF.md)

### 解决的关键问题
- **DeepSeek API 兼容性**：正确处理 `reasoning_content` 字段，避免 400 错误
- **OpenAI 客户端兼容**：转换为标准格式，无需修改客户端代码
- **多步工具调用**：智能识别当前轮次，保留必要的推理内容
- **UI 优化**：推理内容以标签形式嵌入，支持客户端自定义渲染

## 开发

### 项目结构
```
ds-proxy/
├── main.go          # 主程序
├── config.json      # 配置文件（不提交）
├── config.example.json  # 配置模板
├── go.mod          # Go 模块定义
├── justfile        # 任务运行器脚本
├── README.md       # 说明文档
├── VERSION.md      # 版本历史
├── bin/            # 本地构建产物
└── dist/           # 发布构建产物
```

### 依赖
- Go 1.16+
- 标准库：`net/http`, `encoding/json`, `bufio`, `flag`, `os`, `time`

## 许可证

MIT License

## 贡献

1. Fork 仓库
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 支持

- 问题反馈：[GitHub Issues](https://github.com/你的用户名/ds-proxy/issues)
- 功能建议：[GitHub Discussions](https://github.com/你的用户名/ds-proxy/discussions)