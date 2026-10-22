import { useCallback, useEffect, useMemo, useState } from 'react';
import { App, Button, Modal, Space, Splitter, Table, Tag, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useOntologyStore } from '@/stores';
import type {
  ontologyservicev1_EntityUsage,
  ontologyservicev1_Individual,
  ontologyservicev1_UpdateIndividualRequest,
} from '@/api/generated/admin/service/v1';
import {
  apiClient,
} from '@/api/client';
import {
  getOntologyErrorReason,
  useDeleteIndividual,
  useIndividualUsage,
  useUpdateIndividual,
} from '@/api/hooks/ontologyIndividual';

import OntologySwitcher from '../components/OntologySwitcher';
import IndividualClassTree, { type ClassTreeNodeAction } from './components/IndividualClassTree';
import IndividualListTable from './components/IndividualListTable';
import IndividualDrawer from './components/IndividualDrawer';
import './index.style.css';

interface DrawerState {
  open: boolean;
  individualId: number | null;
}

/**
 * 个体管理页（06 期，左类树 + 右实例列表，对照 Protégé "Individuals by class"）：
 * 左 IndividualClassTree（只读导航 + 直接实例数徽标），右 IndividualListTable（classId 联动过滤）。
 * 点击树节点 = 按 class_id 过滤右表（含子类实例开关在右表搜索区）。
 */
