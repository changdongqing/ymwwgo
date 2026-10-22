import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_AnnotationPropertyFrame,
  type ontologyservicev1_AnnotationPropertyTreeResponse,
  type ontologyservicev1_CreateAnnotationPropertyRequest,
  type ontologyservicev1_CreateAnnotationPropertyResponse,
  type ontologyservicev1_DeleteAnnotationPropertyRequest,
  type ontologyservicev1_EntityUsage,
  type ontologyservicev1_GetAnnotationPropertyFrameRequest,
  type ontologyservicev1_GetAnnotationPropertyTreeRequest,
  type ontologyservicev1_GetAnnotationPropertyUsageRequest,
  type ontologyservicev1_ListAnnotationPropertiesRequest,
  type ontologyservicev1_ListAnnotationPropertiesResponse,
  type ontologyservicev1_UpdateAnnotationPropertyRequest,
} from '@/api/generated/admin/service/v1';
import { queryClient } from '@/core';
import { apiClient } from '@/api/client';

/**
 * 注解属性 hooks（04 期）：注解属性是自定义 RPC（公理事务管线），请求体收扁平对象——
 * 勿按通用 CRUD 惯例包 { data: {...} }（protojson 会把 data 当未知字段丢弃，
 * 接口 200 但字段全空，铁律 3 已知坑）。
 */

// 全量树（owl:topAnnotationProperty 虚拟根由前端固定渲染；属性树无 nav 剪枝）
export function useOntologyAnnotationPropertyTree(
  ontologyId: number | undefined,
  options?: Omit<UseQueryOptions<ontologyservicev1_AnnotationPropertyTreeResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyAnnotationPropertyTree', ontologyId],
    queryFn: () =>
      apiClient.ontologyAnnotationPropertyService.Tree({
        ontologyId: ontologyId!,
      } as ontologyservicev1_GetAnnotationPropertyTreeRequest),
    enabled: options?.enabled ?? ontologyId != null,
    ...options,
  });
}

// 分页列表（req 手动构建，直传 camelCase 字段，不走 PaginationQuery.toRawParams()——自定义消息）
export function useListAnnotationProperties(
  req: ontologyservicev1_ListAnnotationPropertiesRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ListAnnotationPropertiesResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listAnnotationProperties', req],
    queryFn: () => apiClient.ontologyAnnotationPropertyService.List(req),
    ...options,
  });
}

// 列表：fetch 版（ListTable request 回调里用）
export async function fetchListAnnotationProperties(req: ontologyservicev1_ListAnnotationPropertiesRequest) {
  return queryClient.fetchQuery({
    queryKey: ['listAnnotationProperties', req],
    queryFn: () => apiClient.ontologyAnnotationPropertyService.List(req),
    retry: 0,
  });
}

// frame 详情（含 base_version 与自定义注解区 annotations，提交回传作乐观锁基准）
export function useAnnotationPropertyFrame(
  req: ontologyservicev1_GetAnnotationPropertyFrameRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_AnnotationPropertyFrame, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['annotationPropertyFrame', req],
    queryFn: () => apiClient.ontologyAnnotationPropertyService.Frame(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 引用明细（删除保护弹窗：50 子注解属性 / 100 被注解）
export function useAnnotationPropertyUsage(
  req: ontologyservicev1_GetAnnotationPropertyUsageRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_EntityUsage, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['annotationPropertyUsage', req],
    queryFn: () => apiClient.ontologyAnnotationPropertyService.Usage(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 创建（扁平请求体，禁止包 { data }）
export function useCreateAnnotationProperty(
  options?: UseMutationOptions<
    ontologyservicev1_CreateAnnotationPropertyResponse,
    Error,
    { values: Partial<ontologyservicev1_CreateAnnotationPropertyRequest> }
  >,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyAnnotationPropertyService.Create({ ...values } as ontologyservicev1_CreateAnnotationPropertyRequest),
    ...options,
  });
}

// 更新（扁平 + base_version；frame 全量提交语义：字段缺省 = 不变、空串 = 清空注解、
// annotations 缺省/空数组统一 = 清空自定义注解区）
export function useUpdateAnnotationProperty(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_UpdateAnnotationPropertyRequest> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyAnnotationPropertyService.Update({ ...values } as ontologyservicev1_UpdateAnnotationPropertyRequest),
    ...options,
  });
}

// 删除（force=true 级联移除引用公理：50 子属性边 + 他人挂的自定义注解断言，子属性自动变根属性）
export function useDeleteAnnotationProperty(
  options?: UseMutationOptions<unknown, Error, ontologyservicev1_DeleteAnnotationPropertyRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyAnnotationPropertyService.Delete(data),
    ...options,
  });
}

// Kratos 错误分流复用类页实现（reason 字符串分流语义一致）
export { getOntologyErrorReason } from '@/api/hooks/ontologyClass';
