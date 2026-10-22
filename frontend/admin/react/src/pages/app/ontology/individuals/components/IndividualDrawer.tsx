import { useEffect, useMemo, useState } from 'react';
import { DrawerForm, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { App, Button, Form, Input, Modal, Radio, Select, Space, Spin, TreeSelect, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_CreateIndividualRequest,
  ontologyservicev1_DataAssertionInput,
  ontologyservicev1_ObjectAssertionInput,
  ontologyservicev1_OntologyClassTreeNode,
  ontologyservicev1_UpdateIndividualRequest,
} from '@/api/generated/admin/service/v1';
import {
  fetchListIndividuals,
  getOntologyErrorReason,
  useCreateIndividual,
  useIndividualCodePreview,
  useIndividualFrame,
  useUpdateIndividual,
} from '@/api/hooks/ontologyIndividual';
import { useOntologyClassTree } from '@/api/hooks/ontologyClass';
import { useOntologyDataPropertyTree } from '@/api/hooks/ontologyDataProperty';
import { useOntologyObjectPropertyTree } from '@/api/hooks/ontologyObjectProperty';
import AnnotationListEditor, { cleanAnnotationRows } from '../../components/AnnotationListEditor';

interface IndividualDrawerProps {
  open: boolean;
  ontologyId: number | null;
  /** 编辑目标个体 id；create 时为 null */
  individualId: number | null;
  onClose: () => void;
  /** 保存成功（页面刷新列表/计数徽标） */
  onSuccess: () => void;
  /** 乐观锁冲突回调（页面统一刷新） */
  onVersionConflict: () => void;
}

/** 与后端同口径的编码字符集（06 号文档 §3.3：首字符字母/数字，允许点/下划线/连字符） */
const CODE_CHARSET_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

/**
 * 个体表单（DrawerForm + Form 实例，06 号文档 §6.3/§6.4）：
 * 创建动线 = 主类目（必选，赋码依据）→ 预计编码（AUTO 实时预览，"预计"文案）→ 其余类型 →
 * 编码模式（AUTO/MANUAL，手输编码跳过取号器）→ 显示名/备注/废弃 → 自定义注解 →
 * 高级折叠（localName 手输 + 编码属性指定，防"编码当标识"回潮——§2.1 落点）。
 * 编辑 = 顶部实体横幅（完整 IRI）+ 类型区（改主类目不影响已赋编码）+ 编码区（默认只读 + 显式「改码」）
 * + 属性断言区（61/62 行编辑）+ 注解区；提交 frame 全量 + base_version，服务端逐区 diff
 * （断言区/注解区全量替换语义：空数组 = 清空，故编辑态始终回传整区）。
 */
const IndividualDrawer: React.FC<IndividualDrawerProps> = ({
  open,
  ontologyId,
  individualId,
  onClose,
  onSuccess,
  onVersionConflict,
}) => {
  const { t } = useTranslation('ontology');
  const [form] = Form.useForm();
  const queryClient = useQueryClient();
  const { message } = App.useApp();
  const [saving, setSaving] = useState(false);

  const isEdit = individualId != null;

  // frame 详情（编辑态回显 + base_version 乐观锁基准）
  const frameQuery = useIndividualFrame({ id: individualId ?? 0 }, { enabled: open && isEdit });
  const frame = frameQuery.data;

  // 选择器数据源：类树 / 对象属性树（谓词收 IRI）/ 数据属性树（谓词收实体 id）
  const fullTreeQuery = useOntologyClassTree(ontologyId ?? undefined, { enabled: open && ontologyId != null });
  const treeRoots = fullTreeQuery.data?.roots ?? [];
  const objectPropTreeQuery = useOntologyObjectPropertyTree(ontologyId ?? undefined, {
    enabled: open && ontologyId != null,
  });
  const dataPropTreeQuery = useOntologyDataPropertyTree(ontologyId ?? undefined, {
    enabled: open && ontologyId != null,
  });

  const classTreeData = useMemo<(DataNode & { value: string })[]>(() => {
    // TreeSelect 选项必须显式带 value（其内部 valueEntities 按节点 value 字段注册；
    // 缺省时全部注册为 undefined，单选确认时内部 .find(...).label 崩）。
    // key/value 统一 string，提交侧 Number() 转回实体 id。
    const toNode = (n: ontologyservicev1_OntologyClassTreeNode): DataNode & { value: string } => ({
      key: String(n.id!),
      value: String(n.id!),
      title: (n.displayLabel || n.localName || `#${n.id}`) + (n.deprecated ? `（${t('individual.deprecatedYes')}）` : ''),
      children: (n.children ?? []).map(toNode),
    });
    return treeRoots.map((root) => toNode(root));
  }, [treeRoots, t]);

  /** 属性树拍平为选项（61 谓词 value=IRI / 62 谓词 value=实体 id） */
  const objectPropOptions = useMemo(
    () => flattenPropOptions(objectPropTreeQuery.data?.roots ?? [], 'iri'),
    [objectPropTreeQuery.data],
  );
  const dataPropOptions = useMemo(
    () => flattenPropOptions(dataPropTreeQuery.data?.roots ?? [], 'id'),
    [dataPropTreeQuery.data],
  );

  // 预计编码（AUTO 模式，主类目选定后实时调 code-preview；预览不加锁——文案必须写"预计"）
  const watchedMainClassId = Form.useWatch('mainClassId', form);
  const watchedCodeSource = Form.useWatch('codeSource', form);
  const autoMode = (watchedCodeSource ?? 'AUTO') !== 'MANUAL';
  const watchedMainClassIdNum = watchedMainClassId != null ? Number(watchedMainClassId) : 0;
  const previewQuery = useIndividualCodePreview(
    { ontologyId: ontologyId ?? 0, mainClassId: watchedMainClassIdNum },
    {
      enabled:
        open && !isEdit && ontologyId != null && watchedMainClassIdNum > 0 && autoMode,
    },
  );

  // 编辑态：显式「改码」开启编码编辑（默认只读展示当前编码——§6.4）
  const [codeEditing, setCodeEditing] = useState(false);

  // 编辑态回显（destroyOnHidden 时需延迟赋值）；断言区始终整区回传（全量替换语义）
  useEffect(() => {
    if (!open) return;
    if (isEdit) {
      if (!frame) return;
      setCodeEditing(false);
      setTimeout(() => {
        form.setFieldsValue({
          displayLabel: frame.displayLabel ?? undefined,
          comment: frame.comment ?? undefined,
          deprecated: frame.deprecated ?? false,
          mainClassId: frame.types?.[0]?.id != null ? String(frame.types[0].id) : undefined,
          typeIds: (frame.types ?? []).slice(1).map((n) => String(n.id!)),
          code: frame.code?.value ?? undefined,
          objectAssertions: (frame.objectAssertions ?? []).map((o) => ({
            predicateIri: o.predicateIri || undefined,
            targetId: o.target?.id,
          })),
          dataAssertions: (frame.dataAssertions ?? []).map((d) => ({
            propertyId: d.propertyId,
            value: d.value ?? '',
            lang: d.lang || undefined,
          })),
          annotations: (frame.annotations ?? []).map((a) => ({
            propertyId: a.propertyId,
            value: a.value ?? '',
            lang: a.lang || undefined,
          })),
        });
      }, 0);
    } else {
      setTimeout(() => {
        form.setFieldsValue({
          mainClassId: undefined,
          typeIds: [],
          codeSource: 'AUTO',
          code: undefined,
          displayLabel: undefined,
          comment: undefined,
          deprecated: false,
          localName: undefined,
          codePropertyId: undefined,
          objectAssertions: [],
          dataAssertions: [],
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
          title: t('individualErrors.versionConflictTitle'),
          content: detail || t('individualErrors.versionConflict'),
          okText: t('common:button.retry'),
          cancelText: t('common:button.cancel'),
          onOk: () => {
            onVersionConflict();
            onClose();
          },
        });
        return true;
      case 'INDIVIDUAL_MAIN_CLASS_REQUIRED':
        form.setFields([{ name: 'mainClassId', errors: [detail || t('individualErrors.mainClassRequired')] }]);
        return true;
      case 'INDIVIDUAL_CODE_SOURCE_INVALID':
      case 'INDIVIDUAL_CODE_PREFIX_INVALID':
      case 'INDIVIDUAL_CODE_REQUIRED':
      case 'INDIVIDUAL_CODE_DUPLICATE':
        form.setFields([{ name: 'code', errors: [detail || t('individualErrors.codeDuplicate')] }]);
        return true;
      case 'INDIVIDUAL_SERIAL_CONFLICT':
        message.error(detail || t('individualErrors.serialConflict'));
        return true;
      case 'LOCAL_NAME_INVALID':
      case 'ENTITY_IRI_DUPLICATE':
        form.setFields([{ name: 'localName', errors: [detail || t('individualErrors.localNameInvalid')] }]);
        return true;
      case 'INDIVIDUAL_TARGET_INVALID':
        message.error(detail || t('individualErrors.targetInvalid'));
        return true;
      case 'ENTITY_TYPE_MISMATCH':
        form.setFields([{ name: 'mainClassId', errors: [detail] }]);
        return true;
      case 'PROPERTY_TARGET_INVALID':
      case 'DATA_PROPERTY_TARGET_INVALID':
        message.error(detail);
        return true;
      case 'ANNOTATION_PROPERTY_TARGET_INVALID':
      case 'ANNOTATION_VALUE_INVALID':
        form.setFields([{ name: 'annotations', errors: [detail] }]);
        return true;
      default:
        return false;
    }
  };

  const createMutation = useCreateIndividual({
    onSuccess: (response) => {
      message.success(
        response.code
          ? t('individual.createSuccessWithCode', { code: response.code })
          : t('individual.createSuccess'),
      );
      queryClient.invalidateQueries({ queryKey: ['listIndividuals'] });
      queryClient.invalidateQueries({ queryKey: ['individualsCountByClass'] });
      onSuccess();
      onClose();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——reason 分流已呈现，未命中走通用报错
      if (!handleReasonError(error)) {
        message.error(error.message || t('individual.createFailed'));
      }
    },
  });

  const updateMutation = useUpdateIndividual({
    onSuccess: () => {
      message.success(t('individual.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listIndividuals'] });
      queryClient.invalidateQueries({ queryKey: ['individualFrame'] });
      queryClient.invalidateQueries({ queryKey: ['individualsCountByClass'] });
      onSuccess();
      onClose();
    },
    onError: (error: Error) => {
      if (!handleReasonError(error)) {
        message.error(error.message || t('individual.updateFailed'));
      }
    },
  });

  /** 61 宾语个体远程搜索（List 接口 keyword 通道共用——06 号文档 §5） */
  const searchIndividuals = async (keyword: string): Promise<{ value: number; label: string }[]> => {
    if (ontologyId == null) return [];
    const response = await fetchListIndividuals({
      ontologyId,
      page: 1,
      pageSize: 20,
      keyword: keyword || undefined,
      code: '',
      codeQueryType: 'EXACT',
      classId: 0,
      includeDescendants: false,
      orderBy: '["-updated_at"]',
    });
    return (response.items ?? []).map((item) => ({
      value: item.id!,
      label: `${item.displayLabel || item.localName || `#${item.id}`}${item.code ? `（${item.code}）` : ''}`,
    }));
  };

  const handleSubmit = async (values: Record<string, any>) => {
    if (ontologyId == null) return false;
    try {
      setSaving(true);
      if (isEdit && individualId != null) {
        const codeChanged = codeEditing && values.code != null && values.code !== (frame?.code?.value ?? '');
        await updateMutation.mutateAsync({
          values: {
            id: individualId,
            mainClassId: Number(values.mainClassId),
            typeIds: (values.typeIds ?? []).map(Number),
            // 改码：显式开启且值有变化才提交；未开启 = 编码不变（undefined = 服务端跳过编码区）
            code: codeChanged ? values.code : undefined,
            displayLabel: values.displayLabel ?? '',
            comment: values.comment ?? '',
            deprecated: !!values.deprecated,
            objectAssertions: cleanObjectAssertionRows(values.objectAssertions),
            dataAssertions: cleanDataAssertionRows(values.dataAssertions),
            annotations: cleanAnnotationRows(values.annotations),
            baseVersion: frame?.baseVersion ?? undefined,
          } as Partial<ontologyservicev1_UpdateIndividualRequest>,
        });
      } else {
        const payload: Partial<ontologyservicev1_CreateIndividualRequest> = {
          ontologyId,
          mainClassId: Number(values.mainClassId),
          typeIds: (values.typeIds ?? []).map(Number),
          codeSource: values.codeSource ?? 'AUTO',
          code: values.codeSource === 'MANUAL' ? values.code || undefined : undefined,
          localName: values.localName || undefined,
          displayLabel: values.displayLabel || undefined,
          comment: values.comment || undefined,
          deprecated: !!values.deprecated,
          codePropertyId: values.codePropertyId || undefined,
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
      title={isEdit ? t('individual.editTitle') : t('individual.createTitle')}
      open={open}
      width={680}
      onOpenChange={(visible) => {
        if (!visible) {
          form.resetFields();
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
      {isEdit && frame && (
        <div className="ontology-individual-banner">
          <Typography.Text strong>{frame.displayLabel || frame.localName}</Typography.Text>
          <Typography.Text code copyable={{ text: frame.iri ?? '' }} className="ontology-individual-banner-iri">
            {frame.iri}
          </Typography.Text>
        </div>
      )}

      {/* 主类目（必选，赋码依据；编辑时提示"改主类目不影响已赋编码"——§4.3） */}
      <Form.Item
        name="mainClassId"
        label={t('individual.mainClass')}
        rules={[{ required: true, message: t('individualErrors.mainClassRequired') }]}
        extra={isEdit ? t('individual.mainClassHintEdit') : t('individual.mainClassHintCreate')}
      >
        <TreeSelect
          treeData={classTreeData}
          placeholder={t('individual.mainClassPlaceholder')}
          allowClear
          treeLine
          showSearch
          treeNodeFilterProp="title"
        />
      </Form.Item>

      {!isEdit && autoMode && watchedMainClassIdNum > 0 && (
        <Form.Item label={t('individual.codePreview')} extra={t('individual.codePreviewHint')}>
          {previewQuery.isLoading ? (
            <Spin size="small" />
          ) : previewQuery.error ? (
            <Typography.Text type="warning">{t('individual.codePreviewUnavailable')}</Typography.Text>
          ) : (
            <Typography.Text code strong>
              {previewQuery.data?.previewCode}
            </Typography.Text>
          )}
        </Form.Item>
      )}

      {/* 其余类型（可空；主类目之外按提交序存储） */}
      <Form.Item
        name="typeIds"
        label={t('individual.typeIds')}
        extra={t('individual.typeIdsHint')}
        getValueFromEvent={(value: unknown) =>
          Array.isArray(value)
            ? value.map((item: unknown) =>
                typeof item === 'object' && item !== null ? (item as { value: number }).value : item,
              )
            : value
        }
      >
        <TreeSelect
          treeData={classTreeData}
          placeholder={t('individual.typeIdsPlaceholder')}
          allowClear
          multiple
          treeCheckable
          treeCheckStrictly
          showCheckedStrategy={TreeSelect.SHOW_ALL}
          treeLine
          maxTagCount={6}
        />
      </Form.Item>

      {!isEdit && (
        <>
          {/* 编码模式（AUTO 取号 / MANUAL 手输——手输编码将跳过取号器） */}
          <Form.Item name="codeSource" label={t('individual.codeSource')} initialValue="AUTO">
            <Radio.Group
              options={[
                { label: t('individual.codeSourceAuto'), value: 'AUTO' },
                { label: t('individual.codeSourceManual'), value: 'MANUAL' },
              ]}
            />
          </Form.Item>
          {watchedCodeSource === 'MANUAL' && (
            <Form.Item
              name="code"
              label={t('individual.code')}
              rules={[
                { required: true, message: t('individualErrors.codeRequired') },
                {
                  validator: (_rule, value: string) => {
                    if (!value || CODE_CHARSET_PATTERN.test(value)) return Promise.resolve();
                    return Promise.reject(new Error(t('individualErrors.codeCharset')));
                  },
                },
              ]}
              extra={t('individual.codeManualHint')}
            >
              <Input allowClear style={{ fontFamily: 'monospace' }} placeholder={t('individual.codePlaceholder')} />
            </Form.Item>
          )}
        </>
      )}

      {isEdit && (
        <>
          {/* 编码区：默认只读 + 显式「改码」开启编辑（§6.4） */}
          <Form.Item label={t('individual.code')} extra={t('individual.codeImmutableHint')}>
            <Space.Compact style={{ width: '100%' }}>
              <Input
                value={codeEditing ? undefined : frame?.code?.value || '-'}
                placeholder={codeEditing ? t('individual.codePlaceholder') : undefined}
                style={{ fontFamily: 'monospace' }}
                disabled={!codeEditing}
              />
              {codeEditing ? (
                <Button onClick={() => setCodeEditing(false)}>{t('individual.codeCancel')}</Button>
              ) : (
                <Button onClick={() => setCodeEditing(true)}>{t('individual.codeEdit')}</Button>
              )}
            </Space.Compact>
          </Form.Item>
          {codeEditing && (
            <Form.Item
              name="code"
              label={t('individual.codeNew')}
              rules={[
                { required: true, message: t('individualErrors.codeRequired') },
                {
                  validator: (_rule, value: string) => {
                    if (!value || CODE_CHARSET_PATTERN.test(value)) return Promise.resolve();
                    return Promise.reject(new Error(t('individualErrors.codeCharset')));
                  },
                },
              ]}
              extra={t('individual.codeEditHint')}
            >
              <Input allowClear style={{ fontFamily: 'monospace' }} placeholder={t('individual.codePlaceholder')} />
            </Form.Item>
          )}
        </>
      )}

      <ProFormText
        name="displayLabel"
        label={t('individual.displayLabel')}
        placeholder={t('individual.displayLabelPlaceholder')}
        fieldProps={{ allowClear: true, maxLength: 255 }}
      />

      <ProFormTextArea
        name="comment"
        label={t('individual.comment')}
        placeholder={t('individual.commentPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 } }}
      />

      {/* 属性断言区（编辑态；61 对象 / 62 数据两 section，对照 Protégé 属性断言视图——§6.4） */}
      {isEdit && (
        <>
          <Form.Item label={t('individual.objectAssertions')} extra={t('individual.objectAssertionsHint')}>
            <AssertionRows
              name="objectAssertions"
              addLabel={t('individual.addObjectAssertion')}
              predicateName="predicateIri"
              predicateOptions={objectPropOptions}
              renderValueRow={(key) => (
                <Form.Item
                  key={key}
                  name={[key, 'targetId']}
                  noStyle
                  rules={[{ required: true, message: t('individualErrors.targetRequired') }]}
                >
                  <IndividualRemoteSelect searchIndividuals={searchIndividuals} placeholder={t('individual.targetPlaceholder')} />
                </Form.Item>
              )}
            />
          </Form.Item>

          <Form.Item label={t('individual.dataAssertions')} extra={t('individual.dataAssertionsHint')}>
            <AssertionRows
              name="dataAssertions"
              addLabel={t('individual.addDataAssertion')}
              predicateName="propertyId"
              predicateOptions={dataPropOptions}
              renderValueRow={(key) => (
                <>
                  <Form.Item
                    key={`${key}-value`}
                    name={[key, 'value']}
                    noStyle
                    rules={[{ required: true, message: t('individualErrors.valueRequired') }]}
                  >
                    <Input placeholder={t('individual.valuePlaceholder')} style={{ flex: 1, minWidth: 160 }} />
                  </Form.Item>
                  <Form.Item key={`${key}-lang`} name={[key, 'lang']} noStyle>
                    <Input placeholder={t('individual.langPlaceholder')} style={{ width: 90 }} allowClear />
                  </Form.Item>
                </>
              )}
            />
          </Form.Item>
        </>
      )}

      {/* 自定义注解区（04 期共享组件零成本复用——06 号文档 §6.3） */}
      <Form.Item name="annotations" label={t('individual.annotations')} extra={t('annotationEditor.hint')}>
        <AnnotationListEditor />
      </Form.Item>

      <ProFormSwitch name="deprecated" label={t('individual.deprecated')} />

      {/* 高级折叠区（仅创建态）：localName 默认折叠于"高级"，编码是独立字段——防止"编码当标识"回潮 */}
      {!isEdit && (
        <details className="ontology-individual-advanced">
          <summary>{t('individual.advanced')}</summary>
          <ProFormText
            name="localName"
            label={t('individual.localName')}
            placeholder={t('individual.localNamePlaceholder')}
            extra={t('individual.localNameHint')}
          />
          <Form.Item name="codePropertyId" label={t('individual.codePropertyId')} extra={t('individual.codePropertyIdHint')}>
            <Select
              showSearch
              allowClear
              placeholder={t('individual.codePropertyIdPlaceholder')}
              optionFilterProp="label"
              options={dataPropOptions}
            />
          </Form.Item>
        </details>
      )}
    </DrawerForm>
  );
};

// ---- 断言区与下拉的小组件 ----

interface PropOption {
  value: string | number;
  label: string;
}

/** 属性树拍平（对象/数据属性共用）：object 模式 value=IRI（61 提交收谓词 IRI）、id 模式 value=实体 id */
function flattenPropOptions<T extends { id?: number; iri?: string; localName?: string; displayLabel?: string | null; deprecated?: boolean; children?: T[] }>(
  roots: T[],
  mode: 'iri' | 'id',
): PropOption[] {
  const out: PropOption[] = [];
  const walk = (nodes: T[]) => {
    for (const n of nodes) {
      const label = n.displayLabel || n.localName || `#${n.id}`;
      out.push({ value: mode === 'iri' ? n.iri ?? '' : n.id!, label });
      walk((n.children ?? []) as T[]);
    }
  };
  walk(roots);
  return out.filter((o) => o.value !== '' && o.value != null);
}

interface AssertionRowsProps {
  name: string;
  addLabel: string;
  /** 谓词字段名：61 用 predicateIri、62 用 propertyId */
  predicateName: string;
  predicateOptions: PropOption[];
  renderValueRow: (key: number) => React.ReactNode;
}

/** 断言行编辑器（Form.List；谓词下拉 + 值行 + 删除按钮） */
function AssertionRows({ name, addLabel, predicateName, predicateOptions, renderValueRow }: AssertionRowsProps) {
  return (
    <Form.List name={name}>
      {(fields, { add, remove }) => (
        <div className="ontology-individual-assertions">
          {fields.map((field) => (
            <Space.Compact key={field.key} style={{ display: 'flex', marginBottom: 8, width: '100%' }} block>
              <Form.Item name={[field.name, predicateName]} noStyle rules={[{ required: true, message: '' }]}>
                <Select
                  showSearch
                  allowClear
                  style={{ minWidth: 200 }}
                  placeholder={addLabel}
                  optionFilterProp="label"
                  options={predicateOptions}
                />
              </Form.Item>
              {renderValueRow(field.name)}
              <Button type="text" icon={<DeleteOutlined />} onClick={() => remove(field.name)} aria-label="remove" />
            </Space.Compact>
          ))}
          <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add()}>
            {addLabel}
          </Button>
        </div>
      )}
    </Form.List>
  );
}

/** 61 宾语个体远程搜索下拉（keyword 过滤走 List 接口；onFocus 预载首批） */
function IndividualRemoteSelect({
  searchIndividuals,
  placeholder,
  value,
  onChange,
}: {
  searchIndividuals: (keyword: string) => Promise<{ value: number; label: string }[]>;
  placeholder: string;
  value?: number;
  onChange?: (v: number) => void;
}) {
  const [options, setOptions] = useState<{ value: number; label: string }[]>([]);
  const [fetching, setFetching] = useState(false);
  const load = (keyword: string) => {
    setFetching(true);
    searchIndividuals(keyword)
      .then(setOptions)
      .catch((error) => {
        // 铁律 1：远程搜索失败带出原始错误
        console.error('individual remote search failed', error);
        setOptions([]);
      })
      .finally(() => setFetching(false));
  };
  return (
    <Select
      showSearch
      allowClear
      value={value}
      placeholder={placeholder}
      style={{ flex: 1, minWidth: 200 }}
      filterOption={false}
      notFoundContent={fetching ? <Spin size="small" /> : null}
      onSearch={load}
      onFocus={() => {
        if (options.length === 0) load('');
      }}
      options={options}
      onChange={(v) => onChange?.(v)}
    />
  );
}

// 断言区提交清洗：谓词空/值空行剔除（前端空行防御；全量替换语义）
function cleanObjectAssertionRows(rows?: { predicateIri?: string; targetId?: number }[]): ontologyservicev1_ObjectAssertionInput[] {
  return (rows ?? [])
    .filter((r) => !!r.predicateIri && (r.targetId ?? 0) > 0)
    .map((r) => ({ predicateIri: r.predicateIri!, targetId: r.targetId! }));
}

function cleanDataAssertionRows(rows?: { propertyId?: number; value?: string; lang?: string }[]): ontologyservicev1_DataAssertionInput[] {
  return (rows ?? [])
    .filter((r) => (r.propertyId ?? 0) > 0 && String(r.value ?? '').trim() !== '')
    .map((r) => ({ propertyId: r.propertyId!, value: r.value!, lang: r.lang || undefined }));
}

export default IndividualDrawer;
