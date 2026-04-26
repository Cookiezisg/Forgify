// app.js — Alpine root store and shared utilities.

document.addEventListener('alpine:init', () => {
  Alpine.store('app', {
    conversationId: null,
    conversationTitle: '',
  })
})

function appRoot() {
  return {
    activeRightTab: 'config',
    selectTab(tab) { this.activeRightTab = tab },
  }
}
