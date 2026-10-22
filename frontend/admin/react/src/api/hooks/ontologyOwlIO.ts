import { useAuthStore } from '@/stores';
import { useMutation, type UseMutationOptions } from '@tanstack/react-query';

import {
  type ontologyservicev1_OwlImportExecuteRequest,
  type ontologyservicev1_OwlImportExecuteResponse,
  type ontologyservicev1_OwlImportPreviewRequest,
  type ontologyservicev1_OwlImportReport,
} from '@/api/generated/admin/service/v1';
import { apiClient } from '@/api/client';

/**
 * OWL 导入导出 hooks（07 期）。
 * 全部为自定义 RPC，请求体收扁平对象——勿包 { data: {...} }（铁律 3 已知坑）。
 */

// 大文件安全转 base64（分块避免 btoa 栈溢出；bytes 字段在 protojson 下即 base64 字符串，
// 对齐 ai-knowledge 上传先例）
export function bytesToBase64(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf);
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return btoa(binary);
}

// useOwlImportPreview — 导入预检（解析 + 反向映射 + 逐行预校验，不落库）
export function useOwlImportPreview(
  options?: UseMutationOptions<ontologyservicev1_OwlImportReport, Error, { fileName: string; contentBase64: string }>,
) {
  return useMutation({
    mutationFn: ({ fileName, contentBase64 }) =>
      apiClient.ontologyOwlIOService.ImportPreview({ fileName, fileContent: contentBase64 } as ontologyservicev1_OwlImportPreviewRequest),
    ...options,
  });
}

// useOwlImportExecute — 导入执行（reportId 一次性消费；ignored.count > 0 时须 confirmedIgnore）
export function useOwlImportExecute(
  options?: UseMutationOptions<ontologyservicev1_OwlImportExecuteResponse, Error, ontologyservicev1_OwlImportExecuteRequest>,
) {
  return useMutation({
    mutationFn: (req) => apiClient.ontologyOwlIOService.ImportExecute(req),
    ...options,
  });
}

// base64ToBytes base64 → 二进制（bytes 响应在 protojson 下即 base64 字符串）
function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

// downloadOwlExport — 导出下载（fetch 直连 + JSON 解析；响应 bytes 字段经 protojson 即
// base64 字符串，文件名随响应体 file_name 下发——Kratos v2 下 HttpBody 无二进制特判，
// 07 号文档 §7 #19 预案回退 bytes 响应）
export async function downloadOwlExport(
  ontologyId: number,
  format: 'rdfxml' | 'turtle',
  fallbackName: string,
): Promise<void> {
  const token = useAuthStore.getState().accessToken;
  const apiBase = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '');
  const res = await fetch(`${apiBase}/admin/v1/ontology/owl:export`, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      ...(token ? { authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ ontologyId, format }),
  });
  if (!res.ok) {
    let detail = `${res.status}`;
    try {
      const body = await res.json();
      detail = body?.message || detail;
    } catch (e) {
      console.error('owl export error body parse failed:', e);
    }
    throw new Error(detail);
  }
  const payload = await res.json();
  const filename: string =
    payload?.fileName || `${fallbackName || 'ontology'}.${format === 'turtle' ? 'ttl' : 'owl'}`;
  const bytes = base64ToBytes(payload?.content ?? '');
  const blob = new Blob([bytes as BlobPart], { type: payload?.contentType || 'application/octet-stream' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  URL.revokeObjectURL(a.href);
}
