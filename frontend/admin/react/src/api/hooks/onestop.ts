import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type onestopservicev1_DeleteServiceTypeRequest,
  type onestopservicev1_GetServiceTypeRequest,
  type onestopservicev1_ListServiceTypeResponse,
  type onestopservicev1_ServiceType,
} from '@/api/generated/admin/service/v1';
import { makeUpdateMask, type PaginationQuery, queryClient } from '@/core';
import { apiClient } from '@/api/client';

// 列表：useQuery（组件用）
export function useListServiceTypes(
  query: PaginationQuery,
  options?: UseQueryOptions<onestopservicev1_ListServiceTypeResponse, Error>,
) {
  return useQuery({
    queryKey: ['listServiceTypes', query],
    queryFn: () => apiClient.oneStopServiceTypeService.List(query.toRawParams()),
    ...options,
  });
}

// 列表：fetch 版（非组件环境用，retry 必须 0）
export async function fetchListServiceTypes(params: PaginationQuery) {
  return queryClient.fetchQuery({
    queryKey: ['listServiceTypes', params],
    queryFn: () => apiClient.oneStopServiceTypeService.List(params.toRawParams()),
    retry: 0,
  });
}

// 详情
export function useGetServiceType(
  req: onestopservicev1_GetServiceTypeRequest,
  options?: Omit<UseQueryOptions<onestopservicev1_ServiceType, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['getServiceType', req],
    queryFn: () => apiClient.oneStopServiceTypeService.Get(req),
    ...options,
  });
}

// 创建
export function useCreateServiceType(
  options?: UseMutationOptions<unknown, Error, { values: Partial<onestopservicev1_ServiceType> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.oneStopServiceTypeService.Create({ data: { ...values } as onestopservicev1_ServiceType }),
    ...options,
  });
}

// 更新（{ data: {...} } 包装 + updateMask 在 hooks 层完成）
export function useUpdateServiceType(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<onestopservicev1_ServiceType> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.oneStopServiceTypeService.Update({
        id,
        data: { ...values } as onestopservicev1_ServiceType,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

// 删除
export function useDeleteServiceType(
  options?: UseMutationOptions<unknown, Error, onestopservicev1_DeleteServiceTypeRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.oneStopServiceTypeService.Delete(data),
    ...options,
  });
}
