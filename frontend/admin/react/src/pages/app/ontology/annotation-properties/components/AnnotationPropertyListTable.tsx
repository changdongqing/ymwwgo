import { useRef } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Popconfirm, Switch, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_AnnotationProperty } from '@/api/generated/admin/service/v1';
import { fetchListAnnotationProperties } from '@/api/hooks/ontologyAnnotationProperty';

interface AnnotationPropertyListTableProps {
  ontologyId: number | null;
  onCreate: () => void;
  onCreateChild: (record: ontologyservicev1_AnnotationProperty) => void;
  onEdit: (record: ontologyservicev1_AnnotationProperty) => void;
  onDelete: (record: ontologyservicev1_AnnotationProperty) => void;
  /** 外部定位高亮的行 id（左树点击联动；不做子树范围切换） */
  highlightId?: number | null;
}

/**
 * 注解属性右表（ListTable）：聚合列（父属性数/直接子属性数）由 List 接口返回，
 * 避免前端逐行算；查询表单 contains 搜索由服务端显式实现（铁律 3）。
 * 与数据属性表的差异（04 号文档 §6.2）：无特征/值域列（注解属性无逻辑结构），
 * 信息量主列退回局部名/显示名本身。
 */
const AnnotationPropertyListTable: React.FC<AnnotationPropertyListTableProps> = ({
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

  const columns: ProColumns<ontologyservicev1_AnnotationProperty>[] = [
    {
      title: t('annotationProperty.localName'),
      dataIndex: 'localName',
      width: 180,
      render: (_, record) => (
        <Typography.Text code style={record.deprecated ? { textDecoration: 'line-through', color: 'var(--ant-color-text-tertiary)' } : undefined}>
          {record.localName}
        </Typography.Text>
      ),
    },
    {
      title: t('annotationProperty.displayLabel'),
      dataIndex: 'displayLabel',
      width: 200,
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
    { title: t('annotationProperty.parentCount'), dataIndex: 'parentCount', width: 90, align: 'center', hideInSearch: true },
    { title: t('annotationProperty.childCount'), dataIndex: 'childCount', width: 110, align: 'center', hideInSearch: true },
    {
      title: t('annotationProperty.deprecated'),
      dataIndex: 'deprecated',
      width: 90,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('annotationProperty.deprecatedYes'), value: 'true' },
          { label: t('annotationProperty.deprecatedNo'), value: 'false' },
        ],
      },
      render: (_, record) => <Switch size="small" checked={!!record.deprecated} disabled />,
    },
    {
      title: 'IRI',
      dataIndex: 'iri',
      width: 260,
      hideInSearch: true,
      render: (_, record) => (
        <Typography.Text code ellipsis style={{ maxWidth: 240 }}>
          {record.iri}
        </Typography.Text>
      ),
    },
    { title: t('annotationProperty.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('ontology.action'),
      valueType: 'option',
      width: 160,
      fixed: 'right',
      render: (_, record) => [
        <a key="create-child" onClick={() => onCreateChild(record)}>
          {t('annotationProperty.createChildAction')}
        </a>,
        <a key="edit" onClick={() => onEdit(record)}>
          {t('common:button.edit')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('annotationProperty.deleteConfirmTitle')}
          description={t('annotationProperty.deleteConfirmDesc', { name: record.displayLabel || record.localName || '' })}
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
      <ListTable<ontologyservicev1_AnnotationProperty>
        actionRef={actionRef}
        columns={columns}
        title={t('annotationProperty.listTitle')}
        request={async (params) => {
          if (ontologyId == null) {
            return { data: [], total: 0, success: true };
          }
          const { current, pageSize, localName, displayLabel, deprecated, ...rest } = params;
          void rest;
          const response = await fetchListAnnotationProperties({
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
            {t('annotationProperty.createRoot')}
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
        locale={{ emptyText: ontologyId == null ? t('annotationProperty.noOntologySelected') : undefined }}
      />
    </div>
  );
};

export default AnnotationPropertyListTable;
