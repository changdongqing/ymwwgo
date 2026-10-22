import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_CreateObjectPropertyRequest,
  type ontologyservicev1_CreateObjectPropertyResponse,
  type ontologyservicev1_DeleteObjectPropertyRequest,
  type ontologyservicev1_GetObjectPropertyFrameRequest,
  type ontologyservicev1_GetObjectPropertyTreeRequest,
  type ontologyservicev1_GetObjectPropertyUsageRequest,
  type ontologyservicev1_ListObjectPropertiesRequest,
  type ontologyservicev1_ListObjectPropertiesResponse,
  type ontologyservicev1_ObjectPropertyFrame,
  type ontologyservicev1_ObjectPropertyTreeResponse,
  type ontologyservicev1_UpdateObjectPropertyRequest,
  type ontologyservicev1_EntityUsage,
} from '@/api/generated/admin/service/v1';
import { queryClient } from '@/core';
import { apiClient } from '@/api/client';

/**
 * 对象属性 hooks：对象属性是自定义 RPC（公理事务管线），请求体收扁平对象——
 * 勿按通用 CRUD 惯例包 { data: {...} }（protojson 会把 data 当未知字段丢弃，
 * 接口 200 但字段全空，铁律 3 已知坑）。
 */

// 全量树（属性树无 nav 剪枝：属性量级小，全量 + antd Tree 虚拟滚动足够）
export function useOntologyObjectPropertyTree(
  ontologyId: number | undefined,
  options?: Omit<UseQueryOptions<ontologyservicev1_ObjectPropertyTreeResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyObjectPropertyTree', ontologyId],
    queryFn: () =>
      apiClient.ontologyObjectPropertyService.Tree({
        ontologyId: ontologyId!,
      } as ontologyservicev1_GetObjectPropertyTreeRequest),
    enabled: options?.enabled ?? ontologyId != null,
    ...options,
  });
}

// 分页列表（req 手动构建，直传 camelCase 字段，不走 PaginationQuery.toRawParams()——自定义消息）
export function useListObjectProperties(
  req: ontologyservicev1_ListObjectPropertiesRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ListObjectPropertiesResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listObjectProperties', req],
    queryFn: () => apiClient.ontologyObjectPropertyService.List(req),
    ...options,
  });
}

// 列表：fetch 版（ListTable request 回调里用）
export async function fetchListObjectProperties(req: ontologyservicev1_ListObjectPropertiesRequest) {
  return queryClient.fetchQuery({
    queryKey: ['listObjectProperties', req],
    queryFn: () => apiClient.ontologyObjectPropertyService.List(req),
    retry: 0,
  });
}

// frame 详情（含 base_version，提交回传作乐观锁基准）
export function useObjectPropertyFrame(
  req: ontologyservicev1_GetObjectPropertyFrameRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ObjectPropertyFrame, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['objectPropertyFrame', req],
    queryFn: () => apiClient.ontologyObjectPropertyService.Frame(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 引用明细（删除保护弹窗）
export function useObjectPropertyUsage(
  req: ontologyservicev1_GetObjectPropertyUsageRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_EntityUsage, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['objectPropertyUsage', req],
    queryFn: () => apiClient.ontologyObjectPropertyService.Usage(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 创建（扁平请求体，禁止包 { data }）
export function useCreateObjectProperty(
  options?: UseMutationOptions<
    ontologyservicev1_CreateObjectPropertyResponse,
    Error,
    { values: Partial<ontologyservicev1_CreateObjectPropertyRequest> }
  >,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyObjectPropertyService.Create({ ...values } as ontologyservicev1_CreateObjectPropertyRequest),
    ...options,
  });
}

// 更新（扁平 + base_version；frame 全量提交语义：字段缺省 = 不变、空串 = 清空注解）
export function useUpdateObjectProperty(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_UpdateObjectPropertyRequest> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyObjectPropertyService.Update({ ...values } as ontologyservicev1_UpdateObjectPropertyRequest),
    ...options,
  });
}

// 删除（force=true 级联移除引用公理，子属性自动变根属性）
export function useDeleteObjectProperty(
  options?: UseMutationOptions<unknown, Error, ontologyservicev1_DeleteObjectPropertyRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyObjectPropertyService.Delete(data),
    ...options,
  });
}

// Kratos 错误分流复用类页实现（reason 字符串分流语义一致）
export { getOntologyErrorReason } from '@/api/hooks/ontologyClass';
