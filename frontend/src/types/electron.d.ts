interface ElectronAPI {
  platform: string;
  onFullscreenChange: (callback: (isFullscreen: boolean) => void) => (() => void);
}

declare global {
  interface Window {
    electronAPI?: ElectronAPI;
  }
}

export {};
