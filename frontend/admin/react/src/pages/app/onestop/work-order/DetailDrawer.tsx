import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  App,
  Button,
  Descriptions,
  Drawer,
  Input,
  Modal,
  Select,
  Space,
  Spin,
  Table,
  Tag,
  Timeline,
  Typography,
} from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import type { onestopservicev1_WorkOrder, onestopservicev1_WorkOrderItem } from '@/api/generated/admin/service/v1';
import {
  invalidateWorkOrders,
  useGetWorkOrder,
  useListWorkOrderEvents,
  useListWorkOrderItems,
  useWorkOrderAction,
} from '@/api/hooks/onestop';
import {
  isTerminal,
  kindColor,
  kindName,
  priorityColor,
  priorityName,
  profileOf,
  sourceName,
  statusColor,
  statusName,
  temperatureName,
} from './constants';

const { Text } = Typography;

/** 动作参数收集弹窗的轻量实现：按动作需要渲染字段 */
interface ActionModalState {
  action: string;
  title: string;
  fields: {
    key: string;
    label: string;
    type: 'text' | 'textarea' | 'select' | 'number';
    required?: boolean;
    options?: { label: string; value: string }[];
    placeholder?: string;
  }[];
  // 提交回调：values → 对应动作请求体
  submit: (values: Record<string, unknown>, id: number) => void;
}

const detailDescriptions = (d: onestopservicev1_WorkOrder) => [
  { key: 'type', label: '服务类型', children: `${d.serviceTypeCode ?? ''} ${d.serviceTypeName ?? ''}` },
  { key: 'source', label: '来源', children: sourceName(d.source) },
  { key: 'post', label: '执行岗位', children: d.executionPost || '-' },
  { key: 'assignee', label: '执行人', children: d.assigneeName || '-' },
  { key: 'priority', label: '优先级', children: <Tag color={priorityColor(d.priority)}>{priorityName(d.priority)}</Tag> },
  { key: 'requester', label: '发起人', children: `${d.requesterName ?? '-'} ${d.requesterPhone ?? ''}` },
  { key: 'location', label: '位置', children: d.location || '-' },
  { key: 'equipment', label: '服务对象', children: d.equipmentDesc || '-' },
  {
    key: 'sla',
    label: '时限快照',
    children: d.responseTimeLimit ? `响应 ${d.responseTimeLimit} 分钟 / 完成 ${d.completionTimeLimit ?? '-'} 分钟` : '不限时',
  },
  {
    key: 'deadline',
    label: '响应截止',
    children: d.responseDeadline ? new Date(d.responseDeadline).toLocaleString() : '-',
  },
];

/**
 * 工单详情抽屉：状态头 + 动作按钮（按状态机矩阵与形态收敛）+ 形态面板
 * （运送交接 / 执行项）+ 时间线。动作参数经轻量弹窗收集后走扁平 body RPC。
 */
