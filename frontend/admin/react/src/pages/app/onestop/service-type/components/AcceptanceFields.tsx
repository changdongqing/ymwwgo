import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Input, Select, Switch } from 'antd';
import type { onestopservicev1_ServiceAcceptanceItem } from '@/api/generated/admin/service/v1';
import { ACCEPTANCE_MODE_OPTIONS } from '../constants';

/** 验收项清单编辑器：每项 = 必检开关 + 名称 + 删除；底部自由输入添加 */
export function AcceptanceItemsEditor({ value = [], onChange }: {
  value?: onestopservicev1_ServiceAcceptanceItem[];
  onChange?: (items: onestopservicev1_ServiceAcceptanceItem[]) => void;
}) {
  const { t } = useTranslation('onestop');
  const [adding, setAdding] = useState('');

  const existing = new Set(value.map((item) => item.name));
  const addItem = () => {
    const name = adding.trim();
    if (name && !existing.has(name)) {
      onChange?.([...value, { name, required: true }]);
    }
    setAdding('');
  };

  return (
    <div style={{ width: '100%' }}>
      {value.length === 0 && (
        <div style={{ color: 'var(--ant-color-text-tertiary)', marginBottom: 8 }}>
          {t('acceptance.emptyItems')}
        </div>
      )}
      {value.map((item, index) => (
        <div
          key={`${item.name}-${index}`}
          style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}
        >
          <Switch
            size="small"
            checked={item.required}
            onChange={(checked) => {
              const next = [...value];
              next[index] = { ...item, required: checked };
              onChange?.(next);
            }}
          />
          <Input
            value={item.name}
            onChange={(e) => {
              const next = [...value];
              next[index] = { ...item, name: e.target.value };
              onChange?.(next);
            }}
            style={{ flex: 1 }}
            maxLength={64}
          />
          <Button
            type="text"
            danger
            size="small"
            onClick={() => onChange?.(value.filter((_, i) => i !== index))}
          >
            {t('delete')}
          </Button>
        </div>
      ))}
      <div style={{ display: 'flex', gap: 8 }}>
        <Input
          style={{ flex: 1 }}
          value={adding}
          maxLength={64}
          placeholder={t('acceptance.addItemPlaceholder')}
          onChange={(e) => setAdding(e.target.value)}
          onPressEnter={addItem}
        />
        <Button onClick={addItem}>{t('acceptance.addItem')}</Button>
      </div>
    </div>
  );
}

/** 验收方式多选（值域受控，与后端服务层校验一致） */
export function AcceptanceModesSelect({ value, onChange }: {
  value?: string[];
  onChange?: (modes: string[]) => void;
}) {
  const { t } = useTranslation('onestop');
  return (
    <Select
      mode="multiple"
      allowClear
      options={ACCEPTANCE_MODE_OPTIONS}
      value={value}
      onChange={onChange}
      placeholder={t('acceptance.modesPlaceholder')}
      style={{ width: '100%' }}
    />
  );
}
