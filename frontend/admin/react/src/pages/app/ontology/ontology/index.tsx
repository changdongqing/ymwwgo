import { useRef, useState } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { App, Button, Modal, Popconfirm, Space, Tag } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { PaginationQuery } from '@/core';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_Ontology } from '@/api/generated/admin/service/v1';
import { fetchListOntologies, useDeleteOntology } from '@/api/hooks/ontology';
import OntologySwitcher from '../components/OntologySwitcher';
import OntologyDrawer from './components/OntologyDrawer';

/**
 * 本体文档管理页（owl:Ontology，标准通用 CRUD，镜像 DictTypeList + DictTypeDrawer）。
 * 删除为级联操作：同事务软删该本体下全部实体/公理/变更日志，二次确认弹窗固定警示。
 */
const OntologyManagement = () => {
  const { t } = useTranslation('ontology');
  const actionRef = useRef<ActionType>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);
  const queryClient = useQueryClient();
  const { message } = App.useApp();

  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerMode, setDrawerMode] = useState<'create' | 'edit'>('create');
  const [editingOntology, setEditingOntology] = useState<ontologyservicev1_Ontology | null>(null);

  const refreshList = () => {
    actionRef.current?.reload();
    queryClient.invalidateQueries({ queryKey: ['listOntologies'] });
    // 本体增删影响切换器简况与各页实体数据
    queryClient.invalidateQueries({ queryKey: ['simpleOntologies'] });
  };

  const deleteMutation = useDeleteOntology({
    onSuccess: () => {
      message.success(t('ontology.deleteSuccess'));
      refreshList();
    },
    onError: (error: Error) => message.error(error.message || t('ontology.deleteFailed')),
  });

  const confirmDelete = (record: ontologyservicev1_Ontology) => {
    Modal.confirm({
      title: t('ontology.deleteConfirmTitle'),
      content: t('ontology.deleteCascadeWarning', { name: record.ontologyName ?? '' }),
      okText: t('common:button.ok'),
      okButtonProps: { danger: true },
      cancelText: t('common:button.cancel'),
      onOk: () => deleteMutation.mutateAsync({ ids: [record.id!] }),
    });
  };

  const columns: ProColumns<ontologyservicev1_Ontology>[] = [
    { title: t('ontology.ontologyName'), dataIndex: 'ontologyName', width: 180 },
    { title: t('ontology.ontologyIri'), dataIndex: 'ontologyIri', width: 260, ellipsis: true, copyable: true, hideInSearch: true },
    { title: t('ontology.baseIri'), dataIndex: 'baseIri', width: 260, ellipsis: true, copyable: true, hideInSearch: true },
    { title: t('ontology.description'), dataIndex: 'description', ellipsis: true, hideInSearch: true },
    { title: t('ontology.version'), dataIndex: 'version', width: 80, hideInSearch: true },
    {
      title: t('ontology.isEnabled'),
      dataIndex: 'isEnabled',
      width: 90,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('ontology.enabled'), value: 'true' },
          { label: t('ontology.disabled'), value: 'false' },
        ],
      },
      render: (_, record) =>
        record.isEnabled ? (
          <Tag color="success">{t('ontology.enabled')}</Tag>
        ) : (
          <Tag color="default">{t('ontology.disabled')}</Tag>
        ),
    },
    { title: t('ontology.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('ontology.action'),
      valueType: 'option',
      width: 120,
      fixed: 'right',
      render: (_, record) => [
        <a
          key="edit"
          onClick={() => {
            setEditingOntology(record);
            setDrawerMode('edit');
            setDrawerOpen(true);
          }}
        >
          {t('common:button.edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('ontology.deleteConfirmTitle')}
          description={t('ontology.deleteConfirmDesc')}
          onConfirm={() => confirmDelete(record)}
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
        <ListTable<ontologyservicev1_Ontology>
          actionRef={actionRef}
          columns={columns}
          title={t('ontology.listTitle')}
          request={async (params) => {
            const { current, pageSize, ...rest } = params;
            const { isEnabled, ...filters } = rest;
            const query = new PaginationQuery({
              paging: { page: current || 1, pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE },
              formValues: {
                ...filters,
                // 下拉给的是字符串，转回布尔走 EQ 精确匹配（铁律 3：bool 列不能 contains）
                isEnabled: isEnabled === 'true' ? true : isEnabled === 'false' ? false : undefined,
              },
            });
            const response = await fetchListOntologies(query);
            return { data: response.items ?? [], total: response.total ?? 0, success: true };
          }}
          rowKey="id"
          search={{
            span: 12,
          }}
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
                icon={<PlusOutlined />}
                onClick={() => {
                  setEditingOntology(null);
                  setDrawerMode('create');
                  setDrawerOpen(true);
                }}
              >
                {t('ontology.create')}
              </Button>
            </Space>
          }
          options={{ density: true, fullScreen: true, setting: true, reload: true }}
          size="middle"
          bordered
          scroll={{ y: tableScrollY, x: 1100 }}
        />
      </div>

      <OntologyDrawer
        open={drawerOpen}
        mode={drawerMode}
        data={editingOntology}
        onClose={() => {
          setDrawerOpen(false);
          setEditingOntology(null);
        }}
        onSuccess={refreshList}
      />
    </ContentContainer>
  );
};

export default OntologyManagement;
