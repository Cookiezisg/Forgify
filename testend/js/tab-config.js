document.addEventListener('alpine:init', () => {
  Alpine.data('configTab', () => ({
    keys: [],
    modelConfig: null,
    busyKeyId: '',     // id of key currently being tested — string compare is reliably reactive
    showAdd: false,
    ap: 'deepseek', akey: '', aurl: '',
    addBusy: false,
    mp: '', mid: '',
    modelBusy: false,

    allProviders: ['openai','anthropic','google','deepseek','openrouter',
                   'qwen','zhipu','moonshot','doubao','ollama','custom'],

    async init() {
      await Promise.all([this._loadKeys(), this._loadModel()])
      this.$watch('mp', p => {
        const ms = this._modelsFor(p)
        this.mid = ms[0] ?? ''
      })
    },

    async _fetch(method, url, body) {
      const opts = { method, headers: {} }
      if (body != null) {
        opts.headers['Content-Type'] = 'application/json'
        opts.body = JSON.stringify(body)
      }
      const r = await fetch(url, opts)
      const j = await r.json().catch(() => null)
      return { r, j }
    },

    async _loadKeys() {
      const { j } = await this._fetch('GET', '/api/v1/api-keys?limit=50')
      this.keys = j?.data ?? []
    },

    async _loadModel() {
      const { j } = await this._fetch('GET', '/api/v1/model-configs')
      const chat = (j?.data ?? []).find(m => m.scenario === 'chat')
      if (chat) {
        this.modelConfig = { provider: chat.provider, modelId: chat.modelId }
        this.mp = chat.provider
        this.mid = chat.modelId
      } else {
        const ok = this.keys.find(k => k.testStatus === 'ok')
        if (ok) { this.mp = ok.provider; this.mid = this._modelsFor(ok.provider)[0] ?? '' }
      }
    },

    okProviders()       { return [...new Set(this.keys.filter(k => k.testStatus === 'ok').map(k => k.provider))] },
    _modelsFor(p)       { return this.keys.find(k => k.provider === p && k.testStatus === 'ok')?.modelsFound ?? [] },
    modelsForMP()       { return this._modelsFor(this.mp) },
    needsURL()          { return this.ap === 'ollama' || this.ap === 'custom' },

    async addKey() {
      if (!this.akey.trim()) return
      this.addBusy = true
      try {
        await this._fetch('POST', '/api/v1/api-keys', {
          provider: this.ap, displayName: this.ap,
          key: this.akey, baseUrl: this.aurl, apiFormat: '',
        })
        this.akey = ''; this.aurl = ''; this.showAdd = false
        await this._loadKeys()
      } finally { this.addBusy = false }
    },

    testKey(id) {
      // Non-async wrapper so the busyKeyId assignment is synchronous before the event loop yields.
      // 非 async 包装，让 busyKeyId 赋值在事件循环让出前同步完成。
      this.busyKeyId = id
      this._runTest(id)
    },

    async _runTest(id) {
      try {
        await this._fetch('POST', `/api/v1/api-keys/${id}:test`)
        await this._loadKeys()
        const ms = this._modelsFor(this.mp)
        if (ms.length > 0 && !this.mid) this.mid = ms[0]
      } finally {
        this.busyKeyId = ''
      }
    },

    async delKey(id) {
      await this._fetch('DELETE', `/api/v1/api-keys/${id}`)
      await this._loadKeys()
    },

    async saveModel() {
      if (!this.mp || !this.mid.trim()) return
      this.modelBusy = true
      try {
        await this._fetch('PUT', '/api/v1/model-configs/chat', { provider: this.mp, modelId: this.mid })
        await this._loadModel()
      } finally { this.modelBusy = false }
    },
  }))
})
