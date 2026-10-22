import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input } from 'antd';
import {
  ProForm,
  ProFormRadio,
  ProFormText,
  ProFormTextArea,
  ProFormTreeSelect,
} from '@ant-design/pro-components';
import type { ProFormInstance } from '@ant-design/pro-components';
import { App } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import {
  type onestopservicev1_Level,
  type onestopservicev1_ServiceType,
} from '@/api/generated/admin/service/v1';
import { PaginationQuery } from '@/core';
import { fetchListServiceTypes, useCreateServiceType, useUpdateServiceType } from '@/api/hooks/onestop';
import { CODE_PATTERN, getLevelMap, levelOfCode } from '../constants';

interface ServiceTypeFormProps {
  mode: 'create' | 'edit';
  data?: onestopservicev1_ServiceType;
  /** 保存成功后回调（父页面负责关闭页签并刷新列表） */
  onSaved: () => void;
  /** 取消编辑回调（父页面负责关闭页签） */
  onCancel: () => void;
}

interface TreeNode {
  title: string;
  value: number;
  key: number;
  children?: TreeNode[];
}

const LEVEL_ORDER: onestopservicev1_Level[] = [
  'LEVEL_UNSPECIFIED',
  'LEVEL_MAJOR',
  'LEVEL_MIDDLE',
  'LEVEL_MINOR',
  'LEVEL_DETAIL',
];

/**
 * 服务类型编辑表单：不再承载在抽屉里，直接嵌入页签内容区
 * （列表页「新增/编辑」各自打开一个页签，本组件即页签内容）。
 */
export default function ServiceTypeForm({ mode, data, onSaved, onCancel }: ServiceTypeFormProps) {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();
  const formRef = useRef<ProFormInstance>(null);

  // 监听编码输入：驱动层级展示与上级候选树
  const watchedCode = Form.useWatch('code', form) as string | undefined;

  // 全量平铺列表：用于构建“上级服务类型”候选树
  const [allItems, setAllItems] = useState<onestopservicev1_ServiceType[]>([]);

  useEffect(() => {
    // 不传 paging 走 noPaging=true（常规分页钳制 100 条会让上级候选树缺枝）
    fetchListServiceTypes(new PaginationQuery())
      .then((response) => setAllItems(response.items ?? []))
      .catch((error: Error) => message.error(error.message || t('fetchFailed')));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 编辑回填 / 新建重置（组件随页签挂载，仅执行一次）
  useEffect(() => {
    if (mode === 'edit' && data) {
      form.setFieldsValue({
        code: data.code,
        name: data.name,
        parentId: data.parentId,
        description: data.description,
        isEnabled: data.isEnabled,
      });
    } else {
      form.resetFields();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const levelMap = getLevelMap(t);

  // 当前层级：编辑时以记录编码为准，新建时跟随输入
  const currentLevel = useMemo(
    () => levelOfCode(mode === 'edit' ? data?.code : watchedCode),
    [mode, data?.code, watchedCode],
  );

  // 上级候选树：只允许挂在恰好上一级之下；编辑时排除自身。
  // useMemo 保证 treeData 引用稳定（每次渲染重建数组会导致 rc-tree 选中事件丢失）。
  const parentTree = useMemo<TreeNode[]>(() => {
    if (!currentLevel || currentLevel === 'LEVEL_MAJOR') {
      return [];
    }
    const parentLevel = LEVEL_ORDER[LEVEL_ORDER.indexOf(currentLevel) - 1] as onestopservicev1_Level;
    const selfId = mode === 'edit' ? data?.id : undefined;
    const build = (parentId: number | undefined): TreeNode[] =>
      allItems
        .filter(
          (item) => item.parentId === parentId && item.level === parentLevel && item.id !== selfId,
        )
        .map((item) => ({
          title: `${item.code} ${item.name ?? ''}`,
          value: item.id as number,
          key: item.id as number,
          children: build(item.id),
        }));
    return build(undefined);
  }, [currentLevel, allItems, mode, data?.id]);

  const createMutation = useCreateServiceType({
    onSuccess: () => {
      message.success(t('createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listServiceTypes'] });
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('createFailed')),
  });

  const updateMutation = useUpdateServiceType({
    onSuccess: () => {
      message.success(t('updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listServiceTypes'] });
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('updateFailed')),
  });

  const handleSubmit = async (values: Partial<onestopservicev1_ServiceType>) => {
    const payload: Partial<onestopservicev1_ServiceType> = {
      code: values.code,
      name: values.name,
      parentId: values.parentId,
      description: values.description,
      isEnabled: values.isEnabled,
    };
    if (mode === 'create') {
      await createMutation.mutateAsync({ values: payload });
    } else if (data?.id) {
      await updateMutation.mutateAsync({ id: data.id, values: payload });
    }
    return true;
  };

  return (
    <div className="service-type-form-pane">
      <ProForm
        form={form}
        formRef={formRef}
        onFinish={handleSubmit}
        grid
        style={{ maxWidth: 760 }}
        submitter={{
          searchConfig: {
            submitText: t('common:button.submit'),
            resetText: t('common:button.cancel'),
          },
          resetButtonProps: { onClick: () => onCancel() },
        }}
      >
        <ProFormText
          name="code"
          label={t('code')}
          rules={[
            { required: true, message: t('requiredCode') },
            { pattern: CODE_PATTERN, message: t('invalidCode') },
          ]}
          fieldProps={{ maxLength: 8, showCount: true }}
          placeholder={t('codePlaceholder')}
          extra={t('codeExtra')}
        />

        {/* 层级由编码位数推导（后端同样强制），仅展示不可编辑。
            不绑 name，避免把展示值写进表单存储 */}
        <Form.Item label={t('level')}>
          <Input disabled value={currentLevel ? levelMap[currentLevel] : t('levelUnknown')} placeholder="-" />
        </Form.Item>

        <ProFormText
          name="name"
          label={t('name')}
          rules={[{ required: true, message: t('requiredName') }]}
          fieldProps={{ maxLength: 128 }}
          placeholder={t('namePlaceholder')}
        />

        {/* 大类为根节点无上级；其余层级必须挂恰好上一级（候选树按编码位数过滤） */}
        {currentLevel && currentLevel !== 'LEVEL_MAJOR' && (
          <ProFormTreeSelect
            name="parentId"
            label={t('parentId')}
            rules={[{ required: true, message: t('requiredParentId') }]}
            allowClear
            fieldProps={{
              showSearch: true,
              treeDefaultExpandAll: true,
              treeNodeFilterProp: 'title',
              treeData: parentTree,
            }}
            placeholder={t('parentIdPlaceholder')}
          />
        )}

        <ProFormTextArea
          name="description"
          label={t('description')}
          fieldProps={{ maxLength: 512, rows: 3 }}
          colProps={{ span: 24 }}
        />

        <ProFormRadio.Group
          name="isEnabled"
          label={t('isEnabled')}
          initialValue={true}
          options={[
            { label: t('enabled'), value: true },
            { label: t('disabled'), value: false },
          ]}
        />
      </ProForm>
    </div>
  );
}
