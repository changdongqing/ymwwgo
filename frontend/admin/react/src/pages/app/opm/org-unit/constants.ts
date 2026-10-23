/**
 * 组织架构模块枚举映射常量
 */

type TFn = (key: string, options?: Record<string, any>) => string;

// ========== 组织状态（ON/OFF） ==========

export const STATUS_COLORS: Record<string, string> = {
  ON: 'success',
  OFF: 'error',
};

export function getStatusMap(t: TFn) {
  return {
    ON: { text: t('statusMap.ON'), color: STATUS_COLORS.ON },
    OFF: { text: t('statusMap.OFF'), color: STATUS_COLORS.OFF },
  };
}

export function getStatusOptions(t: TFn) {
  return [
    { label: t('statusMap.ON'), value: 'ON' },
    { label: t('statusMap.OFF'), value: 'OFF' },
  ];
}

// ========== 组织类型 ==========

export const ORG_TYPE_COLORS: Record<string, string> = {
  GROUP: 'gold',
  REGION: 'geekblue',
  COMPANY: 'blue',
  SUBSIDIARY: 'default',
  BRANCH: 'default',
  STORE: 'magenta',
  PROJECT: 'purple',
  DEPARTMENT: 'green',
  DIVISION: 'cyan',
  COMMITTEE: 'orange',
  TEAM: 'processing',
  OTHER: 'default',
};

export function getOrgTypeMap(t: TFn) {
  return {
    GROUP: { text: t('typeMap.GROUP'), color: ORG_TYPE_COLORS.GROUP },
    REGION: { text: t('typeMap.REGION'), color: ORG_TYPE_COLORS.REGION },
    COMPANY: { text: t('typeMap.COMPANY'), color: ORG_TYPE_COLORS.COMPANY },
    SUBSIDIARY: { text: t('typeMap.SUBSIDIARY'), color: ORG_TYPE_COLORS.SUBSIDIARY },
    BRANCH: { text: t('typeMap.BRANCH'), color: ORG_TYPE_COLORS.BRANCH },
    STORE: { text: t('typeMap.STORE'), color: ORG_TYPE_COLORS.STORE },
    PROJECT: { text: t('typeMap.PROJECT'), color: ORG_TYPE_COLORS.PROJECT },
    DEPARTMENT: { text: t('typeMap.DEPARTMENT'), color: ORG_TYPE_COLORS.DEPARTMENT },
    DIVISION: { text: t('typeMap.DIVISION'), color: ORG_TYPE_COLORS.DIVISION },
    COMMITTEE: { text: t('typeMap.COMMITTEE'), color: ORG_TYPE_COLORS.COMMITTEE },
    TEAM: { text: t('typeMap.TEAM'), color: ORG_TYPE_COLORS.TEAM },
    OTHER: { text: t('typeMap.OTHER'), color: ORG_TYPE_COLORS.OTHER },
  };
}

export function getOrgTypeOptions(t: TFn) {
  return [
    { label: t('typeMap.GROUP'), value: 'GROUP' },
    { label: t('typeMap.REGION'), value: 'REGION' },
    { label: t('typeMap.COMPANY'), value: 'COMPANY' },
    { label: t('typeMap.SUBSIDIARY'), value: 'SUBSIDIARY' },
    { label: t('typeMap.BRANCH'), value: 'BRANCH' },
    { label: t('typeMap.STORE'), value: 'STORE' },
    { label: t('typeMap.PROJECT'), value: 'PROJECT' },
    { label: t('typeMap.DEPARTMENT'), value: 'DEPARTMENT' },
    { label: t('typeMap.DIVISION'), value: 'DIVISION' },
    { label: t('typeMap.COMMITTEE'), value: 'COMMITTEE' },
    { label: t('typeMap.TEAM'), value: 'TEAM' },
    { label: t('typeMap.OTHER'), value: 'OTHER' },
  ];
}

// ========== 细分类型（type 之下的受控词表） ==========

export type OrgSubtypeOption = {
  value: string;
  label: string;
  children?: OrgSubtypeOption[];
};

