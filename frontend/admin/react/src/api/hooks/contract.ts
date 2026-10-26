import { useMutation, type UseMutationOptions, useQuery, type UseQueryOptions } from '@tanstack/react-query';
import {
  type contractservicev1_ApproveContractRequest,
  type contractservicev1_ChangeContractRequest,
  type contractservicev1_Contract,
  type contractservicev1_ContractType,
  type contractservicev1_DecomposeFeesRequest,
  type contractservicev1_DeleteContractRequest,
  type contractservicev1_DeleteContractTypeRequest,
  type contractservicev1_EffectiveContractConfig,
  type contractservicev1_GetContractRequest,
  type contractservicev1_GetContractTypeRequest,
  type contractservicev1_InvalidateContractRequest,
  type contractservicev1_ListContractEventsResponse,
  type contractservicev1_ListContractFeesResponse,
  type contractservicev1_ListContractsResponse,
  type contractservicev1_ListContractTypesResponse,
  type contractservicev1_PayFeeRequest,
  type contractservicev1_ReceiveFeeRequest,
  type contractservicev1_RefundFeeRequest,
  type contractservicev1_SubmitContractRequest,
  type contractservicev1_TerminateContractRequest,
} from '@/api/generated/admin/service/v1';
import { makeUpdateMask, type PaginationQuery, queryClient } from '@/core';
import { apiClient } from '@/api/client';

// —— 合同类型（二级分类字典 + 统一合同配置模板）——

/** 类型列表：useQuery（组件用） */
export function useListContractTypes(
  query: PaginationQuery,
  options?: UseQueryOptions<contractservicev1_ListContractTypesResponse, Error>,
) {
  return useQuery({
    queryKey: ['listContractTypes', query],
    queryFn: () => apiClient.contractTypeService.List(query.toRawParams()),
    ...options,
  });
}

/** 类型列表：fetch 版（ListTable request 用，retry 必须 0） */
export async function fetchListContractTypes(params: PaginationQuery) {
  return queryClient.fetchQuery({
    queryKey: ['listContractTypes', params],
    queryFn: () => apiClient.contractTypeService.List(params.toRawParams()),
    retry: 0,
  });
}

/** 类型详情 */
export function useGetContractType(
  req: contractservicev1_GetContractTypeRequest,
  options?: Omit<UseQueryOptions<contractservicev1_ContractType, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['getContractType', req],
    queryFn: () => apiClient.contractTypeService.Get(req),
    ...options,
  });
}

/** 创建类型 */
export function useCreateContractType(
  options?: UseMutationOptions<unknown, Error, { values: Partial<contractservicev1_ContractType> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.contractTypeService.Create({ data: { ...values } as contractservicev1_ContractType }),
    ...options,
  });
}

/** 更新类型（{ data: {...} } 包装 + updateMask 在 hooks 层完成） */
export function useUpdateContractType(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<contractservicev1_ContractType> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.contractTypeService.Update({
        id,
        data: { ...values } as contractservicev1_ContractType,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

/** 删除类型 */
export function useDeleteContractType(
  options?: UseMutationOptions<unknown, Error, contractservicev1_DeleteContractTypeRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.contractTypeService.Delete(data),
    ...options,
  });
}

/** 类型写操作后失效类型列表缓存 */
export function invalidateContractTypes(qc: typeof queryClient) {
  qc.invalidateQueries({ queryKey: ['listContractTypes'] });
  qc.invalidateQueries({ queryKey: ['getContractType'] });
  qc.invalidateQueries({ queryKey: ['getEffectiveContractConfig'] });
}

/** 生效合同配置（细类→大类回退解析；创建表单预填） */
export function useGetEffectiveContractConfig(
  id: number | undefined,
  options?: Omit<UseQueryOptions<contractservicev1_EffectiveContractConfig, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['getEffectiveContractConfig', id],
    enabled: id != null,
    queryFn: () => apiClient.contractTypeService.GetEffectiveConfig({ id: id! }),
    ...options,
  });
}

// —— 合同（统一合同底座：生命周期状态机 + 费用明细 + 动作 RPC）——

export const CONTRACTS_QUERY_KEY = 'listContracts';

/** 合同列表（过滤条件由调用方以 go-crud query JSON 组织） */
export function useListContracts(
  query: PaginationQuery,
  options?: UseQueryOptions<contractservicev1_ListContractsResponse, Error>,
) {
  return useQuery({
    queryKey: [CONTRACTS_QUERY_KEY, query],
    queryFn: () => apiClient.contractService.List(query.toRawParams()),
    ...options,
  });
}

