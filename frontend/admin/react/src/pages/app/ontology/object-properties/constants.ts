/**
 * 对象属性特征（OWL 2 词汇表七特征）共享映射：
 * IRI → i18n 键（`ontology:characteristic.*`），列表 Tag 列与 Drawer Checkbox 组共用。
 * 数组顺序 = proto OntologyPropertyCharacteristic 声明序（后端 characteristics 已按声明序返回，
 * 此处顺序仅作渲染锚点与表单复选框顺序）。
 */

export const PROPERTY_CHARACTERISTICS: ReadonlyArray<{ iri: string; i18nKey: string }> = [
  { iri: 'http://www.w3.org/2002/07/owl#FunctionalProperty', i18nKey: 'characteristic.functional' },
  { iri: 'http://www.w3.org/2002/07/owl#InverseFunctionalProperty', i18nKey: 'characteristic.inverseFunctional' },
  { iri: 'http://www.w3.org/2002/07/owl#TransitiveProperty', i18nKey: 'characteristic.transitive' },
  { iri: 'http://www.w3.org/2002/07/owl#SymmetricProperty', i18nKey: 'characteristic.symmetric' },
  { iri: 'http://www.w3.org/2002/07/owl#AsymmetricProperty', i18nKey: 'characteristic.asymmetric' },
  { iri: 'http://www.w3.org/2002/07/owl#ReflexiveProperty', i18nKey: 'characteristic.reflexive' },
  { iri: 'http://www.w3.org/2002/07/owl#IrreflexiveProperty', i18nKey: 'characteristic.irreflexive' },
];

export const PROPERTY_CHARACTERISTIC_KEY_BY_IRI: Record<string, string> = Object.fromEntries(
  PROPERTY_CHARACTERISTICS.map((c) => [c.iri, c.i18nKey]),
);

/** owl:topObjectProperty 虚拟根的固定 key（不入库，前端固定渲染，owl:Thing 模式镜像） */
export const OWL_TOP_OBJECT_PROPERTY_KEY = 'owl:topObjectProperty';
