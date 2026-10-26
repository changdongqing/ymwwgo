import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { App, Button, Descriptions, Drawer, Input, Modal, Popconfirm, Space, Table, Tag, Timeline, Typography } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import type { contractservicev1_ContractFee } from '@/api/generated/admin/service/v1';
import { invalidateContracts, useContractAction, useDeleteContract, useGetContract, useListContractEvents, useListContractFees } from '@/api/hooks/contract';
import {
  eventActionName,
  expiringTag,
  feeCategoryName,
  feeDirectionName,
  feeStatusColor,
  feeStatusName,
  importanceName,
  partyBKindName,
  payMethodName,
  statusColor,
  statusName,
} from './constants';

const { Text } = Typography;

interface DetailDrawerProps {
  id?: number;
  onClose: () => void;
}

type PendingAction =
  | { kind: 'submit' }
  | { kind: 'approve'; passed: boolean }
  | { kind: 'change' }
  | { kind: 'terminate' }
  | { kind: 'invalidate' }
  | { kind: 'decompose' };

const fmtDate = (v?: string | null) => (v ? dayjs(v).format('YYYY-MM-DD') : '-');
const fmtDateTime = (v?: string | null) => (v ? dayjs(v).format('YYYY-MM-DD HH:mm') : '-');
const fmtAmount = (v?: number | null) =>
  v != null ? Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2 }) : '-';

/**
 * 合同详情抽屉：信息四段（甲方/乙方/基本/差异字段）+ 附件 + 费用明细
 * （收/付/退款/重新拆解）+ 事件时间线 + 按状态动作矩阵。
 */
