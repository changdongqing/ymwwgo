import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Select, Tabs, Tag, Tooltip, Typography } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import ListTable from '@/components/common/ListTable';
import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { PaginationQuery } from '@/core';
import { TABLE as TABLE_CONST } from '@/config/constants';
import type { contractservicev1_Contract } from '@/api/generated/admin/service/v1';
import { fetchListContractTypes, fetchListContracts } from '@/api/hooks/contract';
import CreateContractModal from './CreateModal';
import ContractDetailDrawer from './DetailDrawer';
import { STATUS_TABS, expiringTag, feeDirectionName, importanceName, isTerminal, payMethodName, statusColor, statusName } from './constants';

const { Text } = Typography;

/** 默认业务序：最新合同在前（与后端默认排序一致） */
const DEFAULT_ORDER_BY = ['-created_at'];

/** 到期窗口过滤（原型到期预警的列表化：30/90/180 天内 / 已到期） */
const EXPIRY_OPTIONS = [
  { label: '30 天内到期', value: 'EXPIRING_30' },
  { label: '90 天内到期', value: 'EXPIRING_90' },
  { label: '180 天内到期', value: 'EXPIRING_180' },
  { label: '已到期未关闭', value: 'EXPIRED_OPEN' },
];

function toIsoDay(offsetDays: number): string {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  d.setDate(d.getDate() + offsetDays);
  return d.toISOString();
}

