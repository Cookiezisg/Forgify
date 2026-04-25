// tab-sql.js — read-only SQL query tab (POST /dev/sql).

document.addEventListener('alpine:init', () => {
  Alpine.data('sqlTab', () => ({
    sql: 'SELECT * FROM messages ORDER BY created_at DESC LIMIT 20',
    result: null,
    error: null,
    loading: false,

    shortcuts: [
      { label: 'messages',      sql: 'SELECT * FROM messages ORDER BY created_at DESC LIMIT 20' },
      { label: 'conversations', sql: "SELECT * FROM conversations WHERE deleted_at IS NULL ORDER BY created_at DESC" },
      { label: 'api_keys',      sql: "SELECT id, provider, display_name, key_masked, test_status FROM api_keys WHERE deleted_at IS NULL" },
      { label: 'model_configs', sql: "SELECT * FROM model_configs WHERE deleted_at IS NULL" },
      { label: 'attachments',   sql: 'SELECT * FROM chat_attachments ORDER BY created_at DESC LIMIT 20' },
    ],

    async run() {
      if (!this.sql.trim()) return
      this.loading = true; this.error = null; this.result = null
      try {
        const r = await fetch('/dev/sql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sql: this.sql }),
        })
        const data = await r.json()
        if (data.error) this.error = data.error
        else this.result = data
      } catch (e) {
        this.error = e.message
      }
      this.loading = false
    },

    setSQL(sql) { this.sql = sql },

    handleKeydown(e) {
      // Ctrl/Cmd+Enter = run
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault(); this.run()
      }
    },

    fmt(v) {
      if (v === null || v === undefined) return ''
      if (typeof v === 'string' && v.length > 60) return v.slice(0, 60) + '…'
      return String(v)
    },
  }))
})
