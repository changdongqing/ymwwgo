import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_CreateIndividualRequest,
  type ontologyservicev1_CreateIndividualResponse,
  type ontologyservicev1_DeleteIndividualRequest,
  type ontologyservicev1_EntityUsage,
  type ontologyservicev1_GetIndividualCodePreviewRequest,
  type ontologyservicev1_GetIndividualCountByClassRequest,
  type ontologyservicev1_GetIndividualFrameRequest,
  type ontologyservicev1_GetIndividualUsageRequest,
  type ontologyservicev1_IndividualCodePreview,
  type ontologyservicev1_IndividualCountByClassResponse,
  type ontologyservicev1_IndividualFrame,
  type ontologyservicev1_ListIndividualsRequest,
  type ontologyservicev1_ListIndividualsResponse,
  type ontologyservicev1_UpdateIndividualRequest,
} from '@/api/generated/admin/service/v1';
import { queryClient } from '@/core';
import { apiClient } from '@/api/client';

/**
 * 个体 hooks（06 期）：个体是自定义 RPC（取号 + 公理事务管线），请求体收扁平对象——
 * 勿按通用 CRUD 惯例包 { data: {...} }（protojson 会把 data 当未知字段丢弃，
 * 接口 200 但字段全空，铁律 3 已知坑）。
 */

// 分页列表（两段式查询：keyword/code/classId 服务端过滤；req 手动构建，直传 camelCase 字段）
export function useListIndividuals(
  req: ontologyservicev1_ListIndividualsRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_ListIndividualsResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listIndividuals', req],
    queryFn: () => apiClient.ontologyIndividualService.List(req),
    ...options,
  });
}

// 列表：fetch 版（ListTable request 回调里用，retry 必须 0）；req 允许省略可选过滤字段
export async function fetchListIndividuals(
  req: Partial<ontologyservicev1_ListIndividualsRequest> & Pick<ontologyservicev1_ListIndividualsRequest, 'ontologyId'>,
) {
  return queryClient.fetchQuery({
    queryKey: ['listIndividuals', req],
    queryFn: () =>
      apiClient.ontologyIndividualService.List({
        code: '',
        codeQueryType: 'EXACT',
        classId: 0,
        includeDescendants: false,
        ...req,
      } as ontologyservicev1_ListIndividualsRequest),
    retry: 0,
  });
}

// frame 详情（类型区首位 = 主类目；含 base_version 作乐观锁基准）
export function useIndividualFrame(
  req: ontologyservicev1_GetIndividualFrameRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_IndividualFrame, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['individualFrame', req],
    queryFn: () => apiClient.ontologyIndividualService.Frame(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 预计编码（预览不加锁，并发下实际号可能 +N——文案必须写"预计"）
export function useIndividualCodePreview(
  req: ontologyservicev1_GetIndividualCodePreviewRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_IndividualCodePreview, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['individualCodePreview', req.ontologyId, req.mainClassId],
    queryFn: () => apiClient.ontologyIndividualService.CodePreview(req),
    enabled: options?.enabled ?? (req.ontologyId != null && req.ontologyId > 0 && req.mainClassId != null && req.mainClassId > 0),
    retry: 0,
    ...options,
  });
}

// 类实例计数（类树节点直接实例数徽标；Protégé 计数渲染器对应物）
export function useIndividualsCountByClass(
  req: ontologyservicev1_GetIndividualCountByClassRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_IndividualCountByClassResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['individualsCountByClass', req.ontologyId],
    queryFn: () => apiClient.ontologyIndividualService.CountByClass(req),
    enabled: options?.enabled ?? (req.ontologyId != null && req.ontologyId > 0),
    ...options,
  });
}

// 引用明细（删除保护弹窗；61 断言宾语侧反查）
export function useIndividualUsage(
  req: ontologyservicev1_GetIndividualUsageRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_EntityUsage, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['individualUsage', req],
    queryFn: () => apiClient.ontologyIndividualService.Usage(req),
    enabled: options?.enabled ?? (req.id != null && req.id > 0),
    ...options,
  });
}

// 创建（扁平请求体，禁止包 { data }）
export function useCreateIndividual(
  options?: UseMutationOptions<
    ontologyservicev1_CreateIndividualResponse,
    Error,
    { values: Partial<ontologyservicev1_CreateIndividualRequest> }
  >,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyIndividualService.Create({ ...values } as ontologyservicev1_CreateIndividualRequest),
    ...options,
  });
}

// 更新（扁平 + base_version；frame 全量提交：类型区专用全量替换 diff，编码区显式改码）
export function useUpdateIndividual(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_UpdateIndividualRequest> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyIndividualService.Update({ ...values } as ontologyservicev1_UpdateIndividualRequest),
    ...options,
  });
}

// 删除（usage 保护：force=true 级联删"以我为宾语"的 61 行；号不回收）
export function useDeleteIndividual(
  options?: UseMutationOptions<unknown, Error, ontologyservicev1_DeleteIndividualRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyIndividualService.Delete(data),
    ...options,
  });
}

export { getOntologyErrorReason } from './ontologyClass';
