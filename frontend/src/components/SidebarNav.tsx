import { Home, MessageCircle, Zap, Inbox, Settings } from "lucide-react";
import { useInbox } from "../context/InboxContext";

export type Tab = "home" | "chat" | "assets" | "inbox" | "settings";

interface SidebarNavProps {
  active: Tab;
  onSelect: (tab: Tab) => void;
}

const TABS: { id: Tab; icon: React.ReactNode; label: string }[] = [
  { id: "home",     icon: <Home size={16} strokeWidth={1.6} />,          label: "首页" },
  { id: "chat",     icon: <MessageCircle size={16} strokeWidth={1.6} />, label: "对话" },
  { id: "assets",   icon: <Zap size={16} strokeWidth={1.6} />,           label: "资产" },
  { id: "inbox",    icon: <Inbox size={16} strokeWidth={1.6} />,         label: "收件箱" },
  { id: "settings", icon: <Settings size={16} strokeWidth={1.6} />,      label: "设置" },
];

export function SidebarNav({ active, onSelect }: SidebarNavProps) {
  const { unreadCount } = useInbox();

  return (
    <div style={{
      width: "100%",
      height: 36,
      padding: "0 8px",
      borderBottom: "1px solid #f3f4f6",
      display: "flex",
      alignItems: "center",
      boxSizing: "border-box",
      flexShrink: 0,
    }}>
      <div style={{ display: "inline-flex", alignItems: "center", gap: 2 }}>
        {TABS.map(({ id, icon, label }) => {
          const isActive = active === id;
          return (
            <button
              key={id}
              onClick={() => onSelect(id)}
              title={isActive ? undefined : label}
              style={{
                position: "relative",
                display: "inline-flex",
                alignItems: "center",
                gap: isActive ? 4 : 0,
                height: 28,
                width: isActive ? "auto" : 28,
                padding: isActive ? "0 10px 0 8px" : 0,
                borderRadius: 999,
                backgroundColor: isActive ? "#ebebeb" : "transparent",
                color: isActive ? "#1f2937" : "#9ca3af",
                border: "none",
                cursor: "pointer",
                transition: "background-color 150ms, color 150ms",
                flexShrink: 0,
              }}
            >
              <span style={{ display: "flex", alignItems: "center", justifyContent: "center", width: 20, height: 20, flexShrink: 0 }}>
                {icon}
              </span>

              {isActive && (
                <span style={{ fontSize: 12.5, fontWeight: 500, whiteSpace: "nowrap" }}>
                  {label}
                </span>
              )}

              {id === "inbox" && unreadCount > 0 && (
                <span style={{
                  position: "absolute", top: 0, right: 0,
                  width: 14, height: 14, borderRadius: "50%",
                  backgroundColor: "#3b82f6", color: "white",
                  fontSize: 9, fontWeight: 700,
                  display: "flex", alignItems: "center", justifyContent: "center",
                }}>
                  {unreadCount > 9 ? "9+" : unreadCount}
                </span>
              )}
            </button>
          );
        })}
      </div>
    </div>
  );
}
