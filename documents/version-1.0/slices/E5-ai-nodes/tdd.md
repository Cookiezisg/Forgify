# E5 · AI 节点类型 — 技术设计文档

**切片**：E5  
**状态**：待 Review

---

## 1. 节点组件

```
frontend/src/components/workflow/nodes/
├── LLMNode.tsx
└── AgentNode.tsx

frontend/src/components/workflow/panels/
├── LLMConfigPanel.tsx
└── AgentConfigPanel.tsx
```

---

## 2. LLM 节点

### `LLMNode.tsx`

```tsx
export function LLMNode({ data, selected }: NodeProps) {
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div>
                <p className="text-sm font-medium">🤖 LLM</p>
                <p className="text-xs text-neutral-400 max-w-[140px] truncate">
                    {data.config?.prompt?.substring(0, 40) || '未配置 Prompt'}
                </p>
            </div>
            <Handle type="source" position={Position.Right} />
        </BaseNode>
    )
}
```

### `LLMConfigPanel.tsx`

```tsx
export function LLMConfigPanel({ node, onChange }: { node: Node; onChange: (n: Node) => void }) {
    const config = node.data.config ?? {}
    const update = (key: string, val: any) =>
        onChange({ ...node, data: { ...node.data, config: { ...config, [key]: val } } })

    return (
        <div className="p-4 space-y-3">
            <h3 className="text-sm font-semibold">LLM 节点配置</h3>
            <div>
                <label className="text-xs text-neutral-400 mb-1 block">Prompt（支持 {`{{变量}}`} 语法）</label>
                <textarea value={config.prompt ?? ''} onChange={e => update('prompt', e.target.value)}
                    rows={4} className="w-full px-3 py-2 bg-neutral-800 rounded text-sm resize-none font-mono" />
            </div>
            <div>
                <label className="text-xs text-neutral-400 mb-1 block">输出格式</label>
                <select value={config.output_format ?? 'text'} onChange={e => update('output_format', e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-800 rounded text-sm">
                    <option value="text">纯文本</option>
                    <option value="json">JSON</option>
                </select>
            </div>
            {config.output_format === 'json' && (
                <div>
                    <label className="text-xs text-neutral-400 mb-1 block">JSON Schema</label>
                    <textarea value={config.json_schema ?? ''} onChange={e => update('json_schema', e.target.value)}
                        rows={3} placeholder='{"key": "string"}' className="w-full px-3 py-2 bg-neutral-800 rounded text-sm font-mono resize-none" />
                </div>
            )}
        </div>
    )
}
```

---

## 3. Agent 节点

### `AgentNode.tsx`

```tsx
export function AgentNode({ data, selected }: NodeProps) {
    const running = data.runStatus === 'running'
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div>
                <p className="text-sm font-medium flex items-center gap-1">
                    🧠 Agent {running && <span className="text-xs text-blue-400 animate-pulse">思考中...</span>}
                </p>
                <p className="text-xs text-neutral-400 max-w-[140px] truncate">
                    {data.config?.goal || '未配置目标'}
                </p>
                {data.config?.tools?.length > 0 && (
                    <p className="text-xs text-neutral-500 mt-1">{data.config.tools.length} 个工具</p>
                )}
            </div>
            <Handle type="source" position={Position.Right} />
        </BaseNode>
    )
}
```

### `AgentConfigPanel.tsx`

```tsx
export function AgentConfigPanel({ node, onChange }: { node: Node; onChange: (n: Node) => void }) {
    const config = node.data.config ?? {}
    const [tools, setTools] = useState<Tool[]>([])

    useEffect(() => { ListTools('', '').then(setTools) }, [])

    const update = (key: string, val: any) =>
        onChange({ ...node, data: { ...node.data, config: { ...config, [key]: val } } })

    const toggleTool = (name: string) => {
        const current: string[] = config.tools ?? []
        update('tools', current.includes(name) ? current.filter(t => t !== name) : [...current, name])
    }

    return (
        <div className="p-4 space-y-3">
            <h3 className="text-sm font-semibold">Agent 节点配置</h3>
            <div>
                <label className="text-xs text-neutral-400 mb-1 block">目标描述</label>
                <textarea value={config.goal ?? ''} onChange={e => update('goal', e.target.value)}
                    rows={3} className="w-full px-3 py-2 bg-neutral-800 rounded text-sm resize-none" />
            </div>
            <div>
                <label className="text-xs text-neutral-400 mb-2 block">可用工具</label>
                <div className="grid grid-cols-2 gap-1">
                    {tools.map(t => (
                        <label key={t.id} className="flex items-center gap-2 text-xs cursor-pointer p-1 hover:bg-neutral-800 rounded">
                            <input type="checkbox"
                                checked={(config.tools ?? []).includes(t.name)}
                                onChange={() => toggleTool(t.name)} />
                            {t.displayName}
                        </label>
                    ))}
                </div>
            </div>
            <div>
                <label className="text-xs text-neutral-400 mb-1 block">最大迭代次数</label>
                <input type="number" value={config.max_iterations ?? 10}
                    onChange={e => update('max_iterations', parseInt(e.target.value))}
                    min={1} max={50} className="w-24 px-3 py-2 bg-neutral-800 rounded text-sm" />
            </div>
        </div>
    )
}
```

---

## 4. 验收测试

```
1. LLM 节点：配置 prompt="总结{{node_1.result}}" → 运行时 AI 输出摘要
2. LLM 节点 JSON 模式：schema={"summary":"string"} → 输出符合 schema 的 JSON
3. Agent 节点：目标="查询天气" + 工具=[http_request] → Agent 自动调用工具完成
4. Agent 节点：超过 max_iterations=3 时 → 节点失败，错误信息"超过最大迭代次数"
5. Agent 节点运行中 → 画布上节点显示"思考中..."动画
```
