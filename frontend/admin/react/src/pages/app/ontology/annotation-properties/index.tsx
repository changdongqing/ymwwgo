import { useCallback, useEffect, useMemo, useState } from 'react';
import { App, Button, Modal, Space, Splitter, Table, Tag, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useOntologyStore } from '@/stores';
import type {
  ontologyservicev1_AnnotationProperty,
  ontologyservicev1_EntityUsage,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useAnnotationPropertyUsage,
  useDeleteAnnotationProperty,
} from '@/api/hooks/ontologyAnnotationProperty';

import OntologySwitcher from '../components/OntologySwitcher';
import AnnotationPropertyTree, { type TreeNodeAction } from './components/AnnotationPropertyTree';
import AnnotationPropertyListTable from './components/AnnotationPropertyListTable';
import AnnotationPropertyDrawer from './components/AnnotationPropertyDrawer';
import { OWL_TOP_ANNOTATION_PROPERTY_KEY } from './constants';
import './index.style.css';

interface DrawerState {
  open: boolean;
  propertyId: number | null;
  defaultSuperIds: number[];
  /** create 入口的父节点树 key（成功后用于合成新节点的路径复合 key，定位选中新属性） */
  parentKey: string | null;
}

/** 引用类型 → 展示标签（注解属性 usage：50 子注解属性 / 100 被注解——04 号文档 §5.2） */
const USAGE_TYPE_LABEL_KEYS: Record<number, string> = {
  50: 'annotationProperty.usageType.subAnnotationProperty',
  100: 'annotationProperty.usageType.annotated',
};

/**
 * 注解属性管理页（左树右表，镜像 03 期属性页骨架）：
 * 左 AnnotationPropertyTree（全量树 + owl:topAnnotationProperty 虚根），
 * 右 AnnotationPropertyListTable。点击树节点 = 定位高亮右表行。
 * 本页是注解词汇表的管理面；通用注解区的消费面在类/对象属性/数据属性三个 Drawer
 * （AnnotationListEditor 共享组件挂载——04 号文档 §2.5）。
 */
