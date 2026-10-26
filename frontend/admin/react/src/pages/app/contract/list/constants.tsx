import { Tag } from 'antd';

// 合同状态（统一生命周期状态机，docs/connn/contract-type-and-contract-design.md §4.1）

export function statusName(v?: string | null): string {
  switch (v) {
    case 'STATUS_DRAFT':
      return '草稿';
    case 'STATUS_REVIEWING':
      return '待审核';
    case 'STATUS_EFFECTIVE':
      return '执行中';
    case 'STATUS_CHANGED':
      return '变更终止';
    case 'STATUS_EXPIRED':
      return '过期终止';
    case 'STATUS_TERMINATED':
      return '已终止';
    case 'STATUS_INVALIDATED':
      return '已作废';
    default:
      return '-';
  }
}

export function statusColor(v?: string | null): string {
  switch (v) {
    case 'STATUS_DRAFT':
      return 'default';
    case 'STATUS_REVIEWING':
      return 'processing';
    case 'STATUS_EFFECTIVE':
      return 'success';
    case 'STATUS_CHANGED':
      return 'orange';
    case 'STATUS_EXPIRED':
      return 'warning';
    case 'STATUS_TERMINATED':
      return 'volcano';
    case 'STATUS_INVALIDATED':
      return 'red';
    default:
      return 'default';
  }
}

export function isTerminal(v?: string | null): boolean {
  return (
    v === 'STATUS_CHANGED' ||
    v === 'STATUS_EXPIRED' ||
    v === 'STATUS_TERMINATED' ||
    v === 'STATUS_INVALIDATED'
  );
}

/** 状态页签（列表顶部）；expired 页签 = 终态四类聚合 */
export const STATUS_TABS: { key: string; label: string; statuses?: string[] }[] = [
  { key: 'all', label: '全部' },
  { key: 'STATUS_DRAFT', label: '草稿', statuses: ['STATUS_DRAFT'] },
  { key: 'STATUS_REVIEWING', label: '待审核', statuses: ['STATUS_REVIEWING'] },
  { key: 'STATUS_EFFECTIVE', label: '执行中', statuses: ['STATUS_EFFECTIVE'] },
  { key: 'closed', label: '已终结', statuses: ['STATUS_CHANGED', 'STATUS_EXPIRED', 'STATUS_TERMINATED', 'STATUS_INVALIDATED'] },
];

export function payMethodName(v?: string | null): string {
  switch (v) {
    case 'PAY_ONCE':
      return '一次性付款';
    case 'PAY_MONTHLY':
      return '按月付款';
    case 'PAY_QUARTERLY':
      return '按季付款';
    case 'PAY_NODE':
      return '按节点付款';
    case 'PAY_PROGRESS':
      return '按进度付款';
    default:
      return '-';
  }
}

export function feeDirectionName(v?: string | null): string {
  switch (v) {
    case 'DIRECTION_PAYABLE':
      return '应付';
    case 'DIRECTION_RECEIVABLE':
      return '应收';
    default:
      return '-';
  }
}

export function importanceName(v?: string | null): string {
  return v === 'IMPORTANCE_IMPORTANT' ? '重要' : '一般';
}

export function partyBKindName(v?: string | null): string {
  switch (v) {
    case 'PARTY_B_ENTERPRISE':
      return '企业法人';
    case 'PARTY_B_ORG':
      return '非法人组织';
    case 'PARTY_B_NATURAL':
      return '自然人';
    default:
      return '-';
  }
}

export function feeCategoryName(v?: string | null): string {
  switch (v) {
    case 'CATEGORY_INCOME':
      return '收入';
    case 'CATEGORY_EXPENSE':
      return '费用';
    case 'CATEGORY_DEPOSIT':
      return '押金';
    default:
      return '-';
  }
}

/** 费用状态显示：方向驱动的待收付/已收付文案（对齐原型六态口径） */
export function feeStatusName(v?: string | null, direction?: string | null): string {
  const receivable = direction === 'DIRECTION_RECEIVABLE';
  switch (v) {
    case 'FEE_STATUS_PENDING':
      return receivable ? '待收取' : '待付款';
    case 'FEE_STATUS_DONE':
      return receivable ? '已收取' : '已付款';
    case 'FEE_STATUS_REFUNDED':
      return '已退款';
    default:
      return '-';
  }
}

export function feeStatusColor(v?: string | null): string {
  switch (v) {
    case 'FEE_STATUS_PENDING':
      return 'default';
    case 'FEE_STATUS_DONE':
      return 'success';
    case 'FEE_STATUS_REFUNDED':
      return 'purple';
    default:
      return 'default';
  }
}

/** 事件动作显示 */
export function eventActionName(v?: string | null): string {
  switch (v) {
    case 'ACTION_CREATE':
      return '创建';
    case 'ACTION_UPDATE':
      return '修改';
    case 'ACTION_SUBMIT':
      return '提交';
    case 'ACTION_APPROVE':
      return '审核通过';
    case 'ACTION_REJECT':
      return '审核驳回';
    case 'ACTION_CHANGE':
      return '变更';
    case 'ACTION_TERMINATE':
      return '终止';
    case 'ACTION_INVALIDATE':
      return '作废';
    case 'ACTION_EXPIRE':
      return '到期关闭';
    case 'ACTION_DECOMPOSE':
      return '重新拆解';
    case 'ACTION_FEE_PAY':
      return '费用付款';
    case 'ACTION_FEE_RECEIVE':
      return '费用收款';
    case 'ACTION_FEE_REFUND':
      return '押金退款';
    default:
      return String(v ?? '-');
  }
}

/** 到期预警标签：执行中且 30/90/180 天内到期 / 已到期未扫 */
export function expiringTag(endDate?: string | null, status?: string | null) {
  if (!endDate || status !== 'STATUS_EFFECTIVE') return null;
  const days = Math.ceil((new Date(endDate).getTime() - Date.now()) / 86_400_000);
  if (days < 0) return <Tag color="red">已到期 {Math.abs(days)} 天</Tag>;
  if (days <= 30) return <Tag color="red">30 天内到期</Tag>;
  if (days <= 90) return <Tag color="orange">90 天内到期</Tag>;
  if (days <= 180) return <Tag color="gold">180 天内到期</Tag>;
  return null;
}
