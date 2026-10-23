import { useEffect, useMemo, useRef, useState } from 'react';
import type { ProFormInstance } from '@ant-design/pro-components';
import { DrawerForm, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { Alert, App, Button, Form, Input, Modal, Select, Space, Tooltip, Typography } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_CreateDatatypeRequest,
  ontologyservicev1_ReferencePosition,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useCreateDatatype,
  useDatatypeFrame,
  useUpdateDatatype,
} from '@/api/hooks/ontologyDatatype';
import AnnotationListEditor, { cleanAnnotationRows } from '../../components/AnnotationListEditor';
import EntityReferencesPanel from '../../components/EntityReferencesPanel';
import {
  DATATYPE_LABELS,
  FACET_META,
  datatypeShortName,
  detectFacetConflicts,
  FAMILY_GROUPS,
  type DatatypeFamily,
} from '../../constants';

interface DatatypeDrawerProps {
  open: boolean;
  ontologyId: number | null;
  /** 编辑目标 datatype id；create 时为 null */
  datatypeId: number | null;
  onClose: () => void;
  /** 保存成功（列表刷新回调） */
  onSuccess: () => void;
  /** 乐观锁冲突回调（页面统一刷新） */
  onVersionConflict: () => void;
  /** 「被引用」面板点击数据类型节点 → 切到该数据类型的编辑抽屉（同页跳转；缺省禁用跳转） */
  onOpenDatatype?: (datatypeId: number) => void;
}

/** 「被引用」默认集（08 号文档 §6.2：数据类型 42 派生轨 / 46，宾语位）——模块级常量保持引用稳定 */
const DATATYPE_REFERENCE_AXIOM_TYPES: number[] = [42, 46];
const OBJECT_POSITION: ontologyservicev1_ReferencePosition[] = ['REFERENCE_POSITION_OBJECT'];

/** 与后端同口径的局部名正则（NCName 宽松版，允许中文/字母/下划线/连字符，首字符非数字） */
const LOCAL_NAME_PATTERN = /^[\p{L}_][\p{L}\p{N}_-]*$/u;

interface FacetRow {
  facet?: string;
  value?: string;
}

/**
 * 数据类型表单（DrawerForm + formRef，对应 Protégé OWLDatatypeDescriptionFrame——
 * 仅"类型定义"一节：OWL 2 不给 datatype 父级/等价/不相交/特征分区，05 号文档 §6.3）。
 *
 * 类型定义分区（§6.4 facet 行式编辑器）：基类型 Select 按族分组 + facet 行式编辑器
 * （facet 下拉按当前基类型适用族过滤 + 值输入 + 删除行）；基类型切换后不适用族的行红字提示
 * （不自动删除，交由用户确认）；静态冲突五规则前端即时提示（后端为准）；"清除定义"开关
 * 提交显式空定义 {}（presence=true），不提交字段则跳过 diff——两段语义前端必须区分。
 */
