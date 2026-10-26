import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, App, AutoComplete, Button, Divider, Form, Input, InputNumber, Select, Space } from 'antd';
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons';
import { ProForm, ProFormRadio, ProFormSelect, ProFormText, ProFormTextArea } from '@ant-design/pro-components';
import type { ProFormInstance } from '@ant-design/pro-components';
import { useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { contractservicev1_ContractType } from '@/api/generated/admin/service/v1';
import { PaginationQuery } from '@/core';
import {
  fetchListContractTypes,
  invalidateContractTypes,
  useCreateContractType,
  useUpdateContractType,
} from '@/api/hooks/contract';
import { CODE_PATTERN, PARTY_B_LABEL_SUGGESTIONS, PAY_METHOD_OPTIONS, WIDGET_OPTIONS, levelOfCode } from '../constants';

interface ContractTypeFormProps {
  mode: 'create' | 'edit';
  data?: contractservicev1_ContractType;
  /** 保存成功后回调（父页面负责关闭页签并刷新列表） */
  onSaved: () => void;
  /** 取消编辑回调（父页面负责关闭页签） */
  onCancel: () => void;
}

/**
 * 合同类型编辑表单：基础信息 + 统一合同配置模板。
 * 两级均可携带模板（细类空字段回退大类——继承在后端 effective-config 解析），
 * 模板四组：称谓与方向 / 付款与拆解 / 差异字段定义 / 附件与提示。
 */
export default function ContractTypeForm({ mode, data, onSaved, onCancel }: ContractTypeFormProps) {
  const { t } = useTranslation('contract');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();
  const formRef = useRef<ProFormInstance>(null);

  // 监听编码输入：驱动层级展示与上级候选
  const watchedCode = Form.useWatch('code', form) as string | undefined;
  // 监听付款方式：按节点/按进度才展示拆解节点编辑器
  const watchedPayMethod = Form.useWatch('payMethod', form) as string | undefined;

  // 全量平铺列表：用于构建「上级类型」候选（细类的上级 = 大类）
  const [allItems, setAllItems] = useState<contractservicev1_ContractType[]>([]);

  useEffect(() => {
    // 不传 paging 走 noPaging=true（常规分页钳制 100 条会让上级候选缺项）
    fetchListContractTypes(new PaginationQuery())
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
        partyBLabel: data.partyBLabel,
        payMethod: data.payMethod === 'PAY_METHOD_UNSPECIFIED' ? undefined : data.payMethod,
        payNodes: (data.payNodes ?? []).map((n) => ({ name: n?.name, ratioPercent: n?.ratioPercent })),
        feeDirection: data.feeDirection === 'DIRECTION_UNSPECIFIED' ? undefined : data.feeDirection,
        requiresApproval: data.requiresApproval ?? undefined,
        requiresDeposit: data.requiresDeposit ?? undefined,
        diffFields: (data.diffFields ?? []).map((f) => ({
          key: f?.key,
          label: f?.label,
          widget: f?.widget,
          options: (f?.options ?? []).join('、'),
        })),
        requiredAttachments: data.requiredAttachments ?? [],
        guidance: data.guidance,
      });
    } else {
      form.resetFields();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const level = levelOfCode(watchedCode);

  // 上级候选：细类的上级 = 大类（启用 + 排除自身）
  const parentOptions = useMemo(() => {
    if (level !== 'LEVEL_DETAIL') return [];
    return allItems
      .filter((item) => item.code?.length === 2 && item.isEnabled !== false && item.id !== data?.id)
      .map((item) => ({ label: `${item.code} ${item.name}`, value: item.id }));
  }, [allItems, level, data?.id]);

  const createMutation = useCreateContractType({
    onSuccess: () => {
      message.success(t('saveSuccess'));
      invalidateContractTypes(queryClient);
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('saveFailed')),
  });

  const updateMutation = useUpdateContractType({
    onSuccess: () => {
      message.success(t('saveSuccess'));
      invalidateContractTypes(queryClient);
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('saveFailed')),
  });

  type TypeFormValues = {
    code?: string;
    name?: string;
    parentId?: number;
    description?: string;
    isEnabled?: boolean;
    partyBLabel?: string;
    payMethod?: contractservicev1_ContractType['payMethod'];
    feeDirection?: contractservicev1_ContractType['feeDirection'];
    requiresApproval?: boolean;
    requiresDeposit?: boolean;
    guidance?: string;
    requiredAttachments?: string[];
    payNodes?: { name?: string; ratioPercent?: number }[];
    diffFields?: { key?: string; label?: string; widget?: string; options?: string }[];
  };

  const handleFinish = async (raw: Record<string, unknown>) => {
    const values = raw as TypeFormValues;
    const payload: Partial<contractservicev1_ContractType> = {
      code: values.code,
      name: values.name,
      parentId: values.parentId,
      description: values.description,
      isEnabled: values.isEnabled,
      partyBLabel: values.partyBLabel,
      payMethod: values.payMethod,
      feeDirection: values.feeDirection,
      requiresApproval: values.requiresApproval,
      requiresDeposit: values.requiresDeposit,
      guidance: values.guidance,
      requiredAttachments: values.requiredAttachments ?? [],
      payNodes: (values.payNodes ?? [])
        .filter((n) => n?.name)
        .map((n) => ({ name: n.name, ratioPercent: n.ratioPercent ?? 0 })),
      diffFields: (values.diffFields ?? [])
        .filter((f) => f?.key && f?.label)
        .map((f) => ({
          key: f.key,
          label: f.label,
          widget: f.widget ?? 'WIDGET_TEXT',
          options: String(f.options ?? '')
            .split(/[、,，;；\s]+/)
            .map((s) => s.trim())
            .filter(Boolean),
        })),
    };
    if (mode === 'edit' && data?.id) {
      updateMutation.mutate({ id: data.id, values: payload });
    } else {
      createMutation.mutate({ values: payload });
    }
  };

  return (
    <div style={{ maxWidth: 860, margin: '0 auto', padding: '16px 0' }}>
      <ProForm
        formRef={formRef}
        form={form}
        layout="horizontal"
        labelCol={{ flex: '160px' }}
        onFinish={handleFinish}
        submitter={{
          searchConfig: { submitText: t('save'), resetText: t('cancel') },
          resetButtonProps: { onClick: onCancel },
        }}
      >
        <Divider titlePlacement="left">{t('sectionBase')}</Divider>
        <ProFormText
          name="code"
          label={t('code')}
          placeholder={t('codePlaceholder')}
          rules={[
            { required: true, message: t('codeRequired') },
            {
              pattern: CODE_PATTERN,
              message: t('codePatternError'),
            },
          ]}
          extra={level ? t(`levelHint.${level}`) : t('levelHint.none')}
          disabled={mode === 'edit'}
        />
        <ProFormText name="name" label={t('name')} rules={[{ required: true, message: t('nameRequired') }]} />
        {level === 'LEVEL_DETAIL' && (
          <ProFormSelect
            name="parentId"
            label={t('parent')}
            options={parentOptions}
            rules={[{ required: true, message: t('parentRequired') }]}
            extra={t('parentExtra')}
            disabled={mode === 'edit'}
          />
        )}
        <ProFormTextArea name="description" label={t('description')} fieldProps={{ rows: 2, maxLength: 512 }} />
        <ProFormRadio.Group
          name="isEnabled"
          label={t('isEnabled')}
          initialValue={true}
          options={[
            { label: t('enabled'), value: true },
            { label: t('disabled'), value: false },
          ]}
        />

        <Divider titlePlacement="left">{t('sectionTemplate')}</Divider>
        <Alert
          type="info"
          showIcon
          message={t('templateHint')}
          style={{ marginBottom: 16 }}
        />
        <Form.Item name="partyBLabel" label={t('partyBLabel')}>
          <AutoComplete placeholder={t('partyBLabelPlaceholder')} options={PARTY_B_LABEL_SUGGESTIONS.map((s) => ({ value: s }))} />
        </Form.Item>
        <ProFormSelect
          name="payMethod"
          label={t('payMethod')}
          options={PAY_METHOD_OPTIONS.map((o) => ({ label: t(o.label), value: o.value }))}
          extra={t('payMethodExtra')}
          allowClear
        />
        {(watchedPayMethod === 'PAY_NODE' || watchedPayMethod === 'PAY_PROGRESS') && (
          <Form.Item label={t('payNodes')} extra={t('payNodesExtra')}>
            <Form.List name="payNodes">
              {(fields, { add, remove }) => (
                <>
                  {fields.map((field) => (
                    <Space key={field.key} align="baseline" style={{ display: 'flex', marginBottom: 4 }}>
                      <Form.Item
                        name={[field.name, 'name']}
                        rules={[{ required: true, message: t('payNodeNameRequired') }]}
                        style={{ marginBottom: 0 }}
                      >
                        <Input placeholder={t('payNodeNamePlaceholder')} style={{ width: 180 }} />
                      </Form.Item>
                      <Form.Item
                        name={[field.name, 'ratioPercent']}
                        rules={[{ required: true, message: t('payNodeRatioRequired') }]}
                        style={{ marginBottom: 0 }}
                      >
                        <InputNumber min={1} max={100} addonAfter="%" style={{ width: 140 }} />
                      </Form.Item>
                      <MinusCircleOutlined onClick={() => remove(field.name)} />
                    </Space>
                  ))}
                  <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ ratioPercent: 0 })} style={{ width: 340 }}>
                    {t('addPayNode')}
                  </Button>
                </>
              )}
            </Form.List>
          </Form.Item>
        )}
        <ProFormRadio.Group
          name="feeDirection"
          label={t('feeDirection')}
          options={[
            { label: t('direction.PAYABLE'), value: 'DIRECTION_PAYABLE' },
            { label: t('direction.RECEIVABLE'), value: 'DIRECTION_RECEIVABLE' },
          ]}
          extra={t('feeDirectionExtra')}
        />
        <ProFormRadio.Group
          name="requiresApproval"
          label={t('requiresApproval')}
          options={[
            { label: t('approval.yes'), value: true },
            { label: t('approval.no'), value: false },
          ]}
          extra={t('requiresApprovalExtra')}
        />
        <ProFormRadio.Group
          name="requiresDeposit"
          label={t('requiresDeposit')}
          options={[
            { label: t('deposit.yes'), value: true },
            { label: t('deposit.no'), value: false },
          ]}
          extra={t('requiresDepositExtra')}
        />

        <Form.Item label={t('diffFields')} extra={t('diffFieldsExtra')}>
          <Form.List name="diffFields">
            {(fields, { add, remove }) => (
              <>
                {fields.map((field) => (
                  <Space key={field.key} align="baseline" style={{ display: 'flex', marginBottom: 4 }} wrap>
                    <Form.Item
                      name={[field.name, 'key']}
                      rules={[{ required: true, message: t('diffKeyRequired') }]}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder={t('diffKeyPlaceholder')} style={{ width: 140 }} />
                    </Form.Item>
                    <Form.Item
                      name={[field.name, 'label']}
                      rules={[{ required: true, message: t('diffLabelRequired') }]}
                      style={{ marginBottom: 0 }}
                    >
                      <Input placeholder={t('diffLabelPlaceholder')} style={{ width: 140 }} />
                    </Form.Item>
                    <Form.Item name={[field.name, 'widget']} initialValue="WIDGET_TEXT" style={{ marginBottom: 0 }}>
                      <Select options={WIDGET_OPTIONS} style={{ width: 120 }} />
                    </Form.Item>
                    <Form.Item
                      noStyle
                      shouldUpdate={(prev, cur) =>
                        prev.diffFields?.[field.name]?.widget !== cur.diffFields?.[field.name]?.widget
                      }
                    >
                      {({ getFieldValue }) =>
                        getFieldValue(['diffFields', field.name, 'widget']) === 'WIDGET_SELECT' ? (
                          <Form.Item
                            name={[field.name, 'options']}
                            style={{ marginBottom: 0 }}
                            extra={t('diffOptionsExtra')}
                          >
                            <Input placeholder={t('diffOptionsPlaceholder')} style={{ width: 220 }} />
                          </Form.Item>
                        ) : null
                      }
                    </Form.Item>
                    <MinusCircleOutlined onClick={() => remove(field.name)} />
                  </Space>
                ))}
                <Button type="dashed" icon={<PlusOutlined />} onClick={() => add({ widget: 'WIDGET_TEXT' })} style={{ width: 340 }}>
                  {t('addDiffField')}
                </Button>
              </>
            )}
          </Form.List>
        </Form.Item>

        <ProFormSelect
          name="requiredAttachments"
          label={t('requiredAttachments')}
          mode="tags"
          fieldProps={{ tokenSeparators: [',', '、'] }}
          extra={t('requiredAttachmentsExtra')}
        />
        <ProFormTextArea name="guidance" label={t('guidance')} fieldProps={{ rows: 3, maxLength: 2048 }} extra={t('guidanceExtra')} />
      </ProForm>
    </div>
  );
}
