import { app, BrowserWindow, Tray, Menu, nativeImage, ipcMain } from "electron";
import { spawn, ChildProcess } from "child_process";
import * as path from "path";

const isDev = process.argv.includes("--dev");

let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;
let backendProcess: ChildProcess | null = null;
let backendPort = 0;
let isQuitting = false;

function startBackend(): Promise<number> {
  return new Promise((resolve) => {
    const bin = isDev
      ? path.join(__dirname, "../dist-electron/forgify-backend")
      : path.join(process.resourcesPath, "forgify-backend");

    const proc = spawn(bin, [], { stdio: ["ignore", "pipe", "pipe"] });
    backendProcess = proc;

    proc.stdout?.on("data", (data: Buffer) => {
      const match = data.toString().match(/FORGIFY_PORT=(\d+)/);
      if (match) resolve(parseInt(match[1]));
    });

    proc.stderr?.on("data", (d: Buffer) => console.error("[backend]", d.toString().trim()));
    proc.on("error", () => resolve(0)); // no binary in dev — continue without backend
    setTimeout(() => resolve(0), 8000);
  });
}

async function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 960,
    minHeight: 640,
    titleBarStyle: "hiddenInset",
    trafficLightPosition: { x: 12, y: 10 },
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  const url = isDev
    ? `http://localhost:5173?port=${backendPort}`
    : `file://${path.join(__dirname, "../frontend/dist/index.html")}?port=${backendPort}`;

  await mainWindow.loadURL(url);

  mainWindow.on("enter-full-screen", () => mainWindow?.webContents.send("fullscreen-change", true));
  mainWindow.on("leave-full-screen", () => mainWindow?.webContents.send("fullscreen-change", false));

  mainWindow.on("close", (e) => {
    if (!isQuitting) {
      e.preventDefault();
      mainWindow?.hide();
    }
  });
}

function setupTray() {
  const iconPath = isDev
    ? path.join(__dirname, "../build/trayicon.png")
    : path.join(process.resourcesPath, "trayicon.png");

  let icon: Electron.NativeImage;
  try {
    icon = nativeImage.createFromPath(iconPath);
    if (icon.isEmpty()) icon = nativeImage.createEmpty();
  } catch {
    icon = nativeImage.createEmpty();
  }

  tray = new Tray(icon);
  tray.setToolTip("Forgify");

  tray.on("click", () => {
    if (mainWindow?.isVisible()) {
      mainWindow.hide();
    } else {
      mainWindow?.show();
      mainWindow?.focus();
    }
  });

  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: "显示 Forgify", click: () => { mainWindow?.show(); mainWindow?.focus(); } },
      { type: "separator" },
      { label: "退出 Forgify", click: () => { isQuitting = true; app.quit(); } },
    ])
  );
}

app.whenReady().then(async () => {
  ipcMain.handle("get-backend-port", () => backendPort);

  backendPort = await startBackend();
  await createWindow();
  setupTray();

  app.on("activate", () => {
    mainWindow?.show();
    mainWindow?.focus();
  });
});

app.on("before-quit", () => {
  isQuitting = true;
  backendProcess?.kill();
});

// Keep alive on macOS — quit only via tray
app.on("window-all-closed", () => {
  if (process.platform !== "darwin") app.quit();
});
