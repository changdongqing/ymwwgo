import type {
  onestopservicev1_FreqType,
  onestopservicev1_Level,
  onestopservicev1_ServiceAcceptanceItem,
  onestopservicev1_ServicePriority,
  onestopservicev1_TemperatureRequirement,
} from '@/api/generated/admin/service/v1';

/** 编码位数 → 层级枚举值 */
export function levelOfCode(code?: string): onestopservicev1_Level | undefined {
  switch (code?.length) {
    case 2:
      return 'LEVEL_MAJOR';
    case 4:
      return 'LEVEL_MIDDLE';
    case 6:
      return 'LEVEL_MINOR';
    case 8:
      return 'LEVEL_DETAIL';
    default:
      return undefined;
  }
}

/** 层级展示名 */
export function getLevelMap(t: (key: string) => string): Record<onestopservicev1_Level, string> {
  return {
    LEVEL_UNSPECIFIED: t('levelMap.unspecified'),
    LEVEL_MAJOR: t('levelMap.major'),
    LEVEL_MIDDLE: t('levelMap.middle'),
    LEVEL_MINOR: t('levelMap.minor'),
    LEVEL_DETAIL: t('levelMap.detail'),
  };
}

export function getLevelOptions(t: (key: string) => string) {
  const levelMap = getLevelMap(t);
  return (['LEVEL_MAJOR', 'LEVEL_MIDDLE', 'LEVEL_MINOR', 'LEVEL_DETAIL'] as onestopservicev1_Level[]).map(
    (level) => ({ label: levelMap[level], value: level }),
  );
}

/** 层级对应的 antd Tag 颜色 */
export function getLevelColor(level?: onestopservicev1_Level): string {
  switch (level) {
    case 'LEVEL_MAJOR':
      return 'geekblue';
    case 'LEVEL_MIDDLE':
      return 'cyan';
    case 'LEVEL_MINOR':
      return 'green';
    case 'LEVEL_DETAIL':
      return 'orange';
    default:
      return 'default';
  }
}

/** 服务编码：2/4/6/8 位数字 */
export const CODE_PATTERN = /^\d{2}$|^\d{4}$|^\d{6}$|^\d{8}$/;

/**
 * 该编码是否可配置履约参数（统一配置模板门控）：小类（6 位）/细类（8 位）。
 * 大类/中类是纯结构节点；后端同口径校验（结构层提交配置字段 422）。
 * 各字段组的可见性与文案由 CATEGORY_PROFILE 按中类驱动，不再按子树设三个门控。
 */
export function isConfigurable(code?: string): boolean {
  return !!code && (code.length === 6 || code.length === 8);
}

/** 编码所属中类（前 4 位）；不足 4 位返回 undefined（结构层） */
export function categoryOfCode(code?: string): string | undefined {
  return code && code.length >= 6 ? code.slice(0, 4) : undefined;
}

/** 运送配送中类编码：常用运送路线面板仅对该子树开放（路线是运送专属子资源） */
export const TRANSPORT_CATEGORY_CODE = '0106';

/** 常用执行岗位建议值（统一履约模板：维修/运送/保洁/值守等岗位合并输入建议） */
export const EXECUTION_POST_SUGGESTIONS = [
  '电工',
  '弱电工',
  '管工',
  '空调工',
  '电梯维保工',
  '锅炉工',
  '综合维修工',
  '机电维修工',
  '医学工程技师',
  '焊工',
  '运送员',
  '运送员（护工）',
  '标本运送员',
  '药品运送员',
  '后勤运送员',
  '商写运送员',
  '导医',
  '保洁员',
  '消杀技术员',
  '消控员',
  '绿化工',
  '秩序队员',
  '消防专员',
  '消防管理员',
  '特种设备管理员',
  '环境检测员',
  '库管员',
  '物业主管',
  '客服专员',
].map((value) => ({ value }));

/** 巡检执行岗位建议值（周期计划表单用） */
export const INSPECTION_POST_SUGGESTIONS = EXECUTION_POST_SUGGESTIONS;

/** 常用运送物品细目建议值（tags 输入时可选用，允许自由输入） */
export const CARGO_TYPE_SUGGESTIONS = [
  '住院患者',
  '血标本',
  '病理标本',
  '静配成品输液',
  '病区药品',
  '耗材/物资',
  '病历文件',
  '营养餐',
  '被服',
  '快递包裹',
  '大件垃圾',
].map((value) => ({ value }));

// —— 任务触发方式（周期计划统一开关）——

export type TriggerModeValue = Exclude<
  import('@/api/generated/admin/service/v1').onestopservicev1_TriggerMode,
  'TRIGGER_UNSPECIFIED'
>;

/** 触发方式展示名（与后端 TriggerMode 枚举一致） */
export function triggerName(mode?: TriggerModeValue | null): string {
  switch (mode) {
    case 'TRIGGER_ON_DEMAND':
      return '按需发起';
    case 'TRIGGER_SCHEDULED':
      return '计划生成';
    default:
      return '-';
  }
}

