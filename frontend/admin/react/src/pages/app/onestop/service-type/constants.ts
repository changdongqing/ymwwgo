import type { onestopservicev1_Level } from '@/api/generated/admin/service/v1';

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
