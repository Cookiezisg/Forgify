// tab-sse.js — SSE raw event inspector tab.

document.addEventListener('alpine:init', () => {
  Alpine.data('sseTab', () => ({
    events: [],
    _es: null,
    autoScroll: true,

    get conversationId() { return Alpine.store('app').conversationId },

    init() {
      this.$watch('conversationId', id => this._reconnect(id))
    },

    _reconnect(id) {
      if (this._es) { this._es.close(); this._es = null }
      if (!id) return

      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      const eventTypes = [
        'chat.token', 'chat.done', 'chat.error',
        'chat.tool_call', 'chat.tool_result',
        'conversation.title_updated',
      ]
      eventTypes.forEach(type => {
        es.addEventListener(type, e => {
          this.events.push({
            type,
            time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
            data: JSON.parse(e.data),
          })
          if (this.autoScroll) this._scroll()
        })
      })
    },

    clear() { this.events = [] },

    pretty(data) { return JSON.stringify(data, null, 2) },

    cssClass(type) { return type.replace('.', '-') },

    _scroll() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.event-log')
        if (el) el.scrollTop = el.scrollHeight
      })
    },
  }))
})
