# E1 · 工作流画布 — 技术设计文档

**切片**：E1  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 画布引擎 | React Flow | 成熟、文档完善、支持自定义节点 |
| 节点位置存储 | FlowDefinition 的 node.position 字段 | 统一格式，Go 层不需要知道 UI 坐标 |
| 画布状态同步 | canvas.updated 事件（A3）| 对话侧订阅，AI 感知工作流变化 |
| 节点配置面板 | 画布底部滑出的固定 Panel | 不遮挡节点视图 |

---

## 2. 目录结构

```
frontend/src/
└── components/workflow/
    ├── WorkflowCanvas.tsx      # React Flow 容器
    ├── WorkflowToolbar.tsx     # 顶部工具栏
    ├── nodes/
    │   ├── BaseNode.tsx        # 所有节点的公共样式
    │   ├── TriggerNode.tsx
    │   ├── ToolNode.tsx
    │   ├── ConditionNode.tsx
    │   └── ...（其他节点类型）
    ├── edges/
    │   └── DefaultEdge.tsx     # 自定义连线样式
    └── panels/
        └── NodeConfigPanel.tsx  # 底部配置面板

internal/
└── service/
    └── workflow.go
internal/storage/migrations/
    └── 007_workflows.sql
```

---

## 3. 数据库迁移

### `007_workflows.sql`

```sql
CREATE TABLE IF NOT EXISTS workflows (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '新工作流',
    definition  JSON NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    status      TEXT NOT NULL DEFAULT 'draft'
                CHECK(status IN ('draft','ready','deployed','paused','archived')),
    created_at  DATETIME DEFAULT (datetime('now')),
    updated_at  DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_workflows_status ON workflows(status);
```

---

## 4. FlowDefinition 类型（TypeScript）

```typescript
// types/flow.ts
export interface FlowDefinition {
    id: string
    name: string
    nodes: FlowNode[]
    edges: FlowEdge[]
}

export interface FlowNode {
    id: string
    type: NodeType
    position: { x: number; y: number }
    config: Record<string, any>
    disabled?: boolean
}

export type NodeType =
    | 'trigger_manual' | 'trigger_schedule' | 'trigger_webhook' | 'trigger_file'
    | 'tool' | 'condition' | 'variable' | 'approval'
    | 'loop' | 'parallel' | 'subworkflow'
    | 'llm' | 'agent'

export interface FlowEdge {
    id: string
    source: string
    target: string
    sourceHandle?: string  // 条件节点的分支标识
}
```

---

## 5. Go 层

### `internal/service/workflow.go`

```go
package service

import (
    "encoding/json"
    "forgify/internal/storage"
    "time"

    "github.com/google/uuid"
)

type Workflow struct {
    ID         string          `json:"id"`
    Name       string          `json:"name"`
    Definition json.RawMessage `json:"definition"`
    Status     string          `json:"status"`
    CreatedAt  time.Time       `json:"createdAt"`
    UpdatedAt  time.Time       `json:"updatedAt"`
}

type WorkflowService struct{}

func (s *WorkflowService) Create(name string) (*Workflow, error) {
    id := uuid.NewString()
    _, err := storage.DB().Exec(
        `INSERT INTO workflows (id, name) VALUES (?, ?)`, id, name)
    if err != nil { return nil, err }
    return s.Get(id)
}

func (s *WorkflowService) Get(id string) (*Workflow, error) {
    w := &Workflow{}
    var def string
    err := storage.DB().QueryRow(
        `SELECT id, name, definition, status, created_at, updated_at FROM workflows WHERE id=?`, id).
        Scan(&w.ID, &w.Name, &def, &w.Status, &w.CreatedAt, &w.UpdatedAt)
    w.Definition = json.RawMessage(def)
    return w, err
}

func (s *WorkflowService) List() ([]*Workflow, error) {
    rows, err := storage.DB().Query(
        `SELECT id, name, definition, status, created_at, updated_at
         FROM workflows WHERE status != 'archived' ORDER BY updated_at DESC`)
    if err != nil { return nil, err }
    defer rows.Close()
    var wfs []*Workflow
    for rows.Next() {
        w := &Workflow{}
        var def string
        rows.Scan(&w.ID, &w.Name, &def, &w.Status, &w.CreatedAt, &w.UpdatedAt)
        w.Definition = json.RawMessage(def)
        wfs = append(wfs, w)
    }
    return wfs, nil
}

func (s *WorkflowService) UpdateDefinition(id string, def json.RawMessage) error {
    _, err := storage.DB().Exec(
        `UPDATE workflows SET definition=?, updated_at=datetime('now') WHERE id=?`,
        string(def), id)
    return err
}

func (s *WorkflowService) Rename(id, name string) error {
    _, err := storage.DB().Exec(
        `UPDATE workflows SET name=?, updated_at=datetime('now') WHERE id=?`, name, id)
    return err
}
```

---

