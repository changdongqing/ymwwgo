import { useRef } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Popconfirm, Space, Switch, Tag, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_OntologyClass } from '@/api/generated/admin/service/v1';
import { fetchListOntologyClasses } from '@/api/hooks/ontologyClass';

interface ClassListTableProps {
  ontologyId: number | null;
  /** 左树联动注入的子树范围（0/null = 不过滤）；Tag 可关闭复位 */
  rootId: number | null;
  rootLabel: string | null;
  onClearRoot: () => void;
  onCreate: () => void;
  onCreateChild: (record: ontologyservicev1_OntologyClass) => void;
  onCreateSibling: (record: ontologyservicev1_OntologyClass) => void;
  onEdit: (record: ontologyservicev1_OntologyClass) => void;
  onDelete: (record: ontologyservicev1_OntologyClass) => void;
}

/**
 * 本体类右表（ListTable）：聚合列（父类数/直接子类数/不相交数）由 List 接口返回，
 * 避免前端逐行算；查询表单 contains 搜索由服务端显式实现（铁律 3）。
 */
const ClassListTable: React.FC<ClassListTableProps> = ({
  ontologyId,
  rootId,
  rootLabel,
  onClearRoot,
  onCreate,
  onCreateChild,
  onCreateSibling,
  onEdit,
  onDelete,
}) => {
  const { t } = useTranslation('ontology');
  const actionRef = useRef<ActionType>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const columns: ProColumns<ontologyservicev1_OntologyClass>[] = [
    {
      title: t('class.localName'),
      dataIndex: 'localName',
      width: 160,
      render: (_, record) => (
        <Typography.Text code style={record.deprecated ? { textDecoration: 'line-through', color: 'var(--ant-color-text-tertiary)' } : undefined}>
          {record.localName}
        </Typography.Text>
      ),
    },
    {
      title: t('class.displayLabel'),
      dataIndex: 'displayLabel',
      width: 180,
      ellipsis: true,
      render: (_, record) =>
        record.deprecated ? (
          <span style={{ textDecoration: 'line-through', color: 'var(--ant-color-text-tertiary)' }}>
            {record.displayLabel}
          </span>
        ) : (
          record.displayLabel
        ),
    },
    { title: t('class.notation'), dataIndex: 'notation', width: 110, hideInSearch: true, render: (_, record) => record.notation ? <Typography.Text code>{record.notation}</Typography.Text> : '-' },
    { title: t('class.parentCount'), dataIndex: 'parentCount', width: 90, align: 'center', hideInSearch: true },
    { title: t('class.childCount'), dataIndex: 'childCount', width: 100, align: 'center', hideInSearch: true },
    { title: t('class.disjointCount'), dataIndex: 'disjointCount', width: 100, align: 'center', hideInSearch: true },
    {
      title: t('class.deprecated'),
      dataIndex: 'deprecated',
      width: 90,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('class.deprecatedYes'), value: 'true' },
          { label: t('class.deprecatedNo'), value: 'false' },
        ],
      },
      render: (_, record) => (
        <Switch size="small" checked={!!record.deprecated} disabled />
      ),
    },
    { title: t('class.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('ontology.action'),
      valueType: 'option',
      width: 200,
      fixed: 'right',
      render: (_, record) => [
        <a key="create-child" onClick={() => onCreateChild(record)}>
          {t('class.createChildAction')}
        </a>,
        <a key="create-sibling" onClick={() => onCreateSibling(record)}>
          {t('class.createSiblingAction')}
        </a>,
        <a key="edit" onClick={() => onEdit(record)}>
          {t('common:button.edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('class.deleteConfirmTitle')}
          description={t('class.deleteConfirmDesc', { name: record.displayLabel || record.localName || '' })}
          onConfirm={() => onDelete(record)}
          okText={t('common:button.ok')}
          cancelText={t('common:button.cancel')}
        >
          <a style={{ color: 'var(--ant-color-error)' }}>{t('common:button.delete')}</a>
        </Popconfirm>,
      ],
    },
  ];

  return (
    <div ref={containerRef} className="page-container-content" style={{ height: '100%' }}>
      <ListTable<ontologyservicev1_OntologyClass>
        actionRef={actionRef}
        columns={columns}
        title={t('class.listTitle')}
        request={async (params) => {
          if (ontologyId == null) {
            return { data: [], total: 0, success: true };
          }
          const { current, pageSize, localName, displayLabel, deprecated, ...rest } = params;
          void rest;
          const response = await fetchListOntologyClasses({
            ontologyId,
            page: current || 1,
            pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE,
            localName: localName || undefined,
            displayLabel: displayLabel || undefined,
            deprecated: deprecated === 'true' ? true : deprecated === 'false' ? false : undefined,
            rootId: rootId ?? 0,
            orderBy: '["-created_at"]',
          });
          return { data: response.items ?? [], total: response.total ?? 0, success: true };
        }}
        rowKey="id"
        headerTitle={
          <Space>
            <span>{t('class.listTitle')}</span>
            {rootId != null && rootId > 0 && (
              <Tag closable onClose={onClearRoot} color="processing">
                {t('class.subtreeScope', { name: rootLabel ?? `#${rootId}` })}
              </Tag>
            )}
          </Space>
        }
        actions={
          <Button type="primary" size="small" icon={<PlusOutlined />} onClick={onCreate} disabled={ontologyId == null}>
            {t('class.createRoot')}
          </Button>
        }
        search={{
          span: 12,
        }}
        pagination={{
          defaultPageSize: TABLE.DEFAULT_PAGE_SIZE,
          showSizeChanger: true,
          showQuickJumper: true,
        }}
        options={{ density: true, fullScreen: false, setting: true, reload: true }}
        size="small"
        bordered
        scroll={{ y: tableScrollY, x: 1200 }}
        locale={{ emptyText: ontologyId == null ? t('class.noOntologySelected') : undefined }}
      />
    </div>
  );
};

export default ClassListTable;
