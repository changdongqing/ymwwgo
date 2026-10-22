import { useRef } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Popconfirm, Space, Switch, Tag, Tooltip, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_DataProperty } from '@/api/generated/admin/service/v1';
import { fetchListDataProperties } from '@/api/hooks/ontologyDataProperty';
import { DATATYPE_KEY_BY_IRI, datatypeShortName } from '../../constants';

interface DataPropertyListTableProps {
  ontologyId: number | null;
  onCreate: () => void;
  onCreateChild: (record: ontologyservicev1_DataProperty) => void;
  onEdit: (record: ontologyservicev1_DataProperty) => void;
  onDelete: (record: ontologyservicev1_DataProperty) => void;
  /** 外部定位高亮的行 id（左树点击联动；不做子树范围切换） */
  highlightId?: number | null;
}

/**
 * 数据属性右表（ListTable）：聚合列（父属性数/直接子属性数/函数性/值域标签）由 List 接口返回，
 * 避免前端逐行算；查询表单 contains 搜索由服务端显式实现（铁律 3）。
 * 与对象属性表的差异（03 号文档 §6.2）：无逆属性/链展示；函数性单 Tag + 值域 datatype 标签组
 * 成为信息量主列（多值域为 OWL 交集语义，tooltip 提示）。
 */
const DataPropertyListTable: React.FC<DataPropertyListTableProps> = ({
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

  const columns: ProColumns<ontologyservicev1_DataProperty>[] = [
    {
      title: t('dataProperty.localName'),
      dataIndex: 'localName',
      width: 160,
      render: (_, record) => (
        <Typography.Text code style={record.deprecated ? { textDecoration: 'line-through', color: 'var(--ant-color-text-tertiary)' } : undefined}>
          {record.localName}
        </Typography.Text>
      ),
    },
    {
      title: t('dataProperty.displayLabel'),
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
      title: t('dataProperty.functional'),
      dataIndex: 'characteristics',
      width: 90,
      align: 'center',
      hideInSearch: true,
      render: (_, record) =>
        (record.characteristics ?? []).includes('http://www.w3.org/2002/07/owl#FunctionalProperty') ? (
          <Tag color="processing">{t('dataProperty.functionalYes')}</Tag>
        ) : (
          '-'
        ),
    },
    {
      title: t('dataProperty.rangeIris'),
      dataIndex: 'rangeIris',
      width: 200,
      hideInSearch: true,
      render: (_, record) =>
        (record.rangeIris ?? []).length > 0 ? (
          <Space size={4} wrap>
            {(record.rangeIris ?? []).map((iri) => (
              <Tooltip key={iri} title={datatypeShortName(iri)}>
                <Tag color="purple">
                  {t(DATATYPE_KEY_BY_IRI[iri] ?? 'dataProperty.datatypeUnknown')}
                </Tag>
              </Tooltip>
            ))}
          </Space>
        ) : (
          '-'
        ),
    },
    { title: t('dataProperty.parentCount'), dataIndex: 'parentCount', width: 90, align: 'center', hideInSearch: true },
    { title: t('dataProperty.childCount'), dataIndex: 'childCount', width: 100, align: 'center', hideInSearch: true },
    {
      title: t('dataProperty.deprecated'),
      dataIndex: 'deprecated',
      width: 90,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('dataProperty.deprecatedYes'), value: 'true' },
          { label: t('dataProperty.deprecatedNo'), value: 'false' },
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
    { title: t('dataProperty.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('ontology.action'),
      valueType: 'option',
      width: 160,
      fixed: 'right',
      render: (_, record) => [
        <a key="create-child" onClick={() => onCreateChild(record)}>
          {t('dataProperty.createChildAction')}
        </a>,
        <a key="edit" onClick={() => onEdit(record)}>
          {t('common:button.edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('dataProperty.deleteConfirmTitle')}
          description={t('dataProperty.deleteConfirmDesc', { name: record.displayLabel || record.localName || '' })}
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
      <ListTable<ontologyservicev1_DataProperty>
        actionRef={actionRef}
        columns={columns}
        title={t('dataProperty.listTitle')}
        request={async (params) => {
          if (ontologyId == null) {
            return { data: [], total: 0, success: true };
          }
          const { current, pageSize, localName, displayLabel, deprecated, ...rest } = params;
          void rest;
          const response = await fetchListDataProperties({
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
            {t('dataProperty.createRoot')}
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
        locale={{ emptyText: ontologyId == null ? t('dataProperty.noOntologySelected') : undefined }}
      />
    </div>
  );
};

export default DataPropertyListTable;
