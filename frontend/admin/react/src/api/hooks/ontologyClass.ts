import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_CreateOntologyClassRequest,
  type ontologyservicev1_CreateOntologyClassResponse,
  type ontologyservicev1_DeleteOntologyClassRequest,
  type ontologyservicev1_GetOntologyClassFrameRequest,
  type ontologyservicev1_GetOntologyClassTreeRequest,
  type ontologyservicev1_GetOntologyClassUsageRequest,
  type ontologyservicev1_ListOntologyClassesRequest,
  type ontologyservicev1_ListOntologyClassesResponse,
  type ontologyservicev1_OntologyClassFrame,
  type ontologyservicev1_OntologyClassNavTreeResponse,
  type ontologyservicev1_OntologyClassTreeResponse,
  type ontologyservicev1_OntologyClassUsage,
  type ontologyservicev1_UpdateOntologyClassRequest,
} from '@/api/generated/admin/service/v1';
import { queryClient } from '@/core';
import { apiClient } from '@/api/client';

/**
 * 本体类 hooks：本体类是自定义 RPC（公理事务管线），请求体收扁平对象——
 * 勿按通用 CRUD 惯例包 { data: {...} }（protojson 会把 data 当未知字段丢弃，
 * 接口 200 但字段全空，铁律 3 已知坑）。
 */

// 全量树
export function useOntologyClassTree(
  ontologyId: number | undefined,
  options?: Omit<UseQueryOptions<ontologyservicev1_OntologyClassTreeResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyClassTree', ontologyId, 'full'],
    queryFn: () =>
      apiClient.ontologyClassService.Tree({
        ontologyId: ontologyId!,
      } as ontologyservicev1_GetOntologyClassTreeRequest),
    enabled: options?.enabled ?? ontologyId != null,
    ...options,
  });
}

// 骨架树（默认左树；深层无子叶子剪枝，节点带 childCount 供 +N 角标）
export function useOntologyClassNavTree(
  ontologyId: number | undefined,
  options?: Omit<UseQueryOptions<ontologyservicev1_OntologyClassNavTreeResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyClassTree', ontologyId, 'nav'],
    queryFn: () =>
      apiClient.ontologyClassService.NavTree({
        ontologyId: ontologyId!,
      } as ontologyservicev1_GetOntologyClassTreeRequest),
    enabled: options?.enabled ?? ontologyId != null,
    ...options,
  });
}

// 分页列表（req 手动构建，直传 camelCase 字段，不走 PaginationQuery.toRawParams()——自定义消息）
export function useListOntologyClasses(
  req: ontologyservicev1_ListOntologyClassesRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ListOntologyClassesResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listOntologyClasses', req],
    queryFn: () => apiClient.ontologyClassService.List(req),
    ...options,
  });
}

// 列表：fetch 版（ListTable request 回调里用）
export async function fetchListOntologyClasses(req: ontologyservicev1_ListOntologyClassesRequest) {
  return queryClient.fetchQuery({
    queryKey: ['listOntologyClasses', req],
    queryFn: () => apiClient.ontologyClassService.List(req),
    retry: 0,
  });
}

// frame 详情（含 base_version，提交回传作乐观锁基准）
export function useOntologyClassFrame(
  req: ontologyservicev1_GetOntologyClassFrameRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_OntologyClassFrame, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyClassFrame', req],
    queryFn: () => apiClient.ontologyClassService.Frame(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 引用明细（删除保护弹窗）
export function useOntologyClassUsage(
  req: ontologyservicev1_GetOntologyClassUsageRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_OntologyClassUsage, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyClassUsage', req],
    queryFn: () => apiClient.ontologyClassService.Usage(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 创建（扁平请求体，禁止包 { data }）
export function useCreateOntologyClass(
  options?: UseMutationOptions<
    ontologyservicev1_CreateOntologyClassResponse,
    Error,
    { values: Partial<ontologyservicev1_CreateOntologyClassRequest> }
  >,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyClassService.Create({ ...values } as ontologyservicev1_CreateOntologyClassRequest),
    ...options,
  });
}

// 更新（扁平 + base_version；frame 全量提交语义：字段缺省 = 不变、空串 = 清空注解）
export function useUpdateOntologyClass(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_UpdateOntologyClassRequest> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyClassService.Update({ ...values } as ontologyservicev1_UpdateOntologyClassRequest),
    ...options,
  });
}

// 删除（force=true 级联移除引用公理，子类自动变根类）
export function useDeleteOntologyClass(
  options?: UseMutationOptions<unknown, Error, ontologyservicev1_DeleteOntologyClassRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyClassService.Delete(data),
    ...options,
  });
}

/**
 * Kratos 错误分流：request-client 把 response.data（{ code, reason, message }）作为抛出对象，
 * reason 字符串可直接读取（前端按 reason 分流，与源按数字错误码识别等价）。
 */
export function getOntologyErrorReason(error: unknown): string | undefined {
  if (error && typeof error === 'object' && 'reason' in error) {
    const reason = (error as { reason?: unknown }).reason;
    if (typeof reason === 'string') return reason;
  }
  return undefined;
}
