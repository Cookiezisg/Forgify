export function HomeLeftPanel() {
  return (
    <div className="flex flex-col p-3 gap-1">
      <p className="text-[11px] font-semibold text-gray-400 uppercase tracking-wider px-2 mb-1">最近</p>
      <div className="flex items-center justify-center py-8">
        <span className="text-gray-300 text-xs">J1 · 待实现</span>
      </div>
    </div>
  );
}

export function HomeContent() {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-4">
      <p className="text-2xl font-semibold text-gray-800">今天想做什么？</p>
      <p className="text-sm text-gray-400">J1 · 待实现</p>
    </div>
  );
}