const ContractManagement = () => {
  const { t } = useTranslation('contract');
  const actionRef = useRef<ActionType>(null);
  const queryClient = useQueryClient();
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const [activeTab, setActiveTab] = useState('all');
  const [createOpen, setCreateOpen] = useState(false);
  const [detailId, setDetailId] = useState<number>();
  const [typeOptions, setTypeOptions] = useState<{ label: string; value: number }[]>([]);

  useMemo(() => {
    // 类型候选（启用节点；合同挂叶子，过滤交给后端）
    fetchListContractTypes(new PaginationQuery())
      .then((res) => {
        setTypeOptions(
          (res.items ?? [])
            .filter((i) => i.isEnabled !== false)
            .map((i) => ({ label: `${i.code} ${i.name}`, value: i.id! })),
        );
      })
      .catch(() => undefined);
    return null;
  }, [queryClient]);

  const tabStatuses = STATUS_TABS.find((tab) => tab.key === activeTab)?.statuses ?? [];

  const columns: ProColumns<contractservicev1_Contract>[] = [
    {
      title: t('list.contractNo'),
      dataIndex: 'contractNo',
      width: 130,
      copyable: true,
      fixed: 'left',
      render: (_, record) => (
        <Button type="link" size="small" onClick={() => setDetailId(record.id!)}>
          {record.contractNo}
        </Button>
      ),
    },
    {
      title: t('list.version'),
      dataIndex: 'version',
      width: 70,
      hideInSearch: true,
      render: (_, record) => (record.version ? `v${record.version}` : '-'),
    },
    {
      title: t('list.contractType'),
      dataIndex: 'contractTypeId',
      width: 170,
      ellipsis: true,
      valueType: 'select',
      fieldProps: { options: typeOptions, showSearch: true },
      render: (_, record) => (
        <Tooltip title={record.contractTypeCode}>
          {record.contractTypeName ?? '-'}
        </Tooltip>
      ),
    },
    {
      title: t('list.partyBName'),
      dataIndex: 'partyBName',
      width: 180,
      ellipsis: true,
    },
    {
      title: t('list.amount'),
      dataIndex: 'amount',
      width: 120,
      hideInSearch: true,
      render: (_, record) =>
        record.amount != null ? (
          <Text strong>{Number(record.amount).toLocaleString('zh-CN', { minimumFractionDigits: 2 })}</Text>
        ) : (
          '-'
        ),
    },
    {
      title: t('list.importance'),
      dataIndex: 'importance',
      width: 80,
      hideInSearch: true,
      render: (_, record) =>
        record.importance === 'IMPORTANCE_IMPORTANT' ? <Tag color="red">{importanceName(record.importance)}</Tag> : importanceName(record.importance),
    },
    {
      title: t('list.payMethod'),
      dataIndex: 'payMethod',
      width: 110,
      hideInSearch: true,
      render: (_, record) => payMethodName(record.payMethod),
    },
    {
      title: t('list.feeDirection'),
      dataIndex: 'feeDirection',
      width: 80,
      hideInSearch: true,
      render: (_, record) => feeDirectionName(record.feeDirection),
    },
    { title: t('list.region'), dataIndex: 'region', width: 110, ellipsis: true, hideInSearch: true },
    { title: t('list.dept'), dataIndex: 'dept', width: 120, ellipsis: true, hideInSearch: true },
    {
      title: t('list.period'),
      key: 'period',
      width: 220,
      hideInSearch: true,
      render: (_, record) => (
        <span>
          {record.startDate ? new Date(record.startDate).toLocaleDateString() : '-'} ~{' '}
          {record.endDate ? new Date(record.endDate).toLocaleDateString() : '-'} {expiringTag(record.endDate, record.status)}
        </span>
      ),
    },
    {
      title: t('list.status'),
      dataIndex: 'status',
      width: 100,
      valueType: 'select',
      valueEnum: Object.fromEntries(
        ['STATUS_DRAFT', 'STATUS_REVIEWING', 'STATUS_EFFECTIVE', 'STATUS_CHANGED', 'STATUS_EXPIRED', 'STATUS_TERMINATED', 'STATUS_INVALIDATED'].map(
          (s) => [s, { text: statusName(s) }],
        ),
      ),
      render: (_, record) => <Tag color={statusColor(record.status)}>{statusName(record.status)}</Tag>,
    },
    {
      title: t('list.expiryWindow'),
      dataIndex: 'expiryType',
      width: 140,
      hideInTable: true,
      renderFormItem: () => <Select options={EXPIRY_OPTIONS} allowClear placeholder={t('list.expiryPlaceholder')} />,
    },
    {
      title: t('list.createdAt'),
      dataIndex: 'createdAt',
      width: 150,
      hideInSearch: true,
      render: (_, record) => (record.createdAt ? new Date(record.createdAt).toLocaleString() : '-'),
    },
  ];

  return (
    <ContentContainer>
      <div ref={containerRef} className="page-container-content" style={{ height: '100%' }}>
      <Tabs
        activeKey={activeTab}
        onChange={(key) => {
          setActiveTab(key);
          actionRef.current?.reload();
        }}
        items={[
          { key: 'all', label: t('list.tabAll') },
          ...STATUS_TABS.filter((tab) => tab.key !== 'all').map((tab) => ({
            key: tab.key,
            label: tab.label,
          })),
        ]}
      />
      <ListTable<contractservicev1_Contract>
        headerTitle={t('list.listTitle')}
        actionRef={actionRef}
        rowKey="id"
        scroll={{ x: 1600, y: tableScrollY }}
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
          if (rest.contractNo) {
            delete formValues.contractNo;
            formValues.contractNo__eq = rest.contractNo;
          }
          if (rest.contractTypeId) {
            delete formValues.contractTypeId;
            formValues.contractTypeId__eq = rest.contractTypeId;
          }
          // 到期窗口 → endDate 区间 + 执行中状态（即时计算，不做物化）
          const expiry = formValues.expiryType as string | undefined;
          delete formValues.expiryType;
          if (expiry) {
            formValues.status__in = ['STATUS_EFFECTIVE'];
            if (expiry === 'EXPIRED_OPEN') {
              formValues.endDate__lt = toIsoDay(0);
            } else {
              const days = expiry === 'EXPIRING_30' ? 30 : expiry === 'EXPIRING_90' ? 90 : 180;
              formValues.endDate__gte = toIsoDay(0);
              formValues.endDate__lt = toIsoDay(days);
            }
          }
          const query = new PaginationQuery({
            paging: { page: current || 1, pageSize: pageSize || TABLE_CONST.DEFAULT_PAGE_SIZE },
            formValues,
            orderBy: DEFAULT_ORDER_BY,
          });
          try {
            const response = await fetchListContracts(query);
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
        onRow={(record) => ({
          onClick: () => !isTerminal(record.status) && setDetailId(record.id!),
          style: { cursor: 'pointer' },
        })}
        toolBarRender={() => [
          <Button key="create" type="primary" onClick={() => setCreateOpen(true)}>
            {t('list.create')}
          </Button>,
        ]}
      />
      <CreateContractModal
        open={createOpen}
        onClose={() => {
          setCreateOpen(false);
          actionRef.current?.reload();
        }}
      />
      <ContractDetailDrawer
        id={detailId}
        onClose={() => {
          setDetailId(undefined);
          actionRef.current?.reload();
        }}
      />
      </div>
    </ContentContainer>
  );
};

export default ContractManagement;
