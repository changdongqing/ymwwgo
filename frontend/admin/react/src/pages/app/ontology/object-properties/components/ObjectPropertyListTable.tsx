import { useRef } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Popconfirm, Space, Switch, Tag, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_ObjectProperty } from '@/api/generated/admin/service/v1';
import { fetchListObjectProperties } from '@/api/hooks/ontologyObjectProperty';
import { PROPERTY_CHARACTERISTIC_KEY_BY_IRI } from '../constants';

interface ObjectPropertyListTableProps {
  ontologyId: number | null;
  onCreate: () => void;
  onCreateChild: (record: ontologyservicev1_ObjectProperty) => void;
  onEdit: (record: ontologyservicev1_ObjectProperty) => void;
  onDelete: (record: ontologyservicev1_ObjectProperty) => void;
  /** 外部定位高亮的行 id（左树点击联动；不做子树范围切换） */
  highlightId?: number | null;
}

/**
 * 对象属性右表（ListTable）：聚合列（父属性数/直接子属性数/特征标签）由 List 接口返回，
 * 避免前端逐行算；查询表单 contains 搜索由服务端显式实现（铁律 3）。
 * 特征标签顺序 = 后端 characteristics 数组顺序（proto 枚举声明序）。
 */
const ObjectPropertyListTable: React.FC<ObjectPropertyListTableProps> = ({
  ontologyId,
  onCreate,
  onCreateChild,
  onEdit,
  onDelete,
  highlightId,
}) => {
  const { t } = useTranslation('ontology');
  const actionRef = useRef<ActionType>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const columns: ProColumns<ontologyservicev1_ObjectProperty>[] = [
    {
      title: t('objectProperty.localName'),
      dataIndex: 'localName',
      width: 160,
      render: (_, record) => (
        <Typography.Text code style={record.deprecated ? { textDecoration: 'line-through', color: 'var(--ant-color-text-tertiary)' } : undefined}>
          {record.localName}
        </Typography.Text>
      ),
    },
    {
      title: t('objectProperty.displayLabel'),
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
    {
      title: t('objectProperty.characteristics'),
      dataIndex: 'characteristics',
      width: 220,
      hideInSearch: true,
      render: (_, record) =>
        (record.characteristics ?? []).length > 0 ? (
          <Space size={4} wrap>
            {(record.characteristics ?? []).map((iri) => (
              <Tag key={iri} color="processing">
                {t(PROPERTY_CHARACTERISTIC_KEY_BY_IRI[iri] ?? 'objectProperty.characteristicUnknown')}
              </Tag>
            ))}
          </Space>
        ) : (
          '-'
        ),
    },
    { title: t('objectProperty.parentCount'), dataIndex: 'parentCount', width: 90, align: 'center', hideInSearch: true },
    { title: t('objectProperty.childCount'), dataIndex: 'childCount', width: 100, align: 'center', hideInSearch: true },
    {
      title: t('objectProperty.deprecated'),
      dataIndex: 'deprecated',
      width: 90,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('objectProperty.deprecatedYes'), value: 'true' },
          { label: t('objectProperty.deprecatedNo'), value: 'false' },
        ],
      },
      render: (_, record) => <Switch size="small" checked={!!record.deprecated} disabled />,
    },
    {
      title: 'IRI',
      dataIndex: 'iri',
      width: 240,
      hideInSearch: true,
      render: (_, record) => (
        <Typography.Text code ellipsis style={{ maxWidth: 220 }}>
          {record.iri}
        </Typography.Text>
      ),
    },
    { title: t('objectProperty.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('ontology.action'),
      valueType: 'option',
      width: 160,
      fixed: 'right',
      render: (_, record) => [
        <a key="create-child" onClick={() => onCreateChild(record)}>
          {t('objectProperty.createChildAction')}
        </a>,
        <a key="edit" onClick={() => onEdit(record)}>
          {t('common:button.edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('objectProperty.deleteConfirmTitle')}
          description={t('objectProperty.deleteConfirmDesc', { name: record.displayLabel || record.localName || '' })}
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
      <ListTable<ontologyservicev1_ObjectProperty>
        actionRef={actionRef}
        columns={columns}
        title={t('objectProperty.listTitle')}
        request={async (params) => {
          if (ontologyId == null) {
            return { data: [], total: 0, success: true };
          }
          const { current, pageSize, localName, displayLabel, deprecated, ...rest } = params;
          void rest;
          const response = await fetchListObjectProperties({
            ontologyId,
            page: current || 1,
            pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE,
            localName: localName || undefined,
            displayLabel: displayLabel || undefined,
            deprecated: deprecated === 'true' ? true : deprecated === 'false' ? false : undefined,
            orderBy: '["-created_at"]',
          });
          return { data: response.items ?? [], total: response.total ?? 0, success: true };
        }}
        rowKey="id"
        onRow={(record) =>
          highlightId != null && record.id === highlightId
            ? { style: { backgroundColor: 'var(--ant-color-primary-bg)' } }
            : {}
        }
        actions={
          <Button type="primary" size="small" icon={<PlusOutlined />} onClick={onCreate} disabled={ontologyId == null}>
            {t('objectProperty.createRoot')}
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
        locale={{ emptyText: ontologyId == null ? t('objectProperty.noOntologySelected') : undefined }}
      />
    </div>
  );
};

export default ObjectPropertyListTable;
