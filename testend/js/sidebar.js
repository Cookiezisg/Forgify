// sidebar.js — conversation list panel.

document.addEventListener('alpine:init', () => {
  Alpine.data('sidebar', () => ({
    conversations: [],
    loading: false,

    get selected() { return Alpine.store('app').conversationId },

    async init() {
      await this.load()
      // Refresh list periodically to catch auto-title updates.
      // 定期刷新对话列表以捕获自动命名更新。
      setInterval(() => this.load(), 8000)
    },

    async load() {
      const r = await fetch('/api/v1/conversations?limit=50')
      if (r.ok) {
        const j = await r.json()
        this.conversations = j.data || []
      }
    },

    async create() {
      const r = await fetch('/api/v1/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: '' }),
      })
      if (r.ok) {
        const j = await r.json()
        await this.load()
        this.select(j.data.id, j.data.title)
      }
    },

    select(id, title) {
      Alpine.store('app').conversationId = id
      Alpine.store('app').conversationTitle = title || '(untitled)'
    },

    formatTime(ts) {
      if (!ts) return ''
      const d = new Date(ts)
      const now = new Date()
      if (d.toDateString() === now.toDateString()) {
        return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      }
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
    },
  }))
})
