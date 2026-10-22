import { useEffect, useMemo, useRef, useState } from 'react';
import type { ProFormInstance } from '@ant-design/pro-components';
import { DrawerForm, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { App, Form, Modal, Select, TreeSelect } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_CreateOntologyClassRequest,
  ontologyservicev1_OntologyClassTreeNode,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useCreateOntologyClass,
  useOntologyClassFrame,
  useOntologyClassTree,
  useUpdateOntologyClass,
} from '@/api/hooks/ontologyClass';
import AnnotationListEditor, { cleanAnnotationRows } from '../../components/AnnotationListEditor';

interface ClassDrawerProps {
  open: boolean;
  ontologyId: number | null;
  /** 编辑目标类 id；create 时为 null */
  classId: number | null;
  /** create 时的默认父类集合（新建子类/兄弟/根类入口注入） */
  defaultParentIds: number[];
  onClose: () => void;
  /** 保存成功：携带新类 id 与默认选中的父类（树定位用） */
  onSuccess: (saved: { id: number | null; parentIds: number[] }) => void;
  /** 乐观锁冲突回调（页面统一刷新树/列表/frame） */
  onVersionConflict: () => void;
}

/** 与后端同口径的局部名正则（NCName 宽松版，允许中文/字母/下划线/连字符，首字符非数字） */
const LOCAL_NAME_PATTERN = /^[\p{L}_][\p{L}\p{N}_-]*$/u;
/** skos:notation 字符集（06 号文档 §3.3 定案回填：允许点号、首字符须字母/数字，与个体编码同一正则） */
const NOTATION_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

/**
 * 本体类表单（DrawerForm + formRef）：父类 TreeSelect（多选 treeCheckStrictly，数据源全量树按需加载，
 * 禁自身）、不相交 Select（multiple showSearch，禁自身）、等价 Select（单选）。
 *
 * 提交语义（frame 全量提交）：字段缺省 = 不变、空串 = 清空注解（protojson 无法区分 null 与缺省，
 * 服务端定案见 internal/ontology/changeset.go DiffAnnotations 注释）；create 不带 base_version。
 */
