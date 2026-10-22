import { useEffect, useMemo, useRef, useState } from 'react';
import type { ProFormInstance } from '@ant-design/pro-components';
import { DrawerForm, ProFormSwitch, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import { Form, Modal, TreeSelect } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_AnnotationAssertionInput,
  ontologyservicev1_AnnotationPropertyTreeNode,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useAnnotationPropertyFrame,
  useCreateAnnotationProperty,
  useOntologyAnnotationPropertyTree,
  useUpdateAnnotationProperty,
} from '@/api/hooks/ontologyAnnotationProperty';
import AnnotationListEditor from '../../components/AnnotationListEditor';

interface AnnotationPropertyDrawerProps {
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

/**
 * 注解属性表单（DrawerForm + formRef，对应 Protégé Annotation Property frame 分区，04 号文档 §6.3）：
 * 基本信息 + 父注解属性 TreeSelect（多选 treeCheckStrictly，禁自身）+ 自定义注解
 * （AnnotationListEditor 共享组件——注解属性自身也可挂注解，递归）。
 * 无定义域/值域/特征/不相交/等价分区——UI 减法是 OWL 2 语言约束的同构表达
 * （Protégé frame 亦无这些节，04 号文档 §1.3）。
 *
 * 提交语义（frame 全量提交）：字段缺省 = 不变、空串 = 清空；annotations 全量替换
 * （nil/缺失/空数组统一 = 清空自定义注解区）；create 不带 base_version。
 */
const AnnotationPropertyDrawer: React.FC<AnnotationPropertyDrawerProps> = ({
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
  const [saving, setSaving] = useState(false);

  const isEdit = propertyId != null;

  // frame 详情（编辑态回显 + base_version 乐观锁基准）
  const frameQuery = useAnnotationPropertyFrame({ id: propertyId ?? 0 }, { enabled: open && isEdit });
  const frame = frameQuery.data;

  // 选择器数据源：注解属性全量树（父属性 TreeSelect；自身禁选由树数据 disabled 承载）
  const propertyTreeQuery = useOntologyAnnotationPropertyTree(ontologyId ?? undefined, { enabled: open && ontologyId != null });
  const propertyRoots = propertyTreeQuery.data?.roots ?? [];

  const propertyTreeData = useMemo<DataNode[]>(() => {
    const toNode = (n: ontologyservicev1_AnnotationPropertyTreeNode): DataNode => ({
      key: n.id!,
      title:
        (n.displayLabel || n.localName || `#${n.id}`) +
        (n.deprecated ? `（${t('annotationProperty.deprecatedYes')}）` : ''),
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
          title: t('annotationPropertyErrors.versionConflictTitle'),
          content: detail || t('annotationPropertyErrors.versionConflict'),
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
        formRef.current?.setFields([{ name: 'localName', errors: [detail || t('annotationPropertyErrors.localNameInvalid')] }]);
        return true;
      case 'HIERARCHY_CYCLIC':
        formRef.current?.setFields([{ name: 'superPropertyIds', errors: [detail || t('annotationPropertyErrors.hierarchyCyclic')] }]);
        return true;
      case 'ANNOTATION_PROPERTY_TARGET_INVALID':
        formRef.current?.setFields([{ name: 'superPropertyIds', errors: [detail] }]);
        return true;
      case 'ANNOTATION_VALUE_INVALID':
        formRef.current?.setFields([{ name: 'annotations', errors: [detail] }]);
        return true;
      default:
        return false;
    }
  };

  const createMutation = useCreateAnnotationProperty({
    onSuccess: (response) => {
      queryClient.invalidateQueries({ queryKey: ['ontologyAnnotationPropertyTree'] });
      queryClient.invalidateQueries({ queryKey: ['listAnnotationProperties'] });
      onSuccess({ id: response.id ?? null, superIds: (formRef.current?.getFieldValue('superPropertyIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——reason 分流已呈现，未命中走通用报错
      if (!handleReasonError(error)) {
        console.error('注解属性创建失败：', error);
      }
    },
  });

  const updateMutation = useUpdateAnnotationProperty({
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ontologyAnnotationPropertyTree'] });
      queryClient.invalidateQueries({ queryKey: ['listAnnotationProperties'] });
      queryClient.invalidateQueries({ queryKey: ['annotationPropertyFrame'] });
      onSuccess({ id: propertyId, superIds: (formRef.current?.getFieldValue('superPropertyIds') as number[]) ?? [] });
      onClose();
    },
    onError: (error: Error) => {
      if (!handleReasonError(error)) {
        console.error('注解属性更新失败：', error);
      }
    },
  });

  const handleSubmit = async (values: Record<string, any>) => {
    if (ontologyId == null) return false;
    try {
      setSaving(true);
      // 行内红字由 AnnotationListEditor status 承载；提交前再拦一次空白值（铁律 3 陷阱：脏行不上送）
      const annotations: ontologyservicev1_AnnotationAssertionInput[] = (values.annotations ?? [])
        .map((row: ontologyservicev1_AnnotationAssertionInput) => ({
          propertyId: row.propertyId,
          value: row.value ?? '',
          lang: row.lang || undefined,
        }))
        .filter((row: ontologyservicev1_AnnotationAssertionInput) => (row.propertyId ?? 0) > 0 && String(row.value).trim() !== '');
      if (isEdit && propertyId != null) {
        await updateMutation.mutateAsync({
          values: {
            id: propertyId,
            // frame 全量提交：undefined = 不变、空串 = 清空（antd 清空文本框得 ''）
            displayLabel: values.displayLabel ?? '',
            comment: values.comment ?? '',
            deprecated: !!values.deprecated,
            superPropertyIds: values.superPropertyIds ?? [],
            annotations, // 全量替换语义：空数组 = 清空自定义注解区
            baseVersion: frame?.baseVersion ?? undefined, // 乐观锁基准；nil = 跳过（内部路径）
          },
        });
      } else {
        await createMutation.mutateAsync({
          values: {
            ontologyId,
            localName: values.localName,
            displayLabel: values.displayLabel || undefined,
            comment: values.comment || undefined,
            deprecated: !!values.deprecated,
            superPropertyIds: values.superPropertyIds ?? [],
            annotations,
          },
        });
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
      title={isEdit ? t('annotationProperty.editTitle') : t('annotationProperty.createTitle')}
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
        label={t('annotationProperty.localName')}
        placeholder={t('annotationProperty.localNamePlaceholder')}
        rules={[
          { required: true, message: t('annotationProperty.requiredLocalName') },
          {
            validator: (_rule, value: string) => {
              if (!value || LOCAL_NAME_PATTERN.test(value)) return Promise.resolve();
              return Promise.reject(new Error(t('annotationProperty.invalidLocalName')));
            },
          },
        ]}
        fieldProps={{ allowClear: true, disabled: isEdit }}
        extra={isEdit ? t('annotationProperty.localNameImmutable') : t('annotationProperty.localNameHint')}
      />

      <ProFormText
        name="displayLabel"
        label={t('annotationProperty.displayLabel')}
        placeholder={t('annotationProperty.displayLabelPlaceholder')}
        fieldProps={{ allowClear: true, maxLength: 255 }}
      />

      <ProFormTextArea
        name="comment"
        label={t('annotationProperty.comment')}
        placeholder={t('annotationProperty.commentPlaceholder')}
        fieldProps={{ autoSize: { minRows: 2, maxRows: 4 } }}
      />

      <Form.Item
        name="superPropertyIds"
        label={t('annotationProperty.superPropertyIds')}
        extra={t('annotationProperty.superPropertyIdsHint')}
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
          placeholder={t('annotationProperty.superPropertyIdsPlaceholder')}
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
        name="annotations"
        label={t('annotationProperty.annotations')}
        extra={t('annotationProperty.annotationsHint')}
      >
        <AnnotationListEditor />
      </Form.Item>

      <ProFormSwitch name="deprecated" label={t('annotationProperty.deprecated')} />
    </DrawerForm>
  );
};

export default AnnotationPropertyDrawer;
