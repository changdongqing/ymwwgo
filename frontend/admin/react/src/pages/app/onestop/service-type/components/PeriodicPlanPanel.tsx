import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { App, Button, Popconfirm, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  type onestopservicev1_PeriodicPlan,
} from '@/api/generated/admin/service/v1';
import { useDeletePeriodicPlan, useListPeriodicPlans, invalidatePeriodicPlans } from '@/api/hooks/onestop';
import { useQueryClient } from '@tanstack/react-query';
import { summarizePlanFreq } from '../constants';
import PeriodicPlanDrawer from './PeriodicPlanDrawer';

interface PeriodicPlanPanelProps {
  serviceTypeId: number;
  serviceTypeLabel: string;
  /** 类别标准作业项目库（新建计划时点表的建议来源） */
  standardItems: string[];
}

/**
 * 周期计划列表面板：嵌入类别（0103 子树）编辑页签底部，
 * 管理该类别下的计划（一个类别可挂多套周期不同的计划）。
 */
export default function PeriodicPlanPanel({ serviceTypeId, serviceTypeLabel, standardItems }: PeriodicPlanPanelProps) {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editing, setEditing] = useState<onestopservicev1_PeriodicPlan | undefined>(undefined);

  const plansQuery = useListPeriodicPlans(serviceTypeId);

  const deleteMutation = useDeletePeriodicPlan({
    onSuccess: () => {
      message.success(t('plan.deleteSuccess'));
      invalidatePeriodicPlans(queryClient);
    },
    onError: (error: Error) => message.error(error.message || t('plan.deleteFailed')),
  });

  const columns: ColumnsType<onestopservicev1_PeriodicPlan> = [
    { title: t('plan.name'), dataIndex: 'name', ellipsis: true },
    {
      title: t('plan.freqSummary'),
      key: 'freq',
      width: 340,
      render: (_, record) => summarizePlanFreq(record),
    },
    { title: t('plan.post'), dataIndex: 'post', width: 120, render: (v) => v || '-' },
    {
      title: t('plan.itemsCount'),
      key: 'items',
      width: 120,
      render: (_, record) => {
        const total = (record.spaceItems?.length ?? 0) + (record.equipmentItems?.length ?? 0);
        const disabled =
          (record.spaceItems ?? []).filter((i) => !i.enabled).length +
          (record.equipmentItems ?? []).filter((i) => !i.enabled).length;
        return disabled > 0 ? `${total}（${t('plan.disabledCount', { count: disabled })}）` : `${total}`;
      },
    },
    {
      title: t('plan.range'),
      key: 'range',
      width: 220,
      render: (_, record) => {
        const from = record.effectiveFrom ? record.effectiveFrom.slice(0, 10) : '';
        const to = record.effectiveTo ? record.effectiveTo.slice(0, 10) : '';
        if (!from && !to) return t('plan.rangeAlways');
        return `${from || '…'} ~ ${to || '…'}`;
      },
    },
    {
      title: t('plan.isEnabled'),
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
          title={t('plan.deleteConfirmTitle')}
          onConfirm={() => deleteMutation.mutate({ id: record.id! })}
        >
          <a style={{ color: 'var(--ant-color-error)' }}>{t('delete')}</a>
        </Popconfirm>,
      ],
    },
  ];

  return (
    <div style={{ marginTop: 8 }}>
      <Table<onestopservicev1_PeriodicPlan>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={plansQuery.data?.items ?? []}
        loading={plansQuery.isLoading}
        pagination={false}
        title={() => (
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>{t('plan.panelTitle')}</span>
            <Button
              type="primary"
              size="small"
              onClick={() => {
                setEditing(undefined);
                setDrawerOpen(true);
              }}
            >
              {t('plan.create')}
            </Button>
          </div>
        )}
        locale={{ emptyText: t('plan.empty') }}
      />

      <PeriodicPlanDrawer
        open={drawerOpen}
        mode={editing ? 'edit' : 'create'}
        serviceTypeId={serviceTypeId}
        serviceTypeLabel={serviceTypeLabel}
        plan={editing}
        standardItems={standardItems}
        onSaved={() => setDrawerOpen(false)}
        onClose={() => setDrawerOpen(false)}
      />
    </div>
  );
}
