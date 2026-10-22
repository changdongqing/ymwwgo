import { useEffect, useMemo } from 'react';
import { Button, Empty, Input, Select, Space, Typography } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import type { ontologyservicev1_AnnotationAssertionInput } from '@/api/generated/admin/service/v1';
import { useOntologyAnnotationPropertyTree } from '@/api/hooks/ontologyAnnotationProperty';
import { useOntologyStore } from '@/stores';

interface AnnotationOption {
  id: number;
  label: string;
  deprecated: boolean;
}

interface AnnotationListEditorProps {
  /** 受控值（antd Form 标准受控模式：Form.Item name 直接挂载本组件） */
  value?: ontologyservicev1_AnnotationAssertionInput[];
  onChange?: (value: ontologyservicev1_AnnotationAssertionInput[]) => void;
  disabled?: boolean;
}

/** 行载荷清洗（各 Drawer 提交前统一调用）：过滤空谓词/空白值行（脏行不上送），lang 空串归 undefined */
export const cleanAnnotationRows = (
  rows?: ontologyservicev1_AnnotationAssertionInput[],
): ontologyservicev1_AnnotationAssertionInput[] =>
  (rows ?? [])
    .map((row) => ({ propertyId: row.propertyId, value: row.value ?? '', lang: row.lang || undefined }))
    .filter((row) => (row.propertyId ?? 0) > 0 && String(row.value).trim() !== '');

/**
 * 通用注解区行式编辑器（04 期前端核心增量——Protégé OWLAnnotationsFrame 的对应物，
 * 04 号文档 §6.4）：每行 = 注解属性下拉（showSearch）+ 值输入（TextArea 单行高度）+
 * 语言输入（可空）+ 删除；底部"添加注解"。同属性多行合法（OWL 注解断言集合语义）。
 *
 * 被类/对象属性/数据属性/注解属性四个 Drawer 共用（06 期个体表单零成本复用）。
 * 谓词候选仅列本体内声明的注解属性实体（内建 rdfs:label 等走专属字段，双入口规避——
 * 04 号文档 §2.3）；数据源为注解属性树接口扁平化，权限缺失降级空选项（铁律 1：
 * 原始错误 console.error 带出）。
 *
 * 提交载荷 = AnnotationAssertionInput[]（{ propertyId, value, lang }）；
 * 行内红字校验属性/值必填由各 Drawer 的表单规则执行（本组件只做受控渲染）。
 */
const AnnotationListEditor: React.FC<AnnotationListEditorProps> = ({ value, onChange, disabled }) => {
  const { t } = useTranslation('ontology');
  const rows = value ?? [];

  // 当前活动本体（切换器全局状态；抽屉挂载点均在本体页内，store 与抽屉 ontologyId 同源）
  const ontologyId = useOntologyStore((s) => s.currentOntologyId);

  // 谓词候选数据源：注解属性全量树扁平化（复用 02 期"拉树扁平化 + 权限缺失降级空选项"模式）
  const treeQuery = useOntologyAnnotationPropertyTree(ontologyId ?? undefined, { retry: false });
  useEffect(() => {
    if (treeQuery.error) {
      // eslint-disable-next-line no-console
      console.error('注解区加载注解属性树失败（谓词候选降级为空）：', treeQuery.error);
    }
  }, [treeQuery.error]);

  const options = useMemo<AnnotationOption[]>(() => {
    const out: AnnotationOption[] = [];
    const walk = (nodes: Array<{ id?: number; displayLabel?: string; localName?: string; deprecated?: boolean; children?: unknown[] }>) => {
      for (const n of nodes) {
        out.push({ id: n.id!, label: n.displayLabel || n.localName || `#${n.id}`, deprecated: !!n.deprecated });
        walk((n.children ?? []) as typeof nodes);
      }
    };
    walk(treeQuery.data?.roots ?? []);
    return out;
  }, [treeQuery.data]);

  const emit = (next: ontologyservicev1_AnnotationAssertionInput[]) => onChange?.(next);

  const updateRow = (index: number, patch: Partial<ontologyservicev1_AnnotationAssertionInput>) => {
    emit(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));
  };

  const removeRow = (index: number) => {
    emit(rows.filter((_, i) => i !== index));
  };

  const addRow = () => {
    emit([...rows, { propertyId: undefined as unknown as number, value: '', lang: undefined }]);
  };

  return (
    <div className="annotation-list-editor">
      {rows.length === 0 && options.length > 0 && (
        <Typography.Text type="secondary" style={{ display: 'block', marginBottom: 4 }}>
          {t('annotationEditor.emptyHint')}
        </Typography.Text>
      )}
      {rows.map((row, index) => (
        <Space.Compact key={index} block style={{ display: 'flex', marginBottom: 8, gap: 8 }}>
          <Select
            style={{ minWidth: 200, flex: '0 0 200px' }}
            showSearch
            allowClear
            placeholder={t('annotationEditor.propertyPlaceholder')}
            optionFilterProp="label"
            disabled={disabled}
            status={row.propertyId == null || row.propertyId <= 0 ? 'error' : undefined}
            value={row.propertyId ?? undefined}
            onChange={(v) => updateRow(index, { propertyId: v })}
            notFoundContent={
              treeQuery.error ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('annotationEditor.optionsUnavailable')} />
              ) : options.length === 0 ? (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('annotationEditor.noPropertyHint')} />
              ) : undefined
            }
            options={options.map((o) => ({
              value: o.id,
              label: o.deprecated ? `${o.label}（${t('annotationEditor.deprecated')}）` : o.label,
            }))}
          />
          <Input.TextArea
            style={{ flex: 1, minHeight: 32 }}
            autoSize={{ minRows: 1, maxRows: 4 }}
            placeholder={t('annotationEditor.valuePlaceholder')}
            disabled={disabled}
            status={!row.value || !String(row.value).trim() ? 'error' : undefined}
            value={row.value ?? ''}
            onChange={(e) => updateRow(index, { value: e.target.value })}
          />
          <Input
            style={{ width: 90, flex: '0 0 90px' }}
            placeholder={t('annotationEditor.langPlaceholder')}
            disabled={disabled}
            value={row.lang ?? ''}
            onChange={(e) => updateRow(index, { lang: e.target.value })}
          />
          <Button
            type="text"
            danger
            disabled={disabled}
            icon={<DeleteOutlined />}
            onClick={() => removeRow(index)}
            title={t('annotationEditor.removeRow')}
          />
        </Space.Compact>
      ))}
      <Button size="small" type="dashed" block disabled={disabled} icon={<PlusOutlined />} onClick={addRow}>
        {t('annotationEditor.addRow')}
      </Button>
    </div>
  );
};

export default AnnotationListEditor;
