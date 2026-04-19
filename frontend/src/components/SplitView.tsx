import { ReactNode, useState, useRef, useCallback } from "react";

interface SplitViewProps {
  left: ReactNode;
  right: ReactNode;
  defaultLeftWidth?: number;
  minLeft?: number;
  minRight?: number;
}

export function SplitView({
  left,
  right,
  defaultLeftWidth = 320,
  minLeft = 200,
  minRight = 320,
}: SplitViewProps) {
  const [leftWidth, setLeftWidth] = useState(defaultLeftWidth);
  const containerRef = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    dragging.current = true;

    const onMouseMove = (ev: MouseEvent) => {
      if (!dragging.current || !containerRef.current) return;
      const rect = containerRef.current.getBoundingClientRect();
      const newWidth = ev.clientX - rect.left;
      const maxLeft = rect.width - minRight;
      setLeftWidth(Math.max(minLeft, Math.min(newWidth, maxLeft)));
    };

    const onMouseUp = () => {
      dragging.current = false;
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };

    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
  }, [minLeft, minRight]);

  return (
    <div ref={containerRef} className="flex h-full w-full overflow-hidden">
      <div style={{ width: leftWidth, flexShrink: 0 }} className="h-full overflow-y-auto">
        {left}
      </div>
      <div
        className="w-px bg-gray-200 cursor-col-resize hover:bg-blue-400/30 flex-shrink-0 transition-colors"
        onMouseDown={onMouseDown}
      />
      <div className="flex-1 min-w-0 h-full overflow-y-auto">
        {right}
      </div>
    </div>
  );
}
