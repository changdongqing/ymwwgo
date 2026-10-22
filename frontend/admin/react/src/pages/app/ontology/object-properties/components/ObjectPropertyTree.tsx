import { useEffect, useMemo, useRef, useState } from 'react';
import { Empty, Input, Space, Tooltip, Tree, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import {
  DeleteOutlined,
  EditOutlined,
  MinusSquareOutlined,
  PlusOutlined,
  PlusSquareOutlined,
  ReloadOutlined,
  SisternodeOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useOntologyObjectPropertyTree } from '@/api/hooks/ontologyObjectProperty';
import type { ontologyservicev1_ObjectPropertyTreeNode } from '@/api/generated/admin/service/v1';

import { OWL_TOP_OBJECT_PROPERTY_KEY } from '../constants';

/** 树节点路径复合 key 的分隔符（多父下同一实体多现，直接用 id 会 key 冲突致展开状态错乱） */
const KEY_SEPARATOR = '-';

export interface TreeNodeAction {
  /** 命中的实体 id；虚拟根为 0 */
  id: number;
  /** 该节点在当前路径下的父实体 id（虚拟根为 0） */
  parentId: number;
  key: string;
}

interface ObjectPropertyTreeProps {
  ontologyId: number | null;
  selectedKey: string | null;
  onSelectNode: (action: TreeNodeAction | null) => void;
  onCreateChild: (action: TreeNodeAction) => void;
  onCreateSibling: (action: TreeNodeAction) => void;
  onEdit: (action: TreeNodeAction) => void;
  onDelete: (action: TreeNodeAction) => void;
}

/**
 * 对象属性左树（antd Tree，height 虚拟滚动）：全量单数据源（无骨架剪枝）、
 * owl:topObjectProperty 虚根、路径复合 key、deprecated 删除线、本地过滤与展开/折叠。
 */
const ObjectPropertyTree: React.FC<ObjectPropertyTreeProps> = ({
  ontologyId,
  selectedKey,
  onSelectNode,
  onCreateChild,
  onCreateSibling,
  onEdit,
  onDelete,
}) => {
  const { t } = useTranslation('ontology');
  const [filterText, setFilterText] = useState('');
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>([OWL_TOP_OBJECT_PROPERTY_KEY]);
  const containerRef = useRef<HTMLDivElement>(null);
  const [treeHeight, setTreeHeight] = useState(480);

  const treeQuery = useOntologyObjectPropertyTree(ontologyId ?? undefined, { enabled: ontologyId != null });
  const roots = treeQuery.data?.roots ?? [];

  /** 路径复合 key：${parentKey}-${id}；返回节点与其父实体 id */
  const toTreeData = (
    nodes: ontologyservicev1_ObjectPropertyTreeNode[],
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

      out.push({
        key,
        title: (
          <span className="ontology-tree-node">
            <span className={node.deprecated ? 'ontology-tree-node-title deprecated' : 'ontology-tree-node-title'}>
              {label}
            </span>
            <span className="ontology-tree-node-actions">
              <Tooltip title={t('objectPropertyTree.createChild')}>
                <PlusOutlined
                  onClick={(e) => {
                    e.stopPropagation();
                    onCreateChild({ id: node.id!, parentId, key });
                  }}
                />
              </Tooltip>
              {parentId > 0 && (
                <Tooltip title={t('objectPropertyTree.createSibling')}>
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
  const propertyNodes = useMemo(
    () => toTreeData(roots, OWL_TOP_OBJECT_PROPERTY_KEY, 0, filteredText),
    // toTreeData 闭包依赖 action 回调（父组件稳定引用），roots/过滤词为数据键
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [roots, filteredText, onCreateChild, onCreateSibling, onEdit, onDelete],
  );

  const treeData: DataNode[] = useMemo(
    () => [
      {
        key: OWL_TOP_OBJECT_PROPERTY_KEY,
        title: (
          <span className="ontology-tree-node">
            <Typography.Text type="secondary" italic>
              owl:topObjectProperty
            </Typography.Text>
            <span className="ontology-tree-node-actions">
              <Tooltip title={t('objectPropertyTree.createChild')}>
                <PlusOutlined
                  onClick={(e) => {
                    e.stopPropagation();
                    onCreateChild({ id: 0, parentId: 0, key: OWL_TOP_OBJECT_PROPERTY_KEY });
                  }}
                />
              </Tooltip>
            </span>
          </span>
        ),
        children: propertyNodes,
      },
    ],
    [propertyNodes, t, onCreateChild],
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

  const noMatch = filteredText.length > 0 && propertyNodes.length === 0;

  return (
    <div ref={containerRef} className="ontology-class-tree" style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Space style={{ padding: '0 4px 8px', width: '100%', justifyContent: 'space-between' }}>
        <Input.Search
          size="small"
          allowClear
          placeholder={t('objectPropertyTree.filterPlaceholder')}
          style={{ width: 160 }}
          value={filterText}
          onChange={(e) => setFilterText(e.target.value)}
        />
        <Space size={0}>
          <Tooltip title={t('objectPropertyTree.expandAll')}>
            <PlusSquareOutlined
              onClick={() => {
                const keys: React.Key[] = [OWL_TOP_OBJECT_PROPERTY_KEY];
                const walk = (nodes: DataNode[]) => {
                  for (const n of nodes) {
                    keys.push(n.key);
                    if (n.children) walk(n.children);
                  }
                };
                walk(propertyNodes);
                setExpandedKeys(keys);
              }}
            />
          </Tooltip>
          <Tooltip title={t('objectPropertyTree.collapseAll')}>
            <MinusSquareOutlined onClick={() => setExpandedKeys([OWL_TOP_OBJECT_PROPERTY_KEY])} />
          </Tooltip>
          <Tooltip title={t('objectPropertyTree.refresh')}>
            <ReloadOutlined onClick={() => treeQuery.refetch()} />
          </Tooltip>
        </Space>
      </Space>

      {noMatch ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t('objectPropertyTree.noMatch')} />
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
            if (key === OWL_TOP_OBJECT_PROPERTY_KEY) {
              // 虚根仅承担"新建子属性"入口，不作为子树范围
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

export default ObjectPropertyTree;
