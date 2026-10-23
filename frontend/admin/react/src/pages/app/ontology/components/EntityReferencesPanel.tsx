import { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Empty, Select, Space, Table, Tag, Tooltip, Typography } from 'antd';
import type { TableColumnsType } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import type {
  ontologyservicev1_ListReferencesRequest,
  ontologyservicev1_ReferenceEdge,
  ontologyservicev1_ReferenceNode,
  ontologyservicev1_ReferencePosition,
} from '@/api/generated/admin/service/v1';
import { useOntologyReferences } from '@/api/hooks/ontologyReference';
import { useOntologyObjectPropertyTree } from '@/api/hooks/ontologyObjectProperty';
import { useOntologyDataPropertyTree } from '@/api/hooks/ontologyDataProperty';
import { formatDateTime } from '@/utils/date';

/** 公理号段 → i18n 键（08 号文档 §3.2 反查可用性矩阵的全集；未知号段回退数字） */
const AXIOM_TYPE_KEYS: Record<number, string> = {
  10: 'declaration',
  20: 'subClassOf',
  21: 'disjointWith',
  22: 'equivalentClasses',
  30: 'subObjectPropertyOf',
  31: 'objectPropertyDomain',
  32: 'objectPropertyRange',
  33: 'objectPropertyCharacteristic',
  34: 'inverseOf',
  35: 'equivalentObjectProperties',
  36: 'disjointObjectProperties',
  37: 'subPropertyChainOf',
  40: 'subDataPropertyOf',
  41: 'dataPropertyDomain',
  42: 'dataPropertyRange',
  43: 'dataPropertyCharacteristic',
  44: 'equivalentDataProperties',
  45: 'disjointDataProperties',
  46: 'datatypeDefinition',
  50: 'subAnnotationPropertyOf',
  60: 'typeAssertion',
  61: 'objectPropertyAssertion',
  62: 'dataPropertyAssertion',
  100: 'annotationAssertion',
};

interface EntityReferencesPanelProps {
  ontologyId: number | null;
  /** 被反查实体 id；null/0 时不请求 */
  entityId: number | null;
  /** 默认公理类型子集（缺省 = 服务端 §3.2 默认集） */
  defaultAxiomTypes?: number[];
  /** 默认命中位置（缺省 = [OBJECT]；属性抽屉传三值全开） */
  defaultPositions?: ontologyservicev1_ReferencePosition[];
  /** 可跳转的实体类型（10 类 / 60 个体等）：节点命中该类型且提供 onOpenEntity 时可点击 */
  openableEntityType?: number;
  /** 点击节点 → 打开同类实体抽屉（一期只接当前页面的实体类型） */
  onOpenEntity?: (entityId: number) => void;
  /** 类目标专用：显示"含子类"开关（60 子类实例 + 20 子类边） */
  allowIncludeDescendants?: boolean;
  /** 面板是否请求（抽屉打开时惰性加载） */
  enabled?: boolean;
}

/**
 * 共享「被引用」面板（08 期核心增量——Protégé Usage 视图的对应物，08 号文档 §6.1）。
 *
 * 消费通用反查单端点：返回物是「边」而非去重实体清单，列为主语/谓词/宾语（或字面量）/
 * 公理类型/位置/更新时间；支持按谓词过滤（对象属性 + 数据属性 IRI 两档）、分页与刷新。
 * 只读投影：就地编辑走各实体既有抽屉（§1.4 #4），故无行操作列。
 */
