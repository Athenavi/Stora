/**
 * 上传工具函数 — 与 Qwik 组件分离，避免优化器分析问题
 */
import { uploadFile, checkUpload, initChunkedUpload, uploadChunk, completeUpload } from "~/lib/api";

export interface TaskState {
  id: string; filename: string; size: number; progress: number;
  status: "pending" | "checking" | "uploading" | "completed" | "error";
  error?: string;
}

const fileRefs = new Map<string, File>();

export function storeFile(id: string, file: File) { fileRefs.set(id, file); }

async function sha256Full(file: File): Promise<string> {
  // Compute full-file SHA-256 hash using chunked reading to avoid memory blowup
  const CHUNK_SIZE = 10 * 1024 * 1024; // 10MB
  const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
  const hashPromises: Promise<ArrayBuffer>[] = [];

  for (let i = 0; i < totalChunks; i++) {
    const start = i * CHUNK_SIZE;
    const end = Math.min(start + CHUNK_SIZE, file.size);
    hashPromises.push(file.slice(start, end).arrayBuffer());
  }

  const chunks = await Promise.all(hashPromises);
  const merged = new Uint8Array(chunks.reduce((acc, c) => acc + c.byteLength, 0));
  let offset = 0;
  for (const c of chunks) {
    merged.set(new Uint8Array(c), offset);
    offset += c.byteLength;
  }
  const h = await crypto.subtle.digest("SHA-256", merged);
  return Array.from(new Uint8Array(h)).map(b => b.toString(16).padStart(2, "0")).join("");
}

export async function processUploadTask(
  t: TaskState,
  folderId?: number | null,
  onProgress?: () => void,
) {
  const file = fileRefs.get(t.id);
  if (!file) { t.status = "error"; t.error = "文件丢失"; onProgress?.(); return; }
  try {
    t.status = "checking"; onProgress?.();
    if (file.size <= 419430400) {
      const hash = await sha256Full(file);
      await checkUpload(hash, file.name, file.size).catch(() => null);
    }
    t.status = "uploading"; onProgress?.();
    if (file.size > 10485760) {
      const totalChunks = Math.ceil(file.size / 4194304);
      const init = await initChunkedUpload({ filename: file.name, total_size: file.size, total_chunks: totalChunks, folder_id: folderId });
      for (let ci = 0; ci < totalChunks; ci++) {
        const start = ci * 4194304;
        await uploadChunk(init.upload_id, ci, file.slice(start, Math.min(start + 4194304, file.size)));
        t.progress = Math.round(((ci + 1) / totalChunks) * 90);
        onProgress?.();
      }
      await completeUpload(init.upload_id, folderId);
    } else {
      await uploadFile(file, folderId, (pct) => { t.progress = pct; onProgress?.(); });
    }
    t.progress = 100; t.status = "completed"; onProgress?.();
    fileRefs.delete(t.id);
  } catch (e: any) {
    t.status = "error"; t.error = e.message || "上传失败"; onProgress?.();
  }
}
