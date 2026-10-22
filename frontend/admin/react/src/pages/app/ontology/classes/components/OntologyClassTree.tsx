import { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Input, Space, Switch, Tag, Tooltip, Tree, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import {
  DeleteOutlined,
  EditOutlined,
  MinusSquareOutlined,
  PlusSquareOutlined,
  PlusOutlined,
  ReloadOutlined,
  SisternodeOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useOntologyClassNavTree, useOntologyClassTree } from '@/api/hooks/ontologyClass';
import type { ontologyservicev1_OntologyClassTreeNode } from '@/api/generated/admin/service/v1';

/** owl:Thing 虚拟根的固定 key（不入库，服务端返回根列表，Thing 节点前端固定渲染——源修订 #5） */
export const OWL_THING_KEY = 'owl:Thing';

/** 树节点路径复合 key 的分隔符（多继承下同一实体多现，直接用 id 会 key 冲突致展开状态错乱） */
const KEY_SEPARATOR = '-';

export interface TreeNodeAction {
  /** 命中的实体 id；owl:Thing 虚拟根为 0 */
  id: number;
  /** 该节点在当前路径下的父实体 id（owl:Thing 为 0） */
  parentId: number;
  key: string;
}

interface OntologyClassTreeProps {
  ontologyId: number | null;
  /** 骨架树（默认）还是全量树 */
  showAll: boolean;
  onShowAllChange: (showAll: boolean) => void;
  selectedKey: string | null;
  onSelectNode: (action: TreeNodeAction | null) => void;
  onCreateChild: (action: TreeNodeAction) => void;
  onCreateSibling: (action: TreeNodeAction) => void;
  onEdit: (action: TreeNodeAction) => void;
  onDelete: (action: TreeNodeAction) => void;
}

/**
 * 本体类左树（antd Tree，height 虚拟滚动）：骨架树/全量树双数据源、owl:Thing 虚根、
 * 路径复合 key、+N 角标、deprecated 删除线、本地过滤与展开/折叠。
 */
const OntologyClassTree: React.FC<OntologyClassTreeProps> = ({
  ontologyId,
  showAll,
  onShowAllChange,
  selectedKey,
  onSelectNode,
  onCreateChild,
  onCreateSibling,
  onEdit,
  onDelete,
}) => {
  const { t } = useTranslation('ontology');
  const [filterText, setFilterText] = useState('');
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>([OWL_THING_KEY]);
  const containerRef = useRef<HTMLDivElement>(null);
  const [treeHeight, setTreeHeight] = useState(480);

  // 双数据源：骨架树默认；勾选"全部类"才拉全量树（惰性缓存）
  const navTreeQuery = useOntologyClassNavTree(ontologyId ?? undefined, { enabled: ontologyId != null && !showAll });
  const fullTreeQuery = useOntologyClassTree(ontologyId ?? undefined, { enabled: ontologyId != null && showAll });

  const query = showAll ? fullTreeQuery : navTreeQuery;
  const roots = query.data?.roots ?? [];

  // 骨架视图过滤无命中时提示切"全部类"（叶子类不在骨架树中）
  const isNavMode = !showAll;

  const thingLabel = useMemo(() => {
    // Protégé 惯例显示 owl:Thing；节点不可编辑删除，仅可"新建子类"
    return 'owl:Thing';
  }, []);

  /** 路径复合 key：${parentKey}-${id}；返回节点与其父实体 id */
  const toTreeData = (
    nodes: ontologyservicev1_OntologyClassTreeNode[],
    parentKey: string,
    parentId: number,
    filter: string,
  ): DataNode[] => {
    const out: DataNode[] = [];
    for (const node of nodes) {
      const key = `${parentKey}${KEY_SEPARATOR}${node.id}`;
      const label = node.displayLabel || node.localName || `#${node.id}`;
      const children = toTreeData(node.children ?? [], key, node.id!, filter);

      const matches = !filter || label.toLowerCase().includes(filter.toLowerCase());
      if (filter && children.length === 0 && !matches) {
        continue; // 过滤：保留命中节点与其祖先链
      }

      const hiddenCount = (node.childCount ?? 0) - (node.children?.length ?? 0);
      out.push({
        key,
        title: (
          <span className="ontology-tree-node">
            <span className={node.deprecated ? 'ontology-tree-node-title deprecated' : 'ontology-tree-node-title'}>
              {label}
            </span>
            {hiddenCount > 0 && (
              <Tooltip title={t('classTree.hiddenChildren', { count: hiddenCount })}>
                <Tag className="ontology-tree-node-badge">+{hiddenCount}</Tag>
              </Tooltip>
            )}
            <span className="ontology-tree-node-actions">
              <Tooltip title={t('classTree.createChild')}>
                <PlusOutlined
                  onClick={(e) => {
                    e.stopPropagation();
                    onCreateChild({ id: node.id!, parentId, key });
                  }}
                />
              </Tooltip>
              {parentId > 0 && (
                <Tooltip title={t('classTree.createSibling')}>
                  <SisternodeOutlined
                    onClick={(e) => {
                      e.stopPropagation();
                      onCreateSibling({ id: node.id!, parentId, key });
                    }}
                  />
                </Tooltip>
              )}
              <Tooltip title={t('common:button.edit')}>
                <EditOutlined
                  onClick={(e) => {
                    e.stopPropagation();
                    onEdit({ id: node.id!, parentId, key });
                  }}
                />
              </Tooltip>
              <Tooltip title={t('common:button.delete')}>
                <DeleteOutlined
                  className="ontology-tree-node-delete"
                  onClick={(e) => {
                    e.stopPropagation();
                    onDelete({ id: node.id!, parentId, key });
                  }}
                />
              </Tooltip>
            </span>
          </span>
        ),
        children: children.length > 0 ? children : undefined,
      });
    }
    return out;
  };

  const filteredText = filterText.trim();
  const classNodes = useMemo(
    () => toTreeData(roots, OWL_THING_KEY, 0, filteredText),
    // toTreeData 闭包依赖 action 回调（父组件稳定引用），roots/过滤词为数据键
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [roots, filteredText, onCreateChild, onCreateSibling, onEdit, onDelete],
  );

  const treeData: DataNode[] = useMemo(
    () => [
      {
        key: OWL_THING_KEY,
        title: (
          <span className="ontology-tree-node">
            <Typography.Text type="secondary" italic>
              {thingLabel}
            </Typography.Text>
            <span className="ontology-tree-node-actions">
              <Tooltip title={t('classTree.createChild')}>
                <PlusOutlined
                  onClick={(e) => {
                    e.stopPropagation();
                    onCreateChild({ id: 0, parentId: 0, key: OWL_THING_KEY });
                  }}
                />
              </Tooltip>
            </span>
          </span>
        ),
        children: classNodes,
      },
    ],
    [classNodes, thingLabel, t, onCreateChild],
  );

  // 容器高度自适应（antd Tree height 虚拟滚动需要像素值）
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setTreeHeight(Math.max(el.clientHeight - 96, 240));
    update();
    const observer = new ResizeObserver(update);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const noMatch = filteredText.length > 0 && classNodes.length === 0;

  return (
    <div ref={containerRef} className="ontology-class-tree" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Space style={{ padding: '0 4px 8px', width: '100%', justifyContent: 'space-between' }}>
        <Input.Search
          size="small"
          allowClear
          placeholder={t('classTree.filterPlaceholder')}
          style={{ width: 160 }}
          value={filterText}
          onChange={(e) => setFilterText(e.target.value)}
        />
        <Space size={0}>
          <Tooltip title={t('classTree.showAllClasses')}>
            <Switch size="small" checked={showAll} onChange={onShowAllChange} />
          </Tooltip>
          <Tooltip title={t('classTree.expandAll')}>
            <PlusSquareOutlined
              onClick={() => {
                const keys: React.Key[] = [OWL_THING_KEY];
                const walk = (nodes: DataNode[]) => {
                  for (const n of nodes) {
                    keys.push(n.key);
                    if (n.children) walk(n.children);
                  }
                };
                walk(classNodes);
                setExpandedKeys(keys);
              }}
            />
          </Tooltip>
          <Tooltip title={t('classTree.collapseAll')}>
            <MinusSquareOutlined onClick={() => setExpandedKeys([OWL_THING_KEY])} />
          </Tooltip>
          <Tooltip title={t('classTree.refresh')}>
            <ReloadOutlined onClick={() => query.refetch()} />
          </Tooltip>
        </Space>
      </Space>

      {noMatch ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={isNavMode ? t('classTree.noMatchNavHint') : t('classTree.noMatch')}
        />
      ) : (
        <Tree<DataNode>
          height={treeHeight}
          blockNode
          showLine={{ showLeafIcon: false }}
          treeData={treeData}
          expandedKeys={expandedKeys}
          onExpand={(keys) => setExpandedKeys(keys)}
          selectedKeys={selectedKey ? [selectedKey] : []}
          onSelect={(keys, info) => {
            if (keys.length === 0) {
              onSelectNode(null);
              return;
            }
            const key = String(info.node.key);
            if (key === OWL_THING_KEY) {
              // 虚根仅承担"新建子类"入口，不作为子树范围
              onSelectNode({ id: 0, parentId: 0, key });
              return;
            }
            const segments = key.split(KEY_SEPARATOR);
            const selfId = Number(segments[segments.length - 1]);
            const parentId = Number(segments[segments.length - 2] ?? 0) || 0;
            onSelectNode({ id: selfId, parentId, key });
          }}
        />
      )}
    </div>
  );
};

export default OntologyClassTree;
