/**
 * 本体建模共享常量（05 期增量：数据类型词汇表全链路）：
 * - 内建 XSD datatype 白名单 31 项（IRI + i18n 键 + 示例文案 + 词法族）——与后端 Go 侧
 *   BuiltinDatatypes 双份维护（OWL 2 规范封闭集合；05 号文档 §3.2，03 期 15 项是子集）。
 * - 词法族分组 FAMILY_GROUPS（Select 按族分组渲染 + facet 适用族判定共用）。
 * - facet 元数据 FACET_META（10 种白名单 + 适用族 + 值形态提示；与后端 FacetFamilies
 *   矩阵同源维护，05 号文档 §6.4）。
 */

export const XSD = 'http://www.w3.org/2001/XMLSchema#';

/** 词法族（与后端 DatatypeFamily 常量逐值一致） */
export type DatatypeFamily =
  | 'NUMERIC_DECIMAL'
  | 'NUMERIC_FLOAT'
  | 'STRING'
  | 'BINARY'
  | 'TEMPORAL'
  | 'OTHER';

export interface DatatypeLabel {
  iri: string;
  i18nKey: string;
  /** 示例词形（tooltip 展示，i18n 键同组） */
  exampleKey: string;
  /** 词法族（05 期：Select 按族分组渲染 + facet 适用族前端判定） */
  family: DatatypeFamily;
}

export const DATATYPE_LABELS: ReadonlyArray<DatatypeLabel> = [
  // 字符串族
  { iri: `${XSD}string`, i18nKey: 'datatype.string', exampleKey: 'datatype.stringExample', family: 'STRING' },
  { iri: `${XSD}normalizedString`, i18nKey: 'datatype.normalizedString', exampleKey: 'datatype.normalizedStringExample', family: 'STRING' },
  { iri: `${XSD}token`, i18nKey: 'datatype.token', exampleKey: 'datatype.tokenExample', family: 'STRING' },
  { iri: `${XSD}language`, i18nKey: 'datatype.language', exampleKey: 'datatype.languageExample', family: 'STRING' },
  { iri: `${XSD}Name`, i18nKey: 'datatype.Name', exampleKey: 'datatype.NameExample', family: 'STRING' },
  { iri: `${XSD}NCName`, i18nKey: 'datatype.NCName', exampleKey: 'datatype.NCNameExample', family: 'STRING' },
  { iri: `${XSD}NMTOKEN`, i18nKey: 'datatype.NMTOKEN', exampleKey: 'datatype.NMTOKENExample', family: 'STRING' },
  // 其他族
  { iri: `${XSD}boolean`, i18nKey: 'datatype.boolean', exampleKey: 'datatype.booleanExample', family: 'OTHER' },
  { iri: `${XSD}anyURI`, i18nKey: 'datatype.anyURI', exampleKey: 'datatype.anyURIExample', family: 'OTHER' },
  // 数值·十进制族
  { iri: `${XSD}decimal`, i18nKey: 'datatype.decimal', exampleKey: 'datatype.decimalExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}integer`, i18nKey: 'datatype.integer', exampleKey: 'datatype.integerExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}long`, i18nKey: 'datatype.long', exampleKey: 'datatype.longExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}int`, i18nKey: 'datatype.int', exampleKey: 'datatype.intExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}short`, i18nKey: 'datatype.short', exampleKey: 'datatype.shortExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}byte`, i18nKey: 'datatype.byte', exampleKey: 'datatype.byteExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}nonNegativeInteger`, i18nKey: 'datatype.nonNegativeInteger', exampleKey: 'datatype.nonNegativeIntegerExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}nonPositiveInteger`, i18nKey: 'datatype.nonPositiveInteger', exampleKey: 'datatype.nonPositiveIntegerExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}positiveInteger`, i18nKey: 'datatype.positiveInteger', exampleKey: 'datatype.positiveIntegerExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}negativeInteger`, i18nKey: 'datatype.negativeInteger', exampleKey: 'datatype.negativeIntegerExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}unsignedLong`, i18nKey: 'datatype.unsignedLong', exampleKey: 'datatype.unsignedLongExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}unsignedInt`, i18nKey: 'datatype.unsignedInt', exampleKey: 'datatype.unsignedIntExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}unsignedShort`, i18nKey: 'datatype.unsignedShort', exampleKey: 'datatype.unsignedShortExample', family: 'NUMERIC_DECIMAL' },
  { iri: `${XSD}unsignedByte`, i18nKey: 'datatype.unsignedByte', exampleKey: 'datatype.unsignedByteExample', family: 'NUMERIC_DECIMAL' },
  // 数值·浮点族
  { iri: `${XSD}float`, i18nKey: 'datatype.float', exampleKey: 'datatype.floatExample', family: 'NUMERIC_FLOAT' },
  { iri: `${XSD}double`, i18nKey: 'datatype.double', exampleKey: 'datatype.doubleExample', family: 'NUMERIC_FLOAT' },
  // 二进制族
  { iri: `${XSD}hexBinary`, i18nKey: 'datatype.hexBinary', exampleKey: 'datatype.hexBinaryExample', family: 'BINARY' },
  { iri: `${XSD}base64Binary`, i18nKey: 'datatype.base64Binary', exampleKey: 'datatype.base64BinaryExample', family: 'BINARY' },
  // 时间族
  { iri: `${XSD}date`, i18nKey: 'datatype.date', exampleKey: 'datatype.dateExample', family: 'TEMPORAL' },
  { iri: `${XSD}dateTime`, i18nKey: 'datatype.dateTime', exampleKey: 'datatype.dateTimeExample', family: 'TEMPORAL' },
  { iri: `${XSD}dateTimeStamp`, i18nKey: 'datatype.dateTimeStamp', exampleKey: 'datatype.dateTimeStampExample', family: 'TEMPORAL' },
  { iri: `${XSD}time`, i18nKey: 'datatype.time', exampleKey: 'datatype.timeExample', family: 'TEMPORAL' },
];

