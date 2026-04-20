import { useState, useEffect } from "react";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { InboxProvider } from "./context/InboxContext";
import { ChatProvider } from "./context/ChatContext";
import { TabProvider, useTabContext } from "./context/TabContext";
import { LocaleProvider } from "./lib/i18n";
import { SidebarNav, Tab } from "./components/SidebarNav";
import { TabBar } from "./components/TabBar";
import { LayoutRouter } from "./components/layouts/LayoutRouter";
import { HomeContent } from "./pages/HomePage";
import { InboxContent } from "./pages/InboxPage";
import { SettingsContent } from "./pages/SettingsPage";

const TRAFFIC_LIGHT_HEIGHT = 38; // macOS traffic lights area
const SIDEBAR_WIDTH = 280;

/**
 * Tab-managed content: Chat and Assets use the Tab system for multi-tab browsing.
 * Home, Inbox, and Settings are direct full-page views — no tabs.
 */
function TabManagedContent() {
  const { tabs, activeTabId } = useTabContext();

  if (tabs.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <p style={{ fontSize: 14, color: "#9b9a97" }}>打开一个对话或资产开始工作</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <TabBar />
      <div className="flex-1 overflow-hidden">
        {tabs.map((tab) => (
          <div
            key={tab.id}
            style={{
              display: tab.id === activeTabId ? "flex" : "none",
              height: "100%",
              flexDirection: "column",
            }}
          >
            <LayoutRouter tab={tab} />
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * Main content area: switches between direct pages (Home/Inbox/Settings)
 * and Tab-managed content (Chat/Assets).
 */
function MainContent({ activeNav }: { activeNav: Tab }) {
  switch (activeNav) {
    case "home":
      return <HomeContent />;
    case "chat":
    case "assets":
      return <TabManagedContent />;
    case "inbox":
      return <InboxContent />;
    case "settings":
      return <SettingsContent />;
  }
}

function App() {
  const [activeNav, setActiveNav] = useState<Tab>("home");
  const [isFullscreen, setIsFullscreen] = useState(false);
  const isElectron = !!window.electronAPI;
  // Only need drag padding in Electron non-fullscreen mode, and only on sidebar
  const sidebarTopPadding = isElectron && !isFullscreen ? TRAFFIC_LIGHT_HEIGHT : 0;

  useEffect(() => {
    const unsubscribe = window.electronAPI?.onFullscreenChange((fs) => {
      setIsFullscreen(fs);
    });
    return () => unsubscribe?.();
  }, []);

  // Allow other components to navigate via custom events
  useEffect(() => {
    const handler = (e: Event) => {
      setActiveNav((e as CustomEvent).detail as Tab);
    };
    window.addEventListener("nav:goTo", handler);
    return () => window.removeEventListener("nav:goTo", handler);
  }, []);

  return (
    <ErrorBoundary>
    <LocaleProvider>
    <InboxProvider>
    <ChatProvider>
    <TabProvider>
      <div className="flex h-screen w-screen overflow-hidden bg-white text-gray-900">
        {/* Left sidebar */}
        <aside
          style={{ width: SIDEBAR_WIDTH }}
          className="flex flex-col flex-shrink-0 bg-white relative"
        >
          {/* Drag region: only the top area of sidebar, like Chrome's tab bar */}
          {sidebarTopPadding > 0 && (
            <div
              style={{ height: sidebarTopPadding, WebkitAppRegion: "drag" } as React.CSSProperties}
              className="flex-shrink-0"
            />
          )}
          <SidebarNav active={activeNav} onSelect={setActiveNav} />
          <div className="flex-1 overflow-y-auto">
            <LeftPanel activeNav={activeNav} />
          </div>
        </aside>

        <div className="w-px bg-gray-200 flex-shrink-0" />

        {/* Main content area — no padding, full height */}
        <main className="flex-1 min-w-0 h-full overflow-hidden bg-white">
          <MainContent activeNav={activeNav} />
        </main>
      </div>
    </TabProvider>
    </ChatProvider>
    </InboxProvider>
    </LocaleProvider>
    </ErrorBoundary>
  );
}

/**
 * Left panel content based on active nav selection.
 * Chat and Assets show their list panels; others show minimal or nothing.
 */
function LeftPanel({ activeNav }: { activeNav: Tab }) {
  switch (activeNav) {
    case "home":
      return <HomeLeftPanel />;
    case "chat":
      return <ChatLeftPanel />;
    case "assets":
      return <AssetsLeftPanel />;
    case "inbox":
      return <InboxLeftPanel />;
    case "settings":
      return <SettingsLeftPanel />;
  }
}

import { HomeLeftPanel } from "./pages/HomePage";
import { ChatLeftPanel } from "./pages/ChatPage";
import { AssetsLeftPanel } from "./pages/AssetsPage";
import { InboxLeftPanel } from "./pages/InboxPage";
import { SettingsLeftPanel } from "./pages/SettingsPage";

export default App;
