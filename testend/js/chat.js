// chat.js — center chat panel: message list, streaming, send/cancel, attachments.
// Adapted for the Block model: messages carry blocks[], not flat content columns.

document.addEventListener('alpine:init', () => {
  Alpine.data('chatPanel', () => ({
    messages: [],
    input: '',
    streaming: false,
    pendingAtts: [],      // [{id, fileName, mimeType}] — attachments queued for next send
    uploading: false,
    _es: null,
    _streamMsgId: null,
    _pendingToolItems: {},  // toolCallId → item ref (for result matching)

    get conversationId() { return Alpine.store('app').conversationId },
    get title() { return Alpine.store('app').conversationTitle },

    init() {
      this.$watch('conversationId', id => {
        this._closeSSE()
        this.messages = []
        this.streaming = false
        this.pendingAtts = []
        if (id) {
          this.loadMessages(id).then(() => this._connectSSE(id))
        }
      })
    },

    // ── loadMessages ──────────────────────────────────────────────────────────
    // Converts the Block model from the API into display-ready message objects.
    // User messages:    blocks → [{type:'text'|'attachment', ...}]
    // Assistant messages: blocks → items[] with reasoning/tool/text entries

    async loadMessages(id) {
      const r = await fetch(`/api/v1/conversations/${id}/messages?limit=200`)
      if (!r.ok) return
      const j = await r.json()
      const raw = (j.data || []).filter(m => m.status !== 'pending')

      this.messages = raw.map(m => {
        if (m.role === 'user') {
          return this._userMsgFromBlocks(m)
        } else {
          return this._assistantMsgFromBlocks(m)
        }
      })
    },

    _userMsgFromBlocks(m) {
      const blocks = []
      for (const b of (m.blocks || [])) {
        try {
          const d = JSON.parse(b.data)
          if (b.type === 'text') {
            blocks.push({ type: 'text', content: d.text })
          } else if (b.type === 'attachment_ref') {
            blocks.push({ type: 'attachment', fileName: d.fileName, mimeType: d.mimeType, id: d.attachmentId })
          }
        } catch {}
      }
      return { id: m.id, role: 'user', blocks, status: m.status }
    },

    _assistantMsgFromBlocks(m) {
      const items = []
      const toolMap = {}  // toolCallId → item (for pairing with tool_results)

      for (const b of (m.blocks || [])) {
        try {
          const d = JSON.parse(b.data)
          if (b.type === 'reasoning') {
            items.push({ type: 'reasoning', content: d.text, done: true })
          } else if (b.type === 'tool_call') {
            const item = {
              type: 'tool', toolCallId: d.id, toolName: d.name,
              summary: d.summary || '', input: JSON.stringify(d.arguments || {}),
              result: null, ok: null,
            }
            items.push(item)
            toolMap[d.id] = item
          } else if (b.type === 'tool_result') {
            const item = toolMap[d.toolCallId]
            if (item) { item.result = d.result; item.ok = d.ok }
          } else if (b.type === 'text') {
            if (d.text) items.push({ type: 'text', content: d.text })
          }
        } catch {}
      }

      return {
        id: m.id, role: 'assistant', items, status: m.status,
        stopReason: m.stopReason,
        inputTokens: m.inputTokens, outputTokens: m.outputTokens,
      }
    },

    // ── Attachment upload ─────────────────────────────────────────────────────

    pickFile() {
      this.$refs.fileInput.click()
    },

    async onFileChange(e) {
      const files = e.target.files
      if (!files || files.length === 0) return
      for (const file of files) {
        await this.uploadAttachment(file)
      }
      e.target.value = ''  // reset so same file can be re-selected
    },

    async uploadAttachment(file) {
      this.uploading = true
      try {
        const fd = new FormData()
        fd.append('file', file)
        const r = await fetch('/api/v1/attachments', { method: 'POST', body: fd })
        if (!r.ok) { alert('Upload failed: ' + r.status); return }
        const j = await r.json()
        this.pendingAtts.push({ id: j.data.id, fileName: j.data.fileName, mimeType: j.data.mimeType })
      } finally {
        this.uploading = false
      }
    },

    removeAtt(idx) {
      this.pendingAtts.splice(idx, 1)
    },

    // ── send ──────────────────────────────────────────────────────────────────

    async send() {
      const content = this.input.trim()
      if ((!content && this.pendingAtts.length === 0) || this.streaming) return
      if (this.uploading) return

      // If no conversation yet, create one first
      let convId = this.conversationId
      if (!convId) {
        const rc = await fetch('/api/v1/conversations', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ title: '' }),
        })
        if (!rc.ok) return
        const jc = await rc.json()
        convId = jc.data.id
        Alpine.store('app').conversationId = convId
        Alpine.store('app').conversationTitle = ''
        // Refresh sidebar
        document.dispatchEvent(new CustomEvent('conv-created'))
        await new Promise(r => setTimeout(r, 100))
      }

      const attIds = this.pendingAtts.map(a => a.id)
      const attsSnapshot = [...this.pendingAtts]
      this.input = ''
      this.pendingAtts = []

      const r = await fetch(`/api/v1/conversations/${convId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content, attachmentIds: attIds }),
      })
      if (!r.ok) return

      const j = await r.json()

      // Push optimistic user message
      const userBlocks = []
      if (content) userBlocks.push({ type: 'text', content })
      for (const a of attsSnapshot) userBlocks.push({ type: 'attachment', fileName: a.fileName, mimeType: a.mimeType, id: a.id })
      this.messages.push({ id: j.data.messageId, role: 'user', blocks: userBlocks, status: 'completed' })

      // Placeholder assistant message
      this._streamMsgId = 'stream-' + Date.now()
      this.messages.push({ id: this._streamMsgId, role: 'assistant', items: [], status: 'streaming' })
      this.streaming = true
      this._scrollBottom()
    },

    async cancel() {
      const id = this.conversationId
      if (!id) return
      await fetch(`/api/v1/conversations/${id}/stream`, { method: 'DELETE' })
    },

    // ── SSE ───────────────────────────────────────────────────────────────────

    _connectSSE(id) {
      this._closeSSE()
      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      const _msg = () => this.messages.find(m => m.id === this._streamMsgId)
      const _lastOfType = (type) => {
        const msg = _msg(); if (!msg) return null
        const items = msg.items
        return (items.length && items[items.length - 1].type === type)
          ? items[items.length - 1] : null
      }

      // Reasoning token (DeepSeek-R1 etc.)
      es.addEventListener('chat.reasoning_token', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        let item = _lastOfType('reasoning')
        if (!item) { item = { type: 'reasoning', content: '', done: false }; msg.items.push(item) }
        item.content += d.delta
        this._scrollBottom()
      })

      // Text token
      es.addEventListener('chat.token', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        const r = msg.items.find(i => i.type === 'reasoning' && !i.done)
        if (r) r.done = true
        let item = _lastOfType('text')
        if (!item) { item = { type: 'text', content: '' }; msg.items.push(item) }
        item.content += d.delta
        this._scrollBottom()
      })

      // Tool call start — show "calling X…" immediately before args arrive
      es.addEventListener('chat.tool_call_start', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        const item = {
          type: 'tool', toolCallId: d.toolCallId, toolName: d.toolName,
          summary: '', input: null, result: null, ok: null,
        }
        msg.items.push(item)
        this._pendingToolItems[d.toolCallId] = item
        this._scrollBottom()
      })

      // Tool call — arguments now complete
      es.addEventListener('chat.tool_call', e => {
        const d = JSON.parse(e.data)
        const msg = _msg(); if (!msg) return
        let item = this._pendingToolItems[d.toolCallId]
        if (item) {
          // Upgrade the existing placeholder
          item.summary = d.summary || ''
          item.input = d.toolInput
        } else {
          // Fallback: no start event received
          item = { type: 'tool', toolCallId: d.toolCallId, toolName: d.toolName,
                   summary: d.summary || '', input: d.toolInput, result: null, ok: null }
          msg.items.push(item)
          this._pendingToolItems[d.toolCallId] = item
        }
        // Open a text slot for post-tool text
        msg.items.push({ type: 'text', content: '' })
        this._scrollBottom()
      })

      // Tool result
      es.addEventListener('chat.tool_result', e => {
        const d = JSON.parse(e.data)
        const item = this._pendingToolItems[d.toolCallId]
        if (item) { item.result = d.result; item.ok = d.ok }
      })

      // Done
      es.addEventListener('chat.done', e => {
        const data = JSON.parse(e.data)
        const msg = _msg()
        if (msg) {
          while (msg.items.length
                 && msg.items[msg.items.length - 1].type === 'text'
                 && !msg.items[msg.items.length - 1].content) {
            msg.items.pop()
          }
          msg.status = data.stopReason === 'cancelled' ? 'cancelled' : 'completed'
          msg.stopReason = data.stopReason
          msg.inputTokens = data.inputTokens
          msg.outputTokens = data.outputTokens
        }
        this.streaming = false
        this._streamMsgId = null
        this._pendingToolItems = {}
        this.loadMessages(id)
      })

      // Error
      es.addEventListener('chat.error', e => {
        const data = JSON.parse(e.data)
        const msg = _msg()
        if (msg) { msg.status = 'error'; msg.errorMsg = data.message }
        this.streaming = false
        this._streamMsgId = null
      })

      // Auto-title update
      es.addEventListener('conversation.title_updated', e => {
        Alpine.store('app').conversationTitle = JSON.parse(e.data).title
        document.dispatchEvent(new CustomEvent('conv-created'))  // trigger sidebar reload
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
      if (s === null || s === undefined) return '…'
      try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return s }
    },

    attIcon(mimeType) {
      if (!mimeType) return '📎'
      if (mimeType.startsWith('image/')) return '🖼'
      if (mimeType === 'application/pdf') return '📄'
      if (mimeType.includes('spreadsheet') || mimeType.includes('excel')) return '📊'
      return '📎'
    },

    handleKeydown(e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.send() }
    },
  }))
})
