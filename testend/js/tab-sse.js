// tab-sse.js — SSE event inspector: Stream view (assembled by turn) + Raw view (per-event JSON).

document.addEventListener('alpine:init', () => {
  Alpine.data('sseTab', () => ({
    view: 'stream',   // 'stream' | 'raw'
    events: [],       // raw mode: [{type, time, data}]
    turns: [],        // stream mode: [{messageId, text, toolCalls[], done, stopReason, error}]
    _es: null,
    autoScroll: true,

    get conversationId() { return Alpine.store('app').conversationId },

    init() {
      this.$watch('conversationId', id => this._reconnect(id))
    },

    _turnFor(messageId) {
      let t = this.turns.find(t => t.messageId === messageId)
      if (!t) {
        t = { messageId, text: '', toolCalls: [], done: false, stopReason: '', error: null }
        this.turns.push(t)
      }
      return t
    },

    _reconnect(id) {
      if (this._es) { this._es.close(); this._es = null }
      if (!id) return

      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      const ALL = [
        'chat.token', 'chat.done', 'chat.error',
        'chat.tool_call', 'chat.tool_result',
        'conversation.title_updated',
      ]
      ALL.forEach(type => {
        es.addEventListener(type, e => {
          const data = JSON.parse(e.data)

          // raw view accumulation
          this.events.push({
            type,
            time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
            data,
          })

          // stream view accumulation
          if (type === 'chat.token') {
            this._turnFor(data.messageId).text += data.delta
          } else if (type === 'chat.tool_call') {
            this._turnFor(data.messageId).toolCalls.push({
              toolCallId: data.toolCallId,
              toolName: data.toolName,
              input: data.toolInput,
              result: null,
              ok: null,
            })
          } else if (type === 'chat.tool_result') {
            for (const t of this.turns) {
              const tc = t.toolCalls.find(c => c.toolCallId === data.toolCallId)
              if (tc) { tc.result = data.result; tc.ok = data.ok; break }
            }
          } else if (type === 'chat.done') {
            const t = this._turnFor(data.messageId)
            t.done = true; t.stopReason = data.stopReason
          } else if (type === 'chat.error') {
            this._turnFor(data.messageId).error = data.message
          }

          if (this.autoScroll) this._scroll()
        })
      })
    },

    clear() { this.events = []; this.turns = [] },

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
