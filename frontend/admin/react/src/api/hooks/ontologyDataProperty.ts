import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_CreateDataPropertyRequest,
  type ontologyservicev1_CreateDataPropertyResponse,
  type ontologyservicev1_DataPropertyFrame,
  type ontologyservicev1_DataPropertyTreeResponse,
  type ontologyservicev1_DeleteDataPropertyRequest,
  type ontologyservicev1_EntityUsage,
  type ontologyservicev1_GetDataPropertyFrameRequest,
  type ontologyservicev1_GetDataPropertyTreeRequest,
  type ontologyservicev1_GetDataPropertyUsageRequest,
  type ontologyservicev1_ListDataPropertiesRequest,
  type ontologyservicev1_ListDataPropertiesResponse,
  type ontologyservicev1_UpdateDataPropertyRequest,
} from '@/api/generated/admin/service/v1';
import { queryClient } from '@/core';
import { apiClient } from '@/api/client';

/**
 * 数据属性 hooks：数据属性是自定义 RPC（公理事务管线），请求体收扁平对象——
 * 勿按通用 CRUD 惯例包 { data: {...} }（protojson 会把 data 当未知字段丢弃，
 * 接口 200 但字段全空，铁律 3 已知坑）。
 */

// 全量树（属性树无 nav 剪枝：属性量级小，全量 + antd Tree 虚拟滚动足够）
export function useOntologyDataPropertyTree(
  ontologyId: number | undefined,
  options?: Omit<UseQueryOptions<ontologyservicev1_DataPropertyTreeResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['ontologyDataPropertyTree', ontologyId],
    queryFn: () =>
      apiClient.ontologyDataPropertyService.Tree({
        ontologyId: ontologyId!,
      } as ontologyservicev1_GetDataPropertyTreeRequest),
    enabled: options?.enabled ?? ontologyId != null,
    ...options,
  });
}

// 分页列表（req 手动构建，直传 camelCase 字段，不走 PaginationQuery.toRawParams()——自定义消息）
export function useListDataProperties(
  req: ontologyservicev1_ListDataPropertiesRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ListDataPropertiesResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listDataProperties', req],
    queryFn: () => apiClient.ontologyDataPropertyService.List(req),
    ...options,
  });
}

// 列表：fetch 版（ListTable request 回调里用）
export async function fetchListDataProperties(req: ontologyservicev1_ListDataPropertiesRequest) {
  return queryClient.fetchQuery({
    queryKey: ['listDataProperties', req],
    queryFn: () => apiClient.ontologyDataPropertyService.List(req),
    retry: 0,
  });
}

// frame 详情（含 base_version，提交回传作乐观锁基准）
export function useDataPropertyFrame(
  req: ontologyservicev1_GetDataPropertyFrameRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_DataPropertyFrame, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['dataPropertyFrame', req],
    queryFn: () => apiClient.ontologyDataPropertyService.Frame(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 引用明细（删除保护弹窗）
export function useDataPropertyUsage(
  req: ontologyservicev1_GetDataPropertyUsageRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_EntityUsage, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['dataPropertyUsage', req],
    queryFn: () => apiClient.ontologyDataPropertyService.Usage(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 创建（扁平请求体，禁止包 { data }）
export function useCreateDataProperty(
  options?: UseMutationOptions<
    ontologyservicev1_CreateDataPropertyResponse,
    Error,
    { values: Partial<ontologyservicev1_CreateDataPropertyRequest> }
  >,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyDataPropertyService.Create({ ...values } as ontologyservicev1_CreateDataPropertyRequest),
    ...options,
  });
}

// 更新（扁平 + base_version；frame 全量提交语义：字段缺省 = 不变、空串 = 清空注解）
export function useUpdateDataProperty(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_UpdateDataPropertyRequest> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyDataPropertyService.Update({ ...values } as ontologyservicev1_UpdateDataPropertyRequest),
    ...options,
  });
}

// 删除（force=true 级联移除引用公理，子属性自动变根属性）
export function useDeleteDataProperty(
  options?: UseMutationOptions<unknown, Error, ontologyservicev1_DeleteDataPropertyRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyDataPropertyService.Delete(data),
    ...options,
  });
}

// Kratos 错误分流复用类页实现（reason 字符串分流语义一致）
export { getOntologyErrorReason } from '@/api/hooks/ontologyClass';
