// tab-logs.js — backend log stream tab (GET /dev/logs SSE).

document.addEventListener('alpine:init', () => {
  Alpine.data('logsTab', () => ({
    entries: [],
    filter: '',
    autoScroll: true,
    _es: null,

    get filtered() {
      if (!this.filter) return this.entries
      const q = this.filter.toLowerCase()
      return this.entries.filter(e =>
        e.msg.toLowerCase().includes(q) ||
        e.level.includes(q) ||
        JSON.stringify(e.fields || {}).toLowerCase().includes(q)
      )
    },

    init() {
      this._connect()
    },

    _connect() {
      const es = new EventSource('/dev/logs')
      this._es = es
      es.addEventListener('log', e => {
        const entry = JSON.parse(e.data)
        entry._time = this._shortTime(entry.time)
        this.entries.push(entry)
        // Cap to 2000 entries to avoid memory bloat.
        // 最多保留 2000 条，避免内存膨胀。
        if (this.entries.length > 2000) this.entries.shift()
        if (this.autoScroll) this._scroll()
      })
    },

    clear() { this.entries = [] },

    fieldsStr(fields) {
      if (!fields || Object.keys(fields).length === 0) return ''
      return Object.entries(fields)
        .map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : v}`)
        .join('  ')
    },

    _shortTime(iso) {
      if (!iso) return ''
      return iso.slice(11, 19) // HH:MM:SS
    },

    _scroll() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.event-log')
        if (el) el.scrollTop = el.scrollHeight
      })
    },
  }))
})