const ClassDrawer: React.FC<ClassDrawerProps> = ({
  open,
  ontologyId,
  classId,
  defaultParentIds,
  onClose,
  onSuccess,
  onVersionConflict,
}) => {
  const { t } = useTranslation('ontology');
  const formRef = useRef<ProFormInstance>(null);
  const queryClient = useQueryClient();
  const { message } = App.useApp();
  const [saving, setSaving] = useState(false);

  const isEdit = classId != null;

  // frame 详情（编辑态回显 + base_version 乐观锁基准）
  const frameQuery = useOntologyClassFrame(
    { id: classId ?? 0 },
    { enabled: open && isEdit },
  );
  const frame = frameQuery.data;

  // 选择器数据源：全量树按需加载（抽屉打开时首拉缓存）
  const fullTreeQuery = useOntologyClassTree(ontologyId ?? undefined, { enabled: open && ontologyId != null });
  const treeRoots = fullTreeQuery.data?.roots ?? [];

  /** 全量树拍平为选项（含 deprecated 标识） */
  interface ClassOption {
    id: number;
    label: string;
    deprecated: boolean;
    childIds: number[];
  }
  const options = useMemo(() => {
    const out: ClassOption[] = [];
    const walk = (nodes: ontologyservicev1_OntologyClassTreeNode[]) => {
      for (const n of nodes) {
        out.push({
          id: n.id!,
          label: n.displayLabel || n.localName || `#${n.id}`,
          deprecated: !!n.deprecated,
          childIds: (n.children ?? []).map((c) => c.id!),
        });
        walk(n.children ?? []);
      }
    };
    walk(treeRoots);
    return out;
  }, [treeRoots]);

  /** 自身与后代禁选（体验层；服务端环校验兜底） */
  const disabledDescendants = useMemo(() => {
    if (!isEdit || classId == null) return new Set<number>([0]);
    const set = new Set<number>([classId]);
    const collect = (id: number) => {
      for (const o of options) {
        if (o.childIds.includes(id)) {
          if (!set.has(o.id)) {
            set.add(o.id);
            collect(o.id);
          }
        }
      }
    };
    collect(classId);
    return set;
  }, [isEdit, classId, options]);

  const treeData = useMemo<DataNode[]>(() => {
    const toNode = (n: ontologyservicev1_OntologyClassTreeNode): DataNode => ({
      key: n.id!,
      title:
        (n.displayLabel || n.localName || `#${n.id}`) +
        (n.deprecated ? `（${t('class.deprecatedYes')}）` : ''),
      disabled: disabledDescendants.has(n.id!),
      children: (n.children ?? []).map(toNode),
    });
    return treeRoots.map((root) => toNode(root));
  }, [treeRoots, disabledDescendants, t]);

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
          notation: frame.notation ?? undefined,
          deprecated: frame.deprecated ?? false,
          parentIds: frame.parentIds ?? [],
          disjointIds: frame.disjointIds ?? [],
          equivalentId: frame.equivalentId ?? undefined,
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
          notation: undefined,
          deprecated: false,
          parentIds: defaultParentIds ?? [],
          disjointIds: [],
          equivalentId: undefined,
          annotations: [],
        });
      }, 0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isEdit, frame, defaultParentIds]);

  const handleReasonError = (error: unknown): boolean => {
    const reason = getOntologyErrorReason(error);
    const detail = (error as { message?: string })?.message ?? '';
    switch (reason) {
      case 'ONTOLOGY_VERSION_CONFLICT':
        Modal.confirm({
          title: t('classErrors.versionConflictTitle'),
          content: detail || t('classErrors.versionConflict'),
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
        formRef.current?.setFields([{ name: 'localName', errors: [detail || t('classErrors.localNameInvalid')] }]);
        return true;
      case 'HIERARCHY_CYCLIC':
        formRef.current?.setFields([{ name: 'parentIds', errors: [detail || t('classErrors.hierarchyCyclic')] }]);
        return true;
      case 'ENTITY_TYPE_MISMATCH':
        formRef.current?.setFields([{ name: 'disjointIds', errors: [detail] }]);
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

  const createMutation = useCreateOntologyClass({
    onSuccess: (response) => {
      message.success(t('class.createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['ontologyClassTree'] });
      queryClient.invalidateQueries({ queryKey: ['listOntologyClasses'] });
      onSuccess({ id: response.id ?? null, parentIds: (formRef.current?.getFieldValue('parentIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——reason 分流已呈现，未命中走通用报错
      if (!handleReasonError(error)) {
        message.error(error.message || t('class.createFailed'));
      }
    },
  });

  const updateMutation = useUpdateOntologyClass({
    onSuccess: () => {
      message.success(t('class.updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['ontologyClassTree'] });
      queryClient.invalidateQueries({ queryKey: ['listOntologyClasses'] });
      queryClient.invalidateQueries({ queryKey: ['ontologyClassFrame'] });
      onSuccess({ id: classId, parentIds: (formRef.current?.getFieldValue('parentIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      if (!handleReasonError(error)) {
        message.error(error.message || t('class.updateFailed'));
      }
    },
  });

  const handleSubmit = async (values: Record<string, any>) => {
    if (ontologyId == null) return false;
    try {
      setSaving(true);
      if (isEdit && classId != null) {
        await updateMutation.mutateAsync({
          values: {
            id: classId,
            // frame 全量提交：undefined = 不变、空串 = 清空（antd 清空文本框得 ''）
            displayLabel: values.displayLabel ?? '',
            comment: values.comment ?? '',
            notation: values.notation ?? '',
            deprecated: !!values.deprecated,
            parentIds: values.parentIds ?? [],
            disjointIds: values.disjointIds ?? [],
            equivalentId: values.equivalentId ?? 0, // 0 = 清空等价（V1 单值）
            annotations: cleanAnnotationRows(values.annotations), // 全量替换语义（04 期增量）
            baseVersion: frame?.baseVersion ?? undefined, // 乐观锁基准；nil = 跳过（内部路径）
          },
        });
      } else {
        const payload: Partial<ontologyservicev1_CreateOntologyClassRequest> = {
          ontologyId,
          localName: values.localName,
          displayLabel: values.displayLabel || undefined,
          comment: values.comment || undefined,
          notation: values.notation || undefined,
          deprecated: !!values.deprecated,
          parentIds: values.parentIds ?? [],
          disjointIds: values.disjointIds ?? [],
          equivalentId: values.equivalentId ?? undefined,
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
      title={isEdit ? t('class.editTitle') : t('class.createTitle')}
      open={open}
      width={620}
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
        label={t('class.localName')}
        placeholder={t('class.localNamePlaceholder')}
        rules={[
          { required: true, message: t('class.requiredLocalName') },
          {
            validator: (_rule, value: string) => {
              if (!value || LOCAL_NAME_PATTERN.test(value)) return Promise.resolve();
              return Promise.reject(new Error(t('class.invalidLocalName')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, disabled: isEdit }}
        extra={isEdit ? t('class.localNameImmutable') : t('class.localNameHint')}
      />

      <ProFormText
        name="displayLabel"
        label={t('class.displayLabel')}
        placeholder={t('class.displayLabelPlaceholder')}
        fieldProps={{ allowClear: true, maxLength: 255 }}
      />

      <ProFormTextArea
        name="comment"
        label={t('class.comment')}
        placeholder={t('class.commentPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 } }}
      />

      <ProFormText
        name="notation"
        label={t('class.notation')}
        placeholder={t('class.notationPlaceholder')}
        rules={[
          {
            validator: (_rule, value: string) => {
              if (!value || NOTATION_PATTERN.test(value)) return Promise.resolve();
              return Promise.reject(new Error(t('class.invalidNotation')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, style: { fontFamily: 'monospace' } }}
        extra={t('class.notationHint')}
      />

      <Form.Item
        name="parentIds"
        label={t('class.parentIds')}
        extra={t('class.parentIdsHint')}
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
          treeData={treeData}
          placeholder={t('class.parentIdsPlaceholder')}
          allowClear
          multiple
          treeCheckable
          treeCheckStrictly
          showCheckedStrategy={TreeSelect.SHOW_ALL}
          treeLine
          maxTagCount={6}
        />
      </Form.Item>

      <Form.Item
        name="disjointIds"
        label={t('class.disjointIds')}
        extra={t('class.disjointIdsHint')}
      >
        <Select
          mode="multiple"
          showSearch
          allowClear
          maxTagCount={6}
          placeholder={t('class.disjointIdsPlaceholder')}
          optionFilterProp="label"
          options={options
            .filter((o) => !isEdit || o.id !== classId)
            .map((o) => ({ value: o.id, label: o.label, disabled: disabledDescendants.has(o.id) }))}
        />
      </Form.Item>

      <Form.Item name="equivalentId" label={t('class.equivalentId')} extra={t('class.equivalentIdHint')}>
        <Select
          showSearch
          allowClear
          placeholder={t('class.equivalentIdPlaceholder')}
          optionFilterProp="label"
          options={options
            .filter((o) => !isEdit || o.id !== classId)
            .map((o) => ({ value: o.id, label: o.label, disabled: disabledDescendants.has(o.id) }))}
        />
      </Form.Item>

      {/* 自定义注解区（04 期增量——AnnotationListEditor 共享组件，六类实体 frame 共用） */}
      <Form.Item
        name="annotations"
        label={t('class.annotations')}
        extra={t('annotationEditor.hint')}
      >
        <AnnotationListEditor />
      </Form.Item>

      <ProFormSwitch name="deprecated" label={t('class.deprecated')} />
    </DrawerForm>
  );
};

export default ClassDrawer;
