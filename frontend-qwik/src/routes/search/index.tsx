/**
 * Stora Search — 文件名与内容搜索
 *
 * 后端支持三种模式：all（默认，文件名 + 内容）、name（仅文件名）、
 * content（仅索引内容：标签、评论、字幕文本等）。
 */
import { component$, useSignal } from "@builder.io/qwik";
import { routeLoader$, useLocation, Link } from "@builder.io/qwik-city";
import { createServerApi, type FileItem } from "~/lib/api";
import { Icon } from "~/components/ui/Icon";
import PreviewModal from "~/components/drive/PreviewModal";

type SearchMode = "all" | "name" | "content";

interface SearchHit extends FileItem {
  /** Why this file matched: its indexed content, or its name. */
  match?: "content" | "name";
  score?: number;
}

export const useSearch = routeLoader$(async ({ url, request }) => {
  const q = url.searchParams.get("q");
  if (!q) return null;
  const raw = url.searchParams.get("mode");
  const mode: SearchMode = raw === "name" || raw === "content" ? raw : "all";
  const srv = createServerApi(request);
  const qs = new URLSearchParams({ q, mode });
  return await srv
    .get<{ items: SearchHit[]; query: string; mode: SearchMode }>("/files/search?" + qs.toString())
    .catch(() => null);
});

function fmtSize(b: number): string {
  if (!b) return "0 B";
  const k = 1024;
  const i = Math.floor(Math.log(b) / Math.log(k));
  return parseFloat((b / Math.pow(k, i)).toFixed(1)) + " " + ["B", "KB", "MB", "GB", "TB"][i];
}

/**
 * 高亮命中片段。
 *
 * 用分段渲染而不是 innerHTML —— 搜索词来自 URL，直接注入 HTML 就是 XSS。
 */
function highlight(text: string, term: string) {
  if (!term) return <>{text}</>;
  const haystack = text.toLowerCase();
  const needle = term.toLowerCase();
  if (!needle || !haystack.includes(needle)) return <>{text}</>;

  const parts: (string | any)[] = [];
  let cursor = 0;
  for (;;) {
    const at = haystack.indexOf(needle, cursor);
    if (at === -1) {
      parts.push(text.slice(cursor));
      break;
    }
    if (at > cursor) parts.push(text.slice(cursor, at));
    parts.push(<mark class="bg-amber-200 text-slate-900 rounded-sm px-0.5">{text.slice(at, at + needle.length)}</mark>);
    cursor = at + needle.length;
  }
  return <>{parts}</>;
}

const MODES: { key: SearchMode; label: string; hint: string }[] = [
  { key: "all", label: "全部", hint: "文件名 + 内容" },
  { key: "name", label: "文件名", hint: "仅按文件名匹配" },
  { key: "content", label: "内容", hint: "标签、评论、字幕等索引文本" },
];

export default component$(() => {
  const data = useSearch();
  const previewFile = useSignal<any>(null);
  const loc = useLocation();
  const q = loc.url.searchParams.get("q") || "";
  const mode: SearchMode = data.value?.mode ?? "all";

  return (
    <div class="flex flex-col h-full">
      <div class="px-6 py-4 border-b border-slate-200 bg-white">
        <h1 class="text-lg font-semibold text-slate-900">搜索：{q}</h1>
        <p class="text-sm text-slate-500 mt-0.5">{data.value?.items.length || 0} 个结果</p>
        <div class="flex gap-1 mt-3">
          {MODES.map((m) => (
            <Link
              key={m.key}
              href={`/search?q=${encodeURIComponent(q)}&mode=${m.key}`}
              title={m.hint}
              class={`px-3 py-1.5 text-xs rounded-md border transition-colors ${
                mode === m.key
                  ? "bg-stora-primary text-white border-stora-primary"
                  : "bg-white text-slate-600 border-slate-200 hover:bg-slate-50"
              }`}
            >
              {m.label}
            </Link>
          ))}
        </div>
      </div>

      <div class="flex-1 overflow-auto scrollbar-thin p-6">
        {!data.value ? (
          <div class="text-center text-slate-400 mt-20"><p>输入关键词开始搜索</p></div>
        ) : data.value.items.length === 0 ? (
          <div class="text-center text-slate-400 mt-20">
            <div class="text-5xl mb-4">🔍</div>
            <p>未找到匹配的文件</p>
            {mode === "name" && <p class="text-xs mt-2">试试切到「全部」或「内容」，标签、评论与字幕文本也能被搜到</p>}
          </div>
        ) : (
          <div class="space-y-2">
            {data.value.items.map((f: SearchHit) => (
              <div
                key={f.id}
                onClick$={() => (previewFile.value = f)}
                class="flex items-center gap-4 px-4 py-3 bg-white rounded-lg border border-slate-200 hover:shadow-sm transition-all cursor-pointer"
              >
                <div class="w-10 h-10 rounded-lg bg-slate-100 flex items-center justify-center text-lg">
                  {f.is_folder ? "📁" : "📄"}
                </div>
                <div class="flex-1 min-w-0">
                  <p class="text-sm font-medium text-slate-700 truncate">
                    {highlight(f.filename || "", q)}
                  </p>
                  <p class="text-xs text-slate-400">
                    {fmtSize(f.file_size || 0)} · {f.file_type}
                    {f.match === "content" && (
                      <span class="ml-2 px-1.5 py-0.5 rounded bg-emerald-50 text-emerald-700">内容命中</span>
                    )}
                  </p>
                </div>
                <Icon name="chevronRight" size={16} class="text-slate-300" />
              </div>
            ))}
          </div>
        )}
      </div>

      {previewFile.value && (
        <PreviewModal fileId={previewFile.value.id} onClose$={() => (previewFile.value = null)} />
      )}
    </div>
  );
});