const EntityReferencesPanel: React.FC<EntityReferencesPanelProps> = ({
  ontologyId,
  entityId,
  defaultAxiomTypes,
  defaultPositions,
  openableEntityType,
  onOpenEntity,
  allowIncludeDescendants = false,
  enabled = true,
}) => {
  const { t } = useTranslation('ontology');

  const [predicateIri, setPredicateIri] = useState<string | undefined>(undefined);
  const [includeDescendants, setIncludeDescendants] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // 谓词过滤只对 61/62 有意义（08 号文档 §3.2/§12 #7）：默认集不含这两号段时隐藏过滤入口，
  // 同时不拉属性树（类/数据类型等抽屉省两次请求）
  const predicateFilterable =
    defaultAxiomTypes == null ||
    defaultAxiomTypes.some((t) => t === 61 || t === 62);

  // 谓词候选：对象属性树（61 谓词）+ 数据属性树（62 谓词），value 统一为 IRI（服务端按 61/62 分流）
  const objectPropTree = useOntologyObjectPropertyTree(ontologyId ?? undefined, {
    enabled: enabled && ontologyId != null && predicateFilterable,
    retry: false,
  });
  const dataPropTree = useOntologyDataPropertyTree(ontologyId ?? undefined, {
    enabled: enabled && ontologyId != null && predicateFilterable,
    retry: false,
  });

  useEffect(() => {
    // 铁律 1：候选降级不阻塞面板，但原始错误必须带出
    if (objectPropTree.error) {
      console.error('「被引用」面板加载对象属性树失败（谓词候选降级）：', objectPropTree.error);
    }
    if (dataPropTree.error) {
      console.error('「被引用」面板加载数据属性树失败（谓词候选降级）：', dataPropTree.error);
    }
  }, [objectPropTree.error, dataPropTree.error]);

  const predicateOptions = useMemo(() => {
    type Option = { value: string; label: string };
    const collect = (
      nodes: Array<{ iri?: string; displayLabel?: string; localName?: string; id?: number; children?: unknown[] }>,
    ): Option[] => {
      const out: Option[] = [];
      const walk = (list: typeof nodes) => {
        for (const n of list) {
          if (n.iri) out.push({ value: n.iri, label: n.displayLabel || n.localName || n.iri });
          walk((n.children ?? []) as typeof nodes);
        }
      };
      walk(nodes);
      return out;
    };
    const objectOptions = collect(objectPropTree.data?.roots ?? []);
    const dataOptions = collect(dataPropTree.data?.roots ?? []);
    const groups: Array<{ label: string; options: Option[] }> = [];
    if (objectOptions.length > 0) {
      groups.push({ label: t('reference.filter.objectProperties'), options: objectOptions });
    }
    if (dataOptions.length > 0) {
      groups.push({ label: t('reference.filter.dataProperties'), options: dataOptions });
    }
    return groups;
  }, [objectPropTree.data, dataPropTree.data, t]);

  // 过滤/开关变化回到第 1 页（避免越界空页）
  useEffect(() => {
    setPage(1);
  }, [predicateIri, includeDescendants, entityId, ontologyId]);

  const request = useMemo<ontologyservicev1_ListReferencesRequest>(
    () => ({
      ontologyId: ontologyId ?? 0,
      targetId: entityId ?? 0,
      axiomTypes: defaultAxiomTypes ?? [],
      positions: defaultPositions ?? ['REFERENCE_POSITION_OBJECT'],
      predicateIri: predicateIri || undefined,
      includeDescendants,
      page,
      pageSize,
      orderBy: '["-id"]',
    }),
    [ontologyId, entityId, defaultAxiomTypes, defaultPositions, predicateIri, includeDescendants, page, pageSize],
  );

  const query = useOntologyReferences(request, {
    enabled: enabled && ontologyId != null && ontologyId > 0 && entityId != null && entityId > 0,
  });

  const edges = query.data?.edges ?? [];

  const renderNode = (node?: ontologyservicev1_ReferenceNode) => {
    if (!node) return <Typography.Text type="secondary">-</Typography.Text>;
    const label = node.displayLabel || node.localName || node.iri || (node.id ? `#${node.id}` : '-');
    const clickable =
      !!onOpenEntity &&
      node.id != null &&
      node.id > 0 &&
      openableEntityType != null &&
      node.entityType === openableEntityType;
    const content = clickable ? (
      <Typography.Link onClick={() => onOpenEntity?.(node.id!)}>{label}</Typography.Link>
    ) : (
      <span>{label}</span>
    );
    if (node.unresolved) {
      return (
        <Tooltip title={t('reference.unresolvedHint', { iri: node.iri ?? '' })}>
          <Typography.Text type="warning">{label}</Typography.Text>
        </Tooltip>
      );
    }
    if (node.deprecated) {
      return (
        <Tooltip title={t('reference.deprecatedHint')}>
          <span>
            {content} <Tag color="default">{t('reference.deprecatedTag')}</Tag>
          </span>
        </Tooltip>
      );
    }
    return content;
  };

  const renderAxiomType = (axiomType?: number) => {
    if (axiomType == null) return '-';
    const key = AXIOM_TYPE_KEYS[axiomType];
    return (
      <Tag color="blue">
        {key ? t(`reference.axiomType.${key}`) : t('reference.axiomType.unknown', { code: axiomType })}
      </Tag>
    );
  };

  const positionLabel = (position?: ontologyservicev1_ReferencePosition) => {
    switch (position) {
      case 'REFERENCE_POSITION_PREDICATE':
        return t('reference.position.predicate');
      case 'REFERENCE_POSITION_CHAIN_MEMBER':
        return t('reference.position.chainMember');
      default:
        return t('reference.position.object');
    }
  };

  const columns: TableColumnsType<ontologyservicev1_ReferenceEdge> = [
    {
      title: t('reference.col.subject'),
      key: 'subject',
      width: 200,
      render: (_, record) => renderNode(record.subject),
    },
    {
      title: t('reference.col.predicate'),
      key: 'predicate',
      width: 180,
      render: (_, record) => renderNode(record.predicate),
    },
    {
      title: t('reference.col.object'),
      key: 'object',
      render: (_, record) => {
        if (record.object) return renderNode(record.object);
        if (record.literalValue) {
          return (
            <Space size={4}>
              <Typography.Text code>{record.literalValue}</Typography.Text>
              {record.lang ? <Tag>{record.lang}</Tag> : null}
            </Space>
          );
        }
        return <Typography.Text type="secondary">-</Typography.Text>;
      },
    },
    {
      title: t('reference.col.axiomType'),
      key: 'axiomType',
      width: 190,
      render: (_, record) => (
        <Space size={4} wrap>
          {renderAxiomType(record.axiomType)}
          {record.paired ? <Tag>{t('reference.pairedTag')}</Tag> : null}
        </Space>
      ),
    },
    {
      title: t('reference.col.position'),
      key: 'position',
      width: 110,
      render: (_, record) => <Tag>{positionLabel(record.position)}</Tag>,
    },
    {
      title: t('reference.col.updatedAt'),
      key: 'updatedAt',
      width: 170,
      render: (_, record) => formatDateTime(record.updatedAt),
    },
  ];

  if (query.error) {
    return (
      <Alert
        type="error"
        showIcon
        message={t('reference.loadFailed')}
        description={query.error.message}
        action={
          <Button size="small" onClick={() => void query.refetch()}>
            {t('common:button.retry')}
          </Button>
        }
      />
    );
  }

  return (
    <div className="ontology-references-panel">
      <Space wrap style={{ marginBottom: 8 }}>
        {predicateFilterable && (
          <Select
            style={{ minWidth: 240 }}
            allowClear
            showSearch
            value={predicateIri}
            placeholder={t('reference.filter.predicate')}
            optionFilterProp="label"
            options={predicateOptions}
            onChange={(value?: string) => setPredicateIri(value || undefined)}
          />
        )}
        {allowIncludeDescendants && (
          <Button
            size="small"
            type={includeDescendants ? 'primary' : 'default'}
            onClick={() => setIncludeDescendants((v) => !v)}
          >
            {t('reference.filter.includeDescendants')}
          </Button>
        )}
        <Button size="small" icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => void query.refetch()}>
          {t('reference.refresh')}
        </Button>
      </Space>
      <Table<ontologyservicev1_ReferenceEdge>
        size="small"
        rowKey={(record) => `${record.axiomId}-${record.position ?? 'OBJECT'}`}
        columns={columns}
        dataSource={edges}
        loading={query.isLoading}
        scroll={{ x: 900 }}
        locale={{
          emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('reference.empty')} />,
        }}
        pagination={{
          current: page,
          pageSize,
          total: Number(query.data?.total ?? 0),
          showSizeChanger: true,
          pageSizeOptions: [10, 20, 50, 100],
          showTotal: (total) => t('reference.total', { total }),
          onChange: (nextPage, nextSize) => {
            setPage(nextPage);
            setPageSize(nextSize);
          },
        }}
      />
    </div>
  );
};

export default EntityReferencesPanel;
