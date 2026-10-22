import { useMemo } from 'react';
import { Spin, Tag, Tree, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useTranslation } from 'react-i18next';

import type { ontologyservicev1_OntologyClassTreeNode } from '@/api/generated/admin/service/v1';
import { useIndividualsCountByClass } from '@/api/hooks/ontologyIndividual';
import { useOntologyClassTree } from '@/api/hooks/ontologyClass';

export interface ClassTreeNodeAction {
  id: number;
  label: string;
  /** 路径复合 key（多继承同实体多现，避免 key 冲突——与类树同规则） */
  key: string;
}

interface IndividualClassTreeProps {
  ontologyId: number;
  selectedKey: string | null;
  onSelectNode: (action: ClassTreeNodeAction | null) => void;
}

/**
 * 个体页左类树（只读导航，对照 Protégé "Individuals by class"）：
 * 复用 01 期类树数据（useOntologyClassTree 全量树，owl:Thing 虚拟根不入库），无节点操作按钮；
 * 节点尾缀直接实例数徽标（count-by-class，对照 CountingOWLObjectTreeCellRenderer——06 号文档 §6.1）。
 */
const IndividualClassTree: React.FC<IndividualClassTreeProps> = ({ ontologyId, selectedKey, onSelectNode }) => {
  const { t } = useTranslation('ontology');
  const treeQuery = useOntologyClassTree(ontologyId);
  const countQuery = useIndividualsCountByClass({ ontologyId });
  const roots = treeQuery.data?.roots ?? [];

  const countByClassId = useMemo(() => {
    const map = new Map<number, number>();
    for (const item of countQuery.data?.items ?? []) {
      if (item.classId != null && item.directCount != null) {
        map.set(item.classId, item.directCount);
      }
    }
    return map;
  }, [countQuery.data]);

  /** key → 节点信息（选择回调反解 label 用） */
  const nodeByKey = useMemo(() => {
    const map = new Map<string, { id: number; label: string }>();
    const walk = (nodes: ontologyservicev1_OntologyClassTreeNode[], parentKey: string) => {
      for (const n of nodes) {
        const key = parentKey ? `${parentKey}-${n.id}` : `${n.id}`;
        map.set(key, { id: n.id!, label: n.displayLabel || n.localName || `#${n.id}` });
        walk((n.children ?? []) as ontologyservicev1_OntologyClassTreeNode[], key);
      }
    };
    walk(roots, '');
    return map;
  }, [roots]);

  const treeData = useMemo<DataNode[]>(() => {
    const toNode = (n: ontologyservicev1_OntologyClassTreeNode, parentKey: string): DataNode => {
      const key = parentKey ? `${parentKey}-${n.id}` : `${n.id}`;
      const count = countByClassId.get(n.id!) ?? 0;
      return {
        key,
        title: (
          <span className="ontology-individual-tree-node">
            <Typography.Text delete={!!n.deprecated} type={n.deprecated ? 'secondary' : undefined}>
              {n.displayLabel || n.localName || `#${n.id}`}
            </Typography.Text>
            {count > 0 && (
              <Tag className="ontology-individual-tree-badge" title={t('individual.classBadgeTip')}>
                {count}
              </Tag>
            )}
          </span>
        ),
        children: (n.children ?? []).map((c) => toNode(c, key)),
      };
    };
    return roots.map((root) => toNode(root, ''));
  }, [roots, countByClassId, t]);

  return (
    <div className="ontology-individual-tree">
      <div className="ontology-individual-tree-header">{t('individual.classTreeTitle')}</div>
      {treeQuery.isLoading ? (
        <Spin className="ontology-individual-tree-loading" />
      ) : (
        <Tree<DataNode>
          blockNode
          showLine={{ showLeafIcon: false }}
          defaultExpandAll
          treeData={treeData}
          selectedKeys={selectedKey != null ? [selectedKey] : []}
          onSelect={(keys) => {
            if (keys.length === 0) {
              onSelectNode(null);
              return;
            }
            const info = nodeByKey.get(String(keys[0]));
            if (info) {
              onSelectNode({ id: info.id, label: info.label, key: String(keys[0]) });
            } else {
              onSelectNode(null);
            }
          }}
        />
      )}
    </div>
  );
};

export default IndividualClassTree;
