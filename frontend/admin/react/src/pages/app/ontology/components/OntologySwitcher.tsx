import { useEffect } from 'react';
import { Button, Select, Space } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';

import { useOntologyStore } from '@/stores';

/**
 * 活动本体切换器：本体文档页与本体类页顶部共用。
 *
 * 活动本体 = 前端全局上下文（zustand + localStorage，见 stores/ontology.ts）；
 * 所有实体级 API 显式携带 ontologyId，服务端无状态、天然支持多标签页。
 * 写入统一走 onChange → setCurrentOntologyId，切换后各页订阅 currentOntologyId 变化重新加载。
 */
export function OntologySwitcher() {
  const { t } = useTranslation('ontology');
  const currentOntologyId = useOntologyStore((s) => s.currentOntologyId);
  const ontologyList = useOntologyStore((s) => s.ontologyList);
  const loading = useOntologyStore((s) => s.loading);
  const loaded = useOntologyStore((s) => s.loaded);
  const loadOntologyList = useOntologyStore((s) => s.loadOntologyList);
  const setCurrentOntologyId = useOntologyStore((s) => s.setCurrentOntologyId);

  useEffect(() => {
    loadOntologyList();
  }, [loadOntologyList]);

  return (
    <Space.Compact>
      <Select<number | undefined>
        style={{ minWidth: 220 }}
        placeholder={t('ontologySwitcher.placeholder')}
        value={currentOntologyId ?? undefined}
        loading={loading && !loaded}
        allowClear={false}
        onChange={(value) => setCurrentOntologyId(value ?? null)}
        options={(ontologyList ?? []).map((item) => ({
          value: item.id!,
          label: item.ontologyName ?? `#${item.id}`,
        }))}
      />
      <Button
        icon={<ReloadOutlined />}
        title={t('ontologySwitcher.refresh')}
        onClick={() => loadOntologyList(true)}
      />
    </Space.Compact>
  );
}

export default OntologySwitcher;