/** 触发方式 Tag 颜色（列表/表单展示用） */
export function triggerColor(mode?: TriggerModeValue | null): string {
  return mode === 'TRIGGER_SCHEDULED' ? 'blue' : 'default';
}

/**
 * 解析类别节点生效的触发方式（与后端 ResolveTriggerMode 同口径）：
 * 自身已设置以自身为准；细类未设置回退小类；仍未设置按 TRIGGER_ON_DEMAND。
 * items 为全量类型列表（表单页已拉平铺全量），按 id 建索引走父链。
 */
type TriggerModeLike =
  | import('@/api/generated/admin/service/v1').onestopservicev1_TriggerMode
  | null
  | undefined;

interface TriggerResolvable {
  id?: number;
  parentId?: number;
  level?: import('@/api/generated/admin/service/v1').onestopservicev1_Level;
  triggerMode?: TriggerModeLike;
}

export function resolveTriggerMode(items: TriggerResolvable[], target: TriggerResolvable): TriggerModeValue {
  if (target.triggerMode && target.triggerMode !== 'TRIGGER_UNSPECIFIED') {
    return target.triggerMode;
  }
  if (target.level === 'LEVEL_DETAIL' && target.parentId) {
    const parent = items.find((item) => item.id === target.parentId);
    if (parent?.triggerMode && parent.triggerMode !== 'TRIGGER_UNSPECIFIED') {
      return parent.triggerMode;
    }
  }
  return 'TRIGGER_ON_DEMAND';
}

// —— 展示矩阵：中类编码 → 专项属性可见性 / 文案覆盖 / 新建触发方式建议 ——

/** 专项属性字段（统一模板 D 组：按矩阵出现；存储层不限子树） */
export type SpecialField =
  | 'temperatureRequirement'
  | 'requiresApproval'
  | 'cargoTypes';

export interface CategoryProfile {
  /** 专项属性组渲染哪些字段；空数组 = 该中类不出现专项属性组 */
  specialFields?: SpecialField[];
  /** 时限文案覆盖（缺省用通用文案「受理 → 到场/完工」） */
  responseTimeLimitLabelKey?: string;
  completionTimeLimitLabelKey?: string;
  /** 新建类型时的触发方式建议初值（编辑回填以库内值为准） */
  defaultTriggerMode?: TriggerModeValue;
}

/**
 * 中类展示矩阵：只管展示（可见性 + 文案），不管存储。
 * 新建中类（0113+）不在表内 → 回落全组可见 + 通用文案，统一模板兜底。
 * 标准作业项目库（standardItems）不进矩阵：解析后触发方式为「计划生成」即出现
 *（它是周期计划点表的模板来源，与是否计划型强相关而非与中类相关）。
 */
export const CATEGORY_PROFILE: Record<string, CategoryProfile> = {
  '0101': {}, // 报修维修：事件型，无专项属性
  '0102': { defaultTriggerMode: 'TRIGGER_SCHEDULED' }, // 计划维保：天然周期
  '0103': { defaultTriggerMode: 'TRIGGER_SCHEDULED' }, // 巡检巡查：天然周期
  '0104': {}, // 保洁环卫：混合（子树级开关）
  '0105': { specialFields: ['requiresApproval'] }, // 动火作业需审批
  '0106': {
    specialFields: ['temperatureRequirement', 'requiresApproval', 'cargoTypes'],
    responseTimeLimitLabelKey: 'responseTimeLimitTransport',
    completionTimeLimitLabelKey: 'completionTimeLimitTransport',
  },
  '0107': {}, // 秩序安保
  '0108': { specialFields: ['requiresApproval'] }, // 资源申请需审批
  '0109': {},
  '0110': {},
  '0111': {},
  '0112': { specialFields: ['requiresApproval'] }, // 资产处置等流程类
};

/** 取中类的展示矩阵；未登记的中类回落空 profile（全组可见 + 通用文案） */
export function categoryProfile(code?: string): CategoryProfile {
  const category = categoryOfCode(code);
  return (category && CATEGORY_PROFILE[category]) || {};
}

/** 优先级展示名（与后端 ServicePriority 枚举一致） */
export function priorityName(priority?: onestopservicev1_ServicePriority): string {
  switch (priority) {
    case 'PRIORITY_NORMAL':
      return '常规';
    case 'PRIORITY_URGENT':
      return '加急';
    case 'PRIORITY_EMERGENCY':
      return '特急';
    default:
      return '-';
  }
}

/** 优先级选项（可清空 = 不配置，按常规处理） */
export const PRIORITY_OPTIONS: { label: string; value: Exclude<onestopservicev1_ServicePriority, 'PRIORITY_UNSPECIFIED'> }[] = [
  { label: '常规', value: 'PRIORITY_NORMAL' },
  { label: '加急', value: 'PRIORITY_URGENT' },
  { label: '特急', value: 'PRIORITY_EMERGENCY' },
];

