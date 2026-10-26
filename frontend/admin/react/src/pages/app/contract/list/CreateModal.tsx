import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, App, Cascader, DatePicker, Form, Input, InputNumber, Modal, Select, Upload } from 'antd';
import type { UploadProps } from 'antd';
import type { UploadFile } from 'antd';
import { InboxOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import type { Dayjs } from 'dayjs';
import type {
  contractservicev1_ContractDiffField,
  contractservicev1_ContractType,
} from '@/api/generated/admin/service/v1';
import { uploadFile } from '@/api/hooks/file-transfer';
import { invalidateContracts, useCreateContract } from '@/api/hooks/contract';
import { apiClient } from '@/api/client';

interface CreateModalProps {
  open: boolean;
  onClose: () => void;
}

/** 平铺类型列表 → 二级 Cascader 树（code 即路径；叶子节点可选） */
function buildTypeTree(items: contractservicev1_ContractType[]) {
  type Node = { value: string; label: string; children?: Node[]; selectable: boolean };
  const roots: Node[] = [];
  const byCode = new Map<string, Node>();
  const codes = new Set(items.map((i) => i.code ?? ''));
  for (const item of items) {
    const code = item.code ?? '';
    const node: Node = {
      value: code,
      label: `${code} ${item.name ?? ''}`,
      // 叶子 = 无子级（大类未拆细类时可直接选大类）；停用不可选
      selectable: !Array.from(codes).some((c) => c.length === 4 && c.startsWith(code) && c !== code) && item.isEnabled !== false,
    };
    byCode.set(node.value, node);
    const parentCode = node.value.length > 2 ? node.value.slice(0, 2) : '';
    const parent = parentCode ? byCode.get(parentCode) : undefined;
    if (parent) {
      parent.children = [...(parent.children ?? []), node];
    } else {
      roots.push(node);
    }
  }
  // 大类有子级后自身不可选（合同挂叶子）
  for (const node of byCode.values()) {
    if ((node.children?.length ?? 0) > 0) node.selectable = false;
  }
  return roots;
}

/**
 * 创建合同：类型必须选叶子；选中后拉取 effective-config 预填
 * （付款方式/费用方向默认 + 差异字段动态渲染 + 提示/必备附件），
 * 提交由后端解析快照（类型编码/名称/审批开关/差异 schema/付款节点）。
 */
const CreateContractModal = ({ open, onClose }: CreateModalProps) => {
  const { t } = useTranslation('contract');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();
  const [typeItems, setTypeItems] = useState<contractservicev1_ContractType[]>([]);
  const [selectedTypeId, setSelectedTypeId] = useState<number>();
  const [diffFields, setDiffFields] = useState<contractservicev1_ContractDiffField[]>([]);
  const [guidance, setGuidance] = useState<string>();
  const [requiredAttachments, setRequiredAttachments] = useState<string[]>([]);
  const [partyBLabel, setPartyBLabel] = useState<string>();
  const [fileList, setFileList] = useState<UploadFile[]>([]);

  useEffect(() => {
    if (!open) return;
    form.resetFields();
    setSelectedTypeId(undefined);
    setDiffFields([]);
    setGuidance(undefined);
    setRequiredAttachments([]);
    setPartyBLabel(undefined);
    setFileList([]);
    queryClient
      .fetchQuery({
        queryKey: ['listContractTypes', 'contract-create'],
        queryFn: () =>
          apiClient.contractTypeService.List({
            page: 1,
            pageSize: 500,
            orderBy: JSON.stringify(['code']),
            sorting: [],
          }),
        staleTime: 5 * 60_000,
      })
      .then((res) => {
        setTypeItems(res?.items ?? []);
      })
      .catch((e) => message.error(String(e)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const typeTree = useMemo(() => buildTypeTree(typeItems), [typeItems]);

  // 选中叶子后取生效配置（后端创建时按同口径快照）
  const onTypeSelected = async (path: (string | number)[]) => {
    const code = String(path[path.length - 1] ?? '');
    const entity = typeItems.find((i) => i.code === code);
    if (!entity?.id) return;
    setSelectedTypeId(entity.id);
    try {
      const eff = await queryClient.fetchQuery({
        queryKey: ['getEffectiveContractConfig', entity.id],
        queryFn: () => apiClient.contractTypeService.GetEffectiveConfig({ id: entity.id! }),
        staleTime: 60_000,
      });
      const e = eff.effective;
      setDiffFields(e?.diffFields ?? []);
      setGuidance(e?.guidance || undefined);
      setRequiredAttachments(e?.requiredAttachments ?? []);
      setPartyBLabel(e?.partyBLabel || undefined);
      // 付款方式/费用方向预填类型默认（可改）
      form.setFieldsValue({
        payMethod: e?.payMethod && e.payMethod !== 'PAY_METHOD_UNSPECIFIED' ? e.payMethod : 'PAY_ONCE',
        feeDirection: e?.feeDirection && e.feeDirection !== 'DIRECTION_UNSPECIFIED' ? e.feeDirection : 'DIRECTION_PAYABLE',
      });
    } catch (e) {
      message.error(String(e));
    }
  };

  const createMutation = useCreateContract({
    onSuccess: () => {
      message.success(t('list.createSuccess'));
      invalidateContracts(queryClient);
      onClose();
    },
    onError: (error: Error) => message.error(error.message || t('list.createFailed')),
  });

  const handleOk = async () => {
    const values = await form.validateFields();
    if (!selectedTypeId) {
      message.warning(t('list.typeRequired'));
      return;
    }
    const diffValues: Record<string, string> = {};
    for (const field of diffFields) {
      const key = field?.key ?? '';
      if (!key) continue;
      const v = values[`diff_${key}`];
      if (v != null && v !== '') diffValues[key] = String(v);
    }
    // 附件统一上传（required_attachments 仅提示性校验，不强制拦截）
    const attachments: string[] = [];
    for (const f of fileList) {
      const raw = f.originFileObj;
      if (!raw) continue;
      try {
        const res = (await uploadFile('contract', 'contracts', raw)) as { publicUrl?: string };
        if (res?.publicUrl) attachments.push(res.publicUrl);
      } catch (e) {
        message.error(`${f.name}: ${String(e)}`);
      }
    }
    createMutation.mutate({
      values: {
        contractTypeId: selectedTypeId,
        region: values.region,
        dept: values.dept,
        partyAContact: values.partyAContact,
        partyAPhone: values.partyAPhone,
        partyBName: values.partyBName,
        partyBCredit: values.partyBCredit,
        partyBKind: values.partyBKind,
        partyBLegal: values.partyBLegal,
        partyBContact: values.partyBContact,
        partyBPhone: values.partyBPhone,
        partyBBank: values.partyBBank,
        partyBAccount: values.partyBAccount,
        importance: values.importance ?? 'IMPORTANCE_NORMAL',
        startDate: (values.period?.[0] as Dayjs)?.toISOString(),
        endDate: (values.period?.[1] as Dayjs)?.toISOString(),
        amount: values.amount,
        taxRate: values.taxRate,
        payMethod: values.payMethod,
        feeDirection: values.feeDirection,
        deposit: values.deposit,
        diffValues,
        attachments,
        remark: values.remark,
      },
    });
  };

  /** 附件入列（beforeUpload=false 不自动传，提交时统一走既有上传设施） */
  const uploadAttachments: UploadProps = {
    multiple: true,
    fileList,
    beforeUpload: () => false,
    onChange: (info) => setFileList(info.fileList),
  };

  return (
    <Modal
      title={t('list.createTitle')}
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      confirmLoading={createMutation.isPending}
      width={760}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item label={t('list.type')} required extra={t('list.typeExtra')}>
          <Cascader
            options={typeTree}
            changeOnSelect={false}
            showSearch
            placeholder={t('list.typePlaceholder')}
            onChange={(path) => onTypeSelected(path as (string | number)[])}
          />
        </Form.Item>
        {guidance && <Alert type="info" showIcon message={guidance} style={{ marginBottom: 16 }} />}
        {requiredAttachments.length > 0 && (
          <Alert
            type="warning"
            showIcon
            message={t('list.requiredAttachmentsTip', { names: requiredAttachments.join('、') })}
            style={{ marginBottom: 16 }}
          />
        )}

        <Form.Item label={t('list.partyASection')} style={{ marginBottom: 0 }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <Form.Item name="region" label={t('list.region')}>
              <Input placeholder={t('list.regionPlaceholder')} />
            </Form.Item>
            <Form.Item name="dept" label={t('list.dept')}>
              <Input placeholder={t('list.deptPlaceholder')} />
            </Form.Item>
            <Form.Item name="partyAContact" label={t('list.partyAContact')}>
              <Input />
            </Form.Item>
            <Form.Item name="partyAPhone" label={t('list.partyAPhone')}>
              <Input />
            </Form.Item>
          </div>
        </Form.Item>

        <Form.Item label={t('list.partyBSection')} style={{ marginBottom: 0 }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <Form.Item
              name="partyBName"
              label={partyBLabel ? `${partyBLabel}（${t('list.partyBName')}）` : t('list.partyBName')}
              rules={[{ required: true, message: t('list.partyBNameRequired') }]}
            >
              <Input placeholder={t('list.partyBNamePlaceholder')} />
            </Form.Item>
            <Form.Item name="partyBCredit" label={t('list.partyBCredit')}>
              <Input />
            </Form.Item>
            <Form.Item name="partyBKind" label={t('list.partyBKind')}>
              <Select
                allowClear
                options={[
                  { label: '企业法人', value: 'PARTY_B_ENTERPRISE' },
                  { label: '非法人组织', value: 'PARTY_B_ORG' },
                  { label: '自然人', value: 'PARTY_B_NATURAL' },
                ]}
              />
            </Form.Item>
            <Form.Item name="partyBLegal" label={t('list.partyBLegal')}>
              <Input />
            </Form.Item>
            <Form.Item name="partyBContact" label={t('list.partyBContact')}>
              <Input />
            </Form.Item>
            <Form.Item name="partyBPhone" label={t('list.partyBPhone')}>
              <Input />
            </Form.Item>
            <Form.Item name="partyBBank" label={t('list.partyBBank')}>
              <Input />
            </Form.Item>
            <Form.Item name="partyBAccount" label={t('list.partyBAccount')}>
              <Input />
            </Form.Item>
          </div>
        </Form.Item>

        <Form.Item label={t('list.basicSection')} style={{ marginBottom: 0 }}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <Form.Item name="importance" label={t('list.importance')} initialValue="IMPORTANCE_NORMAL">
              <Select
                options={[
                  { label: '一般', value: 'IMPORTANCE_NORMAL' },
                  { label: '重要', value: 'IMPORTANCE_IMPORTANT' },
                ]}
              />
            </Form.Item>
            <Form.Item
              name="period"
              label={t('list.period')}
              rules={[{ required: true, message: t('list.periodRequired') }]}
            >
              <DatePicker.RangePicker style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item
              name="amount"
              label={t('list.amount')}
              rules={[{ required: true, message: t('list.amountRequired') }]}
            >
              <InputNumber min={0.01} precision={2} style={{ width: '100%' }} addonAfter="元" />
            </Form.Item>
            <Form.Item name="taxRate" label={t('list.taxRate')}>
              <Select
                allowClear
                options={[0, 1, 3, 5, 6, 9, 13].map((v) => ({ label: `${v}%`, value: v }))}
              />
            </Form.Item>
            <Form.Item name="payMethod" label={t('list.payMethod')} rules={[{ required: true }]}>
              <Select
                options={[
                  { label: '一次性付款', value: 'PAY_ONCE' },
                  { label: '按月付款', value: 'PAY_MONTHLY' },
                  { label: '按季付款', value: 'PAY_QUARTERLY' },
                  { label: '按节点付款', value: 'PAY_NODE' },
                  { label: '按进度付款', value: 'PAY_PROGRESS' },
                ]}
              />
            </Form.Item>
            <Form.Item name="feeDirection" label={t('list.feeDirection')} rules={[{ required: true }]}>
              <Select
                options={[
                  { label: '应付（我方付款）', value: 'DIRECTION_PAYABLE' },
                  { label: '应收（我方收款）', value: 'DIRECTION_RECEIVABLE' },
                ]}
              />
            </Form.Item>
            <Form.Item name="deposit" label={t('list.deposit')}>
              <InputNumber min={0} precision={2} style={{ width: '100%' }} addonAfter="元" />
            </Form.Item>
          </div>
        </Form.Item>

        {diffFields.length > 0 && (
          <Form.Item label={t('list.diffSection', { type: typeItems.find((i) => i.id === selectedTypeId)?.name ?? '' })} style={{ marginBottom: 0 }}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              {diffFields.map((field) => (
                <Form.Item key={field?.key} name={`diff_${field?.key}`} label={field?.label}>
                  {field?.widget === 'WIDGET_SELECT' ? (
                    <Select allowClear options={(field?.options ?? []).map((o) => ({ label: o, value: o }))} />
                  ) : field?.widget === 'WIDGET_DATE' ? (
                    <DatePicker style={{ width: '100%' }} />
                  ) : field?.widget === 'WIDGET_TEXTAREA' ? (
                    <Input.TextArea rows={2} maxLength={500} showCount />
                  ) : (
                    <Input />
                  )}
                </Form.Item>
              ))}
            </div>
          </Form.Item>
        )}

        <Form.Item label={t('list.attachments')} extra={t('list.attachmentsExtra')}>
          <Upload.Dragger {...uploadAttachments}>
            <p className="ant-upload-drag-icon">
              <InboxOutlined />
            </p>
            <p className="ant-upload-text">{t('list.uploadText')}</p>
          </Upload.Dragger>
        </Form.Item>
        <Form.Item name="remark" label={t('list.remark')}>
          <Input.TextArea rows={2} maxLength={1024} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default CreateContractModal;
