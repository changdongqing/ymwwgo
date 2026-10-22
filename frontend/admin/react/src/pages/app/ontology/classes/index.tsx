import { useCallback, useEffect, useMemo, useState } from 'react';
import { App, Button, Modal, Space, Splitter, Switch, Table, Tag, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';

import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useOntologyStore } from '@/stores';
import type {
  ontologyservicev1_OntologyClass,
  ontologyservicev1_EntityUsage,
} from '@/api/generated/admin/service/v1';
import {
  getOntologyErrorReason,
  useDeleteOntologyClass,
  useOntologyClassUsage,
} from '@/api/hooks/ontologyClass';

import OntologySwitcher from '../components/OntologySwitcher';
import OntologyClassTree, { OWL_THING_KEY, type TreeNodeAction } from './components/OntologyClassTree';
import ClassListTable from './components/ClassListTable';
import ClassDrawer from './components/ClassDrawer';
import './index.style.css';

interface DrawerState {
  open: boolean;
  classId: number | null;
  defaultParentIds: number[];
  /** create 入口的父节点树 key（成功后用于合成新节点的路径复合 key，定位选中新类） */
  parentKey: string | null;
}

/** 引用类型 → 展示标签（02 期起含 31 定义域 / 32 值域；41 定义域、60 个体断型标签位归 03/06 期） */
const USAGE_TYPE_LABEL_KEYS: Record<number, string> = {
  20: 'class.usageType.parent',
  21: 'class.usageType.disjoint',
  22: 'class.usageType.equivalent',
  31: 'class.usageType.objectPropertyDomain',
  32: 'class.usageType.objectPropertyRange',
};

/**
 * 本体类管理页（左树右表，Splitter 先例 opm/user、dict）：
 * 左 OntologyClassTree（骨架/全量双源），右 ClassListTable（rootId 子树范围联动）。
 * 点击树节点 = 选中 + 范围切换（子树过滤即 rootId 语义，不与分页冲突）。
 */