/** 优先级 Tag 颜色（列表展示用） */
export function priorityColor(priority?: onestopservicev1_ServicePriority): string {
  switch (priority) {
    case 'PRIORITY_URGENT':
      return 'orange';
    case 'PRIORITY_EMERGENCY':
      return 'red';
    default:
      return 'default';
  }
}

/** 温控要求展示名（与后端 TemperatureRequirement 枚举一致） */
export function temperatureName(req?: onestopservicev1_TemperatureRequirement): string {
  switch (req) {
    case 'TEMP_NORMAL':
      return '常温';
    case 'TEMP_COLD':
      return '冷藏 2-8℃';
    case 'TEMP_FROZEN':
      return '冷冻 ≤-18℃';
    case 'TEMP_WARM':
      return '保温 ≥60℃';
    default:
      return '-';
  }
}

/** 温控要求选项（可清空 = 不配置，按常温处理） */
export const TEMPERATURE_OPTIONS: { label: string; value: Exclude<onestopservicev1_TemperatureRequirement, 'TEMP_UNSPECIFIED'> }[] = [
  { label: '常温', value: 'TEMP_NORMAL' },
  { label: '冷藏 2-8℃', value: 'TEMP_COLD' },
  { label: '冷冻 ≤-18℃', value: 'TEMP_FROZEN' },
  { label: '保温 ≥60℃', value: 'TEMP_WARM' },
];

/** 验收方式值域（与后端服务层 acceptanceModeSet 一致） */
export type AcceptanceModeValue = 'ACCEPT_SIGN' | 'ACCEPT_SCAN' | 'ACCEPT_PHOTO' | 'ACCEPT_DUAL';

export const ACCEPTANCE_MODE_OPTIONS: { label: string; value: AcceptanceModeValue }[] = [
  { label: '签名确认', value: 'ACCEPT_SIGN' },
  { label: '扫码核对', value: 'ACCEPT_SCAN' },
  { label: '拍照留证', value: 'ACCEPT_PHOTO' },
  { label: '双人验收', value: 'ACCEPT_DUAL' },
];

export function acceptanceModeNames(modes?: string[]): string {
  const map = new Map(ACCEPTANCE_MODE_OPTIONS.map((o) => [o.value, o.label]));
  return (modes ?? []).map((m) => map.get(m as AcceptanceModeValue) ?? m).join('、');
}

/** 验收方案摘要（列表展示用）：方式 + 必检项数 */
export function summarizeAcceptance(modes?: string[], items?: onestopservicev1_ServiceAcceptanceItem[]): string {
  const modeText = acceptanceModeNames(modes);
  if (!modeText) return '-';
  const required = (items ?? []).filter((i) => i.required).length;
  return required > 0 ? `${modeText}（必检 ${required} 项）` : modeText;
}


const WEEKDAY_NAMES = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'];

/** 周期类型（与后端 FreqType 枚举一致；UNSPECIFIED 仅作未设置占位） */
export type FreqTypeValue = Exclude<onestopservicev1_FreqType, 'FREQ_UNSPECIFIED'>;

/** 周几值 → 名称（1=周一…7=周日） */
export function weekdayName(day: number): string {
  return WEEKDAY_NAMES[day - 1] ?? `周${day}`;
}

/** 月份值 → 展示名 */
export function monthName(month: number): string {
  return `${month}月`;
}

/** 几号值 → 展示名（-1 = 月末） */
export function monthDayName(day: number): string {
  return day === -1 ? '月末' : `${day}号`;
}

/** 巡检计划周期摘要（列表/详情展示用） */
export function summarizePlanFreq(plan: {
  freqType?: onestopservicev1_FreqType;
  times?: string[];
  weekdays?: number[];
  monthDays?: number[];
  months?: number[];
  excludedDates?: string[];
}): string {
  const times = (plan.times ?? []).join('、');
  let cycle = '';
  switch (plan.freqType) {
    case 'FREQ_DAILY':
      cycle = '每日';
      break;
    case 'FREQ_WEEKLY':
      cycle = `每${(plan.weekdays ?? []).map(weekdayName).join('、')}`;
      break;
    case 'FREQ_MONTHLY':
      cycle = `每月 ${(plan.monthDays ?? []).map(monthDayName).join('、')}`;
      break;
    case 'FREQ_YEARLY':
      cycle = `每年 ${(plan.months ?? []).map(monthName).join('、')} 的 ${(plan.monthDays ?? [])
        .map(monthDayName)
        .join('、')}`;
      break;
    default:
      cycle = '-';
  }
  const excludeSuffix =
    plan.excludedDates && plan.excludedDates.length > 0 ? `（排除 ${plan.excludedDates.length} 天）` : '';
  const timeSuffix = times ? ` ${times}` : '';
  return `${cycle}${timeSuffix}${excludeSuffix}`;
}
