// chat.js — center chat panel: message list, streaming, send/cancel.

document.addEventListener('alpine:init', () => {
  Alpine.data('chatPanel', () => ({
    messages: [],
    input: '',
    streaming: false,
    _es: null,       // EventSource for this panel's streaming
    _streamMsgId: null,

    get conversationId() { return Alpine.store('app').conversationId },
    get title() { return Alpine.store('app').conversationTitle },

    init() {
      this.$watch('conversationId', id => {
        this._closeSSE()
        this.messages = []
        this.streaming = false
        if (id) {
          this.loadMessages(id).then(() => this._connectSSE(id))
        }
      })
    },

    async loadMessages(id) {
      const r = await fetch(`/api/v1/conversations/${id}/messages?limit=200`)
      if (r.ok) {
        const j = await r.json()
        this.messages = (j.data || []).filter(m => m.status !== 'pending')
      }
    },

    async send() {
      const content = this.input.trim()
      if (!content || !this.conversationId || this.streaming) return
      this.input = ''

      const r = await fetch(`/api/v1/conversations/${this.conversationId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content }),
      })
      if (!r.ok) return

      const j = await r.json()
      // Optimistically add user message and a streaming placeholder.
      // 乐观地添加用户消息和流式占位符。
      this.messages.push({ id: j.data.messageId, role: 'user', content, status: 'completed' })
      this._streamMsgId = 'stream-' + Date.now()
      this.messages.push({ id: this._streamMsgId, role: 'assistant', content: '', status: 'streaming' })
      this.streaming = true
      this._scrollBottom()
    },

    async cancel() {
      if (!this.conversationId) return
      await fetch(`/api/v1/conversations/${this.conversationId}/stream`, { method: 'DELETE' })
    },

    _connectSSE(id) {
      this._closeSSE()
      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      es.addEventListener('chat.token', e => {
        const data = JSON.parse(e.data)
        const msg = this.messages.find(m => m.id === this._streamMsgId)
        if (msg) { msg.content += data.delta; this._scrollBottom() }
      })

      es.addEventListener('chat.done', e => {
        const data = JSON.parse(e.data)
        const msg = this.messages.find(m => m.id === this._streamMsgId)
        if (msg) {
          msg.status = 'completed'
          msg.stopReason = data.stopReason
        }
        this.streaming = false
        this._streamMsgId = null
        // Reload to get the persisted assistant message with real ID.
        // 重新加载以获取持久化的真实 ID 的 assistant 消息。
        this.loadMessages(id)
      })

      es.addEventListener('chat.error', e => {
        const data = JSON.parse(e.data)
        const msg = this.messages.find(m => m.id === this._streamMsgId)
        if (msg) { msg.status = 'error'; msg.content = msg.content || data.message }
        this.streaming = false
        this._streamMsgId = null
      })

      es.addEventListener('conversation.title_updated', e => {
        const data = JSON.parse(e.data)
        Alpine.store('app').conversationTitle = data.title
      })
    },

    _closeSSE() {
      if (this._es) { this._es.close(); this._es = null }
    },

    _scrollBottom() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.chat-messages')
        if (el) el.scrollTop = el.scrollHeight
      })
    },

    handleKeydown(e) {
      // Shift+Enter = newline, Enter alone = send.
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        this.send()
      }
    },
  }))
})
