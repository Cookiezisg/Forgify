// app.js — Alpine root store and shared utilities.
// Loaded first; other scripts use Alpine.store('app') for shared state.

document.addEventListener('alpine:init', () => {
  Alpine.store('app', {
    conversationId: null,
    conversationTitle: '',
  })
})

function appRoot() {
  return {
    showSettings: false,
    activeRightTab: 'sse',

    closeSettings() { this.showSettings = false },

    selectTab(tab) { this.activeRightTab = tab },
  }
}

function settingsPanel() {
  return {
    apiKeys: [],
    modelConfig: null,
    // add-key form
    newProvider: 'openai',
    newDisplayName: '',
    newKey: '',
    newBaseURL: '',
    newAPIFormat: '',
    showAddKey: false,
    // model form
    modelProvider: '',
    modelID: '',
    editingModel: false,
    saving: false,
    msg: '',

    providers: [
      { value: 'openai',    label: 'OpenAI' },
      { value: 'anthropic', label: 'Anthropic' },
      { value: 'google',    label: 'Google' },
      { value: 'groq',      label: 'Groq' },
      { value: 'ollama',    label: 'Ollama' },
      { value: 'deepseek',  label: 'DeepSeek' },
      { value: 'custom',    label: 'Custom' },
    ],

    needsBaseURL() {
      return ['ollama', 'custom'].includes(this.newProvider)
    },

    async init() {
      await this.loadAll()
    },

    async loadAll() {
      await Promise.all([this.loadKeys(), this.loadModel()])
    },

    async loadKeys() {
      const r = await fetch('/api/v1/api-keys?limit=50')
      if (r.ok) {
        const j = await r.json()
        this.apiKeys = j.data || []
      }
    },

    async loadModel() {
      const r = await fetch('/api/v1/model-configs')
      if (r.ok) {
        const j = await r.json()
        const chat = (j.data || []).find(m => m.scenario === 'chat')
        if (chat) {
          this.modelConfig = chat
          this.modelProvider = chat.provider
          this.modelID = chat.modelId
        }
      }
    },

    async addKey() {
      if (!this.newKey.trim()) return
      this.saving = true
      const body = {
        provider: this.newProvider,
        displayName: this.newDisplayName || this.newProvider,
        key: this.newKey,
        baseUrl: this.newBaseURL,
        apiFormat: this.newAPIFormat,
      }
      const r = await fetch('/api/v1/api-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      this.saving = false
      if (r.ok) {
        this.newKey = ''; this.newDisplayName = ''; this.newBaseURL = ''; this.showAddKey = false
        await this.loadKeys()
        this.msg = 'API Key 已添加'
        setTimeout(() => this.msg = '', 2000)
      } else {
        const e = await r.json()
        this.msg = e.error?.message || '添加失败'
      }
    },

    async deleteKey(id) {
      await fetch(`/api/v1/api-keys/${id}`, { method: 'DELETE' })
      await this.loadKeys()
    },

    async saveModel() {
      if (!this.modelProvider.trim() || !this.modelID.trim()) return
      this.saving = true
      const r = await fetch('/api/v1/model-configs/chat', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider: this.modelProvider, modelId: this.modelID }),
      })
      this.saving = false
      if (r.ok) {
        await this.loadModel()
        this.editingModel = false
        this.msg = '模型已保存'
        setTimeout(() => this.msg = '', 2000)
      }
    },
  }
}