const ContractDetailDrawer = ({ id, onClose }: DetailDrawerProps) => {
  const { t } = useTranslation('contract');
  const { message } = App.useApp();
  const queryClient = useQueryClient();

  const [pending, setPending] = useState<PendingAction>();
  const [actionNote, setActionNote] = useState('');

  const { data: contract } = useGetContract(id ? { id } : { id: 0 }, { enabled: id != null });
  const { data: events } = useListContractEvents(id, { enabled: id != null });
  const { data: feesRes } = useListContractFees(id, { enabled: id != null });
  const fees = feesRes?.items ?? [];

  const afterAction = () => {
    invalidateContracts(queryClient, id);
  };

  const actionMutation = useContractAction('Submit', {
    onSuccess: () => {
      message.success(t('detail.actionSuccess'));
      setPending(undefined);
      setActionNote('');
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });

  const deleteMutation = useDeleteContract({
    onSuccess: () => {
      message.success(t('detail.actionSuccess'));
      onClose();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });

  const runAction = () => {
    if (!id || !pending) return;
    switch (pending.kind) {
      case 'submit':
        actionMutation.mutate({ id, note: actionNote || undefined } as never);
        break;
      case 'approve':
        // approve 复用 Submit 入口生成的 mutation 会串方法，这里直接按类型分发
        approveDispatch(pending.passed);
        break;
      case 'change':
        changeDispatch();
        break;
      case 'terminate':
        terminateDispatch();
        break;
      case 'invalidate':
        invalidateDispatch();
        break;
      case 'decompose':
        decomposeDispatch();
        break;
    }
  };

  // 各动作独立 mutation（生成器按动作名参数化）
  const approveMutation = useContractAction('Approve', {
    onSuccess: () => {
      message.success(t('detail.actionSuccess'));
      setPending(undefined);
      setActionNote('');
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const changeMutation = useContractAction('Change', {
    onSuccess: () => {
      message.success(t('detail.changeSuccess'));
      setPending(undefined);
      setActionNote('');
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const terminateMutation = useContractAction('Terminate', {
    onSuccess: () => {
      message.success(t('detail.actionSuccess'));
      setPending(undefined);
      setActionNote('');
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const invalidateMutation = useContractAction('Invalidate', {
    onSuccess: () => {
      message.success(t('detail.actionSuccess'));
      setPending(undefined);
      setActionNote('');
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const decomposeMutation = useContractAction('DecomposeFees', {
    onSuccess: () => {
      message.success(t('detail.decomposeSuccess'));
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const payMutation = useContractAction('PayFee', {
    onSuccess: () => {
      message.success(t('detail.feeActionSuccess'));
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const receiveMutation = useContractAction('ReceiveFee', {
    onSuccess: () => {
      message.success(t('detail.feeActionSuccess'));
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });
  const refundMutation = useContractAction('RefundFee', {
    onSuccess: () => {
      message.success(t('detail.feeActionSuccess'));
      afterAction();
    },
    onError: (error: Error) => message.error(error.message || t('detail.actionFailed')),
  });

  const approveDispatch = (passed: boolean) => {
    if (!id) return;
    if (!passed && !actionNote.trim()) {
      message.warning(t('detail.rejectReasonRequired'));
      return;
    }
    approveMutation.mutate({ id, passed, note: actionNote || undefined });
  };
  const changeDispatch = () => {
    if (!id) return;
    changeMutation.mutate({ id, note: actionNote || undefined });
  };
  const terminateDispatch = () => {
    if (!id) return;
    if (!actionNote.trim()) {
      message.warning(t('detail.reasonRequired'));
      return;
    }
    terminateMutation.mutate({ id, reason: actionNote });
  };
  const invalidateDispatch = () => {
    if (!id) return;
    if (!actionNote.trim()) {
      message.warning(t('detail.reasonRequired'));
      return;
    }
    invalidateMutation.mutate({ id, reason: actionNote });
  };
  const decomposeDispatch = () => {
    if (!id) return;
    decomposeMutation.mutate({ id, note: actionNote || undefined });
  };

  if (!contract) {
    return (
      <Drawer open={id != null} onClose={onClose} width={720} title={t('detail.title')}>
        {t('detail.loading')}
      </Drawer>
    );
  }

  const status = contract.status ?? 'STATUS_DRAFT';

  // 按状态动作矩阵（docs/connn/contract-type-and-contract-design.md §4.1）
  const actionButtons = (
    <Space wrap>
      {status === 'STATUS_DRAFT' && (
        <>
          <Button type="primary" size="small" onClick={() => setPending({ kind: 'submit' })}>
            {t('detail.submit')}
          </Button>
          <Button size="small" danger onClick={() => setPending({ kind: 'invalidate' })}>
            {t('detail.invalidate')}
          </Button>
          <Popconfirm title={t('detail.deleteConfirm')} onConfirm={() => deleteMutation.mutate({ id: contract.id! })}>
            <Button size="small" danger>
              {t('detail.delete')}
            </Button>
          </Popconfirm>
        </>
      )}
      {status === 'STATUS_REVIEWING' && (
        <>
          <Button type="primary" size="small" onClick={() => setPending({ kind: 'approve', passed: true })}>
            {t('detail.approvePass')}
          </Button>
          <Button size="small" danger onClick={() => setPending({ kind: 'approve', passed: false })}>
            {t('detail.approveReject')}
          </Button>
          <Button size="small" danger onClick={() => setPending({ kind: 'invalidate' })}>
            {t('detail.invalidate')}
          </Button>
        </>
      )}
      {status === 'STATUS_EFFECTIVE' && (
        <>
          <Button size="small" onClick={() => setPending({ kind: 'change' })}>
            {t('detail.change')}
          </Button>
          <Button size="small" onClick={() => setPending({ kind: 'decompose' })}>
            {t('detail.decompose')}
          </Button>
          <Button size="small" danger onClick={() => setPending({ kind: 'terminate' })}>
            {t('detail.terminate')}
          </Button>
          <Button size="small" danger onClick={() => setPending({ kind: 'invalidate' })}>
            {t('detail.invalidate')}
          </Button>
        </>
      )}
    </Space>
  );

  const feeColumns = [
    { title: t('detail.feeName'), dataIndex: 'name', width: 130 },
    {
      title: t('detail.feeCategory'),
      dataIndex: 'category',
      width: 80,
      render: (_: unknown, record: contractservicev1_ContractFee) => feeCategoryName(record.category),
    },
    {
      title: t('detail.feeDirection'),
      dataIndex: 'direction',
      width: 70,
      render: (_: unknown, record: contractservicev1_ContractFee) => feeDirectionName(record.direction),
    },
    {
      title: t('detail.feeStatus'),
      dataIndex: 'status',
      width: 110,
      render: (_: unknown, record: contractservicev1_ContractFee) => (
        <Tag color={feeStatusColor(record.status)}>
          {feeStatusName(record.status, record.direction)}
          {record.category === 'CATEGORY_DEPOSIT' && record.status === 'FEE_STATUS_DONE' ? '（待退款）' : ''}
        </Tag>
      ),
    },
    {
      title: t('detail.feeAmount'),
      dataIndex: 'amount',
      width: 110,
      align: 'right' as const,
      render: (_: unknown, record: contractservicev1_ContractFee) => fmtAmount(record.amount),
    },
    {
      title: t('detail.feePlanDate'),
      dataIndex: 'planDate',
      width: 100,
      render: (_: unknown, record: contractservicev1_ContractFee) => fmtDate(record.planDate),
    },
    {
      title: t('detail.feeActualDate'),
      dataIndex: 'actualDate',
      width: 100,
      render: (_: unknown, record: contractservicev1_ContractFee) => fmtDate(record.actualDate),
    },
    {
      title: t('detail.action'),
      key: 'action',
      width: 90,
      render: (_: unknown, record: contractservicev1_ContractFee) => {
        if (status !== 'STATUS_EFFECTIVE') return null;
        if (record.status === 'FEE_STATUS_PENDING') {
          return record.direction === 'DIRECTION_PAYABLE' ? (
            <a onClick={() => payMutation.mutate({ id: record.id! })}>{t('detail.feePay')}</a>
          ) : (
            <a onClick={() => receiveMutation.mutate({ id: record.id! })}>{t('detail.feeReceive')}</a>
          );
        }
        if (
          record.category === 'CATEGORY_DEPOSIT' &&
          record.status === 'FEE_STATUS_DONE'
        ) {
          return (
            <a onClick={() => refundMutation.mutate({ id: record.id! })} style={{ color: 'var(--ant-color-error)' }}>
              {t('detail.feeRefund')}
            </a>
          );
        }
        return null;
      },
    },
  ];

  const diffEntries = (contract.diffSchema ?? []).map((f) => ({
    label: f?.label ?? f?.key,
    value: contract.diffValues?.[f?.key ?? ''] ?? '-',
  }));

  const modalTitleKey: Record<PendingAction['kind'], string> = {
    submit: 'detail.submit',
    approve: 'detail.approveTitle',
    change: 'detail.changeTitle',
    terminate: 'detail.terminate',
    invalidate: 'detail.invalidate',
    decompose: 'detail.decompose',
  };
  const noteRequired =
    pending?.kind === 'terminate' ||
    pending?.kind === 'invalidate' ||
    (pending?.kind === 'approve' && pending.passed === false);

  return (
    <Drawer open={id != null} onClose={onClose} width={760} title={t('detail.title')}>
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Space wrap>
          <Text strong copyable>
            {contract.contractNo}
          </Text>
          <Tag color={statusColor(status)}>{statusName(status)}</Tag>
          <Tag>v{contract.version ?? 1}</Tag>
          <Tag color={contract.importance === 'IMPORTANCE_IMPORTANT' ? 'red' : 'default'}>
            {importanceName(contract.importance)}
          </Tag>
          <Tag color="blue">{contract.contractTypeName}</Tag>
          {expiringTag(contract.endDate, status)}
        </Space>
        {actionButtons}

        <Descriptions title={t('detail.partyASection')} size="small" bordered column={2}>
          <Descriptions.Item label={t('list.region')}>{contract.region ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.dept')}>{contract.dept ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyAContact')}>{contract.partyAContact ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyAPhone')}>{contract.partyAPhone ?? '-'}</Descriptions.Item>
        </Descriptions>

        <Descriptions title={t('detail.partyBSection')} size="small" bordered column={2}>
          <Descriptions.Item label={t('list.partyBName')}>{contract.partyBName ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBCredit')}>{contract.partyBCredit ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBKind')}>{partyBKindName(contract.partyBKind)}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBLegal')}>{contract.partyBLegal ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBContact')}>{contract.partyBContact ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBPhone')}>{contract.partyBPhone ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBBank')}>{contract.partyBBank ?? '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.partyBAccount')}>{contract.partyBAccount ?? '-'}</Descriptions.Item>
        </Descriptions>

        <Descriptions title={t('detail.basicSection')} size="small" bordered column={2}>
          <Descriptions.Item label={t('list.amount')}>
            <Text strong>{fmtAmount(contract.amount)}</Text>
          </Descriptions.Item>
          <Descriptions.Item label={t('list.taxRate')}>{contract.taxRate != null ? `${contract.taxRate}%` : '-'}</Descriptions.Item>
          <Descriptions.Item label={t('list.payMethod')}>{payMethodName(contract.payMethod)}</Descriptions.Item>
          <Descriptions.Item label={t('list.feeDirection')}>{feeDirectionName(contract.feeDirection)}</Descriptions.Item>
          <Descriptions.Item label={t('list.deposit')}>{fmtAmount(contract.deposit)}</Descriptions.Item>
          <Descriptions.Item label={t('list.period')}>
            {fmtDate(contract.startDate)} ~ {fmtDate(contract.endDate)}
          </Descriptions.Item>
          {(contract.payNodes ?? []).length > 0 && (
            <Descriptions.Item label={t('detail.payNodes')} span={2}>
              {(contract.payNodes ?? []).map((n) => `${n?.name} ${n?.ratioPercent}%`).join(' / ')}
            </Descriptions.Item>
          )}
          <Descriptions.Item label={t('detail.reviewInfo')} span={2}>
            {contract.reviewerId
              ? `${t('detail.reviewer')} #${contract.reviewerId} · ${fmtDateTime(contract.reviewedAt)}${
                  contract.reviewNote ? ` · ${contract.reviewNote}` : ''
                }`
              : '-'}
          </Descriptions.Item>
          {contract.terminateReason && (
            <Descriptions.Item label={t('detail.closeReason')} span={2}>
              {contract.terminateReason}
            </Descriptions.Item>
          )}
          {contract.remark && (
            <Descriptions.Item label={t('list.remark')} span={2}>
              {contract.remark}
            </Descriptions.Item>
          )}
        </Descriptions>

        {diffEntries.length > 0 && (
          <Descriptions title={t('detail.diffSection', { type: contract.contractTypeName ?? '' })} size="small" bordered column={2}>
            {diffEntries.map((entry) => (
              <Descriptions.Item key={entry.label} label={entry.label}>
                {entry.value}
              </Descriptions.Item>
            ))}
          </Descriptions>
        )}

        {(contract.attachments ?? []).length > 0 && (
          <div>
            <Text strong>{t('list.attachments')}</Text>
            <ul style={{ margin: '8px 0', paddingLeft: 20 }}>
              {(contract.attachments ?? []).map((url) => (
                <li key={url}>
                  <a href={url} target="_blank" rel="noreferrer">
                    {url.split('/').pop() ?? url}
                  </a>
                </li>
              ))}
            </ul>
          </div>
        )}

        <div>
          <Text strong>{t('detail.feeSection')}</Text>
          <Table
            size="small"
            rowKey="id"
            columns={feeColumns}
            dataSource={fees}
            pagination={false}
            style={{ marginTop: 8 }}
          />
        </div>

        <div>
          <Text strong>{t('detail.eventSection')}</Text>
          <Timeline
            style={{ marginTop: 12 }}
            items={(events?.items ?? []).map((event) => ({
              children: (
                <div>
                  <Space>
                    <Text strong>{eventActionName(event.action)}</Text>
                    <Text type="secondary">{fmtDateTime(event.createdAt)}</Text>
                    {event.statusFrom && event.statusTo && (
                      <Text type="secondary">
                        {statusName(event.statusFrom)} → {statusName(event.statusTo)}
                      </Text>
                    )}
                  </Space>
                  {event.note && (
                    <div>
                      <Text type="secondary">{event.note}</Text>
                    </div>
                  )}
                </div>
              ),
            }))}
          />
        </div>
      </Space>

      <Modal
        title={t(modalTitleKey[pending?.kind ?? 'submit'])}
        open={pending != null}
        onCancel={() => {
          setPending(undefined);
          setActionNote('');
        }}
        onOk={runAction}
        confirmLoading={
          actionMutation.isPending ||
          approveMutation.isPending ||
          changeMutation.isPending ||
          terminateMutation.isPending ||
          invalidateMutation.isPending ||
          decomposeMutation.isPending
        }
        destroyOnHidden
      >
        <Input.TextArea
          rows={3}
          maxLength={255}
          value={actionNote}
          onChange={(e) => setActionNote(e.target.value)}
          placeholder={noteRequired ? t('detail.reasonRequiredPlaceholder') : t('detail.notePlaceholder')}
        />
      </Modal>
    </Drawer>
  );
};

export default ContractDetailDrawer;
