import type { onestopservicev1_FreqType, onestopservicev1_Level } from '@/api/generated/admin/service/v1';

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

/** 报修维修中类编码（服务类型字典 0101）：本期「报修维修配置」仅对该子树开放 */
export const REPAIR_CATEGORY_CODE = '0101';

/**
 * 该编码是否可配置报修维修参数：0101 报修维修子树的小类（6 位）/细类（8 位）。
 * 小类配岗位与 SLA，细类配工时/语句/工具/备件；其余类型（含中类本身）不启用。
 */
export function isRepairConfigurable(code?: string): boolean {
  return (
    !!code &&
    code.startsWith(REPAIR_CATEGORY_CODE) &&
    (code.length === 6 || code.length === 8)
  );
}

/** 常用维修岗位建议值（输入时可选用，允许自由输入） */
export const REPAIR_POST_SUGGESTIONS = [
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
].map((value) => ({ value }));

/** 巡检巡查中类编码（服务类型字典 0103）：本期「巡检巡查配置」仅对该子树开放 */
export const INSPECTION_CATEGORY_CODE = '0103';

/**
 * 该编码是否可配置巡检参数：0103 巡检巡查子树的小类（6 位）/细类（8 位）。
 * 类别级配置「标准巡检项目库」，并可在编辑态管理类别下的巡检计划。
 */
export function isInspectionConfigurable(code?: string): boolean {
  return (
    !!code &&
    code.startsWith(INSPECTION_CATEGORY_CODE) &&
    (code.length === 6 || code.length === 8)
  );
}

/** 常用巡检执行岗位建议值（输入时可选用，允许自由输入） */
export const INSPECTION_POST_SUGGESTIONS = [
  '秩序队员',
  '消防专员',
  '电工',
  '管工',
  '空调工',
  '电梯维保工',
  '综合维修工',
  '机电巡检工',
  '保洁班长',
].map((value) => ({ value }));

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
