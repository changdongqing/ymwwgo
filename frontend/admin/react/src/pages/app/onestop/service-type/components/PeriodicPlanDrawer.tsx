import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { App, AutoComplete, Button, DatePicker, Drawer, Form, Input, Select, Switch, TimePicker } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { useQueryClient } from '@tanstack/react-query';
import {
  type onestopservicev1_PeriodicPlan,
  type onestopservicev1_PeriodicPlanItem,
} from '@/api/generated/admin/service/v1';
import {
  useCreatePeriodicPlan,
  useUpdatePeriodicPlan,
  invalidatePeriodicPlans,
} from '@/api/hooks/onestop';
import { type FreqTypeValue, INSPECTION_POST_SUGGESTIONS } from '../constants';

interface PeriodicPlanDrawerProps {
  open: boolean;
  mode: 'create' | 'edit';
  /** 所属类别（0103 子树节点） */
  serviceTypeId: number;
  serviceTypeLabel: string;
  /** 编辑时的计划数据 */
  plan?: onestopservicev1_PeriodicPlan;
  /** 类别标准作业项目库（点表「从标准项目选择」的来源） */
  standardItems: string[];
  onSaved: () => void;
  onClose: () => void;
}

const FREQ_OPTIONS: { label: string; value: FreqTypeValue }[] = [
  { label: '每日', value: 'FREQ_DAILY' },
  { label: '每周', value: 'FREQ_WEEKLY' },
  { label: '每月', value: 'FREQ_MONTHLY' },
  { label: '每年', value: 'FREQ_YEARLY' },
];

const WEEKDAY_OPTIONS = [1, 2, 3, 4, 5, 6, 7].map((d) => ({
  label: ['周一', '周二', '周三', '周四', '周五', '周六', '周日'][d - 1],
  value: d,
}));

const MONTH_DAY_OPTIONS = [
  ...Array.from({ length: 31 }, (_, i) => ({ label: `${i + 1}号`, value: i + 1 })),
  { label: '月末（当月最后一天）', value: -1 },
];

const MONTH_OPTIONS = Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: i + 1 }));

/** 执行时间点列表编辑器：每个执行日按列表内时间点各执行一次（每日多次 = 多个时间点） */
function TimesEditor({
  value = [],
  onChange,
  placeholder,
}: {
  value?: string[];
  onChange?: (times: string[]) => void;
  placeholder: string;
}) {
  const times = value.length > 0 ? value : [''];
  const emit = (next: string[]) => onChange?.(next);

  return (
    <div style={{ width: '100%' }}>
      {times.map((tm, index) => (
        <div key={index} style={{ display: 'flex', gap: 8, marginBottom: 6 }}>
          <TimePicker
            format="HH:mm"
            minuteStep={15}
            value={tm ? dayjs(tm, 'HH:mm') : null}
            onChange={(d: Dayjs | null) => {
              const next = [...times];
              next[index] = d ? d.format('HH:mm') : '';
              emit(next.filter((x, i) => x !== '' || i === 0));
            }}
            style={{ width: 140 }}
            placeholder={placeholder}
          />
          {times.length > 1 && (
            <Button
              type="text"
              danger
              size="small"
              onClick={() => emit(times.filter((_, i) => i !== index))}
            >
              删除
            </Button>
          )}
        </div>
      ))}
      <Button
        size="small"
        onClick={() => emit([...times, ''])}
      >
        + 添加时间点
      </Button>
    </div>
  );
}