const DatatypeDrawer: React.FC<DatatypeDrawerProps> = ({
  open,
  ontologyId,
  datatypeId,
  onClose,
  onSuccess,
  onVersionConflict,
  onOpenDatatype,
}) => {
  const { t } = useTranslation('ontology');
  const formRef = useRef<ProFormInstance>(null);
  const [form] = Form.useForm();
  const queryClient = useQueryClient();
  const { message } = App.useApp();
  const [saving, setSaving] = useState(false);

  const isEdit = datatypeId != null;

  // frame 详情（编辑态回显 + base_version 乐观锁基准）
  const frameQuery = useDatatypeFrame({ id: datatypeId ?? 0 }, { enabled: open && isEdit });
  const frame = frameQuery.data;

  // 基类型族 / facet 行 / 清除定义开关的响应式读取（facet 行式编辑器即时提示用）
  const baseIri = Form.useWatch('baseIri', form) as string | undefined;
  const facetRows = Form.useWatch('facets', form) as FacetRow[] | undefined;
  const clearDefinition = Form.useWatch('clearDefinition', form) as boolean | undefined;

  const baseFamily: DatatypeFamily | undefined = useMemo(
    () => DATATYPE_LABELS.find((d) => d.iri === baseIri)?.family,
    [baseIri],
  );

  // 基类型 Select：31 项按族分组（组序 = FAMILY_GROUPS 声明序）
  const baseOptions = useMemo(
    () =>
      FAMILY_GROUPS.map((g) => ({
        label: t(g.i18nKey),
        options: DATATYPE_LABELS.filter((d) => d.family === g.family).map((d) => ({
          value: d.iri,
          label: `${t(d.i18nKey)}（${datatypeShortName(d.iri)}）`,
          title: `${t(d.i18nKey)}（${datatypeShortName(d.iri)}）`,
          example: t(d.exampleKey),
        })),
      })),
    [t],
  );

  // facet 下拉选项：按当前基类型适用族过滤（pattern 全族适用——families 缺省）
  const applicableFacets = useMemo(
    () => FACET_META.filter((f) => !f.families || !baseFamily || f.families.includes(baseFamily)),
    [baseFamily],
  );

  // 静态冲突即时提示（§3.2 五规则前端同源实现；后端 DATATYPE_FACET_CONFLICT 为准）
  const conflictKeys = useMemo(
    () => detectFacetConflicts((facetRows ?? []).map((r) => ({ facet: r.facet ?? '', value: r.value ?? '' }))),
    [facetRows],
  );
  // 基类型切换后不适用族的 facet 行（红字提示，不自动删除——防误删）
  const inapplicableRowIndexes = useMemo(() => {
    if (!baseFamily) return new Set<number>();
    const out = new Set<number>();
    (facetRows ?? []).forEach((row, index) => {
      const meta = FACET_META.find((f) => f.facet === row.facet);
      if (meta?.families && !meta.families.includes(baseFamily)) out.add(index);
    });
    return out;
  }, [facetRows, baseFamily]);
  // 同 facet 名重复行（提交后被去重，后者覆盖——即时提示）
  const duplicatedFacetNames = useMemo(() => {
    const seen = new Set<string>();
    const dup = new Set<string>();
    for (const row of facetRows ?? []) {
      if (!row.facet) continue;
      if (seen.has(row.facet)) dup.add(row.facet);
      seen.add(row.facet);
    }
    return dup;
  }, [facetRows]);

  // 编辑态回显（destroyOnHidden 时需延迟赋值）
  useEffect(() => {
    if (!open) return;
    if (isEdit) {
      if (!frame) return;
      setTimeout(() => {
        formRef.current?.setFieldsValue({
          localName: frame.localName || '',
          displayLabel: frame.displayLabel ?? undefined,
          comment: frame.comment ?? undefined,
          deprecated: frame.deprecated ?? false,
          clearDefinition: false,
          baseIri: frame.definition?.baseIri ?? undefined,
          facets: (frame.definition?.facets ?? []).map((f) => ({ facet: f.facet, value: f.value ?? '' })),
          annotations: (frame.annotations ?? []).map((a) => ({
            propertyId: a.propertyId,
            value: a.value ?? '',
            lang: a.lang ?? undefined,
          })),
        });
      }, 0);
    } else {
      setTimeout(() => {
        formRef.current?.setFieldsValue({
          localName: '',
          displayLabel: undefined,
          comment: undefined,
          deprecated: false,
          clearDefinition: false,
          baseIri: undefined,
          facets: [],
          annotations: [],
        });
      }, 0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isEdit, frame]);

  const handleReasonError = (error: unknown): boolean => {
    const reason = getOntologyErrorReason(error);
    const detail = (error as { message?: string })?.message ?? '';
    switch (reason) {
      case 'ONTOLOGY_VERSION_CONFLICT':
        Modal.confirm({
          title: t('datatypeErrors.versionConflictTitle'),
          content: detail || t('datatypeErrors.versionConflict'),
          okText: t('common:button.retry'),
          cancelText: t('common:button.cancel'),
          onOk: () => {
            onVersionConflict();
            onClose();
          },
        });
        return true;
      case 'LOCAL_NAME_INVALID':
      case 'ENTITY_IRI_DUPLICATE':
        formRef.current?.setFields([{ name: 'localName', errors: [detail || t('datatypeErrors.localNameInvalid')] }]);
        return true;
      case 'DATATYPE_BASE_INVALID':
      case 'DATATYPE_DEFINITION_JSON_INVALID':
        formRef.current?.setFields([{ name: 'baseIri', errors: [detail] }]);
        return true;
      case 'DATATYPE_FACET_INVALID':
      case 'DATATYPE_FACET_CONFLICT':
        formRef.current?.setFields([{ name: 'facets', errors: [detail] }]);
        return true;
      case 'ANNOTATION_PROPERTY_TARGET_INVALID':
      case 'ANNOTATION_VALUE_INVALID':
        formRef.current?.setFields([{ name: 'annotations', errors: [detail] }]);
        return true;
      default:
        return false;
    }
  };

  const createMutation = useCreateDatatype({
    onSuccess: (response) => {
      message.success(t('datatype.createSuccess', { id: response.id ?? '' }));
      queryClient.invalidateQueries({ queryKey: ['listDatatypes'] });
      queryClient.invalidateQueries({ queryKey: ['simpleDatatypes'] });
      onSuccess();
      onClose();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——reason 分流已呈现，未命中走通用报错
      if (!handleReasonError(error)) {
        message.error(error.message || t('datatype.createFailed'));
      }
    },
  });

  const updateMutation = useUpdateDatatype({
    onSuccess: () => {
      message.success(t('datatype.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listDatatypes'] });
      queryClient.invalidateQueries({ queryKey: ['simpleDatatypes'] });
      queryClient.invalidateQueries({ queryKey: ['datatypeFrame'] });
      onSuccess();
      onClose();
    },
    onError: (error: Error) => {
      if (!handleReasonError(error)) {
        message.error(error.message || t('datatype.updateFailed'));
      }
    },
  });

  const handleSubmit = async (values: Record<string, any>) => {
    if (ontologyId == null) return false;
    try {
      setSaving(true);
      // definition 两段语义（05 号文档 §4.3/§6.4）：
      //   清除定义开关开启 → 显式空定义 {}（清空存量）；
      //   开关关闭 + 有 base/facets → 提交定义；
      //   开关关闭 + 全空 + frame 原有定义 → 用户清空了表单，视为显式清空 {}；
      //   开关关闭 + 全空 + frame 无定义 → 不提交该字段（跳过 diff，不误生定义）。
      const facetList = ((values.facets ?? []) as FacetRow[])
        .filter((r) => r.facet && r.value != null && String(r.value).trim() !== '')
        .map((r) => ({ facet: r.facet!, value: String(r.value) }));
      const hasContent = !!values.baseIri || facetList.length > 0;
      let definition: { baseIri?: string; facets: { facet: string; value: string }[] } | undefined;
      if (values.clearDefinition) {
        definition = { facets: [] };
      } else if (hasContent) {
        definition = { baseIri: values.baseIri ?? undefined, facets: facetList };
      } else if (isEdit && frame?.definition) {
        definition = { facets: [] };
      }
      if (isEdit && datatypeId != null) {
        await updateMutation.mutateAsync({
          values: {
            id: datatypeId,
            displayLabel: values.displayLabel ?? '',
            comment: values.comment ?? '',
            deprecated: !!values.deprecated,
            definition,
            annotations: cleanAnnotationRows(values.annotations),
            baseVersion: frame?.baseVersion ?? undefined,
          },
        });
      } else {
        const payload: Partial<ontologyservicev1_CreateDatatypeRequest> = {
          ontologyId,
          localName: values.localName,
          displayLabel: values.displayLabel || undefined,
          comment: values.comment || undefined,
          deprecated: !!values.deprecated,
          definition: hasContent ? { baseIri: values.baseIri ?? undefined, facets: facetList } : undefined,
          annotations: cleanAnnotationRows(values.annotations),
        };
        await createMutation.mutateAsync({ values: payload });
      }
      return true;
    } catch {
      return false;
    } finally {
      setSaving(false);
    }
  };

  return (
    <DrawerForm
      form={form}
      formRef={formRef}
      title={isEdit ? t('datatype.editTitle') : t('datatype.createTitle')}
      open={open}
      width={680}
      onOpenChange={(visible) => {
        if (!visible) {
          formRef.current?.resetFields();
          onClose();
        }
      }}
      onFinish={handleSubmit}
      submitter={{
        searchConfig: {
          submitText: t('common:button.submit'),
          resetText: t('common:button.cancel'),
        },
        submitButtonProps: {
          loading: saving || createMutation.isPending || updateMutation.isPending,
          disabled: ontologyId == null,
        },
        resetButtonProps: { onClick: onClose },
      }}
      drawerProps={{ destroyOnHidden: true, onClose }}
    >
      <ProFormText
        name="localName"
        label={t('datatype.localName')}
        placeholder={t('datatype.localNamePlaceholder')}
        rules={[
          { required: true, message: t('datatype.requiredLocalName') },
          {
            validator: (_rule, value: string) => {
              if (!value || LOCAL_NAME_PATTERN.test(value)) return Promise.resolve();
              return Promise.reject(new Error(t('datatype.invalidLocalName')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, disabled: isEdit }}
        extra={isEdit ? t('datatype.localNameImmutable') : t('datatype.localNameHint')}
      />

      <ProFormText
        name="displayLabel"
        label={t('datatype.displayLabel')}
        placeholder={t('datatype.displayLabelPlaceholder')}
        fieldProps={{ allowClear: true, maxLength: 255 }}
      />

      <ProFormTextArea
        name="comment"
        label={t('datatype.comment')}
        placeholder={t('datatype.commentPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 } }}
      />

      {/* 类型定义分区（Protégé OWLDatatypeDescriptionFrame 同构：frame 仅此一节逻辑分区） */}
      <ProFormSwitch
        name="clearDefinition"
        label={t('datatype.clearDefinition')}
        extra={t('datatype.clearDefinitionHint')}
      />

      <Form.Item
        name="baseIri"
        label={t('datatype.baseIri')}
        extra={t('datatype.baseIriHint')}
      >
        <Select
          showSearch
          allowClear
          placeholder={t('datatype.baseIriPlaceholder')}
          optionFilterProp="label"
          disabled={clearDefinition}
          options={baseOptions}
          optionRender={(option) => (
            <Tooltip title={String((option.data as { example?: string }).example ?? '')} placement="right">
              <span>{option.label}</span>
            </Tooltip>
          )}
        />
      </Form.Item>

      <Form.Item
        label={t('datatype.facets')}
        extra={t('datatype.facetsHint')}
        style={{ marginBottom: conflictKeys.length > 0 || inapplicableRowIndexes.size > 0 ? 8 : 24 }}
      >
        <Form.List name="facets">
          {(fields, { add, remove }) => (
            <>
              {fields.map((field) => {
                const row = (facetRows ?? [])[field.name];
                const meta = FACET_META.find((f) => f.facet === row?.facet);
                const inapplicable = inapplicableRowIndexes.has(field.name);
                const duplicated = row?.facet != null && duplicatedFacetNames.has(row.facet);
                return (
                  <Space key={field.key} align="baseline" style={{ display: 'flex', marginBottom: 4 }}>
                    <Form.Item
                      name={[field.name, 'facet']}
                      noStyle
                    >
                      <Select
                        showSearch
                        allowClear
                        placeholder={t('datatype.facetPlaceholder')}
                        optionFilterProp="label"
                        style={{ width: 220 }}
                        disabled={clearDefinition}
                        options={applicableFacets.map((f) => ({ value: f.facet, label: t(f.i18nKey) }))}
                      />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'value']}
                      noStyle
                      rules={[{ required: false }]}
                    >
                      <Input
                        placeholder={meta ? t(meta.valueHintKey) : t('datatype.facetValuePlaceholder')}
                        style={{ width: 260 }}
                        disabled={clearDefinition}
                      />
                    </Form.Item>
                    <Button
                      type="text"
                      danger
                      icon={<DeleteOutlined />}
                      onClick={() => remove(field.name)}
                      disabled={clearDefinition}
                    />
                    {(inapplicable || duplicated) && (
                      <span style={{ color: 'var(--ant-color-error)', fontSize: 12 }}>
                        {inapplicable ? t('datatype.facetFamilyMismatch') : t('datatype.facetDuplicated')}
                      </span>
                    )}
                  </Space>
                );
              })}
              <Button
                type="dashed"
                block
                icon={<PlusOutlined />}
                onClick={() => add({ facet: undefined, value: '' })}
                disabled={clearDefinition || !baseIri}
              >
                {t('datatype.addFacet')}
              </Button>
            </>
          )}
        </Form.List>
      </Form.Item>

      {conflictKeys.length > 0 && !clearDefinition && (
        <Alert
          type="error"
          showIcon
          style={{ marginBottom: 16 }}
          message={conflictKeys.map((k) => t(`facetConflict.${k}`)).join('；')}
        />
      )}

      {/* 自定义注解区（04 期通用注解区共享组件，datatype 也可挂注解——零改动复用） */}
      <Form.Item
        name="annotations"
        label={t('datatype.annotations')}
        extra={t('annotationEditor.hint')}
      >
        <AnnotationListEditor />
      </Form.Item>

      <ProFormSwitch name="deprecated" label={t('datatype.deprecated')} />

      {/* 「被引用」面板（08 期共享组件：数据类型宾语位，42 值域派生轨） */}
      {isEdit && (
        <div style={{ marginTop: 24, borderTop: '1px solid #f0f0f0', paddingTop: 16 }}>
          <Typography.Title level={5} style={{ marginTop: 0 }}>
            {t('reference.sectionTitle')}
          </Typography.Title>
          <EntityReferencesPanel
            ontologyId={ontologyId}
            entityId={datatypeId}
            defaultAxiomTypes={DATATYPE_REFERENCE_AXIOM_TYPES}
            defaultPositions={OBJECT_POSITION}
            openableEntityType={50}
            onOpenEntity={onOpenDatatype}
            enabled={open && datatypeId != null}
          />
        </div>
      )}
    </DrawerForm>
  );
};

export default DatatypeDrawer;
