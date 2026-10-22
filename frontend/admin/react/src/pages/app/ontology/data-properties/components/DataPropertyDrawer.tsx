import { useEffect, useMemo, useRef, useState } from 'react';
import type { ProFormInstance } from '@ant-design/pro-components';
import { DrawerForm, ProFormCheckbox, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { App, Form, Modal, Select, Tooltip, TreeSelect } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_CreateDataPropertyRequest,
  ontologyservicev1_DataPropertyTreeNode,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useCreateDataProperty,
  useDataPropertyFrame,
  useOntologyDataPropertyTree,
  useUpdateDataProperty,
} from '@/api/hooks/ontologyDataProperty';
import { useOntologyClassTree } from '@/api/hooks/ontologyClass';
import AnnotationListEditor, { cleanAnnotationRows } from '../../components/AnnotationListEditor';
import { useSimpleDatatypes } from '@/api/hooks/ontologyDatatype';
import { DATATYPE_LABELS, datatypeShortName, FAMILY_GROUPS } from '../../constants';

interface DataPropertyDrawerProps {
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

/** 函数性特征 IRI（数据属性 V1 唯一特征，OWL 2 语言约束——逆函数性等六种是对象属性专属） */
const FUNCTIONAL_IRI = 'http://www.w3.org/2002/07/owl#FunctionalProperty';

interface EntityOption {
  id: number;
  label: string;
  deprecated: boolean;
}

/**
 * 数据属性表单（DrawerForm + formRef，对应 Protégé Data Property frame 分区）：
 * 父属性 TreeSelect（多选 treeCheckStrictly，禁自身）、定义域 Select（类树扁平化，
 * 类页权限缺失降级空选项）、值域 Select（内建 datatype 白名单选项，前端即拦白名单外）、
 * 不相交/等价 Select（禁自身，等价 V1 单选）、函数性单 Checkbox（无逆属性/属性链分区——
 * UI 减法本身是 Protégé 同构的一部分，03 号文档 §6.3）。
 *
 * 提交语义（frame 全量提交）：字段缺省 = 不变、空串 = 清空；create 不带 base_version。
 */
const DataPropertyDrawer: React.FC<DataPropertyDrawerProps> = ({
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
  const frameQuery = useDataPropertyFrame({ id: propertyId ?? 0 }, { enabled: open && isEdit });
  const frame = frameQuery.data;

  // 选择器数据源：属性全量树（父属性 TreeSelect + 不相交/等价选项）
  const propertyTreeQuery = useOntologyDataPropertyTree(ontologyId ?? undefined, { enabled: open && ontologyId != null });
  const propertyRoots = propertyTreeQuery.data?.roots ?? [];

  // 定义域数据源：类全量树扁平化（复用 02 期通道；类页权限缺失时降级空选项不阻塞，
  // 铁律 1：原始错误 console.error 带出）
  const classTreeQuery = useOntologyClassTree(ontologyId ?? undefined, {
    enabled: open && ontologyId != null,
    retry: false,
  });
  useEffect(() => {
    if (classTreeQuery.error) {
      // eslint-disable-next-line no-console
      console.error('数据属性页加载类树失败（定义域选项降级为空）：', classTreeQuery.error);
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
    const toNode = (n: ontologyservicev1_DataPropertyTreeNode): DataNode => ({
      key: n.id!,
      title:
        (n.displayLabel || n.localName || `#${n.id}`) +
        (n.deprecated ? `（${t('dataProperty.deprecatedYes')}）` : ''),
      disabled: isEdit && n.id === propertyId, // 自身禁选（体验层；服务端环校验兜底）
      children: (n.children ?? []).map(toNode),
    });
    return propertyRoots.map((root) => toNode(root));
  }, [propertyRoots, isEdit, propertyId, t]);

  // 本体派生 datatype 数据源（05 期值域双源下拉第二源；03 号文档 §6.3 → 05 号文档 §6.5）
  const simpleDatatypesQuery = useSimpleDatatypes(
    { ontologyId: ontologyId ?? 0 },
    { enabled: open && ontologyId != null },
  );
  const simpleDatatypes = simpleDatatypesQuery.data?.items ?? [];

  // 值域双源选项（05 号文档 §6.5）：① 内建 31 项按族分组；② 本体派生 datatype
  // （文案 localName（基类型短名），deprecated 删除线但可选）。
  // API 单形态（range_iris 内建 IRI ∪ datatype 实体 IRI），前端无双轨概念。
  const rangeOptions = useMemo(() => {
    const builtinGroups = FAMILY_GROUPS.map((g) => ({
      label: t(g.i18nKey),
      options: DATATYPE_LABELS.filter((d) => d.family === g.family).map((d) => ({
        value: d.iri,
        label: `${t(d.i18nKey)}（${datatypeShortName(d.iri)}）`,
        title: `${t(d.i18nKey)}（${datatypeShortName(d.iri)}）`,
        example: t(d.exampleKey),
      })),
    }));
    const customGroup = {
      label: t('datatype.customGroupTitle'),
      options: simpleDatatypes.map((d) => ({
        value: d.iri!,
        label: `${d.displayLabel || d.localName}（${d.baseIri ? datatypeShortName(d.baseIri) : t('datatype.undefined')}）${d.deprecated ? '（' + t('datatype.deprecatedYes') + '）' : ''}`,
        title: `${d.displayLabel || d.localName}（${d.baseIri ? datatypeShortName(d.baseIri) : t('datatype.undefined')}）`,
        example: d.iri,
      })),
    };
    return simpleDatatypes.length > 0 ? [...builtinGroups, customGroup] : builtinGroups;
  }, [t, simpleDatatypes]);

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
          rangeIris: frame.rangeIris ?? [],
          disjointIds: frame.disjointIds ?? [],
          equivalentId: frame.equivalentId ?? undefined,
          functional: (frame.characteristics ?? []).includes(FUNCTIONAL_IRI),
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
          rangeIris: [],
          disjointIds: [],
          equivalentId: undefined,
          functional: false,
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
          title: t('dataPropertyErrors.versionConflictTitle'),
          content: detail || t('dataPropertyErrors.versionConflict'),
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
        formRef.current?.setFields([{ name: 'localName', errors: [detail || t('dataPropertyErrors.localNameInvalid')] }]);
        return true;
      case 'HIERARCHY_CYCLIC':
        formRef.current?.setFields([{ name: 'superPropertyIds', errors: [detail || t('dataPropertyErrors.hierarchyCyclic')] }]);
        return true;
      case 'DATA_PROPERTY_DOMAIN_INVALID':
        formRef.current?.setFields([{ name: 'domainIds', errors: [detail] }]);
        return true;
      case 'DATA_PROPERTY_RANGE_INVALID':
      case 'DATATYPE_RANGE_TARGET_INVALID': // 05 期派生轨：值域引用类型不符/跨本体（§2.2 三分支）
        formRef.current?.setFields([{ name: 'rangeIris', errors: [detail] }]);
        return true;
      case 'DATA_PROPERTY_SELF_REFERENCE_INVALID':
        formRef.current?.setFields([{ name: 'disjointIds', errors: [detail] }]);
        return true;
      case 'DATA_PROPERTY_CHARACTERISTIC_INVALID':
        formRef.current?.setFields([{ name: 'functional', errors: [detail] }]);
        return true;
      case 'DATA_PROPERTY_TARGET_INVALID':
        formRef.current?.setFields([{ name: 'superPropertyIds', errors: [detail] }]);
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

  const createMutation = useCreateDataProperty({
    onSuccess: (response) => {
      message.success(t('dataProperty.createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['ontologyDataPropertyTree'] });
      queryClient.invalidateQueries({ queryKey: ['listDataProperties'] });
      onSuccess({ id: response.id ?? null, superIds: (formRef.current?.getFieldValue('superPropertyIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——reason 分流已呈现，未命中走通用报错
      if (!handleReasonError(error)) {
        message.error(error.message || t('dataProperty.createFailed'));
      }
    },
  });

  const updateMutation = useUpdateDataProperty({
    onSuccess: () => {
      message.success(t('dataProperty.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['ontologyDataPropertyTree'] });
      queryClient.invalidateQueries({ queryKey: ['listDataProperties'] });
      queryClient.invalidateQueries({ queryKey: ['dataPropertyFrame'] });
      onSuccess({ id: propertyId, superIds: (formRef.current?.getFieldValue('superPropertyIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      if (!handleReasonError(error)) {
        message.error(error.message || t('dataProperty.updateFailed'));
      }
    },
  });

  const handleSubmit = async (values: Record<string, any>) => {
    if (ontologyId == null) return false;
    try {
      setSaving(true);
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
            rangeIris: values.rangeIris ?? [],
            disjointIds: values.disjointIds ?? [],
            equivalentId: values.equivalentId ?? 0, // 0 = 清空等价（V1 单值）
            characteristics: values.functional ? [FUNCTIONAL_IRI] : [],
            annotations: cleanAnnotationRows(values.annotations), // 全量替换语义（04 期增量）
            baseVersion: frame?.baseVersion ?? undefined, // 乐观锁基准；nil = 跳过（内部路径）
          },
        });
      } else {
        const payload: Partial<ontologyservicev1_CreateDataPropertyRequest> = {
          ontologyId,
          localName: values.localName,
          displayLabel: values.displayLabel || undefined,
          comment: values.comment || undefined,
          deprecated: !!values.deprecated,
          superPropertyIds: values.superPropertyIds ?? [],
          domainIds: values.domainIds ?? [],
          rangeIris: values.rangeIris ?? [],
          disjointIds: values.disjointIds ?? [],
          equivalentId: values.equivalentId ?? undefined,
          characteristics: values.functional ? [FUNCTIONAL_IRI] : [],
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

  return (
    <DrawerForm
      formRef={formRef}
      title={isEdit ? t('dataProperty.editTitle') : t('dataProperty.createTitle')}
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
        label={t('dataProperty.localName')}
        placeholder={t('dataProperty.localNamePlaceholder')}
        rules={[
          { required: true, message: t('dataProperty.requiredLocalName') },
          {
            validator: (_rule, value: string) => {
              if (!value || LOCAL_NAME_PATTERN.test(value)) return Promise.resolve();
              return Promise.reject(new Error(t('dataProperty.invalidLocalName')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, disabled: isEdit }}
        extra={isEdit ? t('dataProperty.localNameImmutable') : t('dataProperty.localNameHint')}
      />

      <ProFormText
        name="displayLabel"
        label={t('dataProperty.displayLabel')}
        placeholder={t('dataProperty.displayLabelPlaceholder')}
        fieldProps={{ allowClear: true, maxLength: 255 }}
      />

      <ProFormTextArea
        name="comment"
        label={t('dataProperty.comment')}
        placeholder={t('dataProperty.commentPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 } }}
      />

      <Form.Item
        name="superPropertyIds"
        label={t('dataProperty.superPropertyIds')}
        extra={t('dataProperty.superPropertyIdsHint')}
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
          placeholder={t('dataProperty.superPropertyIdsPlaceholder')}
          allowClear
          multiple
          treeCheckable
          treeCheckStrictly
          showCheckedStrategy={TreeSelect.SHOW_ALL}
          treeLine
          maxTagCount={6}
        />
      </Form.Item>

      <Form.Item name="domainIds" label={t('dataProperty.domainIds')} extra={t('dataProperty.domainHint')}>
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('dataProperty.domainIdsPlaceholder')}
          optionFilterProp="label"
          options={classOptions.map((o) => ({ value: o.id, label: o.label }))}
          notFoundContent={classTreeQuery.error ? t('dataProperty.classOptionsUnavailable') : undefined}
        />
      </Form.Item>

      <Form.Item
        name="rangeIris"
        label={t('dataProperty.rangeIris')}
        extra={t('dataProperty.rangeHint')}
      >
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('dataProperty.rangeIrisPlaceholder')}
          optionFilterProp="label"
          options={rangeOptions}
          optionRender={(option) => (
            <Tooltip title={String((option.data as { example?: string }).example ?? '')} placement="right">
              <span>{option.label}</span>
            </Tooltip>
          )}
        />
      </Form.Item>

      <Form.Item name="disjointIds" label={t('dataProperty.disjointIds')} extra={t('dataProperty.symmetricHint')}>
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('dataProperty.disjointIdsPlaceholder')}
          optionFilterProp="label"
          options={propertyOptions
            .filter((o) => !isEdit || o.id !== selfId)
            .map((o) => ({ value: o.id, label: o.label }))}
        />
      </Form.Item>

      <Form.Item name="equivalentId" label={t('dataProperty.equivalentId')} extra={t('dataProperty.equivalentIdHint')}>
        <Select
          showSearch
          allowClear
          placeholder={t('dataProperty.equivalentIdPlaceholder')}
          optionFilterProp="label"
          options={propertyOptions
            .filter((o) => !isEdit || o.id !== selfId)
            .map((o) => ({ value: o.id, label: o.label }))}
        />
      </Form.Item>

      {/* 单复选框（Protégé 数据属性特征面板同构：仅 Functional）；提交为 [owl:FunctionalProperty] */}
      <ProFormCheckbox
        name="functional"
        label={t('dataProperty.characteristics')}
        extra={t('dataProperty.characteristicsHint')}
      >
        {t('characteristic.functional')}
      </ProFormCheckbox>

      {/* 自定义注解区（04 期增量——AnnotationListEditor 共享组件，六类实体 frame 共用） */}
      <Form.Item
        name="annotations"
        label={t('dataProperty.annotations')}
        extra={t('annotationEditor.hint')}
      >
        <AnnotationListEditor />
      </Form.Item>

      <ProFormSwitch name="deprecated" label={t('dataProperty.deprecated')} />
    </DrawerForm>
  );
};

export default DataPropertyDrawer;
