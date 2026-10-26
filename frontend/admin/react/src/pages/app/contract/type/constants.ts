import type { TFunction } from 'i18next';

/** 类型编码：2/4 位数字，位数即层级（大类/细类） */
export const CODE_PATTERN = /^(\d{2}|\d{4})$/;

/** 由编码位数推导层级 */
export function levelOfCode(code?: string): 'LEVEL_MAJOR' | 'LEVEL_DETAIL' | undefined {
  if (!code) return undefined;
  if (code.length === 2) return 'LEVEL_MAJOR';
  if (code.length === 4) return 'LEVEL_DETAIL';
  return undefined;
}

export function getLevelMap(t: TFunction) {
  return {
    LEVEL_MAJOR: t('levelMap.LEVEL_MAJOR'),
    LEVEL_DETAIL: t('levelMap.LEVEL_DETAIL'),
  } as Record<string, string>;
}

export function getLevelColor(level?: string | null): string {
  switch (level) {
    case 'LEVEL_MAJOR':
      return 'processing';
    case 'LEVEL_DETAIL':
      return 'success';
    default:
      return 'default';
  }
}

/** 付款方式选项（类型模板 + 合同表单共用） */
export const PAY_METHOD_OPTIONS = [
  { label: 'payMethodOptions.PAY_ONCE', value: 'PAY_ONCE' },
  { label: 'payMethodOptions.PAY_MONTHLY', value: 'PAY_MONTHLY' },
  { label: 'payMethodOptions.PAY_QUARTERLY', value: 'PAY_QUARTERLY' },
  { label: 'payMethodOptions.PAY_NODE', value: 'PAY_NODE' },
  { label: 'payMethodOptions.PAY_PROGRESS', value: 'PAY_PROGRESS' },
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
      return '应付（我方付款）';
    case 'DIRECTION_RECEIVABLE':
      return '应收（我方收款）';
    default:
      return '-';
  }
}

/** 乙方称谓候选（AutoComplete；原型的六类型称谓 + 通用） */
export const PARTY_B_LABEL_SUGGESTIONS = [
  '三方公司',
  '施工单位',
  '合同乙方',
  '服务单位',
  '维保单位',
  '供应商',
];

/** 差异字段控件类型 */
export const WIDGET_OPTIONS = [
  { label: '单行文本', value: 'WIDGET_TEXT' },
  { label: '多行文本', value: 'WIDGET_TEXTAREA' },
  { label: '下拉选择', value: 'WIDGET_SELECT' },
  { label: '日期', value: 'WIDGET_DATE' },
];
