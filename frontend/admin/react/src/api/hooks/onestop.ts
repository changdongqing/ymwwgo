import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type onestopservicev1_DeleteInspectionPlanRequest,
  type onestopservicev1_DeleteServiceTypeRequest,
  type onestopservicev1_GetServiceTypeRequest,
  type onestopservicev1_InspectionPlan,
  type onestopservicev1_ListInspectionPlansResponse,
  type onestopservicev1_ListServiceTypeResponse,
  type onestopservicev1_ServiceType,
  type pagination_PagingRequest,
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

// —— 巡检计划（onestop 巡检计划管理）——

const PLANS_QUERY_KEY = 'listInspectionPlans';

/** 按巡检类别取计划列表（类别下计划量少，一次取全） */
export function useListInspectionPlans(
  serviceTypeId: number | undefined,
  options?: UseQueryOptions<onestopservicev1_ListInspectionPlansResponse, Error>,
) {
  return useQuery({
    queryKey: [PLANS_QUERY_KEY, serviceTypeId],
    enabled: serviceTypeId != null,
    queryFn: () =>
      apiClient.oneStopInspectionPlanService.List({
        page: 1,
        pageSize: 200,
        query: JSON.stringify({ serviceTypeId__eq: serviceTypeId }),
        orderBy: JSON.stringify(['id']),
        sorting: [],
      } as pagination_PagingRequest),
    ...options,
  });
}

/** 创建巡检计划 */
export function useCreateInspectionPlan(
  options?: UseMutationOptions<unknown, Error, { values: Partial<onestopservicev1_InspectionPlan> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.oneStopInspectionPlanService.Create({
        data: { ...values } as onestopservicev1_InspectionPlan,
      }),
    ...options,
  });
}

/** 更新巡检计划（{ data: {...} } 包装 + updateMask 在 hooks 层完成） */
export function useUpdateInspectionPlan(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<onestopservicev1_InspectionPlan> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.oneStopInspectionPlanService.Update({
        id,
        data: { ...values } as onestopservicev1_InspectionPlan,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

/** 删除巡检计划 */
export function useDeleteInspectionPlan(
  options?: UseMutationOptions<unknown, Error, onestopservicev1_DeleteInspectionPlanRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.oneStopInspectionPlanService.Delete(data),
    ...options,
  });
}

/** 巡检计划写操作后失效计划列表缓存 */
export function invalidateInspectionPlans(queryClientInstance: typeof queryClient) {
  queryClientInstance.invalidateQueries({ queryKey: [PLANS_QUERY_KEY] });
}
