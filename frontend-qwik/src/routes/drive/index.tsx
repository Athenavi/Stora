import { component$ } from "@builder.io/qwik";

export default component$(() => {
  return (
    <div class="flex flex-col h-full">
      {/* Top Bar */}
      <div class="flex items-center gap-4 p-4 border-b bg-white">
        <div class="flex-1 relative">
          <input
            type="text"
            placeholder="搜索文件..."
            class="w-full max-w-md pl-10 pr-4 py-2 rounded-lg border border-slate-200 bg-slate-50 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
          />
          <span class="absolute left-3 top-2.5 text-slate-400 text-sm">🔍</span>
        </div>

        <button class="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors">
          ↑ 上传
        </button>
        <button class="px-4 py-2 border border-slate-300 text-slate-700 rounded-lg text-sm font-medium hover:bg-slate-50 transition-colors">
          + 新建文件夹
        </button>
      </div>

      {/* Breadcrumb */}
      <div class="flex items-center gap-2 px-4 py-2 text-sm text-slate-500 border-b">
        <span class="text-indigo-600 hover:text-indigo-800 cursor-pointer">我的文件</span>
      </div>

      {/* File List */}
      <div class="flex-1 overflow-auto p-4">
        <div class="text-center text-slate-400 mt-20">
          <div class="text-5xl mb-4">📂</div>
          <h3 class="text-lg font-medium text-slate-500 mb-1">还没有文件</h3>
          <p class="text-sm">点击"上传"按钮或拖拽文件到此处开始使用</p>
        </div>
      </div>
    </div>
  );
});
