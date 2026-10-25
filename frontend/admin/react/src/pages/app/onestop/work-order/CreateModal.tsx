import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, App, Cascader, Form, Input, Modal, Select } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import type { onestopservicev1_ServiceType } from '@/api/generated/admin/service/v1';
import { invalidateWorkOrders, useCreateWorkOrder } from '@/api/hooks/onestop';
import { apiClient } from '@/api/client';

interface CreateModalProps {
  open: boolean;
  onClose: () => void;
}

/** 平铺类型列表 → 四级 Cascader 树（code 即路径；仅启用节点可选） */
function buildTypeTree(items: onestopservicev1_ServiceType[]) {
  type Node = { value: string; label: string; children?: Node[]; selectable: boolean };
  const roots: Node[] = [];
  const byCode = new Map<string, Node>();
  for (const item of items) {
    const node: Node = {
      value: item.code ?? '',
      label: `${item.code ?? ''} ${item.name ?? ''}`,
      selectable: (item.code?.length ?? 0) === 8 && item.isEnabled !== false,
    };
    byCode.set(node.value, node);
    const parentCode = node.value.length > 2 ? node.value.slice(0, -2) : '';
    const parent = parentCode ? byCode.get(parentCode) : undefined;
    if (parent) {
      parent.children = [...(parent.children ?? []), node];
    } else {
      roots.push(node);
    }
  }
  return roots;
}

/**
 * 坐席代发起：类型必须选到细类；选中后拉取 effective-config 展示
 * 快照预览（岗位/优先级/温控/指导），提交由后端解析快照。
 */
