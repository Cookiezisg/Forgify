# G4 · 错误修复对话 — 技术设计文档

**切片**：G4  
**状态**：待 Review

---

## 1. 错误上下文注入

在 F4 的基础上，扩展 `BuildContextInjection`：

```go
// service/chat.go
func (s *ChatService) BuildContextInjection(convID string) string {
    conv, _ := s.convSvc.Get(convID)
    if conv.AssetID == "" || conv.AssetType != "workflow" { return "" }

    wf, _ := s.workflowSvc.Get(conv.AssetID)
    if wf == nil { return "" }

    // 基础工作流状态
    injection := buildWorkflowStateInjection(wf)

    // 追加最近失败的运行信息
    lastRun, _ := s.runSvc.GetLatestFailed(wf.ID)
    if lastRun != nil {
        nodeResult, _ := s.runSvc.GetFailedNodeResult(lastRun.ID)
        injection += "\n\n[最近一次运行失败]\n" +
            "运行时间：" + lastRun.StartedAt.Format("2006-01-02 15:04") + "\n" +
            "失败节点：" + nodeResult.NodeID + "\n" +
            "错误类型：" + nodeResult.ErrorType + "\n" +
            "错误信息：" + nodeResult.Error + "\n" +
            "该节点输入：" + string(nodeResult.Input)
    }

    return injection
}
```

---

## 2. RunService 扩展

```go
// service/run.go
func (s *RunService) GetLatestFailed(workflowID string) (*Run, error) {
    r := &Run{}
    err := storage.DB().QueryRow(`
        SELECT id, status, started_at, error FROM runs
        WHERE workflow_id = ? AND status = 'failed'
        ORDER BY started_at DESC LIMIT 1`, workflowID).
        Scan(&r.ID, &r.Status, &r.StartedAt, &r.Error)
    if err == sql.ErrNoRows { return nil, nil }
    return r, err
}

func (s *RunService) GetFailedNodeResult(runID string) (*NodeResult, error) {
    nr := &NodeResult{}
    err := storage.DB().QueryRow(`
        SELECT node_id, status, input, output, error FROM run_node_results
        WHERE run_id = ? AND status = 'failed' LIMIT 1`, runID).
        Scan(&nr.NodeID, &nr.Status, &nr.Input, &nr.Output, &nr.Error)
    return nr, err
}
```

---

## 3. 错误修复入口：打开对话并触发 AI 诊断

```go
// backend/internal/server/routes.go handler
// openRepairConversation 打开工作流绑定对话，并发送一条"修复引导"消息触发 AI 诊断
func (s *Server) openRepairConversation(workflowID string) (string, error) {
    // 查找或创建绑定到该工作流的对话
    convs, _ := a.convSvc.ListByAsset(workflowID, "workflow")
    var convID string
    if len(convs) > 0 {
        convID = convs[0].ID
    } else {
        conv, _ := a.convSvc.Create()
        a.convSvc.Bind(conv.ID, workflowID, "workflow")
        convID = conv.ID
    }

    // 发送系统触发消息，引导 AI 主动诊断
    a.chatSvc.SendMessage(convID, "我的工作流运行失败了，帮我诊断一下", "user")

    // 通知前端切换到该对话
    a.bridge.Emit(events.OpenConversation, map[string]any{"conversationId": convID})
    return convID, nil
}
```

---

## 4. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("POST /api/workflows/{id}/repair-conversation", s.openRepairConversation)
```

---

## 5. 前端：Canvas 横幅"通过对话修复"按钮

```tsx
// WorkflowToolbar.tsx
<button onClick={async () => {
    await fetch(`http://127.0.0.1:${port}/api/workflows/${workflowId}/repair-conversation`, { method: 'POST' })
    // OpenConversation 事件会让前端切换到对话侧
}} className="text-xs underline text-red-300">
    通过对话修复
</button>
```

---

## 6. 验收测试

```
1. GetLatestFailed() 返回最近一次失败运行
2. GetFailedNodeResult() 返回失败节点的输入和错误信息
3. BuildContextInjection 包含完整失败信息
4. OpenRepairConversation() → 找到或创建对话 → 发送诊断消息 → 前端切换
5. AI 回复包含对失败原因的具体分析（集成测试）
6. AI 输出 flow-definition → 工作流自动更新（E2 流程验证）
```
