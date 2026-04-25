// drag.js — resizable panel dividers.
// Handles the two drag handles between sidebar/chat and chat/tools.

(function () {
  // Wait for DOM before wiring up drag.
  document.addEventListener('DOMContentLoaded', initDrag)

  function initDrag() {
    const layout = document.querySelector('.layout')
    if (!layout) return

    // Initial widths — kept in JS so we can clamp without touching CSS vars.
    let sidebarW = 220
    let toolsW   = 380

    let active    = null  // 'sidebar' | 'tools'
    let startX    = 0
    let startW    = 0

    applyWidths()

    document.querySelectorAll('.drag-handle').forEach(handle => {
      handle.addEventListener('mousedown', e => {
        e.preventDefault()
        active = handle.dataset.panel
        startX = e.clientX
        startW = active === 'sidebar' ? sidebarW : toolsW
        handle.classList.add('dragging')
        document.body.style.cursor    = 'col-resize'
        document.body.style.userSelect = 'none'
      })
    })

    document.addEventListener('mousemove', e => {
      if (!active) return
      const delta = e.clientX - startX
      const totalW = layout.getBoundingClientRect().width

      if (active === 'sidebar') {
        const maxSidebar = totalW - toolsW - 8 - 250 // leave 250px for chat
        sidebarW = clamp(startW + delta, 140, maxSidebar)
      } else {
        // Tools panel is on the right: dragging left (negative delta) grows it.
        const maxTools = totalW - sidebarW - 8 - 250 // leave 250px for chat
        toolsW = clamp(startW - delta, 220, maxTools)
      }
      applyWidths()
    })

    document.addEventListener('mouseup', () => {
      if (!active) return
      document.querySelectorAll('.drag-handle').forEach(h => h.classList.remove('dragging'))
      active = null
      document.body.style.cursor     = ''
      document.body.style.userSelect = ''
    })

    function applyWidths() {
      layout.style.gridTemplateColumns =
        `${sidebarW}px 4px 1fr 4px ${toolsW}px`
    }

    function clamp(val, min, max) {
      return Math.max(min, Math.min(max, val))
    }
  }
})()
