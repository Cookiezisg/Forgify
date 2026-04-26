// tab-sse.js — SSE event inspector: source × view matrix.
// Source: conversation | forge
// View:   stream | raw

document.addEventListener('alpine:init', () => {
  Alpine.data('sseTab', () => ({
    source: 'conversation',  // 'conversation' | 'forge'
    view:   'stream',        // 'stream' | 'raw'

    events: [],   // raw view: [{type, time, data}]
    turns:  [],   // stream view (conversation): [{messageId, items[], done, stopReason, error}]
    forges: [],   // stream view (forge): [{toolCallId, toolId, toolName, actionType, code, done, result}]

    _es: null,
    autoScroll: true,

    get conversationId() { return Alpine.store('app').conversationId },

    init() {
      this.$watch('conversationId', () => this._reconnect())
    },

    // ── source switch ─────────────────────────────────────────────────────────

    setSource(s) {
      this.source = s
      this.clear()
      this._reconnect()
    },

    // ── SSE connection ────────────────────────────────────────────────────────

    _reconnect() {
      if (this._es) { this._es.close(); this._es = null }
      const id = this.conversationId
      if (!id) return

      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      const types = this.source === 'forge'
        ? ['tool.code_streaming', 'tool.created', 'tool.pending_created']
        : ['chat.reasoning_token', 'chat.token', 'chat.done', 'chat.error',
           'chat.tool_call', 'chat.tool_result', 'conversation.title_updated']

      types.forEach(type => {
        es.addEventListener(type, e => {
          const data = JSON.parse(e.data)

          this.events.push({
            type,
            time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
            data,
          })

          if (this.source === 'forge') {
            this._handleForge(type, data)
          } else {
            this._handleConversation(type, data)
          }

          if (this.autoScroll) this._scroll()
        })
      })
    },

    // ── conversation stream ───────────────────────────────────────────────────

    _turnFor(messageId) {
      let t = this.turns.find(t => t.messageId === messageId)
      if (!t) {
        t = { messageId, items: [], done: false, stopReason: '', error: null }
        this.turns.push(t)
      }
      return t
    },

    _lastOfType(turn, type) {
      const items = turn.items
      return (items.length && items[items.length - 1].type === type)
        ? items[items.length - 1] : null
    },

    _handleConversation(type, data) {
      const turn = this._turnFor(data.messageId || data.conversationId)

      if (type === 'chat.reasoning_token') {
        let item = this._lastOfType(turn, 'thinking')
        if (!item) { item = { type: 'thinking', content: '', done: false }; turn.items.push(item) }
        item.content += data.delta

      } else if (type === 'chat.token') {
        const thinking = turn.items.find(i => i.type === 'thinking' && !i.done)
        if (thinking) thinking.done = true
        let item = this._lastOfType(turn, 'text')
        if (!item) { item = { type: 'text', content: '' }; turn.items.push(item) }
        item.content += data.delta

      } else if (type === 'chat.tool_call') {
        turn.items.push({
          type: 'tool', toolCallId: data.toolCallId,
          toolName: data.toolName, summary: data.summary || '',
          input: data.toolInput, result: null, ok: null,
        })
        turn.items.push({ type: 'text', content: '' })

      } else if (type === 'chat.tool_result') {
        const item = turn.items.find(i => i.type === 'tool' && i.toolCallId === data.toolCallId)
        if (item) { item.result = data.result; item.ok = data.ok }

      } else if (type === 'chat.done') {
        turn.done = true; turn.stopReason = data.stopReason
        while (turn.items.length && turn.items[turn.items.length - 1].type === 'text'
               && !turn.items[turn.items.length - 1].content) {
          turn.items.pop()
        }
      } else if (type === 'chat.error') {
        turn.error = data.message
      }
    },

    // ── forge stream ──────────────────────────────────────────────────────────

    _forgeFor(toolCallId) {
      let f = this.forges.find(f => f.toolCallId === toolCallId)
      if (!f) {
        f = { toolCallId, toolId: '', toolName: '', actionType: '', code: '', done: false, result: '' }
        this.forges.push(f)
      }
      return f
    },

    _handleForge(type, data) {
      if (type === 'tool.code_streaming') {
        const f = this._forgeFor(data.toolCallId)
        if (!f.actionType) f.actionType = data.actionType
        if (data.toolId)   f.toolId = data.toolId
        f.code += data.delta

      } else if (type === 'tool.created') {
        const f = this._forgeFor(data.toolCallId)
        f.toolId   = data.toolId
        f.toolName = data.toolName
        f.done     = true
        f.result   = 'created'

      } else if (type === 'tool.pending_created') {
        const f = this._forgeFor(data.toolCallId)
        f.toolId = data.toolId
        f.done   = true
        f.result = 'pending'
      }
    },

    // ── helpers ───────────────────────────────────────────────────────────────

    clear() { this.events = []; this.turns = []; this.forges = [] },

    pretty(data) { return JSON.stringify(data, null, 2) },
    cssClass(type) { return type.replace('.', '-') },
    shortId(id) { return id ? id.slice(0, 8) : '?' },
    tryFmt(s) { try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return s } },

    _scroll() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.event-log')
        if (el) el.scrollTop = el.scrollHeight
      })
    },
  }))
})