/** 列表：fetch 版（ListTable request 用，retry 必须 0） */
export async function fetchListContracts(params: PaginationQuery) {
  return queryClient.fetchQuery({
    queryKey: [CONTRACTS_QUERY_KEY, params],
    queryFn: () => apiClient.contractService.List(params.toRawParams()),
    retry: 0,
  });
}

/** 合同详情 */
export function useGetContract(
  req: contractservicev1_GetContractRequest,
  options?: Omit<UseQueryOptions<contractservicev1_Contract, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['getContract', req],
    queryFn: () => apiClient.contractService.Get(req),
    ...options,
  });
}

/** 合同时间线（倒序） */
export function useListContractEvents(
  id: number | undefined,
  options?: Omit<UseQueryOptions<contractservicev1_ListContractEventsResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listContractEvents', id],
    enabled: id != null,
    queryFn: () => apiClient.contractService.ListEvents({ id: id! }),
    ...options,
  });
}

/** 合同费用明细 */
export function useListContractFees(
  id: number | undefined,
  options?: Omit<UseQueryOptions<contractservicev1_ListContractFeesResponse, Error>, 'queryKey' | 'queryFn'>,
) {
  return useQuery({
    queryKey: ['listContractFees', id],
    enabled: id != null,
    queryFn: () => apiClient.contractService.ListFees({ id: id! }),
    ...options,
  });
}

/** 创建合同（快照由后端解析） */
export function useCreateContract(
  options?: UseMutationOptions<unknown, Error, { values: Partial<contractservicev1_Contract> }>,
) {
  return useMutation({
    mutationFn: ({ values }) =>
      apiClient.contractService.Create({ data: { ...values } as contractservicev1_Contract }),
    ...options,
  });
}

/** 更新合同（仅草稿可改） */
export function useUpdateContract(
  options?: UseMutationOptions<unknown, Error, { id: number; values: Partial<contractservicev1_Contract> }>,
) {
  return useMutation({
    mutationFn: ({ id, values }) =>
      apiClient.contractService.Update({
        id,
        data: { ...values } as contractservicev1_Contract,
        updateMask: makeUpdateMask(Object.keys(values ?? {})),
      }),
    ...options,
  });
}

/** 删除合同（仅草稿可删） */
export function useDeleteContract(
  options?: UseMutationOptions<unknown, Error, contractservicev1_DeleteContractRequest>,
) {
  return useMutation({
    mutationFn: (data) => apiClient.contractService.Delete(data),
    ...options,
  });
}

/** 合同写操作后失效列表/详情/费用/事件缓存 */
export function invalidateContracts(qc: typeof queryClient, id?: number) {
  qc.invalidateQueries({ queryKey: [CONTRACTS_QUERY_KEY] });
  qc.invalidateQueries({ queryKey: ['getContract'] });
  qc.invalidateQueries({ queryKey: ['listContractEvents'] });
  qc.invalidateQueries({ queryKey: ['listContractFees'] });
  if (id != null) qc.invalidateQueries({ queryKey: ['getContract', { id }] });
}

type ContractActionMap = {
  Submit: contractservicev1_SubmitContractRequest;
  Approve: contractservicev1_ApproveContractRequest;
  Change: contractservicev1_ChangeContractRequest;
  Terminate: contractservicev1_TerminateContractRequest;
  Invalidate: contractservicev1_InvalidateContractRequest;
  DecomposeFees: contractservicev1_DecomposeFeesRequest;
  PayFee: contractservicev1_PayFeeRequest;
  ReceiveFee: contractservicev1_ReceiveFeeRequest;
  RefundFee: contractservicev1_RefundFeeRequest;
};

/** 合同动作（扁平 body）：Submit/Approve/Change/Terminate/Invalidate/DecomposeFees/PayFee/ReceiveFee/RefundFee 统一入口 */
export function useContractAction<K extends keyof ContractActionMap>(
  action: K,
  options?: UseMutationOptions<unknown, Error, ContractActionMap[K]>,
) {
  return useMutation({
    // @ts-expect-error 各动作方法签名一致（扁平请求体 → Empty/Contract），联合类型收窄交给调用点
    mutationFn: (data: ContractActionMap[K]) => apiClient.contractService[action](data),
    ...options,
  });
}
