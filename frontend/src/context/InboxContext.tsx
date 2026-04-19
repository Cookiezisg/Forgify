import { createContext, useContext, useState, useEffect, ReactNode } from "react";
import { onEvent, EventNames } from "../lib/events";

interface InboxContextValue {
  unreadCount: number;
  setUnreadCount: (n: number) => void;
}

const InboxContext = createContext<InboxContextValue>({
  unreadCount: 0,
  setUnreadCount: () => {},
});

export function InboxProvider({ children }: { children: ReactNode }) {
  const [unreadCount, setUnreadCount] = useState(0);

  useEffect(() => {
    return onEvent<{ count: number }>(EventNames.MailboxUpdated, (payload) => {
      setUnreadCount(payload.count);
    });
  }, []);

  return (
    <InboxContext.Provider value={{ unreadCount, setUnreadCount }}>
      {children}
    </InboxContext.Provider>
  );
}

export function useInbox() {
  return useContext(InboxContext);
}