const OntologyClassManagement = () => {
  const { t } = useTranslation('ontology');
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();

  const currentOntologyId = useOntologyStore((s) => s.currentOntologyId);

  const [showAll, setShowAll] = useState(false);
  const [selected, setSelected] = useState<TreeNodeAction | null>(null);
  const [drawer, setDrawer] = useState<DrawerState>({
    open: false,
    classId: null,
    defaultParentIds: [],
    parentKey: null,
  });

  // 删除保护流程：先 force=false 尝试；ENTITY_IN_USE 时 usage 明细弹窗 → 确认后 force 重删
  const [pendingDelete, setPendingDelete] = useState<ontologyservicev1_OntologyClass | null>(null);
  const usageQuery = useOntologyClassUsage(
    { id: pendingDelete?.id ?? 0 },
    { enabled: pendingDelete != null },
  );

  const refreshAll = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['ontologyClassTree'] });
    queryClient.invalidateQueries({ queryKey: ['listOntologyClasses'] });
    queryClient.invalidateQueries({ queryKey: ['ontologyClassFrame'] });
  }, [queryClient]);

  // 切换本体后复位树选区与子树范围
  useEffect(() => {
    setSelected(null);
  }, [currentOntologyId]);

  const openCreate = (parentIds: number[], parentKey: string | null = null) => {
    if (currentOntologyId == null) {
      message.warning(t('class.noOntologySelected'));
      return;
    }
    setDrawer({ open: true, classId: null, defaultParentIds: parentIds.filter((id) => id > 0), parentKey });
  };

  const openEdit = (id: number) => {
    setDrawer({ open: true, classId: id, defaultParentIds: [], parentKey: null });
  };

  const deleteMutation = useDeleteOntologyClass({
    onSuccess: () => {
      message.success(t('class.deleteSuccess'));
      setPendingDelete(null);
      setSelected(null);
      refreshAll();
    },
    onError: (error: Error) => {
      if (getOntologyErrorReason(error) === 'ONTOLOGY_VERSION_CONFLICT') {
        modal.confirm({
          title: t('classErrors.versionConflictTitle'),
          content: error.message || t('classErrors.versionConflict'),
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
        message.error(error.message || t('class.deleteFailed'));
        setPendingDelete(null);
      }
    },
  });

  /** 删除：设置 pendingDelete 触发 force=false 尝试；被引用时展示 usage 明细弹窗 */
  const requestDelete = (record: ontologyservicev1_OntologyClass) => {
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
      force: true, // 红字警示已由弹窗呈现：将同时移除 N 处引用，子类自动变根类
    });
  };

  const handleDrawerSuccess = (saved: { id: number | null; parentIds: number[] }) => {
    // 对应 Protégé setSelectedOWLObject：树定位新类（合成路径复合 key），右表范围切到新类
    if (saved.id != null && drawer.parentKey != null) {
      setSelected({
        id: saved.id,
        parentId: saved.parentIds[saved.parentIds.length - 1] ?? 0,
        key: `${drawer.parentKey}-${saved.id}`,
      });
    }
    refreshAll();
  };

  const usageColumns = useMemo(
    () => [
      {
        title: t('class.usageRef.localName'),
        dataIndex: 'localName',
        render: (v: string) => <Typography.Text code>{v}</Typography.Text>,
      },
      { title: t('class.usageRef.displayLabel'), dataIndex: 'displayLabel' },
      {
        title: t('class.usageRef.axiomType'),
        dataIndex: 'axiomType',
        render: (v: number) => <Tag>{t(USAGE_TYPE_LABEL_KEYS[v] ?? 'class.usageType.other')}</Tag>,
      },
    ],
    [t],
  );

  return (
    <ContentContainer heightMode="fixed" padding="16px" bottomMargin={0}>
      <div className="ontology-classes-toolbar">
        <Space>
          <OntologySwitcher />
          <Button
            type="primary"
            icon={<PlusOutlined />}
            disabled={currentOntologyId == null}
            onClick={() => openCreate([])}
          >
            {t('class.createRoot')}
          </Button>
          <Space size={4}>
            <span className="ontology-classes-toolbar-label">{t('classTree.showAllClasses')}</span>
            <Switch size="small" checked={showAll} onChange={setShowAll} />
          </Space>
        </Space>
      </div>

      {currentOntologyId == null ? (
        <div className="ontology-classes-empty">{t('class.noOntologySelected')}</div>
      ) : (
        <Splitter style={{ height: '100%' }}>
          <Splitter.Panel collapsible defaultSize={360} min={280} max={560} style={{ display: 'flex', flexDirection: 'column' }}>
            <OntologyClassTree
              ontologyId={currentOntologyId}
              showAll={showAll}
              onShowAllChange={setShowAll}
              selectedKey={selected?.key ?? null}
              onSelectNode={(action) => {
                // 点击 = 选中 + 范围切换（owl:Thing 虚根仅作入口，不作为范围）
                setSelected(action && action.id > 0 ? action : null);
              }}
              onCreateChild={(action) =>
                action.id > 0
                  ? openCreate([action.id], action.key)
                  : openCreate([], OWL_THING_KEY)
              }
              onCreateSibling={(action) => {
                if (action.parentId > 0) {
                  // 父节点 key = 当前 key 去掉最后一段（路径复合 key 规则的逆运算）
                  const parentKey = action.key.split('-').slice(0, -1).join('-');
                  openCreate([action.parentId], parentKey);
                } else {
                  openCreate([], OWL_THING_KEY);
                }
              }}
              onEdit={(action) => openEdit(action.id)}
              onDelete={(action) =>
                requestDelete({ id: action.id } as ontologyservicev1_OntologyClass)
              }
            />
          </Splitter.Panel>
          <Splitter.Panel style={{ display: 'flex', flexDirection: 'column' }}>
            <ClassListTable
              ontologyId={currentOntologyId}
              rootId={selected && selected.id > 0 ? selected.id : null}
              rootLabel={selected ? `#${selected.id}` : null}
              onClearRoot={() => setSelected(null)}
              onCreate={() => openCreate([])}
              onCreateChild={(record) => openCreate([record.id!], null)}
              onCreateSibling={(_record) => {
                // 兄弟 = 与目标同父；右表无父信息，默认根类创建、用户可在表单补选父类
                openCreate([], null);
              }}
              onEdit={(record) => openEdit(record.id!)}
              onDelete={requestDelete}
            />
          </Splitter.Panel>
        </Splitter>
      )}

      <ClassDrawer
        open={drawer.open}
        ontologyId={currentOntologyId}
        classId={drawer.classId}
        defaultParentIds={drawer.defaultParentIds}
        onClose={() => setDrawer({ open: false, classId: null, defaultParentIds: [], parentKey: null })}
        onSuccess={handleDrawerSuccess}
        onVersionConflict={refreshAll}
      />

      <Modal
        open={usageVisible}
        title={t('class.usageModalTitle', {
          name: pendingDelete?.displayLabel || pendingDelete?.localName || `#${pendingDelete?.id ?? ''}`,
        })}
        onCancel={() => setPendingDelete(null)}
        footer={[
          <Button key="cancel" onClick={() => setPendingDelete(null)}>
            {t('common:button.cancel')}
          </Button>,
          <Button key="force" type="primary" danger onClick={forceDelete}>
            {t('class.forceDelete')}
          </Button>,
        ]}
      >
        <p className="ontology-usage-warning">{t('class.usageWarning', { count: usage?.refs?.length ?? 0 })}</p>
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

export default OntologyClassManagement;
