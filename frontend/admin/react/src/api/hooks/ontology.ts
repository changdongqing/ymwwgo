import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type ontologyservicev1_GetOntologyRequest,
  type ontologyservicev1_ListOntologiesResponse,
  type ontologyservicev1_ListOntologiesSimpleResponse,
  type ontologyservicev1_Ontology,
} from '@/api/generated/admin/service/v1';
import { makeUpdateMask, type PaginationQuery, queryClient } from '@/core';
import { apiClient } from '@/api/client';

// 列表：useQuery（组件用）
export function useListOntologies(
  query: PaginationQuery,
  options?: UseQueryOptions<ontologyservicev1_ListOntologiesResponse, Error>,
) {
  return useQuery({
    queryKey: ['listOntologies', query],
    queryFn: () => apiClient.ontologyService.List(query.toRawParams()),
    ...options,
  });
}

// 列表：fetch 版（非组件环境用，retry 必须 0）
export async function fetchListOntologies(params: PaginationQuery) {
  return queryClient.fetchQuery({
    queryKey: ['listOntologies', params],
    queryFn: () => apiClient.ontologyService.List(params.toRawParams()),
    retry: 0,
  });
}

// 简况列表（活动本体切换器）：fetch 版（非组件环境，retry 必须 0）
export async function fetchListSimpleOntologies() {
  return queryClient.fetchQuery({
    queryKey: ['simpleOntologies'],
    queryFn: () => apiClient.ontologyService.SimpleList({}),
    retry: 0,
  });
}

// 简况列表：useQuery（组件用）
export function useListSimpleOntologies(
  options?: UseQueryOptions<ontologyservicev1_ListOntologiesSimpleResponse, Error>,
) {
  return useQuery({
    queryKey: ['simpleOntologies'],
    queryFn: () => apiClient.ontologyService.SimpleList({}),
    ...options,
  });
}

// 详情
export function useGetOntology(
  req: ontologyservicev1_GetOntologyRequest,
  options?: Omit<UseQueryOptions<ontologyservicev1_Ontology, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['getOntology', req],
    queryFn: () => apiClient.ontologyService.Get(req),
    ...options,
  });
}

// 创建（通用 CRUD：{ data: {...} } 包装在 hooks 层完成）
export function useCreateOntology(
  options?: UseMutationOptions<unknown, Error, { values: Partial<ontologyservicev1_Ontology> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.ontologyService.Create({ data: { ...values } as ontologyservicev1_Ontology }),
    ...options,
  });
}

// 更新（{ data: {...} } 包装 + updateMask 在 hooks 层完成；base_iri 不可变，调用方不应携带）
export function useUpdateOntology(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<ontologyservicev1_Ontology> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.ontologyService.Update({
        id,
        data: { ...values } as ontologyservicev1_Ontology,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

// 删除（批量；服务端级联软删本体下全部实体/公理/变更日志）
export function useDeleteOntology(
  options?: UseMutationOptions<unknown, Error, { ids: number[] }>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.ontologyService.Delete({ ids: data.ids }),
    ...options,
  });
}
