import { contextBridge, ipcRenderer } from "electron";

contextBridge.exposeInMainWorld("electronAPI", {
  platform: process.platform,
  onFullscreenChange: (callback: (isFullscreen: boolean) => void) => {
    const handler = (_: unknown, isFullscreen: boolean) => callback(isFullscreen);
    ipcRenderer.on("fullscreen-change", handler);
    return () => ipcRenderer.removeListener("fullscreen-change", handler);
  },
});
