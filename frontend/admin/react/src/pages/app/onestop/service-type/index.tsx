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
import type { onestopservicev1_ServiceType } from '@/api/generated/admin/service/v1';
import { fetchListServiceTypes, useDeleteServiceType } from '@/api/hooks/onestop';
import { getLevelColor, getLevelMap } from './constants';
import ServiceTypeForm from './components/ServiceTypeForm';
import './index.style.css';

const LIST_TAB_KEY = 'list';
const CREATE_TAB_KEY = 'create';

/**
 * 类型列表默认业务序：服务编码升序。
 * 编码 2/4/6/8 位天然构成「大类 → 中类 → 小类 → 细类」前缀序，
 * 后端在未收到显式排序时也按 code ASC 兜底；这里显式带上，
 * 避免 PaginationQuery 默认的 -created_at 覆盖业务序。
 */
const DEFAULT_ORDER_BY = ['code'];

/** 编辑/新增内容页签：编辑页签 key = edit-<编码>，同一服务编码天然只开一个 */
type ServiceTypeContentTab =
  | { key: string; kind: 'create' }
  | { key: string; kind: 'edit'; label: string; data: onestopservicev1_ServiceType };

const ServiceTypeManagement = () => {
  const { t } = useTranslation('onestop');
  const actionRef = useRef<ActionType>(null);
  const queryClient = useQueryClient();
  const { message } = App.useApp();
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const [contentTabs, setContentTabs] = useState<ServiceTypeContentTab[]>([]);
  const [activeKey, setActiveKey] = useState<string>(LIST_TAB_KEY);

  const levelMap = getLevelMap(t);

  const refreshList = () => {
    actionRef.current?.reload();
    queryClient.invalidateQueries({ queryKey: ['listServiceTypes'] });
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

  /** 编辑：同一服务编码只保留一个页签，重复点击仅切换 */
  const openEditTab = (record: onestopservicev1_ServiceType) => {
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

  const deleteMutation = useDeleteServiceType({
    onSuccess: () => {
      message.success(t('deleteSuccess'));
      refreshList();
    },
    onError: (error: Error) => message.error(error.message || t('deleteFailed')),
  });

  const columns: ProColumns<onestopservicev1_ServiceType>[] = [
    { title: t('code'), dataIndex: 'code', width: 140, copyable: true },
    {
      title: t('parentCode'),
      key: 'parentCode',
      width: 120,
      hideInSearch: true,
      // 平铺表不展开树：上级编码由编码前缀直接推导（细类 → 前 6 位，小类 → 前 4 位，中类 → 前 2 位）
      render: (_, record) =>
        record.code && record.code.length > 2 ? record.code.slice(0, -2) : '-',
    },
    { title: t('name'), dataIndex: 'name', width: 220, ellipsis: true },
    {
      title: t('level'),
      dataIndex: 'level',
      width: 110,
      valueType: 'select',
      valueEnum: {
        LEVEL_MAJOR: { text: levelMap.LEVEL_MAJOR },
        LEVEL_MIDDLE: { text: levelMap.LEVEL_MIDDLE },
        LEVEL_MINOR: { text: levelMap.LEVEL_MINOR },
        LEVEL_DETAIL: { text: levelMap.LEVEL_DETAIL },
      },
      render: (_, record) => <Tag color={getLevelColor(record.level)}>{levelMap[record.level ?? 'LEVEL_UNSPECIFIED']}</Tag>,
    },
    { title: t('description'), dataIndex: 'description', ellipsis: true, hideInSearch: true },
    {
      title: t('isEnabled'),
      dataIndex: 'isEnabled',
      width: 90,
      hideInSearch: true,
      render: (_, record) =>
        record.isEnabled ? (
          <Tag color="success">{t('enabled')}</Tag>
        ) : (
          <Tag color="default">{t('disabled')}</Tag>
        ),
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
          <ListTable<onestopservicev1_ServiceType>
            actionRef={actionRef}
            columns={columns}
            title={t('listTitle')}
            request={async (params) => {
              const { current, pageSize, ...rest } = params;
              const query = new PaginationQuery({
                paging: { page: current || 1, pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE },
                formValues: rest,
                // 平铺表默认按服务编码升序（编码唯一，服务端同样兜底）
                orderBy: DEFAULT_ORDER_BY,
              });
              const response = await fetchListServiceTypes(query);
              return { data: response.items ?? [], total: response.total ?? 0, success: true };
            }}
            rowKey="id"
            pagination={{
              defaultPageSize: TABLE.DEFAULT_PAGE_SIZE,
              showSizeChanger: true,
              showQuickJumper: true,
            }}
            scroll={{ y: tableScrollY, x: 1150 }}
            actions={
              <Space>
                <TableExportButton
                  fetcher={(query) => {
                    // 导出走本组件自己的分页聚合，这里注入与列表一致的服务编码升序
                    query.orderBy = DEFAULT_ORDER_BY;
                    return fetchListServiceTypes(query);
                  }}
                  columns={columns}
                  filename="onestop-service-types"
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
          <ServiceTypeForm
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
          <ServiceTypeForm
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

export default ServiceTypeManagement;
