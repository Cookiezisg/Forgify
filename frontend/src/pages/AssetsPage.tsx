export function AssetsLeftPanel() {
  return (
    <div className="flex flex-col p-3 gap-1">
      <p className="text-[11px] font-semibold text-gray-400 uppercase tracking-wider px-2 mb-1">工具 / 工作流</p>
      <div className="flex items-center justify-center py-8">
        <span className="text-gray-300 text-xs">C1/D1 · 待实现</span>
      </div>
    </div>
  );
}

export function AssetsContent() {
  return (
    <div className="flex flex-col items-center justify-center h-full gap-4">
      <p className="text-xl font-semibold text-gray-700">选择一个工具或工作流</p>
      <p className="text-sm text-gray-400">C2/D2 · 待实现</p>
    </div>
  );
}
