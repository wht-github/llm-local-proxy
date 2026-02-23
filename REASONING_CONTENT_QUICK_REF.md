# reasoning_content 处理快速参考

## 核心要点

### 1. 字段访问（OpenAI SDK）
```python
# ✅ 正确
if hasattr(obj, "reasoning_content"):
    reasoning = getattr(obj, "reasoning_content")

# ❌ 错误
reasoning = obj.reasoning_content  # 字段不直接存在
```

### 2. 流式输出顺序
```
reasoning_content 先出现 → content 后出现
                          ↑
                   推理结束的标志
```

### 3. Tokens 计数
```
reasoning_content tokens + content tokens ≤ max_tokens
```

## 本代理的自动转换

### 流式响应
```
DeepSeek API:
  reasoning_content: "思考中..."
  content: "答案是..."

↓ 自动转换为 ↓

客户端接收:
  content: "<thought>\n思考中...\n</thought>\n\n答案是..."
```

### 请求处理
```
当前轮次（最后 user 消息之后）:
  <thought>XX</thought> → reasoning_content: "XX" ✅ 保留

历史轮次（最后 user 消息之前）:
  <thought>XX</thought> → 删除 ❌ 节省带宽
```

## 多步工具调用
```
user → assistant(思考1) → tool → assistant(思考2) → ...
       ↑                          ↑
       保留                       保留
```

## 调试命令
```bash
./ds-proxy --debug  # 查看推理过程详情
```

## 边界情况

| 场景 | 处理方式 |
|------|---------|
| tokens 耗尽仍在推理中 | 强制闭合 `</thought>` |
| 只有推理无正文（工具调用） | 正常闭合 `</thought>` |
| content 为 null | 转换为空字符串 "" |
| 无推理直接输出内容 | 正常透传，不添加标签 |