const OntologyIndividualManagement = () => {
  const { t } = useTranslation('ontology');
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();

  const currentOntologyId = useOntologyStore((s) => s.currentOntologyId);

  const [selected, setSelected] = useState<ClassTreeNodeAction | null>(null);
  const [drawer, setDrawer] = useState<DrawerState>({ open: false, individualId: null });

  // 删除保护流程：先 force=false 尝试；ENTITY_IN_USE 时 usage 明细弹窗 → 确认后 force 重删
  const [pendingDelete, setPendingDelete] = useState<ontologyservicev1_Individual | null>(null);
  const usageQuery = useIndividualUsage(
    { id: pendingDelete?.id ?? 0 },
    { enabled: pendingDelete != null },
  );

  const refreshAll = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['listIndividuals'] });
    queryClient.invalidateQueries({ queryKey: ['individualFrame'] });
    queryClient.invalidateQueries({ queryKey: ['individualsCountByClass'] });
  }, [queryClient]);

  // 切换本体后复位树选区
  useEffect(() => {
    setSelected(null);
  }, [currentOntologyId]);

  const openCreate = () => {
    if (currentOntologyId == null) {
      message.warning(t('individual.noOntologySelected'));
      return;
    }
    setDrawer({ open: true, individualId: null });
  };

  const openEdit = (id: number) => {
    setDrawer({ open: true, individualId: id });
  };

  const deleteMutation = useDeleteIndividual({
    onSuccess: () => {
      message.success(t('individual.deleteSuccess'));
      setPendingDelete(null);
      refreshAll();
    },
    onError: (error: Error) => {
      if (getOntologyErrorReason(error) === 'ONTOLOGY_VERSION_CONFLICT') {
        modal.confirm({
          title: t('individualErrors.versionConflictTitle'),
          content: error.message || t('individualErrors.versionConflict'),
          okText: t('common:button.retry'),
          cancelText: t('common:button.cancel'),
          onOk: () => refreshAll(),
        });
        setPendingDelete(null);
        return;
      }
      // ENTITY_IN_USE：usage 明细弹窗自动呈现；其余错误只提示（铁律 1：request-client 已带出原始 reason/message）
      if ((usageQuery.data?.refs?.length ?? 0) === 0) {
        message.error(error.message || t('individual.deleteFailed'));
        setPendingDelete(null);
      }
    },
  });

  /** 删除：设置 pendingDelete 触发 force=false 尝试；被引用时展示 usage 明细弹窗 */
  const requestDelete = (record: ontologyservicev1_Individual) => {
    setPendingDelete(record);
  };

  useEffect(() => {
    if (pendingDelete == null) return;
    deleteMutation.mutate({ id: pendingDelete.id!, force: false });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingDelete]);

  const usage: ontologyservicev1_EntityUsage | undefined = usageQuery.data;
  const usageVisible = pendingDelete != null && (usage?.refs?.length ?? 0) > 0;

  const forceDelete = () => {
    if (pendingDelete == null) return;
    deleteMutation.mutate({
      id: pendingDelete.id!,
      force: true, // 红字警示已由弹窗呈现：引用方个体的断言行将一并移除；编码随个体删除、号不回收
    });
  };

  const usageColumns = useMemo(
    () => [
      {
        title: t('individual.usageRef.localName'),
        dataIndex: 'localName',
        render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
      },
      { title: t('individual.usageRef.displayLabel'), dataIndex: 'displayLabel' },
      {
        title: t('individual.usageRef.axiomType'),
        dataIndex: 'axiomType',
        render: () => <Tag>{t('individual.usageType.objectAssertion')}</Tag>,
      },
    ],
    [t],
  );

  return (
    <ContentContainer heightMode="fixed" padding="16px" bottomMargin={0}>
      <div className="ontology-individuals-toolbar">
        <Space>
          <OntologySwitcher />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            disabled={currentOntologyId == null}
            onClick={openCreate}
          >
            {t('individual.create')}
          </Button>
        </Space>
      </div>

      {currentOntologyId == null ? (
        <div className="ontology-individuals-empty">{t('individual.noOntologySelected')}</div>
      ) : (
        <Splitter style={{ height: '100%' }}>
          <Splitter.Panel collapsible defaultSize={360} min={280} max={560} style={{ display: 'flex', flexDirection: 'column' }}>
            <IndividualClassTree
              ontologyId={currentOntologyId}
              selectedKey={selected?.key ?? null}
              onSelectNode={(action) => setSelected(action)}
            />
          </Splitter.Panel>
          <Splitter.Panel style={{ display: 'flex', flexDirection: 'column' }}>
            <IndividualListTable
              ontologyId={currentOntologyId}
              classId={selected && selected.id > 0 ? selected.id : null}
              classLabel={selected ? selected.label : null}
              onClearClass={() => setSelected(null)}
              onCreate={openCreate}
              onEdit={(record) => openEdit(record.id!)}
              onToggleDeprecated={(record) => requestToggleDeprecated(record)}
              onDelete={requestDelete}
            />
          </Splitter.Panel>
        </Splitter>
      )}

      <IndividualDrawer
        open={drawer.open}
        ontologyId={currentOntologyId}
        individualId={drawer.individualId}
        onClose={() => setDrawer({ open: false, individualId: null })}
        onSuccess={refreshAll}
        onVersionConflict={refreshAll}
      />

      <Modal
        open={usageVisible}
        title={t('individual.usageModalTitle', {
          name: pendingDelete?.displayLabel || pendingDelete?.localName || `#${pendingDelete?.id ?? ''}`,
        })}
        onCancel={() => setPendingDelete(null)}
        footer={[
          <Button key="cancel" onClick={() => setPendingDelete(null)}>
            {t('common:button.cancel')}
          </Button>,
          <Button key="force" type="primary" danger onClick={forceDelete}>
            {t('individual.forceDelete')}
          </Button>,
        ]}
      >
        <p className="ontology-usage-warning">{t('individual.usageWarning', { count: usage?.refs?.length ?? 0 })}</p>
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

  // 废弃/恢复走 Update（frame 全量提交；仅 deprecated 字段变化）
  const toggleMutation = useUpdateIndividual({
    onSuccess: () => {
      refreshAll();
    },
    onError: (error: Error) => {
      message.error(error.message || t('individual.updateFailed'));
    },
  });

  /** 废弃/恢复行内动作（owl:deprecated 断言存在性 diff，无需二次确认页——§4.6）。
   *  更新是 frame 全量提交语义：断言区/注解区空提交 = 清空，故先取 frame 再全量回传，
   *  仅翻转 deprecated（主类目/类型/断言/注解原样保留）。 */
  async function requestToggleDeprecated(record: ontologyservicev1_Individual) {
    if (record.id == null || currentOntologyId == null) return;
    try {
      const frame = await queryClient.fetchQuery({
        queryKey: ['individualFrame', { id: record.id }],
        queryFn: () => apiClient.ontologyIndividualService.Frame({ id: record.id! }),
        retry: 0,
      });
      toggleMutation.mutate({
        values: {
          id: record.id,
          mainClassId: frame.types?.[0]?.id ?? 0,
          typeIds: (frame.types ?? []).slice(1).map((n) => n.id!),
          objectAssertions: (frame.objectAssertions ?? []).map((o) => ({
            predicateIri: o.predicateIri ?? '',
            targetId: o.target?.id ?? 0,
          })),
          dataAssertions: (frame.dataAssertions ?? []).map((d) => ({
            propertyId: d.propertyId ?? 0,
            value: d.value ?? '',
            lang: d.lang || undefined,
          })),
          annotations: (frame.annotations ?? []).map((a) => ({
            propertyId: a.propertyId ?? 0,
            value: a.value ?? '',
            lang: a.lang || undefined,
          })),
          deprecated: !record.deprecated,
          baseVersion: frame.baseVersion ?? undefined,
        } as Partial<ontologyservicev1_UpdateIndividualRequest>,
      });
    } catch (error) {
      // 铁律 1：取 frame 失败带出原始错误
      console.error('individual toggle deprecated: fetch frame failed', error);
      message.error((error as { message?: string })?.message || t('individual.updateFailed'));
    }
  }

};

export default OntologyIndividualManagement;
