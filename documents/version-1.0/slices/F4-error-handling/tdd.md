# F4 · 错误处理 — 技术设计文档

**切片**：F4  
**状态**：待 Review

---

## 1. 错误类型定义

```go
// runner/errors.go
type RunError struct {
    Type    string // node_failed, llm_error, timeout, param_error, max_iterations
    NodeID  string
    Message string
    Detail  string // 完整 stack trace 或原始错误
}

func (e *RunError) Error() string { return e.Message }
```

---

## 2. 重试逻辑

```go
// runner/retry.go
type RetryConfig struct {
    MaxRetries int
    Interval   time.Duration
    OnlyNetwork bool
}

func WithRetry(ctx context.Context, config RetryConfig, fn func() (any, error)) (any, error) {
    var lastErr error
    for attempt := 0; attempt <= config.MaxRetries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(config.Interval):
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }
        result, err := fn()
        if err == nil { return result, nil }
        if config.OnlyNetwork && !isNetworkError(err) { return nil, err }
        lastErr = err
    }
    return nil, lastErr
}
```

---

## 3. 节点执行包装

```go
// runner/runner.go — runNode 函数
func (r *Runner) runNode(ctx context.Context, node FlowNode, input map[string]any, rc *RunContext) (any, error) {
    retryConfig := parseRetryConfig(node.Config)
    timeout := parseTimeout(node.Config) // 默认 30s

    nodeCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    output, err := WithRetry(nodeCtx, retryConfig, func() (any, error) {
        return r.runNodeOnce(nodeCtx, node, input, rc)
    })

    if err != nil {
        return nil, &RunError{
            Type:    classifyError(err),
            NodeID:  node.ID,
            Message: summarizeError(err),
            Detail:  err.Error(),
        }
    }
    return output, nil
}

func classifyError(err error) string {
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return "timeout"
    case isNetworkError(err):
        return "network_error"
    case isLLMError(err):
        return "llm_error"
    default:
        return "node_failed"
    }
}
```

---

## 4. 运行失败时的状态存储

```go
// runner/runner.go — execute() 失败分支
if runErr, ok := err.(*RunError); ok {
    r.runSvc.SetNodeResult(runID, node.ID, "failed", input, nil, runErr.Detail)
    r.runSvc.Finish(runID, "failed", runErr.Message)
    r.bridge.Emit(events.RunFailed, map[string]any{
        "runId":    runID,
        "nodeId":   node.ID,
        "errorType": runErr.Type,
        "message":  runErr.Message,
    })
}
```

---

## 5. 前端：错误面板

```tsx
// panels/NodeErrorPanel.tsx
export function NodeErrorPanel({ nodeResult }: { nodeResult: NodeResult }) {
    return (
        <div className="p-4 space-y-4">
            <div className="flex items-center gap-2 text-red-400">
                <span>❌</span>
                <span className="text-sm font-medium">节点执行失败</span>
            </div>

            <div>
                <p className="text-xs text-neutral-500 mb-1">错误类型</p>
                <p className="text-sm">{nodeResult.errorType}</p>
            </div>

            <div>
                <p className="text-xs text-neutral-500 mb-1">错误信息</p>
                <p className="text-sm text-red-300">{nodeResult.error}</p>
            </div>

            {nodeResult.input && (
                <div>
                    <p className="text-xs text-neutral-500 mb-1">执行时的输入</p>
                    <pre className="text-xs bg-neutral-900 p-2 rounded overflow-auto max-h-32">
                        {JSON.stringify(nodeResult.input, null, 2)}
                    </pre>
                </div>
            )}
        </div>
    )
}
```

---

## 6. 顶部横幅

```tsx
// WorkflowToolbar.tsx
{runFailed && (
    <div className="flex items-center gap-3 px-4 py-2 bg-red-950 border-b border-red-900 text-sm">
        <span className="text-red-400">❌ 工作流运行失败 · 节点"{failedNodeName}"出错</span>
        <button onClick={handleViewError} className="text-xs underline text-red-300">查看详情</button>
        <button onClick={handleOpenConversation} className="text-xs underline text-red-300">通过对话修复</button>
    </div>
)}
```

---

## 7. 状态注入扩展（配合 G4）

```go
// service/chat.go — BuildContextInjection 扩展
func (s *ChatService) BuildContextInjection(convID string) string {
    // ... 现有工作流状态注入 ...

    // 追加最近一次运行的错误信息
    lastRun, _ := s.runSvc.GetLatestRun(wf.ID)
    if lastRun != nil && lastRun.Status == "failed" {
        injection += "\n[最近一次运行失败]\n" +
            "错误节点：" + lastRun.Error + "\n" +
            "错误详情：" + lastRun.Error
    }
    return injection
}
```

---

## 8. 验收测试

```
1. 工具节点抛出 Python 异常 → RunError{Type:"node_failed"} → 节点变红
2. LLM 调用超时 → RunError{Type:"timeout"} → 节点变红
3. 配置 MaxRetries=2 → 失败时自动重试 2 次
4. NodeErrorPanel 显示错误类型、信息和输入参数
5. RunFailed 事件触发 → 顶部横幅出现
6. BuildContextInjection 包含最近失败信息 → AI 感知到错误
```
