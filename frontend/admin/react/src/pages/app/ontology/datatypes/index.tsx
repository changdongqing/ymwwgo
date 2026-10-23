import { useEffect, useRef, useState } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { App, Button, Modal, Popconfirm, Space, Table, Tag, Tooltip, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_Datatype, ontologyservicev1_EntityUsage } from '@/api/generated/admin/service/v1';
import {
  fetchListDatatypes,
  getOntologyErrorReason,
  useDatatypeUsage,
  useDeleteDatatype,
} from '@/api/hooks/ontologyDatatype';
import { useOntologyStore } from '@/stores/ontology';
import { datatypeShortName } from '../constants';
import OntologySwitcher from '../components/OntologySwitcher';
import DatatypeDrawer from './components/DatatypeDrawer';
import './index.style.css';

/** 引用类型 → 展示标签（datatype usage：仅 42 派生值域引用——datatype 无其余入边公理） */
const USAGE_TYPE_LABEL_KEYS: Record<number, string> = {
  42: 'datatype.usageType.rangeReference',
};

/**
 * 数据类型管理页（05 期，系列首个无左树实体页——OWL 2 无 subDatatypeOf 层级，
 * Protégé OWLDataTypeViewComponent 同构平铺列表；05 号文档 §6.1）。
 * 删除流镜像 04 期：force=false 被引用即拒（usage 弹窗呈现引用清单）→ force=true 级联。
 */
