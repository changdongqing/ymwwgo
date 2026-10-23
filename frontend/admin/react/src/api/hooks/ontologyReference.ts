import { useQuery, type UseQueryOptions } from '@tanstack/react-query';
import type {
  ontologyservicev1_ListReferencesRequest,
  ontologyservicev1_ListReferencesResponse,
} from '@/api/generated/admin/service/v1';
import { apiClient } from '@/api/client';

/**
 * 通用反查 hook（08 期）：单端点 `OntologyReferenceService.ListReferences`。
 *
 * GET query 绑定：repeated 字段由生成客户端展开为重复参数（axiomTypes=61&axiomTypes=62、
 * positions=REFERENCE_POSITION_OBJECT&...），服务端解码为数组——不需要手工拼字符串。
 * 请求体是扁平对象（本 RPC 非 CRUD，铁律 3 不适用 { data } 包裹）。
 */
export function useOntologyReferences(
  req: ontologyservicev1_ListReferencesRequest,
  options?: Omit<
    UseQueryOptions<ontologyservicev1_ListReferencesResponse, Error>,
    'queryKey' | 'queryFn'
  >,
) {
  return useQuery({
    queryKey: ['ontologyReferences', req],
    queryFn: () => apiClient.ontologyReferenceService.ListReferences(req),
    enabled:
      options?.enabled ??
      (req.ontologyId != null && req.ontologyId > 0 && req.targetId != null && req.targetId > 0),
    retry: 0,
    ...options,
  });
}
