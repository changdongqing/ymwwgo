import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { PlusOutlined } from '@ant-design/icons';
import { App, Button, Popconfirm, Space, Tabs, Tag } from 'antd';
import type { TabsProps } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import ListTable from '@/components/common/ListTable';
import TableExportButton from '@/components/common/TableExportButton';
import ContentContainer from '@/layouts/components/PageContainer/ContentContainer';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { PaginationQuery } from '@/core';
import { TABLE } from '@/config/constants';
import type { contractservicev1_ContractType } from '@/api/generated/admin/service/v1';
import { fetchListContractTypes, useDeleteContractType } from '@/api/hooks/contract';
import { getLevelColor, getLevelMap, payMethodName } from './constants';
import ContractTypeForm from './components/TypeForm';
import '../../onestop/service-type/index.style.css';

const LIST_TAB_KEY = 'list';
const CREATE_TAB_KEY = 'create';

/**
 * 类型列表默认业务序：类型编码升序。
 * 编码 2/4 位天然构成「大类 → 细类」前缀序，后端在未收到显式排序时
 * 也按 code ASC 兜底；这里显式带上，避免 PaginationQuery 默认的
 * -created_at 覆盖业务序。
 */
const DEFAULT_ORDER_BY = ['code'];

/** 编辑/新增内容页签：编辑页签 key = edit-<编码>，同一编码天然只开一个 */
type ContractTypeContentTab =
  | { key: string; kind: 'create' }
  | { key: string; kind: 'edit'; label: string; data: contractservicev1_ContractType };

