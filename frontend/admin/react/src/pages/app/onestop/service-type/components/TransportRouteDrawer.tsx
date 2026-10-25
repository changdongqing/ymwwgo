import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { App, Button, Drawer, Form, Input, InputNumber, Select, Switch, TimePicker } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import { useQueryClient } from '@tanstack/react-query';
import { type onestopservicev1_TransportRoute } from '@/api/generated/admin/service/v1';
import {
  useCreateTransportRoute,
  useUpdateTransportRoute,
  invalidateTransportRoutes,
} from '@/api/hooks/onestop';

interface TransportRouteDrawerProps {
  open: boolean;
  mode: 'create' | 'edit';
  /** 所属运送类别（0106 子树节点） */
  serviceTypeId: number;
  serviceTypeLabel: string;
  /** 编辑时的路线数据 */
  route?: onestopservicev1_TransportRoute;
  onSaved: () => void;
  onClose: () => void;
}

/** 发班时间点列表编辑器：每日按点发班；空列表 = 按需即时派单 */
function DepartureTimesEditor({
  value = [],
  onChange,
}: {
  value?: string[];
  onChange?: (times: string[]) => void;
}) {
  const { t } = useTranslation('onestop');
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
            placeholder="HH:mm"
          />
          {times.length > 1 && (
            <Button
              type="text"
              danger
              size="small"
              onClick={() => emit(times.filter((_, i) => i !== index))}
            >
              {t('delete')}
            </Button>
          )}
        </div>
      ))}
      <Button size="small" onClick={() => emit([...times, ''])}>
        {t('route.addDepartureTime')}
      </Button>
    </div>
  );
}

/** 表单数据形态 */
interface RouteFormValues {
  name: string;
  origin: string;
  destination: string;
  viaPoints?: string[];
  estDurationMinutes?: number;
  roundTrip: boolean;
  departureTimes: string[];
  isEnabled: boolean;
}

/**
 * 运送路线编辑抽屉：起终点/途经点/单程时长/往返/定时班次。
 * 班次留空 = 按需即时路线（调度即时派单），定时班次供工单对象生成定时任务。
 */
export default function TransportRouteDrawer({
  open,
  mode,
  serviceTypeId,
  serviceTypeLabel,
  route,
  onSaved,
  onClose,
}: TransportRouteDrawerProps) {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();

  useEffect(() => {
    if (!open) {
      return;
    }
    if (mode === 'edit' && route) {
      form.setFieldsValue({
        name: route.name ?? '',
        origin: route.origin ?? '',
        destination: route.destination ?? '',
        viaPoints: route.viaPoints ?? [],
        estDurationMinutes: route.estDurationMinutes,
        roundTrip: route.roundTrip ?? false,
        departureTimes: route.departureTimes ?? [],
        isEnabled: route.isEnabled ?? true,
      });
    } else {
      form.resetFields();
      form.setFieldsValue({ roundTrip: false, departureTimes: [], isEnabled: true, viaPoints: [] });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, mode, route]);

  const createMutation = useCreateTransportRoute({
    onSuccess: () => {
      message.success(t('route.createSuccess'));
      invalidateTransportRoutes(queryClient);
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('route.createFailed')),
  });

  const updateMutation = useUpdateTransportRoute({
    onSuccess: () => {
      message.success(t('route.updateSuccess'));
      invalidateTransportRoutes(queryClient);
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('route.updateFailed')),
  });

  const handleSubmit = async (values: RouteFormValues) => {
    const times = (values.departureTimes ?? []).filter((tm) => !!tm);
    const payload: Partial<onestopservicev1_TransportRoute> = {
      name: values.name,
      origin: values.origin,
      destination: values.destination,
      serviceTypeId,
      viaPoints: values.viaPoints ?? [],
      estDurationMinutes: values.estDurationMinutes,
      roundTrip: values.roundTrip,
      departureTimes: times,
      isEnabled: values.isEnabled,
    };
    if (mode === 'create') {
      await createMutation.mutateAsync({ values: payload });
    } else if (route?.id) {
      await updateMutation.mutateAsync({ id: route.id, values: payload });
    }
    return true;
  };

  const submitting = createMutation.isPending || updateMutation.isPending;

  return (
    <Drawer
      title={mode === 'create' ? t('route.createTitle') : t('route.editTitle')}
      open={open}
      onClose={onClose}
      width={640}
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
          label={t('route.name')}
          rules={[{ required: true, message: t('route.requiredName') }]}
        >
          <Input maxLength={128} placeholder={t('route.namePlaceholder')} />
        </Form.Item>

        <Form.Item label={t('route.serviceType')}>
          <Input value={serviceTypeLabel} disabled />
        </Form.Item>

        <Form.Item
          name="origin"
          label={t('route.origin')}
          rules={[{ required: true, message: t('route.requiredOrigin') }]}
        >
          <Input maxLength={128} placeholder={t('route.originPlaceholder')} />
        </Form.Item>

        <Form.Item
          name="destination"
          label={t('route.destination')}
          rules={[{ required: true, message: t('route.requiredDestination') }]}
        >
          <Input maxLength={128} placeholder={t('route.destinationPlaceholder')} />
        </Form.Item>

        <Form.Item name="viaPoints" label={t('route.viaPoints')} extra={t('route.viaPointsExtra')}>
          <Select
            mode="tags"
            tokenSeparators={[',', '，']}
            open={false}
            suffixIcon={null}
            placeholder={t('route.viaPointsPlaceholder')}
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Form.Item
          name="estDurationMinutes"
          label={t('route.estDuration')}
          rules={[{ required: true, message: t('route.requiredEstDuration') }]}
          extra={t('route.estDurationExtra')}
        >
          <InputNumber min={1} max={1440} precision={0} style={{ width: '100%' }} placeholder={t('minutesPlaceholder')} />
        </Form.Item>

        <Form.Item name="roundTrip" label={t('route.roundTrip')} valuePropName="checked" extra={t('route.roundTripExtra')}>
          <Switch />
        </Form.Item>

        <Form.Item name="departureTimes" label={t('route.departureTimes')} extra={t('route.departureTimesExtra')}>
          <DepartureTimesEditor />
        </Form.Item>

        <Form.Item name="isEnabled" label={t('route.isEnabled')} valuePropName="checked">
          <Switch />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
