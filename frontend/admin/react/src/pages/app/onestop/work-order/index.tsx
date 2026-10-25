import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Space, Tabs, Tag, Tooltip, Typography } from 'antd';
import ListTable from '@/components/common/ListTable';
import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { PaginationQuery } from '@/core';
import { TABLE as TABLE_CONST } from '@/config/constants';
import type { onestopservicev1_WorkOrder } from '@/api/generated/admin/service/v1';
import { fetchListWorkOrders } from '@/api/hooks/onestop';
import CreateWorkOrderModal from './CreateModal';
import WorkOrderDetailDrawer from './DetailDrawer';
import {
  STATUS_TABS,
  isTerminal,
  kindColor,
  kindName,
  priorityColor,
  priorityName,
  sourceName,
  statusColor,
  statusName,
} from './constants';

const { Text } = Typography;

/** 默认业务序：最新单在前（与后端默认排序一致） */
const DEFAULT_ORDER_BY = ['-created_at'];

const WorkOrderManagement = () => {
  const { t } = useTranslation('onestop');
  const actionRef = useRef<ActionType>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const [activeTab, setActiveTab] = useState('all');
  const [createOpen, setCreateOpen] = useState(false);
  const [detailId, setDetailId] = useState<number>();

  const tabStatuses = STATUS_TABS.find((tab) => tab.key === activeTab)?.statuses ?? [];

  const deadlineTag = (order: onestopservicev1_WorkOrder) => {
    if (isTerminal(order.status)) return <Text type="secondary">—</Text>;
    const overdue =
      (!order.respondedAt && order.responseDeadline && new Date(order.responseDeadline) < new Date()) ||
      (!order.completedAt && order.completionDeadline && new Date(order.completionDeadline) < new Date());
    const near =
      !overdue &&
      order.responseDeadline &&
      new Date(order.responseDeadline).getTime() - Date.now() < 30 * 60 * 1000;
    if (overdue) return <Tag color="red">{t('workOrder.overdue')}</Tag>;
    if (near) return <Tag color="orange">{t('workOrder.dueSoon')}</Tag>;
    return <Text type="secondary">—</Text>;
  };

  const columns: ProColumns<onestopservicev1_WorkOrder>[] = [
    {
      title: t('workOrder.orderNo'),
      dataIndex: 'orderNo',
      width: 130,
      copyable: true,
      fixed: 'left',
      render: (_, record) => (
        <Button type="link" size="small" onClick={() => setDetailId(record.id!)}>
          {record.orderNo}
        </Button>
      ),
    },
    {
      title: t('workOrder.serviceType'),
      dataIndex: 'serviceTypeName',
      width: 170,
      ellipsis: true,
      hideInSearch: true,
      render: (_, record) => (
        <Tooltip title={record.serviceTypeCode}>
          <Space size={4}>
            <Tag color={kindColor(record.orderKind)}>{kindName(record.orderKind)}</Tag>
            {record.serviceTypeName}
          </Space>
        </Tooltip>
      ),
    },
    {
      title: t('workOrder.status'),
      dataIndex: 'status',
      width: 100,
      valueType: 'select',
      valueEnum: {
        STATUS_PENDING: { text: statusName('STATUS_PENDING') },
        STATUS_APPROVING: { text: statusName('STATUS_APPROVING') },
        STATUS_DISPATCHED: { text: statusName('STATUS_DISPATCHED') },
        STATUS_PROCESSING: { text: statusName('STATUS_PROCESSING') },
        STATUS_COMPLETED: { text: statusName('STATUS_COMPLETED') },
        STATUS_CLOSED: { text: statusName('STATUS_CLOSED') },
        STATUS_CANCELLED: { text: statusName('STATUS_CANCELLED') },
      },
      render: (_, record) => <Tag color={statusColor(record.status)}>{statusName(record.status)}</Tag>,
    },
    {
      title: t('workOrder.priority'),
      dataIndex: 'priority',
      width: 80,
      hideInSearch: true,
      render: (_, record) => <Tag color={priorityColor(record.priority)}>{priorityName(record.priority)}</Tag>,
    },
    {
      title: t('workOrder.description'),
      dataIndex: 'description',
      width: 200,
      ellipsis: true,
      hideInSearch: false,
    },
    {
      title: t('workOrder.routeColumn'),
      dataIndex: 'origin',
      width: 200,
      hideInSearch: true,
      render: (_, record) =>
        record.orderKind === 'KIND_TRANSPORT' ? `${record.origin ?? '-'} → ${record.destination ?? '-'}` : '-',
    },
    {
      title: t('workOrder.post'),
      dataIndex: 'executionPost',
      width: 110,
      ellipsis: true,
      render: (_, record) => `${record.executionPost ?? '-'}${record.assigneeName ? ` / ${record.assigneeName}` : ''}`,
    },
    {
      title: t('workOrder.requester'),
      dataIndex: 'requesterName',
      width: 110,
      ellipsis: true,
      hideInSearch: true,
    },
    {
      title: t('workOrder.source'),
      dataIndex: 'source',
      width: 100,
      valueType: 'select',
      valueEnum: {
        SOURCE_MANUAL: { text: sourceName('SOURCE_MANUAL') },
        SOURCE_PLAN: { text: sourceName('SOURCE_PLAN') },
        SOURCE_ROUTE: { text: sourceName('SOURCE_ROUTE') },
        SOURCE_TRANSFER: { text: sourceName('SOURCE_TRANSFER') },
        SOURCE_DERIVED: { text: sourceName('SOURCE_DERIVED') },
      },
    },
    {
      title: t('workOrder.plannedAt'),
      dataIndex: 'plannedExecAt',
      width: 150,
      hideInSearch: true,
      render: (_, record) => (record.plannedExecAt ? new Date(record.plannedExecAt).toLocaleString() : '-'),
    },
    {
      title: t('workOrder.createdAt'),
      dataIndex: 'createdAt',
      width: 150,
      hideInSearch: true,
      render: (_, record) => (record.createdAt ? new Date(record.createdAt).toLocaleString() : '-'),
    },
    {
      title: t('workOrder.slaColumn'),
      width: 90,
      hideInSearch: true,
      render: (_, record) => deadlineTag(record),
    },
  ];

  return (
    <ContentContainer containerRef={containerRef}>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => {
          setActiveTab(key);
          actionRef.current?.reload();
        }}
        items={[
          { key: 'all', tab: t('workOrder.tabAll') },
          ...STATUS_TABS.filter((tab) => tab.key !== 'all').map((tab) => ({
            key: tab.key,
            tab: statusName(tab.statuses[0]),
          })),
        ]}
      />
      <ListTable<onestopservicev1_WorkOrder>
        headerTitle={t('workOrder.listTitle')}
        actionRef={actionRef}
        rowKey="id"
        scroll={{ x: 1500, y: tableScrollY }}
        search={{ labelWidth: 'auto' }}
        columns={columns}
        request={async (params) => {
          const { current, pageSize, ...rest } = params;
          // 状态页签 → status__in（go-crud 结构化过滤；ID/枚举列保持精确匹配）
          const formValues: Record<string, unknown> = { ...rest };
          if (tabStatuses.length > 0) {
            delete formValues.status;
            formValues.status__in = tabStatuses;
          }
          if (rest.orderNo) {
            delete formValues.orderNo;
            formValues.orderNo__eq = rest.orderNo;
          }
          if (rest.source) {
            delete formValues.source;
            formValues.source__eq = rest.source;
          }
          const query = new PaginationQuery({
            paging: { page: current || 1, pageSize: pageSize || TABLE_CONST.DEFAULT_PAGE_SIZE },
            formValues,
            orderBy: DEFAULT_ORDER_BY,
          });
          try {
            const response = await fetchListWorkOrders(query);
            return { data: response.items ?? [], total: response.total ?? 0, success: true };
          } catch {
            return { data: [], total: 0, success: false };
          }
        }}
        pagination={{
          defaultPageSize: TABLE_CONST.DEFAULT_PAGE_SIZE,
          showSizeChanger: true,
          showQuickJumper: true,
        }}
        onRow={(record) => ({ onClick: () => setDetailId(record.id!), style: { cursor: 'pointer' } })}
        toolBarRender={() => [
          <Button key="create" type="primary" onClick={() => setCreateOpen(true)}>
            {t('workOrder.create')}
          </Button>,
        ]}
      />
      <CreateWorkOrderModal open={createOpen} onClose={() => { setCreateOpen(false); actionRef.current?.reload(); }} />
      <WorkOrderDetailDrawer
        id={detailId}
        onClose={() => {
          setDetailId(undefined);
          actionRef.current?.reload();
        }}
        onOpenOrder={(oid) => setDetailId(oid)}
      />
    </ContentContainer>
  );
};

export default WorkOrderManagement;
