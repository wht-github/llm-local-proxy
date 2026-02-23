# reasoning_content 字段处理说明

## 概述

DeepSeek API 在流式输出中提供 `reasoning_content` 字段来展示模型的推理过程（思考内容）。本代理服务将该字段转换为 `<thought></thought>` 标签嵌入到 `content` 中，以保证与标准 OpenAI 客户端的兼容性。

## 字段访问方式

### OpenAI SDK
在 OpenAI Python SDK 中，`ChoiceDelta` 和 `ChatCompletionMessage` 类型**不直接提供** `reasoning_content` 字段：

```python
# ❌ 错误方式 - 无法直接访问
reasoning = choice.delta.reasoning_content  

# ✅ 正确方式 - 通过 hasattr 和 getattr
if hasattr(choice.delta, "reasoning_content"):
    reasoning = getattr(choice.delta, "reasoning_content")
```

### 其他框架或 HTTP 直连
如果使用其他框架或直接通过 HTTP 接口对接，可以直接获取与 `content` 同级的 `reasoning_content` 字段：

```json
{
  "choices": [{
    "delta": {
      "reasoning_content": "首先分析问题...",
      "content": "根据分析结果..."
    }
  }]
}
```

## 流式输出特性

### 1. 字段出现顺序
在流式输出（`stream=True`）中：
- **`reasoning_content` 一定先于 `content` 出现**
- 当 `content` 字段首次出现时，标志着推理阶段结束
- 可以通过判断 `content` 是否出现来识别思考内容是否结束

```
时间轴示例：
[chunk 1] reasoning_content: "首先..."
[chunk 2] reasoning_content: "然后..."
[chunk 3] reasoning_content: "最后..."
[chunk 4] content: "答案是..."        <- 推理结束标志
[chunk 5] content: "因为..."
```

### 2. Tokens 计数
- `reasoning_content` 和 `content` 的 tokens 总数受 `max_tokens` 参数控制
- 两者的 tokens 数之和应 ≤ `max_tokens`
- 如果达到 `max_tokens` 限制，可能导致推理内容或正文内容被截断

## 本代理的处理策略

### 流式响应处理（`processSSEResponse`）

代理服务自动将 `reasoning_content` 转换为 `<thought>` 标签：

```
输入（DeepSeek API）:
  {"delta": {"reasoning_content": "首先分析..."}}
  {"delta": {"reasoning_content": "然后考虑..."}}
  {"delta": {"content": "答案是..."}}

输出（客户端接收）:
  {"delta": {"content": "<thought>\n首先分析..."}}
  {"delta": {"content": "然后考虑..."}}
  {"delta": {"content": "\n</thought>\n\n答案是..."}}
```

#### 处理逻辑

1. **推理开始**：首次出现 `reasoning_content` 时，注入 `<thought>\n`
2. **持续推理**：后续 `reasoning_content` 直接追加到 `content`
3. **推理结束**：
   - 当 `content` 字段首次出现时，注入 `\n</thought>\n\n`
   - 或者当 `finish_reason` 出现但无 `content` 时（工具调用场景）
4. **异常处理**：如果流结束（`[DONE]`）时仍处于推理状态，强制闭合标签

### 请求体处理（`ensureReasoningField`）

在发送请求给 DeepSeek API 前，需要正确处理历史消息中的 `reasoning_content`：

#### 原则
- **当前轮次**：必须保留/还原 `reasoning_content`（防止 400 错误）
- **历史轮次**：丢弃 `reasoning_content`（节省带宽，官方建议）

#### 识别当前轮次
以**最后一个 user 消息**为界限：
- 该消息之后的 assistant 消息 = 当前轮次
- 该消息之前的 assistant 消息 = 历史轮次

```
示例对话历史：
[历史] user:    "什么是质数？"
[历史] assistant: <thought>...</thought> "质数是..."  -> 丢弃 reasoning_content
[历史] user:    "7是质数吗？"
[当前] assistant: <thought>...</thought> "是的..."    -> 保留 reasoning_content
[当前] tool:    {...}
[当前] assistant: <thought>...</thought> "综上..."    -> 保留 reasoning_content
[新轮] user:    "那9呢？"                            -> 新一轮开始
```

#### 转换规则

##### 情况 A：历史轮次（在最后 user 消息之前）
```
输入：{"role": "assistant", "content": "<thought>推理</thought> 答案"}
输出：{"role": "assistant", "content": "答案"}  // 丢弃 reasoning_content
```

##### 情况 B：当前轮次（在最后 user 消息之后）
```
输入：{"role": "assistant", "content": "<thought>推理</thought> 答案"}
输出：{
  "role": "assistant",
  "content": "答案",
  "reasoning_content": "推理"  // 还原 reasoning_content
}
```

**重要**：DeepSeek API 要求当前轮次的 assistant 消息必须包含 `reasoning_content` 字段（即使为空字符串）。

## 多步工具调用场景

在 function calling 或 tool use 场景中：

1. 每个 assistant 响应都可能包含独立的推理过程
2. 必须保留当前轮次的所有 `reasoning_content`
3. 只有开启新一轮对话（新 user 消息）后，才丢弃历史 `reasoning_content`

```
完整工具调用流程：
user: "北京天气如何？"
assistant (带推理): <thought>需要调用天气API</thought> [tool_call]
tool: {"temperature": 15}
assistant (带推理): <thought>根据返回结果...</thought> "北京今天15度"
                    ^---- 两次推理都需要保留
```

## 调试模式

启用 `--debug` 标志可查看详细的推理过程处理：

```bash
./ds-proxy --debug
```

输出示例：
```
--- 推理开始 ---
首先需要理解问题...
然后分析可能的方案...
--- 推理结束，正文开始 ---
根据以上分析，答案是...
```

## 注意事项

1. **字段判断**：使用 `hasattr` 和 `getattr` 而非直接属性访问（OpenAI SDK 限制）
2. **null 处理**：某些情况下 `content` 可能为 `null`，需转换为空字符串
3. **tokens 限制**：推理内容也占用 tokens，注意 `max_tokens` 设置
4. **客户端兼容性**：转换后的格式兼容所有标准 OpenAI 客户端库

## 相关文档

- [DeepSeek API 官方文档](https://platform.deepseek.com/api-docs/)
- [OpenAI API 规范](https://platform.openai.com/docs/api-reference)