const ContractTypeManagement = () => {
  const { t } = useTranslation('contract');
  const actionRef = useRef<ActionType>(null);
  const queryClient = useQueryClient();
  const { message } = App.useApp();
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const [contentTabs, setContentTabs] = useState<ContractTypeContentTab[]>([]);
  const [activeKey, setActiveKey] = useState<string>(LIST_TAB_KEY);

  const levelMap = getLevelMap(t);

  const refreshList = () => {
    actionRef.current?.reload();
    queryClient.invalidateQueries({ queryKey: ['listContractTypes'] });
  };

  /** 关闭内容页签；关的是当前激活页签时回到列表 */
  const closeTab = (key: string) => {
    setContentTabs((prev) => prev.filter((tab) => tab.key !== key));
    setActiveKey((cur) => (cur === key ? LIST_TAB_KEY : cur));
  };

  /** 新增：页签固定一个，重复点击仅切换不重建 */
  const openCreateTab = () => {
    setContentTabs((prev) =>
      prev.some((tab) => tab.key === CREATE_TAB_KEY) ? prev : [...prev, { key: CREATE_TAB_KEY, kind: 'create' }],
    );
    setActiveKey(CREATE_TAB_KEY);
  };

  /** 编辑：同一编码只保留一个页签，重复点击仅切换 */
  const openEditTab = (record: contractservicev1_ContractType) => {
    const key = `edit-${record.code}`;
    setContentTabs((prev) =>
      prev.some((tab) => tab.key === key)
        ? prev
        : [
            ...prev,
            {
              key,
              kind: 'edit' as const,
              data: record,
              label: t('editTabLabel', { name: record.name ?? '', code: record.code ?? '' }),
            },
          ],
    );
    setActiveKey(key);
  };

  const deleteMutation = useDeleteContractType({
    onSuccess: () => {
      message.success(t('deleteSuccess'));
      refreshList();
    },
    onError: (error: Error) => message.error(error.message || t('deleteFailed')),
  });

  const columns: ProColumns<contractservicev1_ContractType>[] = [
    { title: t('code'), dataIndex: 'code', width: 140, copyable: true },
    {
      title: t('parentCode'),
      key: 'parentCode',
      width: 120,
      hideInSearch: true,
      // 平铺表不展开树：上级编码由编码前缀直接推导（细类 → 前 2 位）
      render: (_, record) => (record.code && record.code.length > 2 ? record.code.slice(0, 2) : '-'),
    },
    { title: t('name'), dataIndex: 'name', width: 220, ellipsis: true },
    {
      title: t('level'),
      dataIndex: 'level',
      width: 110,
      valueType: 'select',
      valueEnum: {
        LEVEL_MAJOR: { text: levelMap.LEVEL_MAJOR },
        LEVEL_DETAIL: { text: levelMap.LEVEL_DETAIL },
      },
      render: (_, record) => (
        <Tag color={getLevelColor(record.level)}>{levelMap[record.level ?? 'LEVEL_UNSPECIFIED']}</Tag>
      ),
    },
    // 模板概览列（未配置留空）；明细在编辑页签的「合同配置模板」分节
    {
      title: t('partyBLabel'),
      dataIndex: 'partyBLabel',
      width: 110,
      ellipsis: true,
      hideInSearch: true,
      render: (_, record) => record.partyBLabel || '-',
    },
    {
      title: t('payMethod'),
      key: 'payMethod',
      width: 110,
      hideInSearch: true,
      render: (_, record) =>
        record.payMethod && record.payMethod !== 'PAY_METHOD_UNSPECIFIED' ? payMethodName(record.payMethod) : '-',
    },
    {
      title: t('diffFieldCount'),
      key: 'diffFieldCount',
      width: 100,
      hideInSearch: true,
      render: (_, record) => (record.diffFields?.length ? `${record.diffFields.length} ${t('fieldUnit')}` : '-'),
    },
    { title: t('description'), dataIndex: 'description', ellipsis: true, hideInSearch: true },
    {
      title: t('isEnabled'),
      dataIndex: 'isEnabled',
      width: 90,
      hideInSearch: true,
      render: (_, record) =>
        record.isEnabled ? <Tag color="success">{t('enabled')}</Tag> : <Tag color="default">{t('disabled')}</Tag>,
    },
    { title: t('createdAt'), dataIndex: 'createdAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('action'),
      valueType: 'option',
      width: 100,
      fixed: 'right',
      render: (_, record) => [
        <a key="edit" onClick={() => openEditTab(record)}>
          {t('edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('deleteConfirmTitle')}
          description={t('deleteConfirmDesc', { moduleName: t('moduleName') })}
          onConfirm={() => deleteMutation.mutate({ id: record.id! })}
        >
          <a style={{ color: 'var(--ant-color-error)' }}>{t('delete')}</a>
        </Popconfirm>,
      ],
    },
  ];

  const items: TabsProps['items'] = [
    {
      key: LIST_TAB_KEY,
      label: t('tabList'),
      closable: false,
      children: (
        <div ref={containerRef} className="page-container-content">
          <ListTable<contractservicev1_ContractType>
            actionRef={actionRef}
            columns={columns}
            title={t('listTitle')}
            request={async (params) => {
              const { current, pageSize, ...rest } = params;
              const query = new PaginationQuery({
                paging: { page: current || 1, pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE },
                formValues: rest,
                orderBy: DEFAULT_ORDER_BY,
              });
              const response = await fetchListContractTypes(query);
              return { data: response.items ?? [], total: response.total ?? 0, success: true };
            }}
            rowKey="id"
            pagination={{
              defaultPageSize: TABLE.DEFAULT_PAGE_SIZE,
              showSizeChanger: true,
              showQuickJumper: true,
            }}
            scroll={{ y: tableScrollY, x: 1340 }}
            actions={
              <Space>
                <TableExportButton
                  fetcher={(query) => {
                    query.orderBy = DEFAULT_ORDER_BY;
                    return fetchListContractTypes(query);
                  }}
                  columns={columns}
                  filename="contract-types"
                />
                <Button type="primary" icon={<PlusOutlined />} onClick={openCreateTab}>
                  {t('create')}
                </Button>
              </Space>
            }
            options={{ density: true, fullScreen: true, setting: true, reload: true }}
            size="middle"
            bordered
          />
        </div>
      ),
    },
    ...contentTabs.map((tab) => ({
      key: tab.key,
      label: tab.kind === 'create' ? t('create') : tab.label,
      children:
        tab.kind === 'edit' ? (
          <ContractTypeForm
            key={tab.key}
            mode="edit"
            data={tab.data}
            onSaved={() => {
              closeTab(tab.key);
              refreshList();
            }}
            onCancel={() => closeTab(tab.key)}
          />
        ) : (
          <ContractTypeForm
            key={tab.key}
            mode="create"
            onSaved={() => {
              closeTab(tab.key);
              refreshList();
            }}
            onCancel={() => closeTab(tab.key)}
          />
        ),
    })),
  ];

  return (
    <ContentContainer heightMode="fixed" padding="16px" bottomMargin={0}>
      <Tabs
        className="service-type-tabs"
        type="editable-card"
        hideAdd
        activeKey={activeKey}
        onChange={setActiveKey}
        onEdit={(targetKey, action) => {
          if (action === 'remove') {
            closeTab(String(targetKey));
          }
        }}
        items={items}
      />
    </ContentContainer>
  );
};

export default ContractTypeManagement;
