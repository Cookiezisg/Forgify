// chat.js — center chat panel: message list, streaming, send/cancel.

document.addEventListener('alpine:init', () => {
  Alpine.data('chatPanel', () => ({
    messages: [],
    input: '',
    streaming: false,
    _es: null,
    _streamMsgId: null,
    _pendingSteps: {},  // toolCallId → step object, for tool_result lookup

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
      if (!r.ok) return
      const j = await r.json()
      const raw = (j.data || []).filter(m => m.status !== 'pending')

      // Fold tool-call / tool-result rows into the following assistant message's
      // steps array, so they render as step cards rather than separate bubbles.
      const msgs = []
      const pendingSteps = []

      for (const m of raw) {
        if (m.role === 'assistant' && !m.content && m.toolCalls) {
          // LLM tool-call decision: parse into pending steps.
          let calls = []
          try { calls = JSON.parse(m.toolCalls) } catch {}
          for (const tc of calls) {
            pendingSteps.push({
              toolCallId: tc.id,
              toolName:   tc.function?.name ?? '',
              input:      tc.function?.arguments ?? '{}',
              result:     null,
              ok:         null,
            })
          }
        } else if (m.role === 'tool') {
          // Match result to its pending step.
          const step = pendingSteps.find(s => s.toolCallId === m.toolCallId)
          if (step) { step.result = m.content; step.ok = true }
        } else {
          // User or final-text assistant message: attach any accumulated steps.
          if (pendingSteps.length) {
            m.steps = pendingSteps.splice(0)
          }
          msgs.push(m)
        }
      }

      this.messages = msgs
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
      this.messages.push({ id: j.data.messageId, role: 'user', content, status: 'completed' })
      this._streamMsgId = 'stream-' + Date.now()
      this.messages.push({ id: this._streamMsgId, role: 'assistant', content: '', steps: [], status: 'streaming' })
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

      es.addEventListener('chat.tool_call', e => {
        const d = JSON.parse(e.data)
        const msg = this.messages.find(m => m.id === this._streamMsgId)
        if (!msg) return
        if (!msg.steps) msg.steps = []
        const step = { toolCallId: d.toolCallId, toolName: d.toolName, input: d.toolInput, result: null, ok: null }
        msg.steps.push(step)
        this._pendingSteps[d.toolCallId] = step
        this._scrollBottom()
      })

      es.addEventListener('chat.tool_result', e => {
        const d = JSON.parse(e.data)
        const step = this._pendingSteps[d.toolCallId]
        if (step) { step.result = d.result; step.ok = d.ok }
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
      this._pendingSteps = {}
    },

    _scrollBottom() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.chat-messages')
        if (el) el.scrollTop = el.scrollHeight
      })
    },

    tryFmt(s) {
      try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return s }
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
