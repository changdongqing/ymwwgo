import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_CreateDatatypeRequest,
  type ontologyservicev1_CreateDatatypeResponse,
  type ontologyservicev1_DatatypeFrame,
  type ontologyservicev1_DeleteDatatypeRequest,
  type ontologyservicev1_EntityUsage,
  type ontologyservicev1_GetDatatypeFrameRequest,
  type ontologyservicev1_GetDatatypeUsageRequest,
  type ontologyservicev1_ListDatatypesRequest,
  type ontologyservicev1_ListDatatypesResponse,
  type ontologyservicev1_ListSimpleDatatypesRequest,
  type ontologyservicev1_SimpleDatatypeListResponse,
  type ontologyservicev1_UpdateDatatypeRequest,
} from '@/api/generated/admin/service/v1';
import { queryClient } from '@/core';
import { apiClient } from '@/api/client';

/**
 * 数据类型 hooks（05 期）：数据类型是自定义 RPC（公理事务管线），请求体收扁平对象——
 * 勿按通用 CRUD 惯例包 { data: {...} }（protojson 会把 data 当未知字段丢弃，
 * 接口 200 但字段全空，铁律 3 已知坑）。
 * 无 tree 端点（OWL 2 无 subDatatypeOf 层级）；simple-list 供数据属性值域双源下拉。
 */

// 分页列表（req 手动构建，直传 camelCase 字段，不走 PaginationQuery.toRawParams()——自定义消息）
export function useListDatatypes(
  req: ontologyservicev1_ListDatatypesRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ListDatatypesResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listDatatypes', req],
    queryFn: () => apiClient.ontologyDatatypeService.List(req),
    ...options,
  });
}

// 列表：fetch 版（ListTable request 回调里用）
export async function fetchListDatatypes(req: ontologyservicev1_ListDatatypesRequest) {
  return queryClient.fetchQuery({
    queryKey: ['listDatatypes', req],
    queryFn: () => apiClient.ontologyDatatypeService.List(req),
    retry: 0,
  });
}

// 简况列表（数据属性值域双源下拉的"本体数据类型"分组数据源）
export function useSimpleDatatypes(
  req: ontologyservicev1_ListSimpleDatatypesRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_SimpleDatatypeListResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['simpleDatatypes', req],
    queryFn: () => apiClient.ontologyDatatypeService.SimpleList(req),
    ...options,
  });
}

// 简况列表：fetch 版（非组件环境用，retry 必须 0）
export async function fetchSimpleDatatypes(ontologyId: number) {
  return queryClient.fetchQuery({
    queryKey: ['simpleDatatypes', { ontologyId }],
    queryFn: () => apiClient.ontologyDatatypeService.SimpleList({ ontologyId }),
    retry: 0,
  });
}

// frame 详情（含 base_version 与 definition；提交回传作乐观锁基准）
export function useDatatypeFrame(
  req: ontologyservicev1_GetDatatypeFrameRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_DatatypeFrame, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['datatypeFrame', req],
    queryFn: () => apiClient.ontologyDatatypeService.Frame(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 引用明细（删除保护弹窗：被哪些数据属性作派生值域引用）
export function useDatatypeUsage(
  req: ontologyservicev1_GetDatatypeUsageRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_EntityUsage, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['datatypeUsage', req],
    queryFn: () => apiClient.ontologyDatatypeService.Usage(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 创建（扁平请求体，禁止包 { data }）
export function useCreateDatatype(
  options?: UseMutationOptions<
    ontologyservicev1_CreateDatatypeResponse,
    Error,
    { values: Partial<ontologyservicev1_CreateDatatypeRequest> }
  >,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyDatatypeService.Create({ ...values } as ontologyservicev1_CreateDatatypeRequest),
    ...options,
  });
}

// 更新（扁平 + base_version；frame 全量提交语义）。
// definition 为 optional message 字段（presence 可靠）：undefined = 跳过定义区 diff；
// 显式空对象 {} = 清空定义（05 号文档 §4.3 两段语义——前端"清除定义"开关提交 {}）。
export function useUpdateDatatype(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_UpdateDatatypeRequest> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyDatatypeService.Update({ ...values } as ontologyservicev1_UpdateDatatypeRequest),
    ...options,
  });
}

// 删除（force=true 级联移除 42 值域引用行，引用方数据属性其余值域完好）
export function useDeleteDatatype(
  options?: UseMutationOptions<unknown, Error, ontologyservicev1_DeleteDatatypeRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyDatatypeService.Delete(data),
    ...options,
  });
}

// Kratos 错误分流复用类页实现（reason 字符串分流语义一致）
export { getOntologyErrorReason } from '@/api/hooks/ontologyClass';
