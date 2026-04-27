// tab-sql.js — read-only SQL query tab (POST /dev/sql).

document.addEventListener('alpine:init', () => {
  Alpine.data('sqlTab', () => ({
    sql: 'SELECT id, conversation_id, role, status, stop_reason, input_tokens, output_tokens, created_at FROM messages ORDER BY created_at DESC LIMIT 20',
    result: null,
    error: null,
    loading: false,
    wrap: false,

    shortcuts: [
      {
        label: 'messages',
        sql: 'SELECT id, conversation_id, role, status, stop_reason, input_tokens, output_tokens, created_at FROM messages ORDER BY created_at DESC LIMIT 20',
      },
      {
        label: 'message_blocks',
        sql: 'SELECT id, message_id, seq, type, data, created_at FROM message_blocks ORDER BY created_at DESC, seq ASC LIMIT 50',
      },
      {
        label: 'blocks for conv',
        sql: `SELECT b.id, b.message_id, b.seq, b.type, substr(b.data,1,80) as data_preview
FROM message_blocks b
JOIN messages m ON m.id = b.message_id
ORDER BY m.created_at ASC, b.seq ASC
LIMIT 100`,
      },
      {
        label: 'conversations',
        sql: "SELECT id, title, auto_titled, created_at, updated_at FROM conversations WHERE deleted_at IS NULL ORDER BY created_at DESC",
      },
      {
        label: 'api_keys',
        sql: "SELECT id, provider, display_name, key_masked, test_status FROM api_keys WHERE deleted_at IS NULL",
      },
      {
        label: 'model_configs',
        sql: "SELECT * FROM model_configs WHERE deleted_at IS NULL",
      },
      {
        label: 'attachments',
        sql: 'SELECT id, user_id, file_name, mime_type, size_bytes, created_at FROM chat_attachments ORDER BY created_at DESC LIMIT 20',
      },
      {
        label: 'tools',
        sql: "SELECT id, name, description, version_count, tags FROM tools WHERE deleted_at IS NULL ORDER BY created_at DESC",
      },
      {
        label: 'tool_versions',
        sql: "SELECT id, tool_id, version, status, message FROM tool_versions ORDER BY created_at DESC LIMIT 20",
      },
      {
        label: 'tool_test_cases',
        sql: "SELECT id, tool_id, name, input_data, expected_output FROM tool_test_cases WHERE deleted_at IS NULL",
      },
      {
        label: 'tool_run_history',
        sql: "SELECT id, tool_id, tool_version, ok, elapsed_ms, created_at FROM tool_run_history ORDER BY created_at DESC LIMIT 20",
      },
      {
        label: 'tool_test_history',
        sql: "SELECT id, tool_id, batch_id, ok, pass, created_at FROM tool_test_history ORDER BY created_at DESC LIMIT 20",
      },
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
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault(); this.run()
      }
    },

    fmt(v) {
      if (v === null || v === undefined) return ''
      if (typeof v === 'string' && v.length > 80) return v.slice(0, 80) + '…'
      return String(v)
    },

    // clickCell: if the cell contains a known ID, jump to a detail query for it.
    clickCell(v) {
      if (!v) return
      const s = String(v)
      let q = null
      if (s.startsWith('msg_')) {
        q = `SELECT b.id, b.seq, b.type, b.data, b.created_at\nFROM message_blocks b\nWHERE b.message_id = '${s}'\nORDER BY b.seq ASC`
      } else if (s.startsWith('blk_')) {
        q = `SELECT * FROM message_blocks WHERE id = '${s}'`
      } else if (s.startsWith('cv_')) {
        q = `SELECT id, conversation_id, role, status, stop_reason, input_tokens, output_tokens, created_at\nFROM messages WHERE conversation_id = '${s}' ORDER BY created_at ASC`
      } else if (s.startsWith('att_')) {
        q = `SELECT id, user_id, file_name, mime_type, size_bytes, storage_path, created_at FROM chat_attachments WHERE id = '${s}'`
      } else if (s.startsWith('t_')) {
        q = `SELECT id, name, description, code, version_count, tags FROM tools WHERE id = '${s}'`
      }
      if (q) { this.sql = q; this.run() }
    },
  }))
})
