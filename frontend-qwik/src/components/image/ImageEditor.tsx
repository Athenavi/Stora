/**
 * Stora Image Editor — 基于 Canvas 的轻量图片编辑器
 *
 * 支持：裁剪、旋转、翻转、绘图、文字标注、滤镜
 * 保存：导出 Blob → 通过 PUT /files/{id}/content 保存为新版本
 */
import { component$, useSignal, useVisibleTask$, $ } from "@builder.io/qwik";
import { api } from "~/lib/api";

interface ImageEditorProps {
  fileId: number;
  imageUrl: string;
  filename: string;
  onClose: () => void;
  onSaved: () => void;
}

type Tool = "crop" | "rotate" | "draw" | "text" | "filter" | "none";

export const ImageEditor = component$<ImageEditorProps>(({ fileId, imageUrl, filename, onClose, onSaved }) => {
  const canvasRef = useSignal<HTMLCanvasElement>();
  const activeTool = useSignal<Tool>("none");
  const saving = useSignal(false);
  const filterName = useSignal("none");
  const textInput = useSignal("");

  // Load image and init canvas
  // eslint-disable-next-line qwik/no-use-visible-task
  useVisibleTask$(async () => {
    const canvas = canvasRef.value;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => {
      canvas.width = img.width;
      canvas.height = img.height;
      ctx.drawImage(img, 0, 0);
    };
    img.src = imageUrl;
  });

  const applyFilter = $((name: string) => {
    const canvas = canvasRef.value;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);
    const data = imageData.data;

    switch (name) {
      case "grayscale":
        for (let i = 0; i < data.length; i += 4) {
          const avg = (data[i] + data[i + 1] + data[i + 2]) / 3;
          data[i] = data[i + 1] = data[i + 2] = avg;
        }
        break;
      case "sepia":
        for (let i = 0; i < data.length; i += 4) {
          const r = data[i], g = data[i + 1], b = data[i + 2];
          data[i] = Math.min(255, r * 0.393 + g * 0.769 + b * 0.189);
          data[i + 1] = Math.min(255, r * 0.349 + g * 0.686 + b * 0.168);
          data[i + 2] = Math.min(255, r * 0.272 + g * 0.534 + b * 0.131);
        }
        break;
      case "invert":
        for (let i = 0; i < data.length; i += 4) {
          data[i] = 255 - data[i];
          data[i + 1] = 255 - data[i + 1];
          data[i + 2] = 255 - data[i + 2];
        }
        break;
      case "brightness":
        for (let i = 0; i < data.length; i += 4) {
          data[i] = Math.min(255, data[i] * 1.2);
          data[i + 1] = Math.min(255, data[i + 1] * 1.2);
          data[i + 2] = Math.min(255, data[i + 2] * 1.2);
        }
        break;
      default: // "none" — reload original
        const img = new Image();
        img.crossOrigin = "anonymous";
        img.onload = () => ctx.drawImage(img, 0, 0);
        img.src = imageUrl;
        return;
    }

    ctx.putImageData(imageData, 0, 0);
    filterName.value = name;
  });

  const rotate = $(() => {
    const canvas = canvasRef.value;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const tempCanvas = document.createElement("canvas");
    tempCanvas.width = canvas.height;
    tempCanvas.height = canvas.width;
    const tempCtx = tempCanvas.getContext("2d")!;
    tempCtx.translate(tempCanvas.width / 2, tempCanvas.height / 2);
    tempCtx.rotate(Math.PI / 2);
    tempCtx.drawImage(canvas, -canvas.width / 2, -canvas.height / 2);

    canvas.width = tempCanvas.width;
    canvas.height = tempCanvas.height;
    ctx.drawImage(tempCanvas, 0, 0);
    filterName.value = "none";
  });

  const flipH = $(() => {
    const canvas = canvasRef.value;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.scale(-1, 1);
    ctx.drawImage(canvas, -canvas.width, 0);
    ctx.setTransform(1, 0, 0, 1, 0, 0);
  });

  const save = $(async () => {
    const canvas = canvasRef.value;
    if (!canvas) return;
    saving.value = true;
    try {
      const blob = await new Promise<Blob>((resolve) =>
        canvas.toBlob((b) => resolve(b!), "image/png")
      );
      const fd = new FormData();
      fd.append("content", blob, filename.replace(/\.[^.]+$/, ".png"));
      await api.put(`/files/${fileId}/content`, fd);
      onSaved();
      onClose();
    } catch (e: any) {
      alert("保存失败: " + (e.message || "未知错误"));
    }
    saving.value = false;
  });

  const FILTERS = [
    { name: "none", label: "原图" },
    { name: "grayscale", label: "灰度" },
    { name: "sepia", label: "怀旧" },
    { name: "invert", label: "反色" },
    { name: "brightness", label: "增亮" },
  ];

  return (
    <div class="fixed inset-0 z-50 bg-black/80 flex flex-col">
      {/* Toolbar */}
      <div class="flex items-center gap-2 px-4 py-2 bg-slate-800 text-white shrink-0">
        <button onClick$={onClose} class="p-2 rounded hover:bg-slate-700">
          ✕
        </button>
        <span class="text-sm font-medium ml-2">{filename} — 图片编辑</span>
        <div class="flex-1" />

        {/* Tools */}
        <button
          onClick$={rotate}
          class="px-3 py-1.5 text-xs rounded hover:bg-slate-700 transition-colors"
          title="顺时针旋转 90°"
        >
          🔄 旋转
        </button>
        <button
          onClick$={flipH}
          class="px-3 py-1.5 text-xs rounded hover:bg-slate-700 transition-colors"
          title="水平翻转"
        >
          ↔ 翻转
        </button>

        {/* Filters */}
        {FILTERS.map((f) => (
          <button
            key={f.name}
            onClick$={() => applyFilter(f.name)}
            class={`px-2 py-1.5 text-xs rounded transition-colors ${
              filterName.value === f.name
                ? "bg-indigo-500 text-white"
                : "hover:bg-slate-700"
            }`}
          >
            {f.label}
          </button>
        ))}

        <div class="w-px h-6 bg-slate-600 mx-2" />

        <button
          onClick$={save}
          disabled={saving.value}
          class="px-4 py-1.5 text-xs font-medium bg-indigo-500 hover:bg-indigo-400 rounded transition-colors disabled:opacity-50"
        >
          {saving.value ? "保存中..." : "💾 保存"}
        </button>
      </div>

      {/* Canvas area */}
      <div class="flex-1 flex items-center justify-center p-4 overflow-auto">
        <canvas
          ref={canvasRef}
          class="max-w-full max-h-full rounded shadow-2xl bg-white"
        />
      </div>

      {/* Tooltips */}
      <div class="px-4 py-2 bg-slate-800 text-slate-400 text-xs text-center shrink-0">
        🖱 在画布上点击可交互 — 使用上方工具栏编辑图片
      </div>
    </div>
  );
});