const OntologyAnnotationPropertyManagement = () => {
  const { t } = useTranslation('ontology');
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();

  const currentOntologyId = useOntologyStore((s) => s.currentOntologyId);

  const [selected, setSelected] = useState<TreeNodeAction | null>(null);
  const [drawer, setDrawer] = useState<DrawerState>({
    open: false,
    propertyId: null,
    defaultSuperIds: [],
    parentKey: null,
  });

  // 删除保护流程：先 force=false 尝试；ENTITY_IN_USE 时 usage 明细弹窗 → 确认后 force 重删
  const [pendingDelete, setPendingDelete] = useState<ontologyservicev1_AnnotationProperty | null>(null);
  const usageQuery = useAnnotationPropertyUsage(
    { id: pendingDelete?.id ?? 0 },
    { enabled: pendingDelete != null },
  );

  const refreshAll = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['ontologyAnnotationPropertyTree'] });
    queryClient.invalidateQueries({ queryKey: ['listAnnotationProperties'] });
    queryClient.invalidateQueries({ queryKey: ['annotationPropertyFrame'] });
  }, [queryClient]);

  // 切换本体后复位树选区
  useEffect(() => {
    setSelected(null);
  }, [currentOntologyId]);

  const openCreate = (superIds: number[], parentKey: string | null = null) => {
    if (currentOntologyId == null) {
      message.warning(t('annotationProperty.noOntologySelected'));
      return;
    }
    setDrawer({ open: true, propertyId: null, defaultSuperIds: superIds.filter((id) => id > 0), parentKey });
  };

  const openEdit = (id: number) => {
    setDrawer({ open: true, propertyId: id, defaultSuperIds: [], parentKey: null });
  };

  const deleteMutation = useDeleteAnnotationProperty({
    onSuccess: () => {
      message.success(t('annotationProperty.deleteSuccess'));
      setPendingDelete(null);
      setSelected(null);
      refreshAll();
    },
    onError: (error: Error) => {
      if (getOntologyErrorReason(error) === 'ONTOLOGY_VERSION_CONFLICT') {
        modal.confirm({
          title: t('annotationPropertyErrors.versionConflictTitle'),
          content: error.message || t('annotationPropertyErrors.versionConflict'),
          okText: t('common:button.retry'),
          cancelText: t('common:button.cancel'),
          onOk: () => refreshAll(),
        });
        setPendingDelete(null);
        return;
      }
      // ENTITY_IN_USE：usage 明细弹窗自动呈现（usageQuery 以 pendingDelete 拉取），
      // 其余错误只提示（铁律 1：request-client 已带出原始 reason/message）
      if ((usageQuery.data?.refs?.length ?? 0) === 0) {
        message.error(error.message || t('annotationProperty.deleteFailed'));
        setPendingDelete(null);
      }
    },
  });

  /** 删除：设置 pendingDelete 触发 force=false 尝试；被引用时展示 usage 明细弹窗 */
  const requestDelete = (record: ontologyservicev1_AnnotationProperty) => {
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
      force: true, // 红字警示已由弹窗呈现：将同时移除 N 处引用，子属性自动变根属性
    });
  };

  const handleDrawerSuccess = (saved: { id: number | null; superIds: number[] }) => {
    // 对应 Protégé setSelectedOWLObject：树定位新属性（合成路径复合 key），右表高亮该行
    if (saved.id != null && drawer.parentKey != null) {
      setSelected({
        id: saved.id,
        parentId: saved.superIds[saved.superIds.length - 1] ?? 0,
        key: `${drawer.parentKey}-${saved.id}`,
      });
    }
    refreshAll();
  };

  const usageColumns = useMemo(
    () => [
      {
        title: t('annotationProperty.usageRef.localName'),
        dataIndex: 'localName',
        render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
      },
      { title: t('annotationProperty.usageRef.displayLabel'), dataIndex: 'displayLabel' },
      {
        title: t('annotationProperty.usageRef.axiomType'),
        dataIndex: 'axiomType',
        render: (v: number) => <Tag>{t(USAGE_TYPE_LABEL_KEYS[v] ?? 'annotationProperty.usageType.other')}</Tag>,
      },
    ],
    [t],
  );

  return (
    <ContentContainer heightMode="fixed" padding="16px" bottomMargin={0}>
      <div className="ontology-properties-toolbar">
        <Space>
          <OntologySwitcher />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            disabled={currentOntologyId == null}
            onClick={() => openCreate([])}
          >
            {t('annotationProperty.createRoot')}
          </Button>
        </Space>
      </div>

      {currentOntologyId == null ? (
        <div className="ontology-properties-empty">{t('annotationProperty.noOntologySelected')}</div>
      ) : (
        <Splitter style={{ height: '100%' }}>
          <Splitter.Panel collapsible defaultSize={360} min={280} max={560} style={{ display: 'flex', flexDirection: 'column' }}>
            <AnnotationPropertyTree
              ontologyId={currentOntologyId}
              selectedKey={selected?.key ?? null}
              onSelectNode={(action) => {
                // 点击 = 定位高亮右表行（owl:topAnnotationProperty 虚根仅作入口，不作为范围）
                setSelected(action && action.id > 0 ? action : null);
              }}
              onCreateChild={(action) =>
                action.id > 0
                  ? openCreate([action.id], action.key)
                  : openCreate([], OWL_TOP_ANNOTATION_PROPERTY_KEY)
              }
              onCreateSibling={(action) => {
                if (action.parentId > 0) {
                  // 父节点 key = 当前 key 去掉最后一段（路径复合 key 规则的逆运算）
                  const parentKey = action.key.split('-').slice(0, -1).join('-');
                  openCreate([action.parentId], parentKey);
                } else {
                  openCreate([], OWL_TOP_ANNOTATION_PROPERTY_KEY);
                }
              }}
              onEdit={(action) => openEdit(action.id)}
              onDelete={(action) =>
                requestDelete({ id: action.id } as ontologyservicev1_AnnotationProperty)
              }
            />
          </Splitter.Panel>
          <Splitter.Panel style={{ display: 'flex', flexDirection: 'column' }}>
            <AnnotationPropertyListTable
              ontologyId={currentOntologyId}
              highlightId={selected && selected.id > 0 ? selected.id : null}
              onCreate={() => openCreate([])}
              onCreateChild={(record) => openCreate([record.id!], null)}
              onEdit={(record) => openEdit(record.id!)}
              onDelete={requestDelete}
            />
          </Splitter.Panel>
        </Splitter>
      )}

      <AnnotationPropertyDrawer
        open={drawer.open}
        ontologyId={currentOntologyId}
        propertyId={drawer.propertyId}
        defaultSuperIds={drawer.defaultSuperIds}
        onClose={() => setDrawer({ open: false, propertyId: null, defaultSuperIds: [], parentKey: null })}
        onSuccess={handleDrawerSuccess}
        onVersionConflict={refreshAll}
      />

      <Modal
        open={usageVisible}
        title={t('annotationProperty.usageModalTitle', {
          name: pendingDelete?.displayLabel || pendingDelete?.localName || `#${pendingDelete?.id ?? ''}`,
        })}
        onCancel={() => setPendingDelete(null)}
        footer={[
          <Button key="cancel" onClick={() => setPendingDelete(null)}>
            {t('common:button.cancel')}
          </Button>,
          <Button key="force" type="primary" danger onClick={forceDelete}>
            {t('annotationProperty.forceDelete')}
          </Button>,
        ]}
      >
        <p className="ontology-usage-warning">
          {t('annotationProperty.usageWarning', { count: usage?.refs?.length ?? 0 })}
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

export default OntologyAnnotationPropertyManagement;