/** 点表编辑器：基于类别标准项目增（含自定义）/删/逐项停用启用 */
function ItemListEditor({
  value = [],
  onChange,
  standardItems,
  standardLabel,
  emptyLabel,
}: {
  value?: onestopservicev1_PeriodicPlanItem[];
  onChange?: (items: onestopservicev1_PeriodicPlanItem[]) => void;
  standardItems: string[];
  standardLabel: string;
  emptyLabel: string;
}) {
  const [adding, setAdding] = useState('');

  const existing = useMemo(() => new Set(value.map((item) => item.name)), [value]);
  const importOptions = useMemo(
    () => standardItems.filter((name) => !existing.has(name)).map((name) => ({ value: name })),
    [standardItems, existing],
  );

  const addItem = () => {
    const name = adding.trim();
    if (name && !existing.has(name)) {
      onChange?.([...value, { name, enabled: true }]);
    }
    setAdding('');
  };

  return (
    <div style={{ width: '100%' }}>
      {value.length === 0 && (
        <div style={{ color: 'var(--ant-color-text-tertiary)', marginBottom: 8 }}>{emptyLabel}</div>
      )}
      {value.map((item, index) => (
        <div
          key={`${item.name}-${index}`}
          style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}
        >
          <Switch
            size="small"
            checked={item.enabled}
            onChange={(checked) => {
              const next = [...value];
              next[index] = { ...item, enabled: checked };
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
            style={{ flex: 1, opacity: item.enabled ? 1 : 0.55 }}
          />
          <Button
            type="text"
            danger
            size="small"
            onClick={() => onChange?.(value.filter((_, i) => i !== index))}
          >
            删除
          </Button>
        </div>
      ))}
      <div style={{ display: 'flex', gap: 8 }}>
        <AutoComplete
          style={{ flex: 1 }}
          value={adding || undefined}
          options={importOptions}
          filterOption
          placeholder={standardLabel}
          onChange={(v) => setAdding(v ?? '')}
          onSelect={(v: string) => {
            if (!existing.has(v)) {
              onChange?.([...value, { name: v, enabled: true }]);
            }
            setAdding('');
          }}
        />
        <Button onClick={addItem}>添加</Button>
      </div>
    </div>
  );
}

/** 表单数据形态 */
interface PlanFormValues {
  name: string;
  post?: string;
  freqType: FreqTypeValue;
  times: string[];
  weekdays?: number[];
  monthDays?: number[];
  months?: number[];
  clampToMonthEnd: boolean;
  excludedDates?: Dayjs[];
  range?: [Dayjs | null, Dayjs | null] | null;
  isEnabled: boolean;
  spaceItems: onestopservicev1_PeriodicPlanItem[];
  equipmentItems: onestopservicev1_PeriodicPlanItem[];
}

/**
 * 周期计划编辑抽屉：频率（每日/每周/每月/每年 + 排除日期）、岗位、
 * 生效区间、空间/设备点表。空间/设备范围字段本期预留（对象模块未接入）。
 */
export default function PeriodicPlanDrawer({
  open,
  mode,
  serviceTypeId,
  serviceTypeLabel,
  plan,
  standardItems,
  onSaved,
  onClose,
}: PeriodicPlanDrawerProps) {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();
  const [freqType, setFreqType] = useState<FreqTypeValue>('FREQ_DAILY');

  useEffect(() => {
    if (!open) {
      return;
    }
    let values: PlanFormValues;
    if (mode === 'edit' && plan) {
      values = {
        name: plan.name ?? '',
        post: plan.post,
        freqType: (plan.freqType ?? 'FREQ_DAILY') as FreqTypeValue,
        times: plan.times && plan.times.length > 0 ? plan.times : ['09:00'],
        weekdays: plan.weekdays ?? [],
        monthDays: plan.monthDays ?? [],
        months: plan.months ?? [],
        clampToMonthEnd: plan.clampToMonthEnd ?? true,
        excludedDates: (plan.excludedDates ?? []).map((d) => dayjs(d)),
        range:
          plan.effectiveFrom || plan.effectiveTo
            ? [
                plan.effectiveFrom ? dayjs(plan.effectiveFrom) : null,
                plan.effectiveTo ? dayjs(plan.effectiveTo) : null,
              ]
            : null,
        isEnabled: plan.isEnabled ?? true,
        spaceItems: plan.spaceItems ?? [],
        equipmentItems: plan.equipmentItems ?? [],
      };
    } else {
      values = {
        name: '',
        freqType: 'FREQ_DAILY',
        times: ['09:00'],
        clampToMonthEnd: true,
        excludedDates: [],
        isEnabled: true,
        spaceItems: [],
        equipmentItems: [],
      };
    }
    setFreqType(values.freqType);
    form.setFieldsValue(values);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, mode, plan]);

  const createMutation = useCreatePeriodicPlan({
    onSuccess: () => {
      message.success(t('plan.createSuccess'));
      invalidatePeriodicPlans(queryClient);
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('plan.createFailed')),
  });

  const updateMutation = useUpdatePeriodicPlan({
    onSuccess: () => {
      message.success(t('plan.updateSuccess'));
      invalidatePeriodicPlans(queryClient);
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('plan.updateFailed')),
  });

  const handleSubmit = async (values: PlanFormValues) => {
    // 时间点归一（过滤空行）；周期无关的数组按空提交（mask 语义：置空），
    // 避免切换周期后残留旧字段
    const times = (values.times ?? [])
      .map((tm) => (typeof tm === 'string' ? tm : dayjs(tm as unknown as Dayjs).format('HH:mm')))
      .filter((tm) => !!tm);
    const payload: Partial<onestopservicev1_PeriodicPlan> = {
      name: values.name,
      post: values.post || undefined,
      serviceTypeId,
      freqType: values.freqType,
      times,
      weekdays: values.freqType === 'FREQ_WEEKLY' ? (values.weekdays ?? []) : [],
      monthDays:
        values.freqType === 'FREQ_MONTHLY' || values.freqType === 'FREQ_YEARLY'
          ? (values.monthDays ?? [])
          : [],
      months: values.freqType === 'FREQ_YEARLY' ? (values.months ?? []) : [],
      clampToMonthEnd: values.clampToMonthEnd,
      excludedDates: (values.excludedDates ?? []).map((d) => d.format('YYYY-MM-DD')),
      spaceItems: values.spaceItems ?? [],
      equipmentItems: values.equipmentItems ?? [],
      isEnabled: values.isEnabled,
      effectiveFrom: values.range?.[0] ? values.range[0].startOf('day').toISOString() : undefined,
      effectiveTo: values.range?.[1] ? values.range[1].endOf('day').toISOString() : undefined,
    };
    if (mode === 'create') {
      await createMutation.mutateAsync({ values: payload });
    } else if (plan?.id) {
      await updateMutation.mutateAsync({ id: plan.id, values: payload });
    }
    return true;
  };

  const submitting = createMutation.isPending || updateMutation.isPending;

  return (
    <Drawer
      title={mode === 'create' ? t('plan.createTitle') : t('plan.editTitle')}
      open={open}
      onClose={onClose}
      width={720}
      destroyOnHidden
      extra={
        <Button type="primary" loading={submitting} onClick={() => form.submit()}>
          {t('common:button.submit')}
        </Button>
      }
    >
      <Form form={form} layout="vertical" onFinish={handleSubmit}>
        <Form.Item
          name="name"
          label={t('plan.name')}
          rules={[{ required: true, message: t('plan.requiredName') }]}
        >
          <Input maxLength={128} placeholder={t('plan.namePlaceholder')} />
        </Form.Item>

        <Form.Item label={t('plan.serviceType')}>
          <Input value={serviceTypeLabel} disabled />
        </Form.Item>

        <Form.Item name="post" label={t('plan.post')}>
          <AutoComplete
            options={INSPECTION_POST_SUGGESTIONS}
            filterOption
            allowClear
            placeholder={t('plan.postPlaceholder')}
          />
        </Form.Item>

        <Form.Item name="freqType" label={t('plan.freqType')} rules={[{ required: true }]}>
          <Select options={FREQ_OPTIONS} onChange={(v: FreqTypeValue) => setFreqType(v)} />
        </Form.Item>

        <Form.Item
          name="times"
          label={t('plan.times')}
          rules={[
            {
              required: true,
              validator: (_, val: string[]) =>
                (val ?? []).filter((tm) => !!tm).length > 0
                  ? Promise.resolve()
                  : Promise.reject(new Error(t('plan.requiredTimes'))),
            },
          ]}
          extra={t('plan.timesExtra')}
        >
          <TimesEditor placeholder="HH:mm" />
        </Form.Item>

        {freqType === 'FREQ_WEEKLY' && (
          <Form.Item
            name="weekdays"
            label={t('plan.weekdays')}
            rules={[{ required: true, message: t('plan.requiredWeekdays') }]}
          >
            <Select mode="multiple" options={WEEKDAY_OPTIONS} placeholder={t('plan.weekdaysPlaceholder')} />
          </Form.Item>
        )}

        {(freqType === 'FREQ_MONTHLY' || freqType === 'FREQ_YEARLY') && (
          <>
            <Form.Item
              name="monthDays"
              label={t('plan.monthDays')}
              rules={[{ required: true, message: t('plan.requiredMonthDays') }]}
              extra={t('plan.monthDaysExtra')}
            >
              <Select
                mode="multiple"
                options={MONTH_DAY_OPTIONS}
                placeholder={t('plan.monthDaysPlaceholder')}
              />
            </Form.Item>
            <Form.Item
              name="clampToMonthEnd"
              valuePropName="checked"
              label={t('plan.clampToMonthEnd')}
              extra={t('plan.clampToMonthEndExtra')}
            >
              <Switch />
            </Form.Item>
          </>
        )}

        {freqType === 'FREQ_YEARLY' && (
          <Form.Item
            name="months"
            label={t('plan.months')}
            rules={[{ required: true, message: t('plan.requiredMonths') }]}
          >
            <Select mode="multiple" options={MONTH_OPTIONS} placeholder={t('plan.monthsPlaceholder')} />
          </Form.Item>
        )}

        <Form.Item name="excludedDates" label={t('plan.excludedDates')} extra={t('plan.excludedDatesExtra')}>
          <DatePicker multiple format="YYYY-MM-DD" style={{ width: '100%' }} />
        </Form.Item>

        <Form.Item name="range" label={t('plan.range')} extra={t('plan.rangeExtra')}>
          <DatePicker.RangePicker format="YYYY-MM-DD" style={{ width: '100%' }} />
        </Form.Item>

        {/* 预留：空间/设备对象模块落地后启用 */}
        <Form.Item label={t('plan.spaceRefs')}>
          <Input disabled placeholder={t('plan.reservedPlaceholder')} />
        </Form.Item>
        <Form.Item label={t('plan.equipmentRefs')}>
          <Input disabled placeholder={t('plan.reservedPlaceholder')} />
        </Form.Item>

        <Form.Item
          name="spaceItems"
          label={t('plan.spaceItems')}
          extra={t('plan.itemsExtra')}
        >
          <ItemListEditor
            standardItems={standardItems}
            standardLabel={t('plan.importFromStandard')}
            emptyLabel={t('plan.emptyItems')}
          />
        </Form.Item>

        <Form.Item name="equipmentItems" label={t('plan.equipmentItems')}>
          <ItemListEditor
            standardItems={standardItems}
            standardLabel={t('plan.importFromStandard')}
            emptyLabel={t('plan.emptyItems')}
          />
        </Form.Item>

        <Form.Item name="isEnabled" label={t('plan.isEnabled')} valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
