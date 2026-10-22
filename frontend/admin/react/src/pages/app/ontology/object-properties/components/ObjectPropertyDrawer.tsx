import { useEffect, useMemo, useRef, useState } from 'react';
import type { ProFormInstance } from '@ant-design/pro-components';
import { DrawerForm, ProFormCheckbox, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { App, Button, Form, Modal, Select, Space, Tooltip, TreeSelect, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_CreateObjectPropertyRequest,
  ontologyservicev1_ObjectPropertyChain,
  ontologyservicev1_ObjectPropertyTreeNode,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useCreateObjectProperty,
  useObjectPropertyFrame,
  useOntologyObjectPropertyTree,
  useUpdateObjectProperty,
} from '@/api/hooks/ontologyObjectProperty';
import AnnotationListEditor, { cleanAnnotationRows } from '../../components/AnnotationListEditor';
import { useOntologyClassTree } from '@/api/hooks/ontologyClass';
import { PROPERTY_CHARACTERISTICS } from '../constants';

interface ObjectPropertyDrawerProps {
  open: boolean;
  ontologyId: number | null;
  /** 编辑目标属性 id；create 时为 null */
  propertyId: number | null;
  /** create 时的默认父属性集合（新建子属性/兄弟/根属性入口注入） */
  defaultSuperIds: number[];
  onClose: () => void;
  /** 保存成功：携带新属性 id 与默认选中的父属性（树定位用） */
  onSuccess: (saved: { id: number | null; superIds: number[] }) => void;
  /** 乐观锁冲突回调（页面统一刷新树/列表/frame） */
  onVersionConflict: () => void;
}

/** 与后端同口径的局部名正则（NCName 宽松版，允许中文/字母/下划线/连字符，首字符非数字） */
const LOCAL_NAME_PATTERN = /^[\p{L}_][\p{L}\p{N}_-]*$/u;

interface EntityOption {
  id: number;
  label: string;
  deprecated: boolean;
}

/**
 * 对象属性表单（DrawerForm + formRef，对应 Protégé Object Property frame 分区）：
 * 父属性 TreeSelect（多选 treeCheckStrictly，禁自身）、定义域/值域 Select（数据源类树扁平化，
 * 类页权限缺失降级空选项）、逆/不相交/等价 Select（禁自身）、七特征 Checkbox（不做互斥拦截，
 * 组合语义由推理判定）、属性链 Form.List（选中顺序即链序，允许 P∘P 与含超属性自身）。
 *
 * 提交语义（frame 全量提交）：字段缺省 = 不变、空串 = 清空注解；create 不带 base_version。
 */
const ObjectPropertyDrawer: React.FC<ObjectPropertyDrawerProps> = ({
  open,
  ontologyId,
  propertyId,
  defaultSuperIds,
  onClose,
  onSuccess,
  onVersionConflict,
}) => {
  const { t } = useTranslation('ontology');
  const formRef = useRef<ProFormInstance>(null);
  const queryClient = useQueryClient();
  const { message } = App.useApp();
  const [saving, setSaving] = useState(false);

  const isEdit = propertyId != null;

  // frame 详情（编辑态回显 + base_version 乐观锁基准）
  const frameQuery = useObjectPropertyFrame({ id: propertyId ?? 0 }, { enabled: open && isEdit });
  const frame = frameQuery.data;

  // 选择器数据源：属性全量树（父属性 TreeSelect + 逆/不相交/等价/链成员选项）
  const propertyTreeQuery = useOntologyObjectPropertyTree(ontologyId ?? undefined, { enabled: open && ontologyId != null });
  const propertyRoots = propertyTreeQuery.data?.roots ?? [];

  // 定义域/值域数据源：类全量树扁平化（源修订 #4：免分页拼接）；
  // 类页权限缺失时降级空选项不阻塞属性页（铁律 1：原始错误 console.error 带出）
  const classTreeQuery = useOntologyClassTree(ontologyId ?? undefined, {
    enabled: open && ontologyId != null,
    retry: false,
  });
  useEffect(() => {
    if (classTreeQuery.error) {
      // eslint-disable-next-line no-console
      console.error('对象属性页加载类树失败（定义域/值域选项降级为空）：', classTreeQuery.error);
    }
  }, [classTreeQuery.error]);
  const classRoots = classTreeQuery.data?.roots ?? [];

  interface FlatNode {
    id?: number;
    displayLabel?: string;
    localName?: string;
    deprecated?: boolean;
    children?: FlatNode[];
  }

  const flattenTree = (nodes: FlatNode[]): EntityOption[] => {
    const out: EntityOption[] = [];
    const walk = (list: FlatNode[]) => {
      for (const n of list) {
        out.push({ id: n.id!, label: n.displayLabel || n.localName || `#${n.id}`, deprecated: !!n.deprecated });
        walk(n.children ?? []);
      }
    };
    walk(nodes);
    return out;
  };

  const propertyOptions = useMemo<EntityOption[]>(() => flattenTree(propertyRoots), [propertyRoots]);
  const classOptions = useMemo<EntityOption[]>(() => flattenTree(classRoots), [classRoots]);

  const selfId = isEdit ? propertyId : null;

  const propertyTreeData = useMemo<DataNode[]>(() => {
    const toNode = (n: ontologyservicev1_ObjectPropertyTreeNode): DataNode => ({
      key: n.id!,
      title:
        (n.displayLabel || n.localName || `#${n.id}`) +
        (n.deprecated ? `（${t('objectProperty.deprecatedYes')}）` : ''),
      disabled: isEdit && n.id === propertyId, // 自身禁选（体验层；服务端环校验兜底）
      children: (n.children ?? []).map(toNode),
    });
    return propertyRoots.map((root) => toNode(root));
  }, [propertyRoots, isEdit, propertyId, t]);

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
          superPropertyIds: frame.superPropertyIds ?? [],
          domainIds: frame.domainIds ?? [],
          rangeIds: frame.rangeIds ?? [],
          inverseIds: frame.inverseIds ?? [],
          disjointIds: frame.disjointIds ?? [],
          equivalentId: frame.equivalentId ?? undefined,
          characteristics: frame.characteristics ?? [],
          chains: (frame.chains ?? []).map((c: ontologyservicev1_ObjectPropertyChain) => ({
            id: c.id,
            memberIds: c.memberIds ?? [],
          })),
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
          superPropertyIds: defaultSuperIds ?? [],
          domainIds: [],
          rangeIds: [],
          inverseIds: [],
          disjointIds: [],
          equivalentId: undefined,
          characteristics: [],
          chains: [],
          annotations: [],
        });
      }, 0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isEdit, frame, defaultSuperIds]);

  const handleReasonError = (error: unknown): boolean => {
    const reason = getOntologyErrorReason(error);
    const detail = (error as { message?: string })?.message ?? '';
    switch (reason) {
      case 'ONTOLOGY_VERSION_CONFLICT':
        Modal.confirm({
          title: t('objectPropertyErrors.versionConflictTitle'),
          content: detail || t('objectPropertyErrors.versionConflict'),
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
        formRef.current?.setFields([{ name: 'localName', errors: [detail || t('objectPropertyErrors.localNameInvalid')] }]);
        return true;
      case 'HIERARCHY_CYCLIC':
        formRef.current?.setFields([{ name: 'superPropertyIds', errors: [detail || t('objectPropertyErrors.hierarchyCyclic')] }]);
        return true;
      case 'PROPERTY_DOMAIN_INVALID':
      case 'PROPERTY_RANGE_INVALID':
        formRef.current?.setFields([{ name: reason === 'PROPERTY_DOMAIN_INVALID' ? 'domainIds' : 'rangeIds', errors: [detail] }]);
        return true;
      case 'PROPERTY_INVERSE_SELF_INVALID':
        formRef.current?.setFields([{ name: 'inverseIds', errors: [detail] }]);
        return true;
      case 'PROPERTY_SELF_REFERENCE_INVALID':
        formRef.current?.setFields([{ name: 'disjointIds', errors: [detail] }]);
        return true;
      case 'PROPERTY_CHARACTERISTIC_INVALID':
        formRef.current?.setFields([{ name: 'characteristics', errors: [detail] }]);
        return true;
      case 'PROPERTY_CHAIN_INVALID':
      case 'PROPERTY_TARGET_INVALID':
        formRef.current?.setFields([{ name: 'chains', errors: [detail] }]);
        return true;
      case 'ANNOTATION_PROPERTY_TARGET_INVALID':
      case 'ANNOTATION_VALUE_INVALID':
        // 自定义注解区错误（04 期增量）：值空白/谓词非法落行内红字
        formRef.current?.setFields([{ name: 'annotations', errors: [detail] }]);
        return true;
      default:
        return false;
    }
  };

  const createMutation = useCreateObjectProperty({
    onSuccess: (response) => {
      message.success(t('objectProperty.createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['ontologyObjectPropertyTree'] });
      queryClient.invalidateQueries({ queryKey: ['listObjectProperties'] });
      onSuccess({ id: response.id ?? null, superIds: (formRef.current?.getFieldValue('superPropertyIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——reason 分流已呈现，未命中走通用报错
      if (!handleReasonError(error)) {
        message.error(error.message || t('objectProperty.createFailed'));
      }
    },
  });

  const updateMutation = useUpdateObjectProperty({
    onSuccess: () => {
      message.success(t('objectProperty.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['ontologyObjectPropertyTree'] });
      queryClient.invalidateQueries({ queryKey: ['listObjectProperties'] });
      queryClient.invalidateQueries({ queryKey: ['objectPropertyFrame'] });
      onSuccess({ id: propertyId, superIds: (formRef.current?.getFieldValue('superPropertyIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      if (!handleReasonError(error)) {
        message.error(error.message || t('objectProperty.updateFailed'));
      }
    },
  });

  const handleSubmit = async (values: Record<string, any>) => {
    if (ontologyId == null) return false;
    try {
      setSaving(true);
      const chains = (values.chains ?? [])
        .map((c: { id?: number; memberIds?: number[] }) => ({
          id: c.id,
          memberIds: (c.memberIds ?? []).filter((m) => m > 0),
        }))
        .filter((c: { memberIds: number[] }) => c.memberIds.length > 0);
      if (isEdit && propertyId != null) {
        await updateMutation.mutateAsync({
          values: {
            id: propertyId,
            // frame 全量提交：undefined = 不变、空串 = 清空（antd 清空文本框得 ''）
            displayLabel: values.displayLabel ?? '',
            comment: values.comment ?? '',
            deprecated: !!values.deprecated,
            superPropertyIds: values.superPropertyIds ?? [],
            domainIds: values.domainIds ?? [],
            rangeIds: values.rangeIds ?? [],
            inverseIds: values.inverseIds ?? [],
            disjointIds: values.disjointIds ?? [],
            equivalentId: values.equivalentId ?? 0, // 0 = 清空等价（V1 单值）
            characteristics: values.characteristics ?? [],
            chains,
            annotations: cleanAnnotationRows(values.annotations), // 全量替换语义（04 期增量）
            baseVersion: frame?.baseVersion ?? undefined, // 乐观锁基准；nil = 跳过（内部路径）
          },
        });
      } else {
        const payload: Partial<ontologyservicev1_CreateObjectPropertyRequest> = {
          ontologyId,
          localName: values.localName,
          displayLabel: values.displayLabel || undefined,
          comment: values.comment || undefined,
          deprecated: !!values.deprecated,
          superPropertyIds: values.superPropertyIds ?? [],
          domainIds: values.domainIds ?? [],
          rangeIds: values.rangeIds ?? [],
          inverseIds: values.inverseIds ?? [],
          disjointIds: values.disjointIds ?? [],
          equivalentId: values.equivalentId ?? undefined,
          characteristics: values.characteristics ?? [],
          chains,
          annotations: cleanAnnotationRows(values.annotations), // 全量提交（04 期增量）
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

  /** 链预览：P1 ∘ P2 ⊑ 本属性（成员按选中顺序；shouldUpdate 内实时取当前属性名） */
  const selfLabelFallback = frame?.localName || t('objectProperty.chainSelfPlaceholder');

  return (
    <DrawerForm
      formRef={formRef}
      title={isEdit ? t('objectProperty.editTitle') : t('objectProperty.createTitle')}
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
        label={t('objectProperty.localName')}
        placeholder={t('objectProperty.localNamePlaceholder')}
        rules={[
          { required: true, message: t('objectProperty.requiredLocalName') },
          {
            validator: (_rule, value: string) => {
              if (!value || LOCAL_NAME_PATTERN.test(value)) return Promise.resolve();
              return Promise.reject(new Error(t('objectProperty.invalidLocalName')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, disabled: isEdit }}
        extra={isEdit ? t('objectProperty.localNameImmutable') : t('objectProperty.localNameHint')}
      />

      <ProFormText
        name="displayLabel"
        label={t('objectProperty.displayLabel')}
        placeholder={t('objectProperty.displayLabelPlaceholder')}
        fieldProps={{ allowClear: true, maxLength: 255 }}
      />

      <ProFormTextArea
        name="comment"
        label={t('objectProperty.comment')}
        placeholder={t('objectProperty.commentPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 } }}
      />

      <Form.Item
        name="superPropertyIds"
        label={t('objectProperty.superPropertyIds')}
        extra={t('objectProperty.superPropertyIdsHint')}
        // treeCheckStrictly 下 onChange 给 { value, label } 数组，归一为 number[] 入库
        getValueFromEvent={(value: unknown) =>
          Array.isArray(value)
            ? value.map((item: unknown) =>
                typeof item === 'object' && item !== null ? (item as { value: number }).value : item,
              )
            : value
        }
      >
        <TreeSelect
          treeData={propertyTreeData}
          placeholder={t('objectProperty.superPropertyIdsPlaceholder')}
          allowClear
          multiple
          treeCheckable
          treeCheckStrictly
          showCheckedStrategy={TreeSelect.SHOW_ALL}
          treeLine
          maxTagCount={6}
        />
      </Form.Item>

      <Form.Item name="domainIds" label={t('objectProperty.domainIds')} extra={t('objectProperty.domainRangeHint')}>
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('objectProperty.domainIdsPlaceholder')}
          optionFilterProp="label"
          options={classOptions.map((o) => ({ value: o.id, label: o.label }))}
          notFoundContent={classTreeQuery.error ? t('objectProperty.classOptionsUnavailable') : undefined}
        />
      </Form.Item>

      <Form.Item name="rangeIds" label={t('objectProperty.rangeIds')} extra={t('objectProperty.domainRangeHint')}>
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('objectProperty.rangeIdsPlaceholder')}
          optionFilterProp="label"
          options={classOptions.map((o) => ({ value: o.id, label: o.label }))}
          notFoundContent={classTreeQuery.error ? t('objectProperty.classOptionsUnavailable') : undefined}
        />
      </Form.Item>

      <Form.Item name="inverseIds" label={t('objectProperty.inverseIds')} extra={t('objectProperty.symmetricHint')}>
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('objectProperty.inverseIdsPlaceholder')}
          optionFilterProp="label"
          options={propertyOptions
            .filter((o) => !isEdit || o.id !== selfId)
            .map((o) => ({ value: o.id, label: o.label }))}
        />
      </Form.Item>

      <Form.Item name="disjointIds" label={t('objectProperty.disjointIds')} extra={t('objectProperty.symmetricHint')}>
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('objectProperty.disjointIdsPlaceholder')}
          optionFilterProp="label"
          options={propertyOptions
            .filter((o) => !isEdit || o.id !== selfId)
            .map((o) => ({ value: o.id, label: o.label }))}
        />
      </Form.Item>

      <Form.Item name="equivalentId" label={t('objectProperty.equivalentId')} extra={t('objectProperty.equivalentIdHint')}>
        <Select
          showSearch
          allowClear
          placeholder={t('objectProperty.equivalentIdPlaceholder')}
          optionFilterProp="label"
          options={propertyOptions
            .filter((o) => !isEdit || o.id !== selfId)
            .map((o) => ({ value: o.id, label: o.label }))}
        />
      </Form.Item>

      <ProFormCheckbox.Group
        name="characteristics"
        label={
          <Tooltip title={t('objectProperty.characteristicsHint')} placement="top">
            <span>{t('objectProperty.characteristics')}</span>
          </Tooltip>
        }
        options={PROPERTY_CHARACTERISTICS.map((c) => ({ label: t(c.i18nKey), value: c.iri }))}
      />

      <Form.List name="chains">
        {(fields, { add, remove }) => (
          <Form.Item label={t('objectProperty.chains')} extra={t('objectProperty.chainsHint')} style={{ marginBottom: 0 }}>
            {fields.map(({ key, name, ...restField }) => (
              <Space key={key} align="baseline" style={{ display: 'flex', marginBottom: 4 }} className="ontology-chain-row">
                <Form.Item
                  {...restField}
                  name={[name, 'memberIds']}
                  noStyle
                >
                  <Select
                    mode="multiple"
                    showSearch
                    allowClear
                    maxTagCount={4}
                    placeholder={t('objectProperty.chainMembersPlaceholder')}
                    optionFilterProp="label"
                    style={{ minWidth: 380 }}
                    options={propertyOptions.map((o) => ({ value: o.id, label: o.label }))}
                  />
                </Form.Item>
                <Form.Item {...restField} name={[name, 'id']} hidden>
                  <input type="hidden" />
                </Form.Item>
                <Button
                  type="text"
                  danger
                  size="small"
                  icon={<DeleteOutlined />}
                  onClick={() => remove(name)}
                />
              </Space>
            ))}
            <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add({ memberIds: [] })}>
              {t('objectProperty.addChain')}
            </Button>
            <Form.Item shouldUpdate noStyle>
              {() => {
                const chains = (formRef.current?.getFieldValue('chains') ?? []) as Array<{ memberIds?: number[] }>;
                const currentLabel =
                  (formRef.current?.getFieldValue('displayLabel') as string) ||
                  (formRef.current?.getFieldValue('localName') as string) ||
                  selfLabelFallback;
                const previews = chains
                  .filter((c) => (c.memberIds ?? []).length > 0)
                  .map((c) => {
                    const labels = (c.memberIds ?? []).map(
                      (id) => propertyOptions.find((o) => o.id === id)?.label ?? `#${id}`,
                    );
                    return `${labels.join(' ∘ ')} ⊑ ${currentLabel}`;
                  });
                return previews.length > 0 ? (
                  <div className="ontology-chain-preview">
                    {previews.map((p, i) => (
                      <Typography key={i}>
                        <span style={{ fontFamily: 'monospace' }}>{p}</span>
                      </Typography>
                    ))}
                  </div>
                ) : null;
              }}
            </Form.Item>
          </Form.Item>
        )}
      </Form.List>

      {/* 自定义注解区（04 期增量——AnnotationListEditor 共享组件，六类实体 frame 共用） */}
      <Form.Item
        name="annotations"
        label={t('objectProperty.annotations')}
        extra={t('annotationEditor.hint')}
      >
        <AnnotationListEditor />
      </Form.Item>

      <ProFormSwitch name="deprecated" label={t('objectProperty.deprecated')} />
    </DrawerForm>
  );
};

export default ObjectPropertyDrawer;