const CreateWorkOrderModal = ({ open, onClose }: CreateModalProps) => {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();
  const [typeItems, setTypeItems] = useState<onestopservicev1_ServiceType[]>([]);
  const [selectedTypeId, setSelectedTypeId] = useState<number>();
  const [phrases, setPhrases] = useState<string[]>([]);
  const [guidance, setGuidance] = useState<string>();
  const [kind, setKind] = useState<string>();

  useEffect(() => {
    if (!open) return;
    form.resetFields();
    setSelectedTypeId(undefined);
    setPhrases([]);
    setGuidance(undefined);
    setKind(undefined);
    queryClient
      .fetchQuery({
        queryKey: ['listServiceTypes', 'work-order-create'],
        queryFn: () =>
          apiClient.oneStopServiceTypeService.List({
            page: 1,
            pageSize: 500,
            orderBy: JSON.stringify(['code']),
            sorting: [],
          }),
        staleTime: 5 * 60_000,
      })
      .then((res) => {
        const items = (res?.items ?? []).filter((i) => i.isEnabled !== false);
        setTypeItems(items);
      })
      .catch((e) => message.error(String(e)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const typeTree = useMemo(() => buildTypeTree(typeItems), [typeItems]);

  // 选中细类后取生效配置（岗位/温控/语句/指导预览；后端创建时按同口径快照）
  const onTypeSelected = async (path: (string | number)[]) => {
    const code = String(path[path.length - 1] ?? '');
    const entity = typeItems.find((i) => i.code === code);
    if (!entity?.id) return;
    setSelectedTypeId(entity.id);
    try {
      const eff = await queryClient.fetchQuery({
        queryKey: ['effectiveConfig', entity.id],
        queryFn: () => apiClient.oneStopServiceTypeService.GetEffectiveConfig({ id: entity.id! }),
        staleTime: 60_000,
      });
      setPhrases(eff.effective?.commonPhrases ?? []);
      setGuidance(eff.effective?.guidance || undefined);
      setKind(eff.effective?.orderKind);
    } catch (e) {
      message.error(String(e));
    }
  };

  const createMutation = useCreateWorkOrder({
    onSuccess: () => {
      message.success(t('workOrder.createSuccess'));
      invalidateWorkOrders(queryClient);
      onClose();
    },
    onError: (error: Error) => message.error(error.message || t('workOrder.createFailed')),
  });

  const handleOk = async () => {
    const values = await form.validateFields();
    if (!selectedTypeId) {
      message.warning(t('workOrder.typeRequired'));
      return;
    }
    createMutation.mutate({
      values: {
        serviceTypeId: selectedTypeId,
        description: values.description,
        location: values.location,
        equipmentDesc: values.equipmentDesc,
        requesterName: values.requesterName,
        requesterPhone: values.requesterPhone,
        commonPhrase: values.commonPhrase,
        priority: values.priority,
        origin: values.origin,
        destination: values.destination,
        cargoDesc: values.cargoDesc,
        channel: 'CHANNEL_DESK',
      },
    });
  };

  return (
    <Modal
      title={t('workOrder.createTitle')}
      open={open}
      onCancel={onClose}
      onOk={handleOk}
      confirmLoading={createMutation.isPending}
      width={640}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item
          label={t('workOrder.type')}
          required
          extra={kind ? t('workOrder.kindPreview', { kind: kindNameLocal(kind) }) : t('workOrder.typeExtra')}
        >
          <Cascader
            options={typeTree}
            changeOnSelect={false}
            showSearch
            placeholder={t('workOrder.typePlaceholder')}
            onChange={(path) => onTypeSelected(path as (string | number)[])}
          />
        </Form.Item>
        {guidance && (
          <Form.Item noStyle>
            <Alert text={guidance} />
          </Form.Item>
        )}
        <Form.Item
          name="description"
          label={t('workOrder.description')}
          rules={[{ required: true, message: t('workOrder.descriptionRequired') }]}
        >
          <Input.TextArea rows={2} placeholder={t('workOrder.descriptionPlaceholder')} />
        </Form.Item>
        {phrases.length > 0 && (
          <Form.Item label={t('workOrder.quickPhrases')}>
            <Select
              allowClear
              placeholder={t('workOrder.quickPhrasesPlaceholder')}
              options={phrases.map((p) => ({ label: p, value: p }))}
              onChange={(v) => form.setFieldValue('commonPhrase', v)}
            />
          </Form.Item>
        )}
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <Form.Item name="location" label={t('workOrder.location')}>
            <Input placeholder={t('workOrder.locationPlaceholder')} />
          </Form.Item>
          <Form.Item name="equipmentDesc" label={t('workOrder.equipment')}>
            <Input placeholder={t('workOrder.equipmentPlaceholder')} />
          </Form.Item>
          <Form.Item name="requesterName" label={t('workOrder.requester')}>
            <Input placeholder={t('workOrder.requesterPlaceholder')} />
          </Form.Item>
          <Form.Item name="requesterPhone" label={t('workOrder.requesterPhone')}>
            <Input />
          </Form.Item>
          <Form.Item name="priority" label={t('workOrder.priorityOverride')} initialValue={undefined}>
            <Select
              allowClear
              options={[
                { label: t('workOrder.priority.normal'), value: 'PRIORITY_NORMAL' },
                { label: t('workOrder.priority.urgent'), value: 'PRIORITY_URGENT' },
                { label: t('workOrder.priority.emergency'), value: 'PRIORITY_EMERGENCY' },
              ]}
            />
          </Form.Item>
          <Form.Item name="commonPhrase" hidden>
            <Input />
          </Form.Item>
        </div>
        {kind === 'KIND_TRANSPORT' && (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <Form.Item
              name="origin"
              label={t('workOrder.origin')}
              rules={[{ required: true, message: t('workOrder.originRequired') }]}
            >
              <Input placeholder={t('workOrder.originPlaceholder')} />
            </Form.Item>
            <Form.Item
              name="destination"
              label={t('workOrder.destination')}
              rules={[{ required: true, message: t('workOrder.destinationRequired') }]}
            >
              <Input placeholder={t('workOrder.destinationPlaceholder')} />
            </Form.Item>
            <Form.Item name="cargoDesc" label={t('workOrder.cargo')}>
              <Input />
            </Form.Item>
          </div>
        )}
      </Form>
    </Modal>
  );
};

function kindNameLocal(kind?: string | null): string {
  switch (kind) {
    case 'KIND_TASK':
      return '计划任务';
    case 'KIND_TRANSPORT':
      return '运送';
    default:
      return '事件处置';
  }
}

export default CreateWorkOrderModal;
