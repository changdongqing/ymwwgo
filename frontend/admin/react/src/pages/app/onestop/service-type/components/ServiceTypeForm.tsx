import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { App, AutoComplete, Divider, Form, Input } from 'antd';
import {
  ProForm,
  ProFormDigit,
  ProFormItem,
  ProFormRadio,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import type { ProFormInstance } from '@ant-design/pro-components';
import { useQueryClient } from '@tanstack/react-query';
import {
  type onestopservicev1_Level,
  type onestopservicev1_ServiceType,
} from '@/api/generated/admin/service/v1';
import { PaginationQuery } from '@/core';
import { fetchListServiceTypes, useCreateServiceType, useUpdateServiceType } from '@/api/hooks/onestop';
import { CODE_PATTERN, getLevelMap, isRepairConfigurable, levelOfCode, REPAIR_POST_SUGGESTIONS } from '../constants';

interface ServiceTypeFormProps {
  mode: 'create' | 'edit';
  data?: onestopservicev1_ServiceType;
  /** 保存成功后回调（父页面负责关闭页签并刷新列表） */
  onSaved: () => void;
  /** 取消编辑回调（父页面负责关闭页签） */
  onCancel: () => void;
}

/** 层级 → 恰好上一级（上级候选只取这一层，平铺下拉，不做树选择） */
const PARENT_LEVEL: Partial<Record<onestopservicev1_Level, onestopservicev1_Level>> = {
  LEVEL_MIDDLE: 'LEVEL_MAJOR',
  LEVEL_MINOR: 'LEVEL_MIDDLE',
  LEVEL_DETAIL: 'LEVEL_MINOR',
};

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

  // 监听编码输入：驱动层级展示与上级候选
  const watchedCode = Form.useWatch('code', form) as string | undefined;

  // 全量平铺列表：用于构建“上级服务类型”候选
  const [allItems, setAllItems] = useState<onestopservicev1_ServiceType[]>([]);

  useEffect(() => {
    // 不传 paging 走 noPaging=true（常规分页钳制 100 条会让上级候选缺项）
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
        repairPost: data.repairPost,
        standardWorkHours: data.standardWorkHours,
        responseTimeLimit: data.responseTimeLimit,
        completionTimeLimit: data.completionTimeLimit,
        guidance: data.guidance,
        commonPhrases: data.commonPhrases ?? [],
        standardTools: data.standardTools ?? [],
        spareParts: data.spareParts ?? [],
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

  // 报修维修配置仅对 0101 报修维修子树的小类/细类开放（新建时随编码输入出现）
  const effectiveCode = mode === 'edit' ? data?.code : watchedCode;
  const showRepairConfig = isRepairConfigurable(effectiveCode);

  // 上级候选：只允许挂在恰好上一级之下；编辑时排除自身。
  // 同一层级的候选天然是平铺列表，无需树形结构。
  const parentOptions = useMemo(() => {
    if (!currentLevel) {
      return [];
    }
    const parentLevel = PARENT_LEVEL[currentLevel];
    if (!parentLevel) {
      return [];
    }
    const selfId = mode === 'edit' ? data?.id : undefined;
    return allItems
      .filter((item) => item.level === parentLevel && item.id !== selfId)
      .sort((a, b) => (a.code ?? '').localeCompare(b.code ?? ''))
      .map((item) => ({
        label: `${item.code ?? ''} ${item.name ?? ''}`.trim(),
        value: item.id as number,
      }));
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
    if (showRepairConfig) {
      // 空串归一为 undefined：清空的字段走 mask 内 nil → 置 NULL，而不是落空串
      payload.repairPost = values.repairPost || undefined;
      payload.guidance = values.guidance || undefined;
      payload.standardWorkHours = values.standardWorkHours;
      payload.responseTimeLimit = values.responseTimeLimit;
      payload.completionTimeLimit = values.completionTimeLimit;
      payload.commonPhrases = values.commonPhrases ?? [];
      payload.standardTools = values.standardTools ?? [];
      payload.spareParts = values.spareParts ?? [];
    }
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

        {/* 大类为根节点无上级；其余层级必须挂恰好上一级（候选只列该层，平铺下拉） */}
        {currentLevel && currentLevel !== 'LEVEL_MAJOR' && (
          <ProFormSelect
            name="parentId"
            label={t('parentId')}
            rules={[{ required: true, message: t('requiredParentId') }]}
            options={parentOptions}
            fieldProps={{
              allowClear: true,
              showSearch: true,
              optionFilterProp: 'label',
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

        {/* 报修维修配置（配置详情）：仅 0101 报修维修子树的小类/细类出现。
            小类配岗位/时限/指导，细类配工时/语句/工具/备件，均可留空 = 未配置 */}
        {showRepairConfig && (
          <>
            <Divider titlePlacement="left">
              {t('repairConfigSection')}
            </Divider>

            <ProFormItem
              name="repairPost"
              label={t('repairPost')}
              extra={t('repairPostExtra')}
            >
              <AutoComplete
                options={REPAIR_POST_SUGGESTIONS}
                filterOption
                allowClear
                placeholder={t('repairPostPlaceholder')}
              />
            </ProFormItem>

            <ProFormDigit
              name="standardWorkHours"
              label={t('standardWorkHours')}
              min={0.1}
              max={720}
              fieldProps={{ step: 0.5, precision: 1 }}
              colProps={{ span: 12 }}
              placeholder={t('standardWorkHoursPlaceholder')}
              extra={t('standardWorkHoursExtra')}
            />

            <ProFormDigit
              name="responseTimeLimit"
              label={t('responseTimeLimit')}
              min={1}
              max={43200}
              fieldProps={{ precision: 0 }}
              colProps={{ span: 12 }}
              placeholder={t('minutesPlaceholder')}
            />

            <ProFormDigit
              name="completionTimeLimit"
              label={t('completionTimeLimit')}
              min={1}
              max={43200}
              fieldProps={{ precision: 0 }}
              colProps={{ span: 12 }}
              placeholder={t('minutesPlaceholder')}
            />

            <ProFormTextArea
              name="guidance"
              label={t('guidance')}
              fieldProps={{ rows: 4, maxLength: 2048, showCount: true }}
              colProps={{ span: 24 }}
              placeholder={t('guidancePlaceholder')}
            />

            <ProFormSelect
              name="commonPhrases"
              label={t('commonPhrases')}
              fieldProps={{
                mode: 'tags',
                tokenSeparators: [',', '，'],
                open: false,
                suffixIcon: null,
              }}
              colProps={{ span: 24 }}
              placeholder={t('commonPhrasesPlaceholder')}
              extra={t('commonPhrasesExtra')}
            />

            <ProFormSelect
              name="standardTools"
              label={t('standardTools')}
              fieldProps={{
                mode: 'tags',
                tokenSeparators: [',', '，'],
                open: false,
                suffixIcon: null,
              }}
              colProps={{ span: 24 }}
              placeholder={t('standardToolsPlaceholder')}
            />

            <ProFormSelect
              name="spareParts"
              label={t('spareParts')}
              fieldProps={{
                mode: 'tags',
                tokenSeparators: [',', '，'],
                open: false,
                suffixIcon: null,
              }}
              colProps={{ span: 24 }}
              placeholder={t('sparePartsPlaceholder')}
            />
          </>
        )}
      </ProForm>
    </div>
  );
}
