/**
 * E2EE 私密空间 API 封装。
 *
 * 服务端在这条链路上只是**密文仓库**：这里发送与取回的一切都已经是密文，
 * 绝不要把明文、口令或任何密钥交给它。
 */
import { api } from "./api-client";

/** 服务端保存的公开参数与包裹后的密钥（它无法用来解密）。 */
export interface E2eeVault {
  id: number;
  name: string;
  kdf_salt: string;
  kdf_iterations: number;
  wrapped_key: string;
  recovery_wrapped_key: string;
  created_at: string;
  item_count: number;
}

export interface E2eeItem {
  id: number;
  /** 密文文件名；要用 nameKey 解密后才能展示。 */
  enc_name: string;
  /** 密文元数据（含被包裹的 fileKey）。 */
  enc_meta: string;
  /** 密文长度，不是明文大小。 */
  size_cipher: number;
  created_at: string;
}

export const listE2eeVaults = () => api.get<E2eeVault[]>("/e2ee/vaults");

export const createE2eeVault = (payload: {
  name: string;
  kdf_salt: string;
  kdf_iterations: number;
  wrapped_key: string;
  recovery_wrapped_key: string;
}) => api.post<{ id: number }>("/e2ee/vaults", payload);

export const deleteE2eeVault = (vaultId: number) => api.delete(`/e2ee/vaults/${vaultId}`);

export const listE2eeItems = (vaultId: number) => api.get<E2eeItem[]>(`/e2ee/vaults/${vaultId}/items`);

export const uploadE2eeItem = (vaultId: number, cipher: Blob, encName: string, encMeta: string) => {
  const fd = new FormData();
  fd.append("file", cipher, "cipher.bin");
  fd.append("enc_name", encName);
  fd.append("enc_meta", encMeta);
  return api.upload<{ id: number }>(`/e2ee/vaults/${vaultId}/items`, fd);
};

/**
 * 取回密文。
 *
 * 走裸 fetch 而不是 api 客户端：响应体是二进制，api 客户端会当作 JSON 解析。
 */
export async function downloadE2eeItem(itemId: number): Promise<Blob> {
  const res = await fetch(`/api/v2/e2ee/items/${itemId}/content`, { credentials: "same-origin" });
  if (!res.ok) throw new Error("下载失败");
  return res.blob();
}

export const deleteE2eeItem = (itemId: number) => api.delete(`/e2ee/items/${itemId}`);
