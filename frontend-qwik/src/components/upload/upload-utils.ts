/**
 * 上传工具函数 — 与 Qwik 组件分离，避免优化器分析问题
 */
import { uploadFile, initChunkedUpload, uploadChunk, completeUpload } from "~/lib/api";

export interface TaskState {
  id: string; filename: string; size: number; progress: number;
  status: "pending" | "checking" | "uploading" | "completed" | "error";
  error?: string;
}

const fileRefs = new Map<string, File>();

export function storeFile(id: string, file: File) { fileRefs.set(id, file); }

/** 分片大小，必须与后端的分片上传约定一致。 */
const CHUNK_SIZE = 4194304; // 4 MiB
/** 并发分片数。后端 UploadChunk 支持乱序到达并按 chunk_hash 去重，所以并发是安全的。 */
const UPLOAD_CONCURRENCY = 3;
const MAX_CHUNK_RETRIES = 3;
/** 分片阶段占进度条的 0–90%，末尾的合并落库占 90–100%。 */
const CHUNK_PHASE_PERCENT = 90;
/** 超过该大小才走分片上传。 */
const CHUNKED_UPLOAD_THRESHOLD = 10485760; // 10 MiB

/** 单个分片上传，带指数退避重试（网络抖动时不必让整个文件重来）。 */
async function uploadChunkWithRetry(uploadId: string, index: number, blob: Blob): Promise<void> {
  let lastErr: unknown;
  for (let attempt = 0; attempt <= MAX_CHUNK_RETRIES; attempt++) {
    if (attempt > 0) {
      await new Promise((resolve) => setTimeout(resolve, 300 * 2 ** (attempt - 1)));
    }
    try {
      await uploadChunk(uploadId, index, blob);
      return;
    } catch (err) {
      lastErr = err;
    }
  }
  throw lastErr;
}

/**
 * 以固定并发度上传分片，并跳过服务端已持有的分片（断点续传）。
 *
 * 原实现是串行 `await`：每个分片都要等上一个 RTT 结束，在正常带宽下只用到很小一部分
 * 吞吐。并发池把这一点补上，同时保持内存占用恒定——分片是 slice 出来的引用，
 * 不会把整个文件读进内存。
 */
async function uploadChunks(
  uploadId: string,
  file: File,
  totalChunks: number,
  alreadyReceived: number[],
  onProgress: (done: number, total: number) => void,
): Promise<void> {
  const received = new Set(alreadyReceived);
  const pending = totalChunks - received.size;
  let nextIndex = 0;
  let done = received.size;
  onProgress(done, totalChunks);

  const worker = async () => {
    for (;;) {
      const index = nextIndex++;
      if (index >= totalChunks) return;
      if (received.has(index)) continue;

      const start = index * CHUNK_SIZE;
      await uploadChunkWithRetry(
        uploadId,
        index,
        file.slice(start, Math.min(start + CHUNK_SIZE, file.size)),
      );

      done++;
      onProgress(done, totalChunks);
    }
  };

  const workerCount = Math.max(1, Math.min(UPLOAD_CONCURRENCY, pending));
  await Promise.all(Array.from({ length: workerCount }, worker));
}

export async function processUploadTask(
  t: TaskState,
  folderId?: number | null,
  onProgress?: () => void,
) {
  const file = fileRefs.get(t.id);
  if (!file) { t.status = "error"; t.error = "文件丢失"; onProgress?.(); return; }
  try {
    t.status = "uploading"; onProgress?.();

    if (file.size > CHUNKED_UPLOAD_THRESHOLD) {
      const totalChunks = Math.ceil(file.size / CHUNK_SIZE);
      const init = await initChunkedUpload({
        filename: file.name,
        total_size: file.size,
        total_chunks: totalChunks,
        folder_id: folderId,
      });
      await uploadChunks(init.upload_id, file, totalChunks, init.received_chunks ?? [], (done, total) => {
        t.progress = Math.round((done / total) * CHUNK_PHASE_PERCENT);
        onProgress?.();
      });
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