/** IRI → i18n 键（列表 Tag 渲染共用） */
export const DATATYPE_KEY_BY_IRI: Record<string, string> = Object.fromEntries(
  DATATYPE_LABELS.map((d) => [d.iri, d.i18nKey]),
);

/** datatype IRI → 英文短名（xsd:integer 形态，label 辅助展示） */
export const datatypeShortName = (iri: string): string => (iri.startsWith(XSD) ? `xsd:${iri.slice(XSD.length)}` : iri);

/** 族分组元数据（组序 = 下拉展示序；组内按 DATATYPE_LABELS 声明序） */
export const FAMILY_GROUPS: ReadonlyArray<{ family: DatatypeFamily; i18nKey: string }> = [
  { family: 'STRING', i18nKey: 'family.STRING' },
  { family: 'NUMERIC_DECIMAL', i18nKey: 'family.NUMERIC_DECIMAL' },
  { family: 'NUMERIC_FLOAT', i18nKey: 'family.NUMERIC_FLOAT' },
  { family: 'TEMPORAL', i18nKey: 'family.TEMPORAL' },
  { family: 'BINARY', i18nKey: 'family.BINARY' },
  { family: 'OTHER', i18nKey: 'family.OTHER' },
];

/** facet 元数据（与后端 FacetWhitelist/FacetFamilies 同源维护；families 缺省 = 全族适用） */
export interface FacetMeta {
  facet: string;
  i18nKey: string;
  families?: ReadonlyArray<DatatypeFamily>;
  /** 值输入提示（数值/位数提示数字、pattern 提示正则，05 号文档 §6.4） */
  valueHintKey: string;
}

