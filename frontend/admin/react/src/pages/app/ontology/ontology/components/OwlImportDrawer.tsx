import { useMemo, useState } from 'react';
import { Alert, App, Button, Checkbox, Collapse, Descriptions, Drawer, Input, Steps, Table, Typography, Upload } from 'antd';
import { InboxOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';

import type { ontologyservicev1_OwlImportExecuteRequest, ontologyservicev1_OwlImportReport } from '@/api/generated/admin/service/v1';
import { bytesToBase64, useOwlImportExecute, useOwlImportPreview } from '@/api/hooks/ontologyOwlIO';

/**
 * OWL 导入向导（07 期，07 号文档 §6.2）：四步——上传 → 预检报告 → 基本信息 → 执行。
 * 忽略清单确认纪律：ignored.count > 0 时必须勾选确认方可执行（两段式确认）。
 */
interface OwlImportDrawerProps {
  open: boolean;
  onClose: () => void;
  onSuccess: (ontologyId: number) => void;
}

const AXIOM_TYPE_LABELS: Record<string, string> = {
  '10': '实体声明',
  '20': '子类',
  '21': '不相交类',
  '22': '等价类',
  '30': '子对象属性',
  '31': '对象属性定义域',
  '32': '对象属性值域',
  '33': '对象属性特征',
  '34': '逆属性',
  '35': '等价对象属性',
  '36': '不相交对象属性',
  '37': '属性链',
  '40': '子数据属性',
  '41': '数据属性定义域',
  '42': '数据属性值域',
  '43': '数据属性特征',
  '44': '等价数据属性',
  '45': '不相交数据属性',
  '46': '数据类型定义',
  '50': '子注解属性',
  '60': '类型断言',
  '61': '对象属性断言',
  '62': '数据属性断言',
  '100': '注解断言',
};

const ENTITY_TYPE_LABELS: Record<string, string> = {
  CLASS: '类',
  OBJECT_PROPERTY: '对象属性',
  DATA_PROPERTY: '数据属性',
  ANNOTATION_PROPERTY: '注解属性',
  DATATYPE: '数据类型',
  INDIVIDUAL: '个体',
};

interface StatRow {
  type: string;
  label: string;
  count: number;
}

const statColumns: ColumnsType<StatRow> = [
  { title: '类型', dataIndex: 'label' },
  { title: '行数', dataIndex: 'count', width: 100, align: 'right' },
];

const OwlImportDrawer = ({ open, onClose, onSuccess }: OwlImportDrawerProps) => {
  const { t } = useTranslation('ontology');
  const { message } = App.useApp();
  const queryClient = useQueryClient();

  const [current, setCurrent] = useState(0);
  const [previewing, setPreviewing] = useState(false);
  const [report, setReport] = useState<ontologyservicev1_OwlImportReport | null>(null);
  const [confirmedIgnore, setConfirmedIgnore] = useState(false);
  const [name, setName] = useState('');
  const [baseIri, setBaseIri] = useState('');
  const [ontologyIri, setOntologyIri] = useState('');
  const [description, setDescription] = useState('');

  const previewMutation = useOwlImportPreview({
    onSuccess: (data) => {
      setReport(data);
      setName(data.suggested?.ontologyName ?? '');
      setBaseIri(data.suggested?.baseIri ?? '');
      setOntologyIri(data.suggested?.ontologyIri ?? '');
      setDescription(data.suggested?.description ?? '');
      setConfirmedIgnore(false);
      setCurrent(1);
    },
  });

  const executeMutation = useOwlImportExecute({
    onSuccess: (data) => {
      message.success(t('owl.import.executeSuccess'));
      queryClient.invalidateQueries({ queryKey: ['simpleOntologies'] });
      queryClient.invalidateQueries({ queryKey: ['listOntologies'] });
      onSuccess(data.ontologyId ?? 0);
      handleClose();
    },
  });

  const handleClose = () => {
    setCurrent(0);
    setReport(null);
    setConfirmedIgnore(false);
    setName('');
    setBaseIri('');
    setOntologyIri('');
    setDescription('');
    onClose();
  };

  const handleBeforeUpload = async (file: File) => {
    setPreviewing(true);
    try {
      const b64 = bytesToBase64(await file.arrayBuffer());
      await previewMutation.mutateAsync({ fileName: file.name, contentBase64: b64 });
    } catch (error) {
      console.error('owl import preview failed:', error);
      message.error((error as Error).message || t('owl.import.previewFailed'));
    } finally {
      setPreviewing(false);
    }
    return false; // 手动控制：beforeUpload 返回 false，不自动上传
  };

  const handleExecute = () => {
    if (!report?.reportId) return;
    if (!name.trim() || !baseIri.trim()) {
      message.warning(t('owl.import.requiredFields'));
      return;
    }
    const req: ontologyservicev1_OwlImportExecuteRequest = {
      reportId: report.reportId,
      ontologyName: name.trim(),
      baseIri: baseIri.trim(),
      ontologyIri: ontologyIri.trim(),
      description,
      confirmedIgnore,
    };
    executeMutation.mutate(req);
  };

  const entityRows = useMemo<StatRow[]>(() => {
    const stats = report?.entityStats ?? {};
    return Object.entries(stats)
      .filter(([, count]) => count > 0)
      .map(([type, count]) => ({ type, label: ENTITY_TYPE_LABELS[type] ?? type, count }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [report]);

  const axiomRows = useMemo<StatRow[]>(() => {
    const stats = report?.axiomStats ?? {};
    return Object.entries(stats)
      .filter(([, count]) => count > 0)
      .map(([type, count]) => ({ type, label: AXIOM_TYPE_LABELS[type] ?? type, count }))
      .sort((a, b) => Number(a.type) - Number(b.type));
  }, [report]);

  const totalEntities = entityRows.reduce((acc, r) => acc + r.count, 0);
  const totalAxioms = axiomRows.reduce((acc, r) => acc + r.count, 0);
  const ignoredCount = report?.ignored?.count ?? 0;
  const ignoredConfirmable = ignoredCount === 0 || confirmedIgnore;

  const collapseItems = [
    entityRows.length > 0 && {
      key: 'entities',
      label: t('owl.import.entityStats', { count: entityRows.length }),
      children: <Table size="small" rowKey="type" columns={statColumns} dataSource={entityRows} pagination={false} />,
    },
    (report?.autoDeclared?.length ?? 0) > 0 && {
      key: 'auto',
      label: t('owl.import.autoDeclared', { count: report?.autoDeclared?.length ?? 0 }),
      children: (
        <ul style={{ margin: 0, paddingLeft: 16 }}>
          {report?.autoDeclared?.slice(0, 50).map((a) => (
            <li key={a.iri}>
              <code>{a.iri}</code> → {ENTITY_TYPE_LABELS[a.entityType ?? ''] ?? a.entityType}（{a.reason}）
            </li>
          ))}
          {(report?.autoDeclared?.length ?? 0) > 50 && <li>…</li>}
        </ul>
      ),
    },
    (report?.renamedLocalNames?.length ?? 0) > 0 && {
      key: 'renamed',
      label: t('owl.import.renamedLocalNames', { count: report?.renamedLocalNames?.length ?? 0 }),
      children: (
        <ul style={{ margin: 0, paddingLeft: 16 }}>
          {report?.renamedLocalNames?.slice(0, 50).map((r) => (
            <li key={r.iri}>
              <code>{r.iri}</code> → {r.localName}（{t('owl.import.original')}: {r.original}）
            </li>
          ))}
        </ul>
      ),
    },
    (report?.warnings?.length ?? 0) > 0 && {
      key: 'warnings',
      label: t('owl.import.warnings', { count: report?.warnings?.length ?? 0 }),
      children: (
        <ul style={{ margin: 0, paddingLeft: 16 }}>
          {report?.warnings?.slice(0, 50).map((w, i) => (
            <li key={i}>{w}</li>
          ))}
        </ul>
      ),
    },
    ignoredCount > 0 && {
      key: 'ignored',
      label: <span style={{ color: 'var(--ant-color-warning)' }}>{t('owl.import.ignored', { count: ignoredCount })}</span>,
      children: (
        <div>
          <ul style={{ margin: 0, paddingLeft: 16 }}>
            {Object.entries(report?.ignored?.byReason ?? {}).map(([reason, count]) => (
              <li key={reason}>
                {reason} × {count}
              </li>
            ))}
          </ul>
          <ul style={{ margin: '8px 0 0', paddingLeft: 16 }}>
            {report?.ignored?.samples?.slice(0, 20).map((s, i) => (
              <li key={i} style={{ color: 'var(--ant-color-text-tertiary)' }}>
                {s.reason}：{s.detail}
              </li>
            ))}
          </ul>
        </div>
      ),
    },
  ].filter(Boolean) as { key: string; label: React.ReactNode; children: React.ReactNode }[];

  const stepsItems = [
    { title: t('owl.import.stepUpload') },
    { title: t('owl.import.stepReport') },
    { title: t('owl.import.stepMeta') },
    { title: t('owl.import.stepExecute') },
  ];

  return (
    <Drawer open={open} onClose={handleClose} width={720} title={t('owl.import.title')} destroyOnHidden>
      <Steps current={current} items={stepsItems} size="small" style={{ marginBottom: 24 }} />

      {current === 0 && (
        <div>
          <Alert
            type="info"
            showIcon
            message={t('owl.import.uploadHint')}
            description={t('owl.import.uploadLimit')}
            style={{ marginBottom: 16 }}
          />
          <Upload.Dragger
            accept=".owl,.rdf,.ttl,.xml"
            maxCount={1}
            showUploadList={false}
            disabled={previewing}
            beforeUpload={(file) => handleBeforeUpload(file as File)}
          >
            <p className="ant-upload-drag-icon">
              <InboxOutlined />
            </p>
            <p className="ant-upload-text">{t('owl.import.uploadText')}</p>
          </Upload.Dragger>
          {previewing && (
            <Typography.Text type="secondary">{t('owl.import.parsing')}</Typography.Text>
          )}
        </div>
      )}

      {current === 1 && report && (
        <div>
          <Alert
            type={ignoredCount > 0 ? 'warning' : 'success'}
            showIcon
            message={t('owl.import.reportSummary', {
              format: report.sourceFormat ?? '',
              entities: totalEntities,
              axioms: totalAxioms,
              ignored: ignoredCount,
            })}
            style={{ marginBottom: 16 }}
          />

          <Descriptions size="small" column={2} style={{ marginBottom: 16 }}>
            <Descriptions.Item label={t('owl.import.suggestedName')}>{report.suggested?.ontologyName}</Descriptions.Item>
            <Descriptions.Item label={t('owl.import.suggestedBaseIri')}>{report.suggested?.baseIri}</Descriptions.Item>
          </Descriptions>

          <Table
            size="small"
            rowKey="type"
            columns={statColumns}
            dataSource={axiomRows}
            pagination={false}
            style={{ marginBottom: 8 }}
          />

          <Collapse items={collapseItems} />

          {ignoredCount > 0 && (
            <Checkbox checked={confirmedIgnore} onChange={(e) => setConfirmedIgnore(e.target.checked)} style={{ marginTop: 16 }}>
              {t('owl.import.confirmIgnore', { count: ignoredCount })}
            </Checkbox>
          )}

          <div style={{ marginTop: 24, display: 'flex', justifyContent: 'space-between' }}>
            <Button onClick={() => setCurrent(0)}>{t('owl.import.back')}</Button>
            <Button type="primary" disabled={!ignoredConfirmable} onClick={() => setCurrent(2)}>
              {t('owl.import.next')}
            </Button>
          </div>
        </div>
      )}

      {current === 2 && (
        <div>
          <Alert type="info" showIcon message={t('owl.import.metaHint')} style={{ marginBottom: 16 }} />
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('ontology.ontologyNamePlaceholder')}
            style={{ marginBottom: 12 }}
            addonBefore={t('ontology.ontologyName')}
          />
          <Input
            value={ontologyIri}
            onChange={(e) => setOntologyIri(e.target.value)}
            placeholder={t('ontology.ontologyIriPlaceholder')}
            style={{ marginBottom: 12 }}
            addonBefore={t('ontology.ontologyIri')}
          />
          <Input
            value={baseIri}
            onChange={(e) => setBaseIri(e.target.value)}
            placeholder={t('ontology.baseIriPlaceholder')}
            style={{ marginBottom: 12 }}
            addonBefore={t('ontology.baseIri')}
          />
          <Input.TextArea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t('ontology.descriptionPlaceholder')}
            rows={2}
          />
          <div style={{ marginTop: 24, display: 'flex', justifyContent: 'space-between' }}>
            <Button onClick={() => setCurrent(1)}>{t('owl.import.back')}</Button>
            <Button type="primary" onClick={() => setCurrent(3)}>
              {t('owl.import.next')}
            </Button>
          </div>
        </div>
      )}

      {current === 3 && (
        <div>
          <Alert
            type="warning"
            showIcon
            message={t('owl.import.executeConfirm')}
            description={t('owl.import.executeSummary', {
              entities: totalEntities,
              axioms: totalAxioms,
              ignored: ignoredCount,
            })}
            style={{ marginBottom: 16 }}
          />
          <Descriptions size="small" column={1} style={{ marginBottom: 16 }}>
            <Descriptions.Item label={t('ontology.ontologyName')}>{name}</Descriptions.Item>
            <Descriptions.Item label={t('ontology.ontologyIri')}>{ontologyIri || '—'}</Descriptions.Item>
            <Descriptions.Item label={t('ontology.baseIri')}>{baseIri}</Descriptions.Item>
            {description && <Descriptions.Item label={t('ontology.description')}>{description}</Descriptions.Item>}
          </Descriptions>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Button onClick={() => setCurrent(2)} disabled={executeMutation.isPending}>
              {t('owl.import.back')}
            </Button>
            <Button type="primary" loading={executeMutation.isPending} onClick={handleExecute}>
              {t('owl.import.execute')}
            </Button>
          </div>
        </div>
      )}
    </Drawer>
  );
};

export default OwlImportDrawer;