type SubtypeSpec = {
  code: string;
  children?: SubtypeSpec[];
};

/**
 * 顶层 type 之下的词表定义。与后端 orgSubtypeVocab 保持一致：
 * 新增细分 code 时，后端策略、这里的词表、locales 文案、本体叶子类四处同步。
 */
const SUBTYPE_SPECS: Record<string, SubtypeSpec[]> = {
  STORE: [
    { code: 'DEPARTMENT_STORE' },
    { code: 'SHOPPING_MALL' },
    { code: 'COMMUNITY_COMMERCIAL' },
    { code: 'OUTLET_STORE' },
    { code: 'STREET_COMMERCIAL' },
    { code: 'MIXED_USE_COMPLEX' },
    { code: 'OTHER' },
  ],
  DEPARTMENT: [
    { code: 'LEASING' },
    { code: 'OPERATIONS' },
    { code: 'MARKETING' },
    { code: 'CUSTOMER_SERVICE' },
    { code: 'FINANCE' },
    { code: 'HR_ADMIN' },
    { code: 'ENGINEERING_PROPERTY' },
    { code: 'SECURITY' },
    { code: 'IT' },
    { code: 'GM_OFFICE' },
    { code: 'INVESTMENT' },
    { code: 'ASSET_MANAGEMENT' },
    { code: 'LEGAL_COMPLIANCE' },
    { code: 'AUDIT' },
    { code: 'OTHER' },
  ],
  PROJECT: [
    { code: 'PRE_OPENING' },
    { code: 'RENOVATION' },
    { code: 'ASSET_LIGHT' },
    { code: 'SPECIAL' },
    { code: 'OTHER' },
  ],
  DIVISION: [
    { code: 'DEPARTMENT_STORE' },
    { code: 'SHOPPING_MALL' },
    { code: 'OUTLET' },
    { code: 'ASSET_LIGHT' },
    { code: 'OTHER' },
  ],
  COMMITTEE: [
    { code: 'LEASING_DECISION' },
    { code: 'BRAND_REVIEW' },
    { code: 'SAFETY' },
    { code: 'COMPENSATION' },
    { code: 'INVESTMENT' },
    { code: 'OTHER' },
  ],
  TEAM: [
    { code: 'LEASING' },
    { code: 'OPERATIONS' },
    { code: 'ENGINEERING' },
    { code: 'OTHER' },
  ],
};

export const SUBTYPE_REQUIRED_TYPES = new Set(['STORE', 'DEPARTMENT', 'PROJECT']);

function buildSubtypeOptions(t: TFn, specs: SubtypeSpec[]): OrgSubtypeOption[] {
  return specs.map((spec) => ({
    value: spec.code,
    label: t(`subtype.${spec.code}`),
    children: spec.children ? buildSubtypeOptions(t, spec.children) : undefined,
  }));
}

/** 返回某个顶层 type 的细分词表；该类型没有细分时返回 null（表单不渲染）。 */
export function getSubtypeOptions(t: TFn, orgType?: string | null): OrgSubtypeOption[] | null {
  if (!orgType) return null;
  const specs = SUBTYPE_SPECS[orgType];
  return specs ? buildSubtypeOptions(t, specs) : null;
}

/** 全部细分 code 的中文标签，用于列表展示与筛选。 */
export function getSubtypeLabelMap(t: TFn): Record<string, string> {
  const map: Record<string, string> = {};
  const visit = (specs: SubtypeSpec[]) => {
    specs.forEach((spec) => {
      map[spec.code] = t(`subtype.${spec.code}`);
      if (spec.children) visit(spec.children);
    });
  };
  Object.values(SUBTYPE_SPECS).forEach(visit);
  return map;
}

export function getSubtypeLabels(t: TFn, path?: string[] | null): string[] {
  const map = getSubtypeLabelMap(t);
  return (path ?? []).map((code) => map[code] ?? code);
}

export function getSubtypeFilterOptions(t: TFn) {
  return Object.entries(getSubtypeLabelMap(t)).map(([value, label]) => ({ value, label }));
}
