# F1 · Flow 编译器 — 技术设计文档

**切片**：F1  
**状态**：待 Review

---

## 1. 目录结构

```
internal/
└── compiler/
    ├── compiler.go       # 主入口：Compile()
    ├── graph_builder.go  # 节点 → Eino 组件映射
    ├── validator.go      # 编译前静态验证
    └── resolver.go       # {{var}} 引用解析
```

---

## 2. 核心类型

```go
package compiler

import (
    "encoding/json"
    "forgify/internal/service"
    "github.com/cloudwego/eino/compose"
)

type CompileResult struct {
    Graph  *compose.Graph // nil if errors exist
    Errors []CompileError
}

type CompileError struct {
    NodeID  string `json:"nodeId"`
    Type    string `json:"type"`    // unknown_node_type, missing_target_node, ...
    Message string `json:"message"`
}

func Compile(def json.RawMessage, toolSvc *service.ToolService) CompileResult {
    var fd FlowDefinition
    if err := json.Unmarshal(def, &fd); err != nil {
        return CompileResult{Errors: []CompileError{{Type: "invalid_json", Message: err.Error()}}}
    }

    errs := validate(fd)
    if len(errs) > 0 {
        return CompileResult{Errors: errs}
    }

    graph, buildErrs := buildGraph(fd, toolSvc)
    if len(buildErrs) > 0 {
        return CompileResult{Errors: buildErrs}
    }

    return CompileResult{Graph: graph}
}
```

---

## 3. 静态验证

```go
// validator.go
func validate(fd FlowDefinition) []CompileError {
    var errs []CompileError
    nodeIDs := map[string]bool{}
    hasTrigger := false

    for _, n := range fd.Nodes {
        nodeIDs[n.ID] = true
        if isTrigger(n.Type) { hasTrigger = true }
        if !isKnownType(n.Type) {
            errs = append(errs, CompileError{NodeID: n.ID, Type: "unknown_node_type",
                Message: "未知节点类型: " + n.Type})
        }
    }

    if !hasTrigger {
        errs = append(errs, CompileError{Type: "no_trigger_node", Message: "工作流必须有至少一个触发节点"})
    }

    for _, e := range fd.Edges {
        if !nodeIDs[e.Source] {
            errs = append(errs, CompileError{Type: "missing_target_node",
                Message: "边的 source 节点不存在: " + e.Source})
        }
        if !nodeIDs[e.Target] {
            errs = append(errs, CompileError{Type: "missing_target_node",
                Message: "边的 target 节点不存在: " + e.Target})
        }
    }

    // 变量引用检查
    for _, n := range fd.Nodes {
        refs := extractRefs(n.Config)
        for _, ref := range refs {
            if !nodeIDs[ref.NodeID] {
                errs = append(errs, CompileError{NodeID: n.ID, Type: "invalid_variable_ref",
                    Message: "引用了不存在的节点: " + ref.NodeID})
            }
        }
    }

    // 环路检测（loop 节点的 loop_body 子图豁免）
    if cycle := detectCycle(fd); cycle != "" {
        errs = append(errs, CompileError{Type: "cycle_detected",
            Message: "检测到非法环路，涉及节点: " + cycle})
    }

    return errs
}
```

---

## 4. Graph 构建

```go
// graph_builder.go
func buildGraph(fd FlowDefinition, toolSvc *service.ToolService) (*compose.Graph, []CompileError) {
    g := compose.NewGraph[map[string]any, map[string]any]()
    var errs []CompileError

    for _, n := range fd.Nodes {
        component, err := nodeToComponent(n, toolSvc)
        if err != nil {
            errs = append(errs, CompileError{NodeID: n.ID, Type: "build_error", Message: err.Error()})
            continue
        }
        g.AddNode(n.ID, component)
    }

    for _, e := range fd.Edges {
        opts := []compose.AddEdgeOpt{}
        if e.SourceHandle != "" {
            opts = append(opts, compose.WithCondition(e.SourceHandle))
        }
        g.AddEdge(e.Source, e.Target, opts...)
    }

    return g, errs
}

func nodeToComponent(n FlowNode, toolSvc *service.ToolService) (compose.Node, error) {
    switch n.Type {
    case "trigger_manual", "trigger_schedule", "trigger_webhook", "trigger_file":
        return NewTriggerComponent(n.Config), nil
    case "tool":
        return NewToolComponent(n.Config, toolSvc)
    case "condition":
        return NewConditionComponent(n.Config)
    case "variable":
        return NewVariableComponent(n.Config)
    case "approval":
        return NewApprovalComponent(n.Config)
    case "loop":
        return NewLoopComponent(n.Config)
    case "parallel":
        return NewParallelComponent(n.Config)
    case "subworkflow":
        return NewSubworkflowComponent(n.Config, toolSvc)
    case "llm":
        return NewLLMComponent(n.Config)
    case "agent":
        return NewAgentComponent(n.Config, toolSvc)
    }
    return nil, fmt.Errorf("unknown type: %s", n.Type)
}
```

---

## 5. 变量引用解析器

```go
// resolver.go
var refRegex = regexp.MustCompile(`\{\{(\w+)\.result(?:\.[\w.]+)?\}\}`)

type VarRef struct {
    NodeID string
    Path   string // "result" or "result.field"
}

func extractRefs(config map[string]any) []VarRef {
    var refs []VarRef
    for _, v := range config {
        s, ok := v.(string)
        if !ok { continue }
        for _, m := range refRegex.FindAllStringSubmatch(s, -1) {
            refs = append(refs, VarRef{NodeID: m[1]})
        }
    }
    return refs
}

// ResolveConfig 在运行时将 {{node_id.result.field}} 替换为实际值
func ResolveConfig(config map[string]any, ctx map[string]any) map[string]any {
    resolved := map[string]any{}
    for k, v := range config {
        s, ok := v.(string)
        if !ok {
            resolved[k] = v
            continue
        }
        resolved[k] = refRegex.ReplaceAllStringFunc(s, func(match string) string {
            m := refRegex.FindStringSubmatch(match)
            nodeID := m[1]
            if nodeCtx, ok := ctx[nodeID].(map[string]any); ok {
                if val, ok := nodeCtx["result"]; ok {
                    return fmt.Sprintf("%v", val)
                }
            }
            return match // 未找到时保留原始占位符
        })
    }
    return resolved
}
```

---

## 6. Wails Binding：编译校验

```go
// app.go
func (a *App) ValidateWorkflow(id string) ([]compiler.CompileError, error) {
    wf, err := a.workflowSvc.Get(id)
    if err != nil { return nil, err }
    result := compiler.Compile(wf.Definition, a.toolSvc)
    return result.Errors, nil
}
```

前端在工具栏"运行"按钮渲染时调用，展示错误并置灰按钮：

```typescript
// WorkflowToolbar.tsx
const [errors, setErrors] = useState<CompileError[]>([])

useEffect(() => {
    ValidateWorkflow(workflowId).then(setErrors)
}, [workflowDefinition]) // definition 变化时重新校验

const hasErrors = errors.length > 0
```

---

## 7. 验收测试

```
1. 合法 FlowDefinition → Compile() 返回 CompileResult{Graph: <非nil>, Errors: []}
2. 边引用不存在节点 → Errors 含 missing_target_node
3. {{node_99.result}} 引用不存在节点 → Errors 含 invalid_variable_ref
4. 无触发节点 → Errors 含 no_trigger_node
5. loop 节点的 loop_body 子图不触发 cycle_detected
6. ValidateWorkflow binding 可从前端调用，返回错误列表
7. 工具栏：有错误时"运行"按钮置灰；无错误时可点击
```
