// tab-tests.js — YAML test collection runner (frontend-side execution).

document.addEventListener('alpine:init', () => {
  Alpine.data('testsTab', () => ({
    collections: [],
    selected: null,
    results: null,
    running: false,
    envStr: '',   // "KEY=VALUE\nKEY2=VALUE2"

    get envVars() {
      const vars = {}
      for (const line of this.envStr.split('\n')) {
        const idx = line.indexOf('=')
        if (idx > 0) vars[line.slice(0, idx).trim()] = line.slice(idx + 1).trim()
      }
      return vars
    },

    async init() {
      await this.loadCollections()
    },

    async loadCollections() {
      const r = await fetch('/dev/collections')
      if (r.ok) this.collections = await r.json()
    },

    select(col) {
      this.selected = col
      this.results = null
    },

    async run() {
      if (!this.selected || this.running) return
      this.running = true

      const steps = this.selected.steps || []
      this.results = steps.map(s => ({ ...s, state: 'pending', status: null, latency: null, response: null, err: null }))

      const vars = { ...this.envVars }

      for (let i = 0; i < steps.length; i++) {
        const step = steps[i]
        this.results[i].state = 'running'

        const path = this._subst(step.path || '', vars)
        const rawBody = step.body ? this._substObj(step.body, vars) : null

        const start = Date.now()
        try {
          const opts = { method: step.method || 'GET', headers: {} }
          if (rawBody) {
            opts.headers['Content-Type'] = 'application/json'
            opts.body = JSON.stringify(rawBody)
          }
          const r = await fetch(path, opts)
          const latency = Date.now() - start
          let responseData = null
          try { responseData = await r.json() } catch (_) {}

          // Capture variables from response.
          // 从响应中捕获变量。
          if (step.capture && responseData) {
            for (const [varName, path] of Object.entries(step.capture)) {
              const val = this._jsonPath(responseData, path)
              if (val !== undefined) vars[varName] = String(val)
            }
          }

          const expectedStatus = step.expect?.status
          const pass = !expectedStatus || r.status === expectedStatus
          this.results[i] = { ...this.results[i], state: pass ? 'pass' : 'fail', status: r.status, latency, response: responseData, pass }
        } catch (e) {
          this.results[i] = { ...this.results[i], state: 'fail', err: e.message, pass: false, latency: Date.now() - start }
        }

        // Yield to Alpine for re-render between steps.
        await new Promise(r => setTimeout(r, 30))
      }

      this.running = false
    },

    stateIcon(state) {
      return { pending: '○', running: '⟳', pass: '✓', fail: '✗' }[state] || '○'
    },

    _subst(str, vars) {
      return str.replace(/\{\{([^}]+)\}\}/g, (_, key) => {
        if (key.startsWith('env.')) return vars[key.slice(4)] ?? ''
        return vars[key] ?? ''
      })
    },

    _substObj(obj, vars) {
      // Deep clone with variable substitution in all string values.
      // 深度克隆并替换所有字符串值中的变量。
      if (typeof obj === 'string') return this._subst(obj, vars)
      if (Array.isArray(obj)) return obj.map(v => this._substObj(v, vars))
      if (obj && typeof obj === 'object') {
        const out = {}
        for (const [k, v] of Object.entries(obj)) out[k] = this._substObj(v, vars)
        return out
      }
      return obj
    },

    _jsonPath(data, path) {
      // Simple JSONPath: "$.data.id" → data["data"]["id"]
      const parts = path.replace(/^\$\./, '').split('.')
      let cur = data
      for (const p of parts) {
        if (cur == null || typeof cur !== 'object') return undefined
        cur = cur[p]
      }
      return cur
    },
  }))
})