## 6. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/workflows", s.listWorkflows)
mux.HandleFunc("GET /api/workflows/{id}", s.getWorkflow)
mux.HandleFunc("PUT /api/workflows/{id}/definition", s.updateWorkflowDefinition)
mux.HandleFunc("PATCH /api/workflows/{id}/rename", s.renameWorkflow)
```

---

## 7. 前端组件

### `WorkflowCanvas.tsx`

```tsx
import ReactFlow, {
    Background, Controls, MiniMap,
    useNodesState, useEdgesState, addEdge,
    Node, Edge, Connection
} from 'reactflow'
import 'reactflow/dist/style.css'

import { TriggerNode } from './nodes/TriggerNode'
import { ToolNode } from './nodes/ToolNode'
// ... 其他节点类型

const NODE_TYPES = {
    trigger_manual: TriggerNode,
    trigger_schedule: TriggerNode,
    tool: ToolNode,
    // ...
}

export function WorkflowCanvas({ workflowId }: { workflowId: string }) {
    const [nodes, setNodes, onNodesChange] = useNodesState([])
    const [edges, setEdges, onEdgesChange] = useEdgesState([])
    const [selectedNode, setSelectedNode] = useState<Node | null>(null)

    // 加载工作流
    useEffect(() => {
        GetWorkflow(workflowId).then(wf => {
            const def = JSON.parse(wf.definition)
            setNodes(def.nodes.map(toRFNode))
            setEdges(def.edges.map(toRFEdge))
        })
    }, [workflowId])

    // 监听画布更新事件（AI 修改工作流后刷新）
    useEffect(() => {
        return onEvent(EV.CanvasUpdated, ({ workflowId: id }) => {
            if (id !== workflowId) return
            GetWorkflow(workflowId).then(wf => {
                const def = JSON.parse(wf.definition)
                setNodes(def.nodes.map(toRFNode))
                setEdges(def.edges.map(toRFEdge))
            })
        })
    }, [workflowId])

    // 节点位置变化 → 静默保存
    const onNodeDragStop = useCallback((_: any, node: Node) => {
        setNodes(ns => {
            const updated = ns.map(n => n.id === node.id ? node : n)
            const def = buildDefinition(workflowId, updated, edges)
            UpdateWorkflowDefinition(workflowId, JSON.stringify(def))
            return updated
        })
    }, [edges, workflowId])

    // 连线变化 → 静默保存
    const onConnect = useCallback((conn: Connection) => {
        setEdges(es => {
            const updated = addEdge(conn, es)
            const def = buildDefinition(workflowId, nodes, updated)
            UpdateWorkflowDefinition(workflowId, JSON.stringify(def))
            return updated
        })
    }, [nodes, workflowId])

    return (
        <div className="h-full w-full">
            <ReactFlow
                nodes={nodes} edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onNodeDragStop={onNodeDragStop}
                onConnect={onConnect}
                onNodeClick={(_, node) => setSelectedNode(node)}
                nodeTypes={NODE_TYPES}
                fitView
            >
                <Background color="#333" gap={16} />
                <Controls />
                <MiniMap nodeColor={nodeColor} />
            </ReactFlow>

            {selectedNode && (
                <NodeConfigPanel
                    node={selectedNode}
                    onClose={() => setSelectedNode(null)}
                    onChange={updated => {
                        setNodes(ns => ns.map(n => n.id === updated.id ? updated : n))
                        const def = buildDefinition(workflowId, nodes, edges)
                        UpdateWorkflowDefinition(workflowId, JSON.stringify(def))
                    }}
                />
            )}
        </div>
    )
}

function nodeColor(node: Node) {
    const status = node.data?.runStatus
    if (status === 'success') return '#22c55e'
    if (status === 'error')   return '#ef4444'
    if (status === 'running') return '#3b82f6'
    if (node.data?.missing)   return '#eab308'
    return '#64748b'
}
```

### `nodes/BaseNode.tsx`

```tsx
export function BaseNode({ data, selected, children }:
    { data: any; selected: boolean; children: React.ReactNode }) {
    const statusBorder = {
        success: 'border-green-500',
        error:   'border-red-500',
        running: 'border-blue-500 animate-pulse',
        missing: 'border-yellow-500',
    }[data.runStatus as string] ?? (selected ? 'border-blue-400' : 'border-neutral-600')

    return (
        <div className={`bg-neutral-800 border-2 ${statusBorder} rounded-xl px-4 py-3 min-w-[160px] shadow-lg`}>
            {children}
        </div>
    )
}
```

---

## 8. 验收测试

```
1. 有节点的工作流在画布上正确渲染（节点 + 连线）
2. 拖拽节点 → 位置保存到 DB → 刷新后位置正确
3. 连接两节点 → 边保存 → 刷新后仍在
4. 删除节点/边 → 实时从画布消失 → 保存到 DB
5. 小地图正常显示，点击跳转正确
6. 缩放/适应画布按钮正常
7. AI 更新工作流后，canvas.updated 事件触发，画布刷新
```
