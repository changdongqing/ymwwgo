import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { App, Button, Popconfirm, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { type onestopservicev1_TransportRoute } from '@/api/generated/admin/service/v1';
import { useDeleteTransportRoute, useListTransportRoutes, invalidateTransportRoutes } from '@/api/hooks/onestop';
import { useQueryClient } from '@tanstack/react-query';
import TransportRouteDrawer from './TransportRouteDrawer';

interface TransportRoutePanelProps {
  serviceTypeId: number;
  serviceTypeLabel: string;
}

/**
 * 常用运送路线列表面板：嵌入运送类别（0106 子树）编辑页签底部，
 * 管理该类别下的路线（一个类别可挂多条起终点/班次不同的路线）。
 */
export default function TransportRoutePanel({ serviceTypeId, serviceTypeLabel }: TransportRoutePanelProps) {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editing, setEditing] = useState<onestopservicev1_TransportRoute | undefined>(undefined);

  const routesQuery = useListTransportRoutes(serviceTypeId);

  const deleteMutation = useDeleteTransportRoute({
    onSuccess: () => {
      message.success(t('route.deleteSuccess'));
      invalidateTransportRoutes(queryClient);
    },
    onError: (error: Error) => message.error(error.message || t('route.deleteFailed')),
  });

  const columns: ColumnsType<onestopservicev1_TransportRoute> = [
    { title: t('route.name'), dataIndex: 'name', ellipsis: true, width: 200 },
    {
      title: t('route.path'),
      key: 'path',
      ellipsis: true,
      render: (_, record) => (
        <>
          {record.origin || '-'} → {record.destination || '-'}
          {record.roundTrip ? <Tag style={{ marginLeft: 8 }}>{t('route.roundTripTag')}</Tag> : null}
        </>
      ),
    },
    {
      title: t('route.estDuration'),
      dataIndex: 'estDurationMinutes',
      width: 110,
      render: (v) => (v != null ? `${v} ${t('minutesUnit')}` : '-'),
    },
    {
      title: t('route.departureTimes'),
      key: 'departureTimes',
      width: 220,
      render: (_, record) =>
        record.departureTimes && record.departureTimes.length > 0
          ? record.departureTimes.join('、')
          : t('route.onDemand'),
    },
    {
      title: t('route.isEnabled'),
      dataIndex: 'isEnabled',
      width: 90,
      render: (_, record) =>
        record.isEnabled ? <Tag color="success">{t('enabled')}</Tag> : <Tag color="default">{t('disabled')}</Tag>,
    },
    {
      title: t('action'),
      key: 'action',
      width: 110,
      render: (_, record) => [
        <a
          key="edit"
          onClick={() => {
            setEditing(record);
            setDrawerOpen(true);
          }}
        >
          {t('edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('route.deleteConfirmTitle')}
          onConfirm={() => deleteMutation.mutate({ id: record.id! })}
        >
          <a style={{ color: 'var(--ant-color-error)' }}>{t('delete')}</a>
        </Popconfirm>,
      ],
    },
  ];

  return (
    <div style={{ marginTop: 8 }}>
      <Table<onestopservicev1_TransportRoute>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={routesQuery.data?.items ?? []}
        loading={routesQuery.isLoading}
        pagination={false}
        title={() => (
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>{t('route.panelTitle')}</span>
            <Button
              type="primary"
              size="small"
              onClick={() => {
                setEditing(undefined);
                setDrawerOpen(true);
              }}
            >
              {t('route.create')}
            </Button>
          </div>
        )}
        locale={{ emptyText: t('route.empty') }}
      />

      <TransportRouteDrawer
        open={drawerOpen}
        mode={editing ? 'edit' : 'create'}
        serviceTypeId={serviceTypeId}
        serviceTypeLabel={serviceTypeLabel}
        route={editing}
        onSaved={() => setDrawerOpen(false)}
        onClose={() => setDrawerOpen(false)}
      />
    </div>
  );
}
