import type {
  onestopservicev1_OrderKind,
  onestopservicev1_WorkOrderSource,
  onestopservicev1_WorkOrderStatus,
} from '@/api/generated/admin/service/v1';

/** 工单状态展示名（与后端状态机枚举一一对应） */
export function statusName(status?: onestopservicev1_WorkOrderStatus | null): string {
  switch (status) {
    case 'STATUS_PENDING':
      return '待派单';
    case 'STATUS_APPROVING':
      return '审批中';
    case 'STATUS_DISPATCHED':
      return '已派单';
    case 'STATUS_PROCESSING':
      return '执行中';
    case 'STATUS_COMPLETED':
      return '已完工';
    case 'STATUS_CLOSED':
      return '已完结';
    case 'STATUS_CANCELLED':
      return '已撤销';
    default:
      return '-';
  }
}

export function statusColor(status?: onestopservicev1_WorkOrderStatus | null): string {
  switch (status) {
    case 'STATUS_PENDING':
      return 'orange';
    case 'STATUS_APPROVING':
      return 'gold';
    case 'STATUS_DISPATCHED':
      return 'processing';
    case 'STATUS_PROCESSING':
      return 'blue';
    case 'STATUS_COMPLETED':
      return 'cyan';
    case 'STATUS_CLOSED':
      return 'green';
    case 'STATUS_CANCELLED':
      return 'default';
    default:
      return 'default';
  }
}

/** 履约形态展示名（沿用类型页 constants 的口径，此处独立避免跨页面耦合） */
export function kindName(kind?: onestopservicev1_OrderKind | null): string {
  switch (kind) {
    case 'KIND_EVENT':
      return '事件处置';
    case 'KIND_TASK':
      return '计划任务';
    case 'KIND_TRANSPORT':
      return '运送';
    default:
      return '-';
  }
}

export function kindColor(kind?: onestopservicev1_OrderKind | null): string {
  switch (kind) {
    case 'KIND_EVENT':
      return 'orange';
    case 'KIND_TASK':
      return 'blue';
    case 'KIND_TRANSPORT':
      return 'purple';
    default:
      return 'default';
  }
}

/** 生单来源展示名 */
export function sourceName(source?: onestopservicev1_WorkOrderSource | null): string {
  switch (source) {
    case 'SOURCE_MANUAL':
      return '手动创建';
    case 'SOURCE_PLAN':
      return '计划展开';
    case 'SOURCE_ROUTE':
      return '路线班次';
    case 'SOURCE_TRANSFER':
      return '转单';
    case 'SOURCE_DERIVED':
      return '派生';
    default:
      return '-';
  }
}

/** 优先级展示（与类型页 PRIORITY_OPTIONS 同口径） */
export function priorityName(priority?: string | null): string {
  switch (priority) {
    case 'PRIORITY_URGENT':
      return '加急';
    case 'PRIORITY_EMERGENCY':
      return '特急';
    default:
      return '常规';
  }
}

export function priorityColor(priority?: string | null): string {
  switch (priority) {
    case 'PRIORITY_URGENT':
      return 'orange';
    case 'PRIORITY_EMERGENCY':
      return 'red';
    default:
      return 'default';
  }
}

/** 温控展示 */
export function temperatureName(temp?: string | null): string {
  switch (temp) {
    case 'TEMP_COLD':
      return '冷藏 2-8℃';
    case 'TEMP_FROZEN':
      return '冷冻 ≤-18℃';
    case 'TEMP_WARM':
      return '保温 ≥60℃';
    default:
      return '常温';
  }
}

/** 是否终态（催办/异常/转单的动作门控用） */
export function isTerminal(status?: onestopservicev1_WorkOrderStatus | null): boolean {
  return status === 'STATUS_CLOSED' || status === 'STATUS_CANCELLED';
}

/**
 * 形态面板（KIND_PROFILE）：列表列/详情面板/动作可见性按形态收敛——
 * 与类型页 CATEGORY_PROFILE 同思路，差异全部参数化而非分支裂页面。
 */
export interface KindProfile {
  /** 列表追加列：运送起终点 / 计划时刻 */
  listColumns: 'route' | 'planned';
  /** 详情面板：路线/交接/执行项/验收 */
  panels: ('route' | 'handover' | 'items' | 'acceptance')[];
  /** 响应动作与文案：到场 / 到场 / 取件签收 */
  respondAction: 'arrive' | 'pickup';
  respondLabel: string;
  /** 完结动作与文案：完工提交 / 提交作业结果 / 送达签收 */
  finishAction: 'complete' | 'deliver';
  finishLabel: string;
}

export const KIND_PROFILE: Record<string, KindProfile> = {
  KIND_EVENT: {
    listColumns: 'planned',
    panels: ['acceptance'],
    respondAction: 'arrive',
    respondLabel: '到场',
    finishAction: 'complete',
    finishLabel: '完工提交',
  },
  KIND_TASK: {
    listColumns: 'planned',
    panels: ['items', 'acceptance'],
    respondAction: 'arrive',
    respondLabel: '到场',
    finishAction: 'complete',
    finishLabel: '提交作业结果',
  },
  KIND_TRANSPORT: {
    listColumns: 'route',
    panels: ['route', 'handover'],
    respondAction: 'pickup',
    respondLabel: '取件签收',
    finishAction: 'deliver',
    finishLabel: '送达签收',
  },
};

export function profileOf(kind?: onestopservicev1_OrderKind | null): KindProfile {
  return KIND_PROFILE[kind ?? 'KIND_EVENT'] ?? KIND_PROFILE.KIND_EVENT;
}

/** 状态页签（列表过滤） */
export const STATUS_TABS: { key: string; statuses: onestopservicev1_WorkOrderStatus[] }[] = [
  { key: 'all', statuses: [] },
  { key: 'STATUS_PENDING', statuses: ['STATUS_PENDING'] },
  { key: 'STATUS_APPROVING', statuses: ['STATUS_APPROVING'] },
  { key: 'STATUS_DISPATCHED', statuses: ['STATUS_DISPATCHED'] },
  { key: 'STATUS_PROCESSING', statuses: ['STATUS_PROCESSING'] },
  { key: 'STATUS_COMPLETED', statuses: ['STATUS_COMPLETED'] },
  { key: 'STATUS_CLOSED', statuses: ['STATUS_CLOSED'] },
  { key: 'STATUS_CANCELLED', statuses: ['STATUS_CANCELLED'] },
];
