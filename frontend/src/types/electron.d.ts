interface Window {
  electronAPI?: {
    platform: string
    onFullscreenChange: (callback: (isFullscreen: boolean) => void) => () => void
  }
}
