import { useRef } from 'react';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Popconfirm, Space, Switch, Tag, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import ListTable from '@/components/common/ListTable';
import { useProTableScrollY } from '@/hooks/useProTableScrollY';
import { TABLE } from '@/config/constants';
import type { ontologyservicev1_Individual } from '@/api/generated/admin/service/v1';
import { fetchListIndividuals } from '@/api/hooks/ontologyIndividual';

interface IndividualListTableProps {
  ontologyId: number | null;
  /** 左树联动注入的类目范围（直接实例过滤；0/null = 不过滤）；Tag 可关闭复位 */
  classId: number | null;
  classLabel: string | null;
  onClearClass: () => void;
  onCreate: () => void;
  onEdit: (record: ontologyservicev1_Individual) => void;
  onToggleDeprecated: (record: ontologyservicev1_Individual) => void;
  onDelete: (record: ontologyservicev1_Individual) => void;
}

/**
 * 个体右表（ListTable，06 号文档 §6.2 列定义）：编码（mono）/显示名（deprecated 删除线灰显）/
 * 主类目（聚合列 brief）/局部名/类型数/状态/更新时间/操作列。
 * 编码精确/前缀检索 + keyword + 含子类实例开关由服务端两段式查询承载（铁律 3：contains 语义除外）。
 */
const IndividualListTable: React.FC<IndividualListTableProps> = ({
  ontologyId,
  classId,
  classLabel,
  onClearClass,
  onCreate,
  onEdit,
  onToggleDeprecated,
  onDelete,
}) => {
  const { t } = useTranslation('ontology');
  const actionRef = useRef<ActionType>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const tableScrollY = useProTableScrollY(containerRef);

  const columns: ProColumns<ontologyservicev1_Individual>[] = [
    {
      title: t('individual.code'),
      dataIndex: 'code',
      width: 130,
      fieldProps: { placeholder: t('individual.codeFilterPlaceholder'), allowClear: true, style: { fontFamily: 'monospace' } },
      render: (_, record) =>
        record.code ? <Typography.Text code copyable={{ text: record.code }}>{record.code}</Typography.Text> : '-',
    },
    {
      title: t('individual.codeQueryType'),
      dataIndex: 'codeQueryType',
      width: 110,
      hideInTable: true,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('individual.codeQueryExact'), value: 'EXACT' },
          { label: t('individual.codeQueryPrefix'), value: 'PREFIX' },
        ],
      },
      initialValue: 'EXACT',
    },
    {
      title: t('individual.includeDescendants'),
      dataIndex: 'includeDescendants',
      width: 110,
      hideInTable: true,
      valueType: 'switch',
      initialValue: false,
    },
    {
      title: t('individual.displayLabel'),
      dataIndex: 'keyword',
      hideInSearch: false,
      width: 180,
      ellipsis: true,
      render: (_, record) =>
        record.deprecated ? (
          <span style={{ textDecoration: 'line-through', color: 'var(--ant-color-text-tertiary)' }}>
            {record.displayLabel || record.localName}
          </span>
        ) : (
          record.displayLabel || record.localName
        ),
    },
    {
      title: t('individual.mainClass'),
      dataIndex: 'mainClass',
      width: 150,
      ellipsis: true,
      hideInSearch: true,
      render: (_, record) => record.mainClass?.displayLabel || record.mainClass?.localName || '-',
    },
    {
      title: t('individual.localName'),
      dataIndex: 'localName',
      width: 150,
      hideInSearch: true,
      render: (_, record) => <Typography.Text code>{record.localName}</Typography.Text>,
    },
    { title: t('individual.typeCount'), dataIndex: 'typeCount', width: 90, align: 'center', hideInSearch: true },
    {
      title: t('individual.deprecated'),
      dataIndex: 'deprecated',
      width: 90,
      valueType: 'select',
      fieldProps: {
        options: [
          { label: t('individual.deprecatedYes'), value: 'true' },
          { label: t('individual.deprecatedNo'), value: 'false' },
        ],
      },
      render: (_, record) => <Switch size="small" checked={!!record.deprecated} disabled />,
    },
    { title: t('individual.updatedAt'), dataIndex: 'updatedAt', valueType: 'dateTime', width: 160, hideInSearch: true },
    {
      title: t('ontology.action'),
      valueType: 'option',
      width: 190,
      fixed: 'right',
      render: (_, record) => [
        <a key="edit" onClick={() => onEdit(record)}>
          {t('common:button.edit')}
        </a>,
        <a key="toggle-deprecated" onClick={() => onToggleDeprecated(record)}>
          {record.deprecated ? t('individual.restore') : t('individual.deprecate')}
        </a>,
        <Popconfirm
          key="delete"
          title={t('individual.deleteConfirmTitle')}
          description={
            record.code
              ? t('individual.deleteConfirmDescWithCode', {
                  code: record.code,
                  name: record.displayLabel || record.localName || '',
                })
              : t('individual.deleteConfirmDesc', { name: record.displayLabel || record.localName || '' })
          }
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
      <ListTable<ontologyservicev1_Individual>
        actionRef={actionRef}
        columns={columns}
        request={async (params) => {
          if (ontologyId == null) {
            return { data: [], total: 0, success: true };
          }
          const { current, pageSize, keyword, code, codeQueryType, includeDescendants, deprecated, ...rest } = params;
          void rest;
          const response = await fetchListIndividuals({
            ontologyId,
            page: current || 1,
            pageSize: pageSize || TABLE.DEFAULT_PAGE_SIZE,
            keyword: (keyword as string) || undefined,
            code: (code as string) || undefined,
            codeQueryType: (codeQueryType as string) || 'EXACT',
            classId: classId ?? 0,
            includeDescendants: includeDescendants === true || includeDescendants === 'true',
            deprecated: deprecated === 'true' ? true : deprecated === 'false' ? false : undefined,
            orderBy: '["-updated_at"]',
          });
          return { data: response.items ?? [], total: response.total ?? 0, success: true };
        }}
        rowKey="id"
        headerTitle={
          <Space>
            <span>{t('individual.listTitle')}</span>
            {classId != null && classId > 0 && (
              <Tag closable onClose={onClearClass} color="processing">
                {t('individual.classScope', { name: classLabel ?? `#${classId}` })}
              </Tag>
            )}
          </Space>
        }
        actions={
          <Button type="primary" size="small" icon={<PlusOutlined />} onClick={onCreate} disabled={ontologyId == null}>
            {t('individual.create')}
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
        locale={{ emptyText: ontologyId == null ? t('individual.noOntologySelected') : undefined }}
      />
    </div>
  );
};

export default IndividualListTable;
