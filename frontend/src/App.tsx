import { useState, useRef, useCallback, useEffect } from "react";
import { InboxProvider } from "./context/InboxContext";
import { ChatProvider } from "./context/ChatContext";
import { LocaleProvider } from "./lib/i18n";
import { SidebarNav, Tab } from "./components/SidebarNav";
import { HomeLeftPanel, HomeContent } from "./pages/HomePage";
import { ChatLeftPanel, ChatContent } from "./pages/ChatPage";
import { AssetsLeftPanel, AssetsContent } from "./pages/AssetsPage";
import { InboxLeftPanel, InboxContent } from "./pages/InboxPage";
import { SettingsLeftPanel, SettingsContent } from "./pages/SettingsPage";

const TITLE_BAR_HEIGHT = 40;
const SIDEBAR_DEFAULT = 280;
const SIDEBAR_MIN = 280;
const SIDEBAR_MAX = 450;

function LeftPanel({ tab }: { tab: Tab }) {
  switch (tab) {
    case "home":     return <HomeLeftPanel />;
    case "chat":     return <ChatLeftPanel />;
    case "assets":   return <AssetsLeftPanel />;
    case "inbox":    return <InboxLeftPanel />;
    case "settings": return <SettingsLeftPanel />;
  }
}

function MainContent({ tab }: { tab: Tab }) {
  switch (tab) {
    case "home":     return <HomeContent />;
    case "chat":     return <ChatContent />;
    case "assets":   return <AssetsContent />;
    case "inbox":    return <InboxContent />;
    case "settings": return <SettingsContent />;
  }
}

function App() {
  const [activeTab, setActiveTab] = useState<Tab>("home");
  const [sidebarWidth, setSidebarWidth] = useState(SIDEBAR_DEFAULT);
  const [titleBarHeight, setTitleBarHeight] = useState(TITLE_BAR_HEIGHT);
  const dragging = useRef(false);

  useEffect(() => {
    const unsubscribe = window.electronAPI?.onFullscreenChange((isFullscreen) => {
      setTitleBarHeight(isFullscreen ? 0 : TITLE_BAR_HEIGHT);
    });
    return () => unsubscribe?.();
  }, []);

  // Allow other components to navigate via custom events
  useEffect(() => {
    const handler = (e: Event) => {
      setActiveTab((e as CustomEvent).detail as Tab);
    };
    window.addEventListener("nav:goTo", handler);
    return () => window.removeEventListener("nav:goTo", handler);
  }, []);

  const onResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    dragging.current = true;
    const startX = e.clientX;
    const startWidth = sidebarWidth;

    const onMove = (ev: MouseEvent) => {
      if (!dragging.current) return;
      const next = startWidth + (ev.clientX - startX);
      setSidebarWidth(Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, next)));
    };
    const onUp = () => {
      dragging.current = false;
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  }, [sidebarWidth]);

  return (
    <LocaleProvider>
    <InboxProvider>
    <ChatProvider>
      <div className="flex h-screen w-screen overflow-hidden bg-white text-gray-900">
        {titleBarHeight > 0 && (
          <div
            style={{ height: titleBarHeight, WebkitAppRegion: "drag" } as React.CSSProperties}
            className="fixed top-0 left-0 right-0 z-50"
          />
        )}
        <aside
          style={{ width: sidebarWidth, paddingTop: titleBarHeight }}
          className="flex flex-col flex-shrink-0 bg-white relative transition-[padding-top] duration-200"
        >
          <SidebarNav active={activeTab} onSelect={setActiveTab} />
          <div className="flex-1 overflow-y-auto">
            <LeftPanel tab={activeTab} />
          </div>
          <div
            onMouseDown={onResizeStart}
            className="absolute right-0 top-0 bottom-0 w-1 cursor-col-resize hover:bg-blue-400/30 active:bg-blue-400/50 transition-colors"
          />
        </aside>

        <div className="w-px bg-gray-200 flex-shrink-0" />

        <main className="flex-1 min-w-0 h-full overflow-hidden bg-white">
          <MainContent tab={activeTab} />
        </main>
      </div>
    </ChatProvider>
    </InboxProvider>
    </LocaleProvider>
  );
}

export default App;