export const FACET_META: ReadonlyArray<FacetMeta> = [
  { facet: 'minInclusive', i18nKey: 'facet.minInclusive', families: ['NUMERIC_DECIMAL', 'NUMERIC_FLOAT', 'TEMPORAL'], valueHintKey: 'facetValueHint.bound' },
  { facet: 'minExclusive', i18nKey: 'facet.minExclusive', families: ['NUMERIC_DECIMAL', 'NUMERIC_FLOAT', 'TEMPORAL'], valueHintKey: 'facetValueHint.bound' },
  { facet: 'maxInclusive', i18nKey: 'facet.maxInclusive', families: ['NUMERIC_DECIMAL', 'NUMERIC_FLOAT', 'TEMPORAL'], valueHintKey: 'facetValueHint.bound' },
  { facet: 'maxExclusive', i18nKey: 'facet.maxExclusive', families: ['NUMERIC_DECIMAL', 'NUMERIC_FLOAT', 'TEMPORAL'], valueHintKey: 'facetValueHint.bound' },
  { facet: 'length', i18nKey: 'facet.length', families: ['STRING', 'BINARY'], valueHintKey: 'facetValueHint.count' },
  { facet: 'minLength', i18nKey: 'facet.minLength', families: ['STRING', 'BINARY'], valueHintKey: 'facetValueHint.count' },
  { facet: 'maxLength', i18nKey: 'facet.maxLength', families: ['STRING', 'BINARY'], valueHintKey: 'facetValueHint.count' },
  { facet: 'totalDigits', i18nKey: 'facet.totalDigits', families: ['NUMERIC_DECIMAL'], valueHintKey: 'facetValueHint.count' },
  { facet: 'fractionDigits', i18nKey: 'facet.fractionDigits', families: ['NUMERIC_DECIMAL'], valueHintKey: 'facetValueHint.count' },
  { facet: 'pattern', i18nKey: 'facet.pattern', valueHintKey: 'facetValueHint.pattern' },
];

/** facet 名 → 元数据（facet 行编辑器查表） */
export const FACET_META_BY_NAME: Record<string, FacetMeta> = Object.fromEntries(
  FACET_META.map((f) => [f.facet, f]),
);

/**
 * 静态冲突五规则的前端同源实现（05 号文档 §3.2 表；后端 DATATYPE_FACET_CONFLICT 为准，
 * 前端即时提示只做体验层拦截——数值比较按 Number 宽松解析即可，精确判定交给后端 big.Rat）。
 * 返回冲突键列表（i18n 键后缀）：保存前端即时红字提示用。
 */
export const detectFacetConflicts = (facets: ReadonlyArray<{ facet: string; value: string }>): string[] => {
  const map = new Map<string, string>();
  for (const f of facets) {
    if (f.facet && f.value) map.set(f.facet, f.value); // 后者覆盖（与后端 NormalizeDefinition 口径一致）
  }
  const num = (v: string | undefined): number | null => {
    if (v == null || v === '') return null;
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  };
  const conflicts: string[] = [];
  if (map.has('minInclusive') && map.has('minExclusive')) conflicts.push('minBoth');
  if (map.has('maxInclusive') && map.has('maxExclusive')) conflicts.push('maxBoth');
  if (map.has('length') && (map.has('minLength') || map.has('maxLength'))) conflicts.push('lengthBoth');
  const min = num(map.get('minInclusive') ?? map.get('minExclusive'));
  const max = num(map.get('maxInclusive') ?? map.get('maxExclusive'));
  if (min != null && max != null) {
    const minExclusive = map.has('minExclusive');
    const maxExclusive = map.has('maxExclusive');
    if (min > max || (min === max && (minExclusive || maxExclusive))) conflicts.push('emptyRange');
  }
  const minLength = num(map.get('minLength'));
  const maxLength = num(map.get('maxLength'));
  if (minLength != null && maxLength != null && minLength > maxLength) conflicts.push('lengthRange');
  return conflicts;
};
