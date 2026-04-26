// chat.js — center chat panel: message list, streaming, send/cancel.

document.addEventListener('alpine:init', () => {
  Alpine.data('chatPanel', () => ({
    messages: [],
    input: '',
    streaming: false,
    _es: null,
    _streamMsgId: null,
    _pendingToolItems: {},  // toolCallId → item ref

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

    // ── loadMessages ──────────────────────────────────────────────────────────

    async loadMessages(id) {
      const r = await fetch(`/api/v1/conversations/${id}/messages?limit=200`)
      if (!r.ok) return
      const j = await r.json()
      const raw = (j.data || []).filter(m => m.status !== 'pending')

      // Rebuild conversation as an ordered list. Each row becomes items[].
      // Multiple DB rows belonging to one assistant turn are merged into one
      // displayed message (intermediate + tool results + final).
      const msgs = []
      let cur = null  // current assistant message being built

      for (const m of raw) {
        if (m.role === 'user') {
          cur = null
          msgs.push(m)
          continue
        }

        if (m.role === 'assistant') {
          const items = []
          // Reasoning first (if any)
          if (m.reasoningContent) {
            items.push({ type: 'reasoning', content: m.reasoningContent, done: true })
          }
          // Text content (if any)
          if (m.content) {
            items.push({ type: 'text', content: m.content })
          }
          // Tool calls (if any)
          if (m.toolCalls) {
            let calls = []
            try { calls = JSON.parse(m.toolCalls) } catch {}
            calls.forEach(tc => items.push({
              type:       'tool',
              toolCallId: tc.id,
              toolName:   tc.function?.name ?? '',
              input:      tc.function?.arguments ?? '{}',
              result:     null,
              ok:         null,
            }))
          }

          if (m.toolCalls && cur) {
            // Intermediate step: append to the in-progress turn
            cur.items.push(...items)
          } else if (m.toolCalls) {
            // New turn that starts with a tool call
            cur = { id: m.id, role: 'assistant', items, status: m.status }
            msgs.push(cur)
          } else {
            // Final response text
            if (cur) {
              cur.items.push(...items)
              cur.status = m.status
              cur = null
            } else {
              msgs.push({ id: m.id, role: 'assistant', items, status: m.status })
            }
          }
          continue
        }

        if (m.role === 'tool' && cur) {
          const item = cur.items.find(i => i.type === 'tool' && i.toolCallId === m.toolCallId)
          if (item) { item.result = m.content; item.ok = true }
        }
      }

      this.messages = msgs
    },

    // ── send ──────────────────────────────────────────────────────────────────

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
      this.messages.push({ id: j.data.messageId, role: 'user', content, status: 'completed' })
      this._streamMsgId = 'stream-' + Date.now()
      this.messages.push({ id: this._streamMsgId, role: 'assistant', items: [], status: 'streaming' })
      this.streaming = true
      this._scrollBottom()
    },

    async cancel() {
      if (!this.conversationId) return
      await fetch(`/api/v1/conversations/${this.conversationId}/stream`, { method: 'DELETE' })
    },

    // ── SSE ───────────────────────────────────────────────────────────────────

    _connectSSE(id) {
      this._closeSSE()
      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      const _msg = () => this.messages.find(m => m.id === this._streamMsgId)
      const _last = (type) => {
        const msg = _msg(); if (!msg) return null
        const items = msg.items
        return (items.length && items[items.length - 1].type === type)
          ? items[items.length - 1] : null
      }

      es.addEventListener('chat.reasoning_token', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        let item = _last('reasoning')
        if (!item) { item = { type: 'reasoning', content: '', done: false }; msg.items.push(item) }
        item.content += d.delta
        this._scrollBottom()
      })

      es.addEventListener('chat.token', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        // Mark any active reasoning as done
        const r = msg.items.find(i => i.type === 'reasoning' && !i.done)
        if (r) r.done = true
        // Append to last text item or start new segment
        let item = _last('text')
        if (!item) { item = { type: 'text', content: '' }; msg.items.push(item) }
        item.content += d.delta
        this._scrollBottom()
      })

      es.addEventListener('chat.tool_call', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        const item = { type: 'tool', toolCallId: d.toolCallId, toolName: d.toolName,
                       input: d.toolInput, result: null, ok: null }
        msg.items.push(item)
        this._pendingToolItems[d.toolCallId] = item
        // New text segment for post-tool text
        msg.items.push({ type: 'text', content: '' })
        this._scrollBottom()
      })

      es.addEventListener('chat.tool_result', e => {
        const d = JSON.parse(e.data)
        const item = this._pendingToolItems[d.toolCallId]
        if (item) { item.result = d.result; item.ok = d.ok }
      })

      es.addEventListener('chat.done', e => {
        const data = JSON.parse(e.data)
        const msg = _msg()
        if (msg) {
          // Drop trailing empty text segments
          while (msg.items.length && msg.items[msg.items.length - 1].type === 'text'
                 && !msg.items[msg.items.length - 1].content) {
            msg.items.pop()
          }
          msg.status = 'completed'
          msg.stopReason = data.stopReason
        }
        this.streaming = false
        this._streamMsgId = null
        this.loadMessages(id)
      })

      es.addEventListener('chat.error', e => {
        const data = JSON.parse(e.data)
        const msg = _msg()
        if (msg) { msg.status = 'error'; msg.errorMsg = data.message }
        this.streaming = false
        this._streamMsgId = null
      })

      es.addEventListener('conversation.title_updated', e => {
        Alpine.store('app').conversationTitle = JSON.parse(e.data).title
      })
    },

    _closeSSE() {
      if (this._es) { this._es.close(); this._es = null }
      this._pendingToolItems = {}
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
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.send() }
    },
  }))
})
