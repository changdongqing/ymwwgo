import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type onestopservicev1_DeletePeriodicPlanRequest,
  type onestopservicev1_DeleteServiceTypeRequest,
  type onestopservicev1_DeleteTransportRouteRequest,
  type onestopservicev1_GetServiceTypeRequest,
  type onestopservicev1_ListPeriodicPlansResponse,
  type onestopservicev1_ListServiceTypeResponse,
  type onestopservicev1_ListTransportRoutesResponse,
  type onestopservicev1_PeriodicPlan,
  type onestopservicev1_ServiceType,
  type onestopservicev1_TransportRoute,
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

// —— 周期计划（触发方式为「计划生成」的类别管理）——

const PLANS_QUERY_KEY = 'listPeriodicPlans';

/** 按类别取周期计划列表（类别下计划量少，一次取全） */
export function useListPeriodicPlans(
  serviceTypeId: number | undefined,
  options?: UseQueryOptions<onestopservicev1_ListPeriodicPlansResponse, Error>,
) {
  return useQuery({
    queryKey: [PLANS_QUERY_KEY, serviceTypeId],
    enabled: serviceTypeId != null,
    queryFn: () =>
      apiClient.oneStopPeriodicPlanService.List({
        page: 1,
        pageSize: 200,
        query: JSON.stringify({ serviceTypeId__eq: serviceTypeId }),
        orderBy: JSON.stringify(['id']),
        sorting: [],
      } as pagination_PagingRequest),
    ...options,
  });
}

/** 创建周期计划 */
export function useCreatePeriodicPlan(
  options?: UseMutationOptions<unknown, Error, { values: Partial<onestopservicev1_PeriodicPlan> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.oneStopPeriodicPlanService.Create({
        data: { ...values } as onestopservicev1_PeriodicPlan,
      }),
    ...options,
  });
}

/** 更新周期计划（{ data: {...} } 包装 + updateMask 在 hooks 层完成） */
export function useUpdatePeriodicPlan(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<onestopservicev1_PeriodicPlan> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.oneStopPeriodicPlanService.Update({
        id,
        data: { ...values } as onestopservicev1_PeriodicPlan,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

/** 删除周期计划 */
export function useDeletePeriodicPlan(
  options?: UseMutationOptions<unknown, Error, onestopservicev1_DeletePeriodicPlanRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.oneStopPeriodicPlanService.Delete(data),
    ...options,
  });
}

/** 周期计划写操作后失效计划列表缓存 */
export function invalidatePeriodicPlans(queryClientInstance: typeof queryClient) {
  queryClientInstance.invalidateQueries({ queryKey: [PLANS_QUERY_KEY] });
}

// —— 运送路线（onestop 运送配送类别管理）——

const ROUTES_QUERY_KEY = 'listTransportRoutes';

/** 按运送类别取路线列表（类别下路线量少，一次取全） */
export function useListTransportRoutes(
  serviceTypeId: number | undefined,
  options?: UseQueryOptions<onestopservicev1_ListTransportRoutesResponse, Error>,
) {
  return useQuery({
    queryKey: [ROUTES_QUERY_KEY, serviceTypeId],
    enabled: serviceTypeId != null,
    queryFn: () =>
      apiClient.oneStopTransportRouteService.List({
        page: 1,
        pageSize: 200,
        query: JSON.stringify({ serviceTypeId__eq: serviceTypeId }),
        orderBy: JSON.stringify(['id']),
        sorting: [],
      } as pagination_PagingRequest),
    ...options,
  });
}

/** 创建运送路线 */
export function useCreateTransportRoute(
  options?: UseMutationOptions<unknown, Error, { values: Partial<onestopservicev1_TransportRoute> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.oneStopTransportRouteService.Create({
        data: { ...values } as onestopservicev1_TransportRoute,
      }),
    ...options,
  });
}

/** 更新运送路线（{ data: {...} } 包装 + updateMask 在 hooks 层完成） */
export function useUpdateTransportRoute(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<onestopservicev1_TransportRoute> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.oneStopTransportRouteService.Update({
        id,
        data: { ...values } as onestopservicev1_TransportRoute,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

/** 删除运送路线 */
export function useDeleteTransportRoute(
  options?: UseMutationOptions<unknown, Error, onestopservicev1_DeleteTransportRouteRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.oneStopTransportRouteService.Delete(data),
    ...options,
  });
}

/** 运送路线写操作后失效路线列表缓存 */
export function invalidateTransportRoutes(queryClientInstance: typeof queryClient) {
  queryClientInstance.invalidateQueries({ queryKey: [ROUTES_QUERY_KEY] });
}
