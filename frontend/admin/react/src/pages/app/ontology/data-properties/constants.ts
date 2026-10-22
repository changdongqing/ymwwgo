/**
 * 数据属性共享常量（03 期）：
 * - owl:topDataProperty 虚拟根固定 key（不入库，前端固定渲染，owl:topObjectProperty 模式镜像）；
 * - 内建 XSD datatype 白名单选项（IRI + i18n 键 + 示例文案）——与后端 Go 侧
 *   BuiltinDatatypes 双份维护（OWL 2 规范封闭集合，漂移风险可控；05 期接管时评估改为接口下发，
 *   03 号文档 §3.2）。数组顺序 = 下拉选项展示序（string 在前，与其余按词法族分组）。
 */

export const OWL_TOP_DATA_PROPERTY_KEY = 'owl:topDataProperty';

export const XSD = 'http://www.w3.org/2001/XMLSchema#';

export interface DatatypeLabel {
  iri: string;
  i18nKey: string;
  /** 示例词形（tooltip 展示，i18n 键同组） */
  exampleKey: string;
}

export const DATATYPE_LABELS: ReadonlyArray<DatatypeLabel> = [
  { iri: `${XSD}string`, i18nKey: 'datatype.string', exampleKey: 'datatype.stringExample' },
  { iri: `${XSD}boolean`, i18nKey: 'datatype.boolean', exampleKey: 'datatype.booleanExample' },
  { iri: `${XSD}decimal`, i18nKey: 'datatype.decimal', exampleKey: 'datatype.decimalExample' },
  { iri: `${XSD}integer`, i18nKey: 'datatype.integer', exampleKey: 'datatype.integerExample' },
  { iri: `${XSD}long`, i18nKey: 'datatype.long', exampleKey: 'datatype.longExample' },
  { iri: `${XSD}int`, i18nKey: 'datatype.int', exampleKey: 'datatype.intExample' },
  { iri: `${XSD}short`, i18nKey: 'datatype.short', exampleKey: 'datatype.shortExample' },
  { iri: `${XSD}byte`, i18nKey: 'datatype.byte', exampleKey: 'datatype.byteExample' },
  { iri: `${XSD}nonNegativeInteger`, i18nKey: 'datatype.nonNegativeInteger', exampleKey: 'datatype.nonNegativeIntegerExample' },
  { iri: `${XSD}float`, i18nKey: 'datatype.float', exampleKey: 'datatype.floatExample' },
  { iri: `${XSD}double`, i18nKey: 'datatype.double', exampleKey: 'datatype.doubleExample' },
  { iri: `${XSD}date`, i18nKey: 'datatype.date', exampleKey: 'datatype.dateExample' },
  { iri: `${XSD}dateTime`, i18nKey: 'datatype.dateTime', exampleKey: 'datatype.dateTimeExample' },
  { iri: `${XSD}time`, i18nKey: 'datatype.time', exampleKey: 'datatype.timeExample' },
  { iri: `${XSD}anyURI`, i18nKey: 'datatype.anyURI', exampleKey: 'datatype.anyURIExample' },
];

/** IRI → i18n 键（列表 Tag 渲染共用） */
export const DATATYPE_KEY_BY_IRI: Record<string, string> = Object.fromEntries(
  DATATYPE_LABELS.map((d) => [d.iri, d.i18nKey]),
);

/** datatype IRI → 英文短名（xsd:integer 形态，label 辅助展示） */
export const datatypeShortName = (iri: string): string => iri.startsWith(XSD) ? `xsd:${iri.slice(XSD.length)}` : iri;