const OntologyDatatypes = () => {
  const { t } = useTranslation('ontology');
  const actionRef = useRef<ActionType>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);
  const queryClient = useQueryClient();
  const { message, modal } = App.useApp();

  const currentOntologyId = useOntologyStore((s) => s.currentOntologyId);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [editingDatatype, setEditingDatatype] = useState<ontologyservicev1_Datatype | null>(null);
  const [pendingDelete, setPendingDelete] = useState<ontologyservicev1_Datatype | null>(null);

  // usage 明细（删除保护弹窗数据源；pendingDelete 挂起时拉取）
  const usageQuery = useDatatypeUsage({ id: pendingDelete?.id ?? 0 }, { enabled: pendingDelete != null });

  const refreshAll = () => {
    actionRef.current?.reload();
    queryClient.invalidateQueries({ queryKey: ['listDatatypes'] });
    queryClient.invalidateQueries({ queryKey: ['simpleDatatypes'] });
  };

  const deleteMutation = useDeleteDatatype({
    onSuccess: () => {
      message.success(t('datatype.deleteSuccess'));
      setPendingDelete(null);
      refreshAll();
    },
    onError: (error: Error) => {
      // 铁律 1：不吞错——版本冲突引导刷新重试；ENTITY_IN_USE 由 usage 弹窗呈现；
      // 其余（无引用明细可展示时）走通用报错
      if (getOntologyErrorReason(error) === 'ONTOLOGY_VERSION_CONFLICT') {
        modal.confirm({
          title: t('datatypeErrors.versionConflictTitle'),
          content: error.message || t('datatypeErrors.versionConflict'),
          okText: t('common:button.retry'),
          cancelText: t('common:button.cancel'),
          onOk: refreshAll,
        });
      } else if ((usageQuery.data?.refs?.length ?? 0) === 0) {
        message.error(error.message || t('datatype.deleteFailed'));
      }
    },
  });

  // 删除流：挂起 → force=false 试删 → 命中引用自动弹 usage 明细 → 确认后 force 级联
  useEffect(() => {
    if (pendingDelete == null) return;
    deleteMutation.mutate({ id: pendingDelete.id!, force: false });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingDelete]);

  const forceDelete = () => {
    if (pendingDelete == null) return;
    deleteMutation.mutate({
      id: pendingDelete.id!,
      force: true, // 红字警示已由弹窗呈现：将同时移除 N 处值域引用
    });
  };

  const usage: ontologyservicev1_EntityUsage | undefined = usageQuery.data;
  const usageVisible = pendingDelete != null && (usage?.refs?.length ?? 0) > 0;

  const usageColumns = [
    { title: t('datatype.usageRef.localName'), dataIndex: 'localName', width: 160 },
    { title: t('datatype.usageRef.displayLabel'), dataIndex: 'displayLabel', width: 160 },
    {
      title: t('datatype.usageRef.axiomType'),
      dataIndex: 'axiomType',
      width: 140,
      render: (v: number) => t(USAGE_TYPE_LABEL_KEYS[v] ?? 'datatype.usageType.other'),
    },
  ];

  const columns: ProColumns<ontologyservicev1_Datatype>[] = [
    {
      title: t('datatype.localName'),
      dataIndex: 'localName',
      width: 160,
      sorter: true,
      render: (_, record) => (
        <Typography.Text code delete={record.deprecated}>
          {record.localName}
        </Typography.Text>
      ),
    },
    {
      title: t('datatype.displayLabel'),
      dataIndex: 'displayLabel',
      width: 160,
      render: (_, record) => (
        <span style={{ textDecoration: record.deprecated ? 'line-through' : undefined, color: record.deprecated ? 'var(--ant-color-text-tertiary)' : undefined }}>
          {record.displayLabel || record.localName}
        </span>
      ),
    },
    {
      title: t('datatype.definition'),
      dataIndex: 'definitionSummary',
      hideInSearch: true,
      width: 260,
      render: (_, record) =>
        record.definitionSummary ? (
          <Tooltip title={record.definitionSummary} placement="topLeft">
            <Tag color="purple" style={{ maxWidth: '100%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {record.definitionSummary}
            </Tag>
          </Tooltip>
        ) : (
          <Tag>{t('datatype.undefined')}</Tag>
        ),
    },
    {
      title: t('datatype.referenceCount'),
      dataIndex: 'referenceCount',
      hideInSearch: true,
      width: 100,
      render: (_, record) =>
        (record.referenceCount ?? 0) > 0 ? (
          <a onClick={() => setPendingDelete(record)}>{record.referenceCount}</a>
        ) : (
          <span style={{ color: 'var(--ant-color-text-tertiary)' }}>0</span>
        ),
    },
    {
      title: t('datatype.baseIri'),
      dataIndex: 'baseIri',
      hideInSearch: true,
      width: 150,
      render: (_, record) => (record.baseIri ? <Typography.Text code>{datatypeShortName(record.baseIri)}</Typography.Text> : '-'),
    },
    { title: 'IRI', dataIndex: 'iri', hideInSearch: true, width: 280, ellipsis: true, copyable: true },
    { title: t('datatype.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('datatype.action'),
      valueType: 'option',
      width: 120,
      fixed: 'right',
      render: (_, record) => [
        <a
          key="edit"
          onClick={() => {
            setEditingDatatype(record);
            setDrawerOpen(true);
          }}
        >
          {t('common:button.edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('datatype.deleteConfirmTitle')}
          onConfirm={() => setPendingDelete(record)}
          okText={t('common:button.ok')}
          cancelText={t('common:button.cancel')}
        >
          <a style={{ color: 'var(--ant-color-error)' }}>{t('common:button.delete')}</a>
        </Popconfirm>,
      ],
    },
  ];

  return (
    <ContentContainer heightMode="fixed" padding="16px" bottomMargin={0}>
      <div ref={containerRef} className="page-container-content">
        <ListTable<ontologyservicev1_Datatype>
          actionRef={actionRef}
          columns={columns}
          title={t('datatype.listTitle')}
          request={async (params) => {
            if (currentOntologyId == null) {
              return { data: [], total: 0, success: true };
            }
            const { current, pageSize, localName, displayLabel, deprecated, sortField, sortDirection, ...rest } = params;
            void rest;
            const orderBy: string[] = [];
            if (sortField === 'localName') {
              orderBy.push(sortDirection === 'desc' ? '-local_name' : 'local_name');
            } else {
              orderBy.push('-created_at');
            }
            const response = await fetchListDatatypes({
              ontologyId: currentOntologyId,
              page: current || 1,
              pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE,
              localName: localName || undefined,
              displayLabel: displayLabel || undefined,
              deprecated: deprecated === 'true' ? true : deprecated === 'false' ? false : undefined,
              orderBy: JSON.stringify(orderBy),
            });
            return { data: response.items ?? [], total: response.total ?? 0, success: true };
          }}
          rowKey="id"
          search={{ span: 8 }}
          pagination={{
            defaultPageSize: TABLE.DEFAULT_PAGE_SIZE,
            showSizeChanger: true,
            showQuickJumper: true,
          }}
          actions={
            <Space>
              <OntologySwitcher />
              <Button
                type="primary"
                size="small"
                icon={<PlusOutlined />}
                disabled={currentOntologyId == null}
                onClick={() => {
                  setEditingDatatype(null);
                  setDrawerOpen(true);
                }}
              >
                {t('datatype.create')}
              </Button>
            </Space>
          }
          options={{ density: true, fullScreen: false, setting: true, reload: true }}
          size="small"
          bordered
          scroll={{ y: tableScrollY, x: 1200 }}
          locale={{ emptyText: currentOntologyId == null ? t('datatype.noOntologySelected') : undefined }}
        />
      </div>

      <DatatypeDrawer
        open={drawerOpen}
        ontologyId={currentOntologyId}
        datatypeId={editingDatatype?.id ?? null}
        onClose={() => {
          setDrawerOpen(false);
          setEditingDatatype(null);
        }}
        onSuccess={refreshAll}
        onVersionConflict={refreshAll}
        onOpenDatatype={(id) => setEditingDatatype({ id } as ontologyservicev1_Datatype)}
      />

      <Modal
        open={usageVisible}
        title={t('datatype.usageModalTitle', {
          name: pendingDelete?.displayLabel || pendingDelete?.localName || `#${pendingDelete?.id ?? ''}`,
        })}
        onCancel={() => setPendingDelete(null)}
        footer={[
          <Button key="cancel" onClick={() => setPendingDelete(null)}>
            {t('common:button.cancel')}
          </Button>,
          <Button key="force" type="primary" danger onClick={forceDelete}>
            {t('datatype.forceDelete')}
          </Button>,
        ]}
      >
        <p className="ontology-usage-warning">
          {t('datatype.usageWarning', { count: usage?.refs?.length ?? 0 })}
        </p>
        <Table
          size="small"
          rowKey={(r) => `${r.entityId}-${r.axiomType}`}
          columns={usageColumns}
          dataSource={usage?.refs ?? []}
          loading={usageQuery.isLoading}
          pagination={false}
        />
      </Modal>
    </ContentContainer>
  );
};

export default OntologyDatatypes;
