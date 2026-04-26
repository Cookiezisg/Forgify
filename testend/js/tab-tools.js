// tab-tools.js — Tools tab: system tool invocation + user tool browser/runner.

document.addEventListener('alpine:init', () => {
  Alpine.data('toolsTab', () => ({
    // sub-tab: 'system' | 'user'
    section: 'system',

    // ── System Tools ──────────────────────────────────────────────────────────
    sysTools: [],        // [{name, desc}] from GET /dev/tools
    sysSelected: '',
    sysArgs: '{}',
    sysResult: null,     // {output, ok, elapsedMs, error?}
    sysLoading: false,

    // ── User Tools ────────────────────────────────────────────────────────────
    userTools: [],
    userSearch: '',
    userSelected: null,  // full tool object
    userInput: '{}',
    userResult: null,    // domain ExecutionResult
    userLoading: false,

    init() {
      this.loadSysTools()
      this.loadUserTools()
    },

    // ── system ────────────────────────────────────────────────────────────────

    async loadSysTools() {
      try {
        const r = await fetch('/dev/tools')
        if (r.ok) this.sysTools = await r.json()
        if (this.sysTools.length && !this.sysSelected) {
          this.sysSelected = this.sysTools[0].name
        }
      } catch { /* server not up yet */ }
    },

    get sysDesc() {
      const t = this.sysTools.find(t => t.name === this.sysSelected)
      return t ? t.desc : ''
    },

    async invokeSystem() {
      if (!this.sysSelected || this.sysLoading) return
      this.sysLoading = true; this.sysResult = null
      try {
        const r = await fetch('/dev/invoke', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ tool: this.sysSelected, args: this.sysArgs }),
        })
        this.sysResult = await r.json()
      } catch (e) {
        this.sysResult = { ok: false, error: e.message, output: '', elapsedMs: 0 }
      }
      this.sysLoading = false
    },

    sysPretty() {
      if (!this.sysResult) return ''
      try { return JSON.stringify(JSON.parse(this.sysResult.output), null, 2) }
      catch { return this.sysResult.output }
    },

    // ── user tools ────────────────────────────────────────────────────────────

    async loadUserTools() {
      try {
        const r = await fetch('/api/v1/tools?limit=200')
        if (r.ok) {
          const j = await r.json()
          this.userTools = j.data || []
        }
      } catch { /* server not up yet */ }
    },

    get filteredUserTools() {
      const q = this.userSearch.trim().toLowerCase()
      if (!q) return this.userTools
      return this.userTools.filter(t =>
        t.name.toLowerCase().includes(q) || (t.description || '').toLowerCase().includes(q)
      )
    },

    selectUser(t) {
      this.userSelected = t
      this.userInput = '{}'
      this.userResult = null
    },

    async runUser() {
      if (!this.userSelected || this.userLoading) return
      this.userLoading = true; this.userResult = null
      try {
        let input = {}
        try { input = JSON.parse(this.userInput) } catch { /* invalid JSON, send empty */ }
        const r = await fetch(`/api/v1/tools/${this.userSelected.id}:run`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ input }),
        })
        const j = await r.json()
        this.userResult = j.data ?? j
      } catch (e) {
        this.userResult = { ok: false, errorMsg: e.message }
      }
      this.userLoading = false
    },

    userResultPretty() {
      if (!this.userResult) return ''
      try { return JSON.stringify(this.userResult, null, 2) } catch { return String(this.userResult) }
    },

    handleSysKeydown(e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); this.invokeSystem() }
    },

    handleUserKeydown(e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); this.runUser() }
    },
  }))
})