const WorkOrderDetailDrawer = ({
  id,
  onClose,
  onOpenOrder,
}: {
  id: number | undefined;
  onClose: () => void;
  onOpenOrder?: (id: number) => void;
}) => {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [actionModal, setActionModal] = useState<ActionModalState>();
  const [itemsDraft, setItemsDraft] = useState<Record<number, Partial<onestopservicev1_WorkOrderItem>>>({});

  const { data: order, isLoading } = useGetWorkOrder(id);
  const showItems = !!order && profileOf(order.orderKind).panels.includes('items');
  const { data: events } = useListWorkOrderEvents(id);
  const { data: itemsResp } = useListWorkOrderItems(id, { enabled: showItems });
  const items = itemsResp?.items ?? [];

  const refresh = () => invalidateWorkOrders(queryClient, id);

  // 全部动作 hook 必须无条件调用（本组件在 order 为空时仍渲染空抽屉骨架）
  const actionOptions = {
    onSuccess: () => {
      message.success(t('workOrder.actionSuccess'));
      refresh();
      setActionModal(undefined);
    },
    onError: (error: Error) => message.error(error.message || t('workOrder.actionFailed')),
  };
  const dispatchMutation = useWorkOrderAction('dispatch', actionOptions);
  const approveMutation = useWorkOrderAction('approve', actionOptions);
  const arriveMutation = useWorkOrderAction('arrive', actionOptions);
  const pickupMutation = useWorkOrderAction('pickup', actionOptions);
  const deliverMutation = useWorkOrderAction('deliver', actionOptions);
  const completeMutation = useWorkOrderAction('complete', actionOptions);
  const verifyMutation = useWorkOrderAction('verify', actionOptions);
  const cancelMutation = useWorkOrderAction('cancel', actionOptions);
  const urgeMutation = useWorkOrderAction('urge', actionOptions);
  const reassignMutation = useWorkOrderAction('reassign', actionOptions);
  const abnormalMutation = useWorkOrderAction('abnormal', actionOptions);
  const satisfactionMutation = useWorkOrderAction('satisfaction', actionOptions);

  const anyPending =
    dispatchMutation.isPending || approveMutation.isPending || arriveMutation.isPending ||
    pickupMutation.isPending || deliverMutation.isPending || completeMutation.isPending ||
    verifyMutation.isPending || cancelMutation.isPending || urgeMutation.isPending ||
    reassignMutation.isPending || abnormalMutation.isPending || satisfactionMutation.isPending;

  const submitAction = (values: Record<string, unknown>) => {
    if (!order?.id || !actionModal) return;
    actionModal.submit(values, order.id);
  };

  if (!order) {
    return (
      <Drawer open={id != null} onClose={onClose} width={720}>
        {isLoading ? <Spin /> : <Text type="secondary">{t('workOrder.notFound')}</Text>}
      </Drawer>
    );
  }

  const status = order.status;
  const isTransport = order.orderKind === 'KIND_TRANSPORT';

  // 动作按钮矩阵（服务层状态机的镜像；按钮只是引导，后端仍是唯一闸门）
  const buttons: React.ReactNode[] = [];
  const openModal = (state: ActionModalState) => setActionModal(state);

  if (status === 'STATUS_PENDING') {
    buttons.push(
      <Button key="dispatch" type="primary" onClick={() =>
        openModal({
          action: 'dispatch',
          title: t('workOrder.dispatch'),
          fields: [
            { key: 'post', label: '岗位', type: 'text' },
            { key: 'assigneeName', label: '执行人', type: 'text' },
            { key: 'note', label: '附言', type: 'text' },
          ],
          submit: (v, oid) => dispatchMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.dispatch')}
      </Button>,
    );
  }
  if (status === 'STATUS_APPROVING') {
    buttons.push(
      <Button key="approve-pass" type="primary" onClick={() =>
        openModal({
          action: 'approve',
          title: t('workOrder.approve'),
          fields: [{ key: 'note', label: '审批意见', type: 'text' }],
          submit: (v, oid) => approveMutation.mutate({ id: oid, passed: true, ...v } as never),
        })}>
        {t('workOrder.approvePass')}
      </Button>,
      <Button key="approve-reject" danger onClick={() =>
        openModal({
          action: 'approve',
          title: t('workOrder.approveReject'),
          fields: [{ key: 'note', label: '驳回原因', type: 'text', required: true }],
          submit: (v, oid) => approveMutation.mutate({ id: oid, passed: false, ...v } as never),
        })}>
        {t('workOrder.approveReject')}
      </Button>,
    );
  }
  if (status === 'STATUS_DISPATCHED' && !isTransport) {
    buttons.push(
      <Button key="arrive" type="primary" onClick={() =>
        openModal({
          action: 'arrive',
          title: t('workOrder.arrive'),
          fields: [{ key: 'note', label: '附言', type: 'text' }],
          submit: (v, oid) => arriveMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.arrive')}
      </Button>,
    );
  }
  if (status === 'STATUS_DISPATCHED' && isTransport) {
    buttons.push(
      <Button key="pickup" type="primary" onClick={() =>
        openModal({
          action: 'pickup',
          title: t('workOrder.pickup'),
          fields: [{ key: 'signer', label: '签收人', type: 'text', required: true }],
          submit: (v, oid) => pickupMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.pickup')}
      </Button>,
    );
  }
  if (status === 'STATUS_PROCESSING' && isTransport) {
    buttons.push(
      <Button key="deliver" type="primary" onClick={() =>
        openModal({
          action: 'deliver',
          title: t('workOrder.deliver'),
          fields: [{ key: 'signer', label: '签收人', type: 'text', required: true }],
          submit: (v, oid) => deliverMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.deliver')}
      </Button>,
    );
  }
  if (status === 'STATUS_PROCESSING' && !isTransport) {
    buttons.push(
      <Button key="complete" type="primary" onClick={() => {
        const itemsPayload = items.map((it) => ({
          id: it.id,
          result: itemsDraft[it.id ?? 0]?.result ?? it.result ?? 'ITEM_PENDING',
          value: itemsDraft[it.id ?? 0]?.value ?? it.value ?? '',
          remark: itemsDraft[it.id ?? 0]?.remark ?? it.remark ?? '',
        }));
        openModal({
          action: 'complete',
          title: t('workOrder.complete'),
          fields: [
            { key: 'resultDesc', label: '完工说明', type: 'textarea', required: true },
            { key: 'actualWorkHours', label: '实际工时（小时）', type: 'number' },
          ],
          submit: (v, oid) => completeMutation.mutate({ id: oid, ...v, items: itemsPayload } as never),
        });
      }}>
        {t('workOrder.complete')}
      </Button>,
    );
  }
  if (status === 'STATUS_COMPLETED') {
    buttons.push(
      <Button key="verify-pass" type="primary" onClick={() =>
        openModal({
          action: 'verify',
          title: t('workOrder.verify'),
          fields: [
            { key: 'signer', label: '验收人', type: 'text' },
            { key: 'note', label: '验收意见', type: 'text' },
          ],
          submit: (v, oid) => verifyMutation.mutate({ id: oid, passed: true, ...v } as never),
        })}>
        {t('workOrder.verifyPass')}
      </Button>,
      <Button key="verify-reject" danger onClick={() =>
        openModal({
          action: 'verify',
          title: t('workOrder.verifyReject'),
          fields: [{ key: 'note', label: '驳回原因', type: 'text', required: true }],
          submit: (v, oid) => verifyMutation.mutate({ id: oid, passed: false, ...v } as never),
        })}>
        {t('workOrder.verifyReject')}
      </Button>,
    );
  }
  if (!isTerminal(status) && status !== 'STATUS_COMPLETED') {
    buttons.push(
      <Button key="cancel" danger onClick={() =>
        openModal({
          action: 'cancel',
          title: t('workOrder.cancel'),
          fields: [{ key: 'reason', label: '撤销原因', type: 'text', required: true }],
          submit: (v, oid) => cancelMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.cancel')}
      </Button>,
    );
  }
  if (!isTerminal(status)) {
    buttons.push(
      <Button key="urge" onClick={() => urgeMutation.mutate({ id: order.id! })}>
        {t('workOrder.urge')}
      </Button>,
      <Button key="reassign" onClick={() =>
        openModal({
          action: 'reassign',
          title: t('workOrder.reassign'),
          fields: [
            { key: 'post', label: '新岗位', type: 'text' },
            { key: 'assigneeName', label: '新执行人', type: 'text' },
          ],
          submit: (v, oid) => reassignMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.reassign')}
      </Button>,
      <Button key="abnormal" onClick={() =>
        openModal({
          action: 'abnormal',
          title: t('workOrder.abnormal'),
          fields: [{ key: 'desc', label: '异常描述', type: 'textarea', required: true }],
          submit: (v, oid) => abnormalMutation.mutate({ id: oid, ...v } as never),
        })}>
        {t('workOrder.abnormal')}
      </Button>,
    );
  }
  if (status === 'STATUS_CLOSED' && !order.satisfactionScore) {
    buttons.push(
      <Button key="satisfaction" onClick={() =>
        openModal({
          action: 'satisfaction',
          title: t('workOrder.satisfaction'),
          fields: [
            {
              key: 'score', label: '评分', type: 'select', required: true,
              options: [1, 2, 3, 4, 5].map((n) => ({ label: '★'.repeat(n), value: String(n) })),
            },
            { key: 'comment', label: '回访意见', type: 'text' },
          ],
          submit: (v, oid) => satisfactionMutation.mutate({ id: oid, score: Number(v.score), comment: v.comment } as never),
        })}>
        {t('workOrder.satisfaction')}
      </Button>,
    );
  }

  return (
    <Drawer open={id != null} onClose={onClose} width={760} title={order.orderNo}>
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        <Space wrap>
          <Tag color={statusColor(status)}>{statusName(status)}</Tag>
          <Tag color={kindColor(order.orderKind)}>{kindName(order.orderKind)}</Tag>
          <Tag color={priorityColor(order.priority)}>{priorityName(order.priority)}</Tag>
          {order.hasAbnormality && <Tag color="red">异常</Tag>}
          {order.plannedExecAt && <Tag color="blue">计划 {new Date(order.plannedExecAt).toLocaleString()}</Tag>}
        </Space>

        <div>{buttons.length > 0 ? <Space wrap>{buttons}</Space> : <Text type="secondary">—</Text>}</div>

        <Descriptions title={t('workOrder.basicInfo')} items={detailDescriptions(order)} size="small" column={2} bordered />

        {isTransport && (
          <Descriptions
            title={t('workOrder.transportInfo')}
            size="small"
            column={2}
            bordered
            items={[
              { key: 'o', label: t('workOrder.origin'), children: order.origin || '-' },
              { key: 'd', label: t('workOrder.destination'), children: order.destination || '-' },
              { key: 'via', label: '途经', children: order.viaPoints?.join(' → ') || '-' },
              { key: 'temp', label: '温控', children: temperatureName(order.temperatureRequirement) },
              { key: 'cargo', label: '物品', children: order.cargoDesc || '-' },
              { key: 'est', label: '预计时长', children: order.estDurationMinutes ? `${order.estDurationMinutes} 分钟` : '-' },
              { key: 'picked', label: t('workOrder.pickup'), children: order.pickedSigner ? `${order.pickedSigner} ${order.pickedUpAt ? new Date(order.pickedUpAt).toLocaleString() : ''}` : '-' },
              { key: 'delivered', label: t('workOrder.deliver'), children: order.deliveredSigner ? `${order.deliveredSigner} ${order.deliveredAt ? new Date(order.deliveredAt).toLocaleString() : ''}` : '-' },
            ]}
          />
        )}

        {order.sourceOrderId != null && (
          <Button type="link" onClick={() => onOpenOrder?.(order.sourceOrderId!)}>
            {t('workOrder.viewSourceOrder')} #{order.sourceOrderId}
          </Button>
        )}

        {showItems && (
          <div>
            <Typography.Title level={5}>{t('workOrder.executionItems')}</Typography.Title>
            <Table
              rowKey="id"
              size="small"
              pagination={false}
              dataSource={items}
              columns={[
                { title: t('workOrder.itemName'), dataIndex: 'name' },
                {
                  title: t('workOrder.itemResult'),
                  dataIndex: 'result',
                  width: 130,
                  render: (_, record) =>
                    status === 'STATUS_PROCESSING' ? (
                      <Select
                        size="small"
                        value={itemsDraft[record.id ?? 0]?.result ?? record.result ?? 'ITEM_PENDING'}
                        onChange={(v) => setItemsDraft((prev) => ({ ...prev, [record.id ?? 0]: { ...prev[record.id ?? 0], result: v } }))}
                        options={[
                          { label: '未执行', value: 'ITEM_PENDING' },
                          { label: '正常', value: 'ITEM_NORMAL' },
                          { label: '异常', value: 'ITEM_ABNORMAL' },
                          { label: '跳过', value: 'ITEM_SKIPPED' },
                        ]}
                      />
                    ) : (
                      <Tag color={record.result === 'ITEM_ABNORMAL' ? 'red' : record.result === 'ITEM_NORMAL' ? 'green' : 'default'}>
                        {record.result === 'ITEM_ABNORMAL' ? '异常' : record.result === 'ITEM_NORMAL' ? '正常' : record.result === 'ITEM_SKIPPED' ? '跳过' : '未执行'}
                      </Tag>
                    ),
                },
                {
                  title: t('workOrder.itemValue'),
                  dataIndex: 'value',
                  width: 150,
                  render: (_, record) =>
                    status === 'STATUS_PROCESSING' ? (
                      <Input
                        size="small"
                        value={itemsDraft[record.id ?? 0]?.value ?? record.value ?? ''}
                        onChange={(e) => setItemsDraft((prev) => ({ ...prev, [record.id ?? 0]: { ...prev[record.id ?? 0], value: e.target.value } }))}
                      />
                    ) : (
                      record.value || '-'
                    ),
                },
                { title: t('workOrder.itemRemark'), dataIndex: 'remark', render: (_, record) => {
                  const remark = itemsDraft[record.id ?? 0]?.remark ?? record.remark ?? '';
                  return status === 'STATUS_PROCESSING' ? (
                    <Input size="small" value={remark ?? ''} onChange={(e) => setItemsDraft((prev) => ({ ...prev, [record.id ?? 0]: { ...prev[record.id ?? 0], remark: e.target.value } }))} />
                  ) : (remark || '-');
                } },
              ]}
            />
          </div>
        )}

        {order.resultDesc && (
          <Descriptions
            title={t('workOrder.resultInfo')}
            size="small"
            column={1}
            bordered
            items={[
              { key: 'desc', label: '结果说明', children: order.resultDesc },
              { key: 'hours', label: '实际工时', children: order.actualWorkHours ? `${order.actualWorkHours} 小时` : '-' },
              ...(order.satisfactionScore
                ? [{ key: 'sat', label: '满意度', children: '★'.repeat(order.satisfactionScore) + (order.satisfactionComment ? `（${order.satisfactionComment}）` : '') }]
                : []),
            ]}
          />
        )}

        <div>
          <Typography.Title level={5}>{t('workOrder.timeline')}</Typography.Title>
          <Timeline
            items={(events?.items ?? []).map((e) => ({
              children: (
                <div>
                  <Text strong>{actionLabel(e.action)}</Text>{' '}
                  <Text type="secondary">{e.createdAt ? new Date(e.createdAt).toLocaleString() : ''}</Text>
                  {e.note && <div><Text type="secondary">{e.note}</Text></div>}
                </div>
              ),
            }))}
          />
        </div>
      </Space>

      <ActionParamModal state={actionModal} onClose={() => setActionModal(undefined)} onSubmit={submitAction} pending={anyPending} />
    </Drawer>
  );
};

function actionLabel(action?: string | null): string {
  const map: Record<string, string> = {
    ACTION_CREATE: '创建',
    ACTION_DISPATCH: '派单',
    ACTION_REASSIGN: '改派',
    ACTION_ARRIVE: '到场',
    ACTION_PICKUP: '取件签收',
    ACTION_DELIVER: '送达签收',
    ACTION_COMPLETE: '完工提交',
    ACTION_VERIFY_PASS: '验收通过',
    ACTION_VERIFY_REJECT: '验收驳回',
    ACTION_CANCEL: '撤销',
    ACTION_URGE: '催办',
    ACTION_ABNORMAL: '异常上报',
    ACTION_TRANSFER: '转单',
    ACTION_DERIVE: '派生',
    ACTION_APPROVE_PASS: '审批通过',
    ACTION_APPROVE_REJECT: '审批驳回',
    ACTION_SATISFACTION: '满意度回访',
  };
  return map[action ?? ''] ?? action ?? '-';
}

const ActionParamModal = ({
  state,
  onClose,
  onSubmit,
  pending,
}: {
  state?: ActionModalState;
  onClose: () => void;
  onSubmit: (values: Record<string, unknown>) => void;
  pending: boolean;
}) => {
  const [values, setValues] = useState<Record<string, unknown>>({});
  return (
    <Modal
      title={state?.title}
      open={!!state}
      onCancel={onClose}
      confirmLoading={pending}
      onOk={() => {
        const missing = state?.fields.find((f) => f.required && !values[f.key]);
        if (missing) {
          return;
        }
        const payload: Record<string, unknown> = {};
        state?.fields.forEach((f) => {
          const v = values[f.key];
          if (f.type === 'number' && v != null && v !== '') payload[f.key] = Number(v);
          else if (v != null && v !== '') payload[f.key] = v;
        });
        onSubmit(payload);
      }}
      destroyOnHidden
    >
      {state?.fields.map((f) => (
        <div key={f.key} style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4 }}>
            {f.label}
            {f.required ? ' *' : ''}
          </div>
          {f.type === 'textarea' ? (
            <Input.TextArea
              rows={3}
              value={(values[f.key] as string) ?? ''}
              onChange={(e) => setValues((prev) => ({ ...prev, [f.key]: e.target.value }))}
            />
          ) : f.type === 'select' ? (
            <Select
              style={{ width: '100%' }}
              options={f.options}
              value={values[f.key] as string}
              onChange={(v) => setValues((prev) => ({ ...prev, [f.key]: v }))}
            />
          ) : (
            <Input
              type={f.type === 'number' ? 'number' : 'text'}
              value={(values[f.key] as string) ?? ''}
              onChange={(e) => setValues((prev) => ({ ...prev, [f.key]: e.target.value }))}
            />
          )}
        </div>
      ))}
    </Modal>
  );
};

export default WorkOrderDetailDrawer;
