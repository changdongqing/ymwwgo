import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, App, AutoComplete, Divider, Form, Input } from 'antd';
import {
  ProForm,
  ProFormDigit,
  ProFormItem,
  ProFormRadio,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
} from '@ant-design/pro-components';
import type { ProFormInstance } from '@ant-design/pro-components';
import { useQueryClient } from '@tanstack/react-query';
import {
  type onestopservicev1_Level,
  type onestopservicev1_ServiceType,
  type onestopservicev1_TriggerMode,
} from '@/api/generated/admin/service/v1';
import { PaginationQuery } from '@/core';
import { fetchListServiceTypes, useCreateServiceType, useUpdateServiceType } from '@/api/hooks/onestop';
import {
  CARGO_TYPE_SUGGESTIONS,
  CODE_PATTERN,
  EXECUTION_POST_SUGGESTIONS,
  PRIORITY_OPTIONS,
  TEMPERATURE_OPTIONS,
  TRANSPORT_CATEGORY_CODE,
  categoryProfile,
  getLevelMap,
  isConfigurable,
  levelOfCode,
  orderKindName,
  resolveOrderKind,
  resolveTriggerMode,
  triggerName,
} from '../constants';
import { AcceptanceItemsEditor, AcceptanceModesSelect } from './AcceptanceFields';
import PeriodicPlanPanel from './PeriodicPlanPanel';
import TransportRoutePanel from './TransportRoutePanel';

interface ServiceTypeFormProps {
  mode: 'create' | 'edit';
  data?: onestopservicev1_ServiceType;
  /** 保存成功后回调（父页面负责关闭页签并刷新列表） */
  onSaved: () => void;
  /** 取消编辑回调（父页面负责关闭页签） */
  onCancel: () => void;
}

/** 层级 → 恰好上一级（上级候选只取这一层，平铺下拉，不做树选择） */
const PARENT_LEVEL: Partial<Record<onestopservicev1_Level, onestopservicev1_Level>> = {
  LEVEL_MIDDLE: 'LEVEL_MAJOR',
  LEVEL_MINOR: 'LEVEL_MIDDLE',
  LEVEL_DETAIL: 'LEVEL_MINOR',
};

/**
 * 服务类型编辑表单：统一履约配置模板（不再按子树设三个门控分节）。
 * 小类/细类（6/8 位编码）出现一个「履约配置」分节，四组字段：
 * 派单与时效 / 作业标准 / 验收方案 / 专项属性；可见性与文案由展示矩阵
 * （CATEGORY_PROFILE）按中类驱动，未登记中类回落全组可见 + 通用文案。
 * 触发方式为「计划生成」时开放周期计划面板（编辑态）；运送子树另有常用路线面板。
 */
export default function ServiceTypeForm({ mode, data, onSaved, onCancel }: ServiceTypeFormProps) {
  const { t } = useTranslation('onestop');
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [form] = Form.useForm();
  const formRef = useRef<ProFormInstance>(null);

  // 监听编码输入：驱动层级展示与上级候选
  const watchedCode = Form.useWatch('code', form) as string | undefined;
  // 监听触发方式：新建态驱动「保存后可配置周期计划」提示
  const watchedTriggerMode = Form.useWatch('triggerMode', form) as onestopservicev1_TriggerMode | undefined;
  const watchedOrderKind = Form.useWatch('orderKind', form) as string | undefined;

  // 全量平铺列表：用于构建"上级服务类型"候选与触发方式继承解析
  const [allItems, setAllItems] = useState<onestopservicev1_ServiceType[]>([]);

  useEffect(() => {
    // 不传 paging 走 noPaging=true（常规分页钳制 100 条会让上级候选缺项）
    fetchListServiceTypes(new PaginationQuery())
      .then((response) => setAllItems(response.items ?? []))
      .catch((error: Error) => message.error(error.message || t('fetchFailed')));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 编辑回填 / 新建重置（组件随页签挂载，仅执行一次）
  useEffect(() => {
    if (mode === 'edit' && data) {
      form.setFieldsValue({
        code: data.code,
        name: data.name,
        parentId: data.parentId,
        description: data.description,
        isEnabled: data.isEnabled,
        executionPost: data.executionPost,
        standardWorkHours: data.standardWorkHours,
        responseTimeLimit: data.responseTimeLimit,
        completionTimeLimit: data.completionTimeLimit,
        guidance: data.guidance,
        commonPhrases: data.commonPhrases ?? [],
        standardTools: data.standardTools ?? [],
        spareParts: data.spareParts ?? [],
        standardItems: data.standardItems ?? [],
        triggerMode: data.triggerMode === 'TRIGGER_UNSPECIFIED' ? undefined : data.triggerMode,
        orderKind: data.orderKind === 'KIND_UNSPECIFIED' ? undefined : data.orderKind,
        defaultPriority: data.defaultPriority === 'PRIORITY_UNSPECIFIED' ? undefined : data.defaultPriority,
        acceptanceModes: data.acceptanceModes ?? [],
        acceptanceItems: data.acceptanceItems ?? [],
        temperatureRequirement:
          data.temperatureRequirement === 'TEMP_UNSPECIFIED' ? undefined : data.temperatureRequirement,
        requiresApproval: data.requiresApproval ?? false,
        cargoTypes: data.cargoTypes ?? [],
      });
    } else {
      form.resetFields();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const levelMap = getLevelMap(t);

  // 当前层级：编辑时以记录编码为准，新建时跟随输入
  const currentLevel = useMemo(
    () => levelOfCode(mode === 'edit' ? data?.code : watchedCode),
    [mode, data?.code, watchedCode],
  );

  // 统一模板门控：小类/细类可配置；展示矩阵决定专项属性与文案
  const effectiveCode = mode === 'edit' ? data?.code : watchedCode;
  const showConfig = isConfigurable(effectiveCode);
  const profile = useMemo(() => categoryProfile(effectiveCode), [effectiveCode]);

  // 生效触发方式：编辑态走继承解析（细类未设置回退小类），新建态跟随选择
  const resolvedTrigger = useMemo(() => {
    if (mode === 'edit' && data) {
      return resolveTriggerMode(allItems, data);
    }
    return watchedTriggerMode === 'TRIGGER_SCHEDULED' ? 'TRIGGER_SCHEDULED' : 'TRIGGER_ON_DEMAND';
  }, [mode, data, allItems, watchedTriggerMode]);
  const isScheduled = resolvedTrigger === 'TRIGGER_SCHEDULED';

  // 生效履约形态：编辑态走继承解析（细类→小类→推导），新建态跟随选择/推导
  const resolvedOrderKind = useMemo(() => {
    if (mode === 'edit' && data) {
      return resolveOrderKind(allItems, data);
    }
    if (watchedOrderKind && watchedOrderKind !== 'KIND_UNSPECIFIED') {
      return watchedOrderKind;
    }
    const asTarget = {
      code: effectiveCode,
      level: currentLevel,
      triggerMode: watchedTriggerMode,
    };
    return resolveOrderKind(allItems, asTarget);
  }, [mode, data, allItems, watchedOrderKind, watchedTriggerMode, effectiveCode, currentLevel]);

  // 运送子树（任意层级的小类/细类）：编辑态管理常用路线（与触发方式无关，
  // 按需押运同样需要路线模板；仅定时班次路线参与后续任务生成）
  const isTransportSubtree =
    !!effectiveCode && effectiveCode.startsWith(TRANSPORT_CATEGORY_CODE) && showConfig;

  // 上级候选：只允许挂在恰好上一级之下；编辑时排除自身。
  // 同一层级的候选天然是平铺列表，无需树形结构。
  const parentOptions = useMemo(() => {
    if (!currentLevel) {
      return [];
    }
    const parentLevel = PARENT_LEVEL[currentLevel];
    if (!parentLevel) {
      return [];
    }
    const selfId = mode === 'edit' ? data?.id : undefined;
    return allItems
      .filter((item) => item.level === parentLevel && item.id !== selfId)
      .sort((a, b) => (a.code ?? '').localeCompare(b.code ?? ''))
      .map((item) => ({
        label: `${item.code ?? ''} ${item.name ?? ''}`.trim(),
        value: item.id as number,
      }));
  }, [currentLevel, allItems, mode, data?.id]);

  const createMutation = useCreateServiceType({
    onSuccess: () => {
      message.success(t('createSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listServiceTypes'] });
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('createFailed')),
  });

  const updateMutation = useUpdateServiceType({
    onSuccess: () => {
      message.success(t('updateSuccess'));
      queryClient.invalidateQueries({ queryKey: ['listServiceTypes'] });
      onSaved();
    },
    onError: (error: Error) => message.error(error.message || t('updateFailed')),
  });

  const handleSubmit = async (values: Partial<onestopservicev1_ServiceType>) => {
    const payload: Partial<onestopservicev1_ServiceType> = {
      code: values.code,
      name: values.name,
      parentId: values.parentId,
      description: values.description,
      isEnabled: values.isEnabled,
    };
    if (showConfig) {
      // —— A 派单与时效 / B 作业标准 / C 验收方案（通用组，全部中类可见）——
      // 空串归一为 undefined：清空的字段走 mask 内 nil → 置 NULL，而不是落空串
      payload.executionPost = values.executionPost || undefined;
      payload.guidance = values.guidance || undefined;
      payload.standardWorkHours = values.standardWorkHours;
      payload.responseTimeLimit = values.responseTimeLimit;
      payload.completionTimeLimit = values.completionTimeLimit;
      payload.commonPhrases = values.commonPhrases ?? [];
      payload.standardTools = values.standardTools ?? [];
      payload.spareParts = values.spareParts ?? [];
      payload.defaultPriority = values.defaultPriority || undefined;
      payload.acceptanceModes = values.acceptanceModes ?? [];
      payload.acceptanceItems = values.acceptanceItems ?? [];
      // 触发方式：清空 = 继承上级（mask 内 nil → NULL）；已挂计划时后端拒绝切回按需
      payload.triggerMode = values.triggerMode || undefined;
      if (values.orderKind) payload.orderKind = values.orderKind;
      // 标准作业项目库：仅计划生成时随表单提交（显式选择优先，未选择按渲染态
      // 解析结果——细类继承小类的计划生成同样要带项目库字段）
      const submittedScheduled =
        values.triggerMode != null ? values.triggerMode === 'TRIGGER_SCHEDULED' : isScheduled;
      if (submittedScheduled) {
        payload.standardItems = values.standardItems ?? [];
      }
      // —— D 专项属性：按展示矩阵出现（后端存储不限子树，前端按矩阵收口）——
      if (profile.specialFields?.includes('temperatureRequirement')) {
        payload.temperatureRequirement = values.temperatureRequirement || undefined;
      }
      if (profile.specialFields?.includes('requiresApproval')) {
        payload.requiresApproval = values.requiresApproval ?? false;
      }
      if (profile.specialFields?.includes('cargoTypes')) {
        payload.cargoTypes = values.cargoTypes ?? [];
      }
    }
    if (mode === 'create') {
      await createMutation.mutateAsync({ values: payload });
    } else if (data?.id) {
      await updateMutation.mutateAsync({ id: data.id, values: payload });
    }
    return true;
  };

  const responseTimeLimitLabel = profile.responseTimeLimitLabelKey
    ? t(profile.responseTimeLimitLabelKey)
    : t('responseTimeLimit');
  const completionTimeLimitLabel = profile.completionTimeLimitLabelKey
    ? t(profile.completionTimeLimitLabelKey)
    : t('completionTimeLimit');

  return (
    <div className="service-type-form-pane">
      <ProForm
        form={form}
        formRef={formRef}
        onFinish={handleSubmit}
        grid
        style={{ maxWidth: 760 }}
        submitter={{
          searchConfig: {
            submitText: t('common:button.submit'),
            resetText: t('common:button.cancel'),
          },
          resetButtonProps: { onClick: () => onCancel() },
        }}
      >
        <ProFormText
          name="code"
          label={t('code')}
          rules={[
            { required: true, message: t('requiredCode') },
            { pattern: CODE_PATTERN, message: t('invalidCode') },
          ]}
          fieldProps={{ maxLength: 8, showCount: true }}
          placeholder={t('codePlaceholder')}
          extra={t('codeExtra')}
        />

        {/* 层级由编码位数推导（后端同样强制），仅展示不可编辑。
            不绑 name，避免把展示值写进表单存储 */}
        <Form.Item label={t('level')}>
          <Input disabled value={currentLevel ? levelMap[currentLevel] : t('levelUnknown')} placeholder="-" />
        </Form.Item>

        <ProFormText
          name="name"
          label={t('name')}
          rules={[{ required: true, message: t('requiredName') }]}
          fieldProps={{ maxLength: 128 }}
          placeholder={t('namePlaceholder')}
        />

        {/* 大类为根节点无上级；其余层级必须挂恰好上一级（候选只列该层，平铺下拉） */}
        {currentLevel && currentLevel !== 'LEVEL_MAJOR' && (
          <ProFormSelect
            name="parentId"
            label={t('parentId')}
            rules={[{ required: true, message: t('requiredParentId') }]}
            options={parentOptions}
            fieldProps={{
              allowClear: true,
              showSearch: true,
              optionFilterProp: 'label',
            }}
            placeholder={t('parentIdPlaceholder')}
          />
        )}

        <ProFormTextArea
          name="description"
          label={t('description')}
          fieldProps={{ maxLength: 512, rows: 3 }}
          colProps={{ span: 24 }}
        />

        <ProFormRadio.Group
          name="isEnabled"
          label={t('isEnabled')}
          initialValue={true}
          options={[
            { label: t('enabled'), value: true },
            { label: t('disabled'), value: false },
          ]}
        />

        {/* 统一履约配置模板：小类/细类出现一个分节，四组字段按矩阵驱动。
            小类配工种层默认（岗位/SLA/指导/项目库），细类配作业层并覆盖（工时/语句/工具/耗材），
            均可留空 = 未配置；细类为空由消费侧回退小类 */}
        {showConfig && (
          <>
            <Divider titlePlacement="left">{t('fulfillmentConfigSection')}</Divider>

            <ProFormSelect
              name="triggerMode"
              label={t('triggerMode')}
              allowClear
              options={[
                { label: t('triggerMode.onDemand'), value: 'TRIGGER_ON_DEMAND' },
                { label: t('triggerMode.scheduled'), value: 'TRIGGER_SCHEDULED' },
              ]}
              colProps={{ span: 12 }}
              placeholder={t('triggerModePlaceholder')}
              extra={
                mode === 'edit' && !data?.triggerMode
                  ? t('triggerModeInheritExtra', { mode: triggerName(resolvedTrigger) })
                  : t('triggerModeExtra')
              }
            />

            <ProFormSelect
              name="orderKind"
              label={t('orderKind')}
              allowClear
              options={[
                { label: t('orderKindMap.event'), value: 'KIND_EVENT' },
                { label: t('orderKindMap.task'), value: 'KIND_TASK' },
                { label: t('orderKindMap.transport'), value: 'KIND_TRANSPORT' },
              ]}
              colProps={{ span: 12 }}
              placeholder={t('orderKindPlaceholder')}
              extra={
                mode === 'edit' && !data?.orderKind
                  ? t('orderKindInheritExtra', { kind: orderKindName(resolvedOrderKind as import('../constants').OrderKindValue) })
                  : t('orderKindExtra')
              }
            />

            {/* —— A 派单与时效 —— */}
            <ProFormItem
              name="executionPost"
              label={t('executionPost')}
              extra={t('executionPostExtra')}
            >
              <AutoComplete
                options={EXECUTION_POST_SUGGESTIONS}
                filterOption
                allowClear
                placeholder={t('executionPostPlaceholder')}
              />
            </ProFormItem>

            <ProFormSelect
              name="defaultPriority"
              label={t('defaultPriority')}
              options={PRIORITY_OPTIONS}
              allowClear
              colProps={{ span: 12 }}
              placeholder={t('defaultPriorityPlaceholder')}
              extra={t('defaultPriorityExtra')}
            />

            <ProFormDigit
              name="responseTimeLimit"
              label={responseTimeLimitLabel}
              min={1}
              max={43200}
              fieldProps={{ precision: 0 }}
              colProps={{ span: 12 }}
              placeholder={t('minutesPlaceholder')}
            />

            <ProFormDigit
              name="completionTimeLimit"
              label={completionTimeLimitLabel}
              min={1}
              max={43200}
              fieldProps={{ precision: 0 }}
              colProps={{ span: 12 }}
              placeholder={t('minutesPlaceholder')}
            />

            <ProFormDigit
              name="standardWorkHours"
              label={t('standardWorkHours')}
              min={0.1}
              max={720}
              fieldProps={{ step: 0.5, precision: 1 }}
              colProps={{ span: 12 }}
              placeholder={t('standardWorkHoursPlaceholder')}
              extra={t('standardWorkHoursExtra')}
            />

            {/* —— B 作业标准 —— */}
            <ProFormTextArea
              name="guidance"
              label={t('guidance')}
              fieldProps={{ rows: 4, maxLength: 2048, showCount: true }}
              colProps={{ span: 24 }}
              placeholder={t('guidancePlaceholder')}
            />

            <ProFormSelect
              name="commonPhrases"
              label={t('commonPhrases')}
              fieldProps={{
                mode: 'tags',
                tokenSeparators: [',', '，'],
                open: false,
                suffixIcon: null,
              }}
              colProps={{ span: 24 }}
              placeholder={t('commonPhrasesPlaceholder')}
              extra={t('commonPhrasesExtra')}
            />

            <ProFormSelect
              name="standardTools"
              label={t('standardTools')}
              fieldProps={{
                mode: 'tags',
                tokenSeparators: [',', '，'],
                open: false,
                suffixIcon: null,
              }}
              colProps={{ span: 24 }}
              placeholder={t('standardToolsPlaceholder')}
            />

            <ProFormSelect
              name="spareParts"
              label={t('spareParts')}
              fieldProps={{
                mode: 'tags',
                tokenSeparators: [',', '，'],
                open: false,
                suffixIcon: null,
              }}
              colProps={{ span: 24 }}
              placeholder={t('sparePartsPlaceholder')}
            />

            {/* —— C 验收方案（报修=完工验收，运送=交接签收，客服=办结回访）—— */}
            <ProFormItem
              name="acceptanceModes"
              label={t('acceptance.modes')}
              extra={t('acceptance.modesExtra')}
            >
              <AcceptanceModesSelect />
            </ProFormItem>

            <ProFormItem
              name="acceptanceItems"
              label={t('acceptance.items')}
              extra={t('acceptance.itemsExtra')}
            >
              <AcceptanceItemsEditor />
            </ProFormItem>

            {/* —— D 专项属性 + 标准作业项目库 —— */}

            {/* 标准作业项目库：计划点表模板，仅「计划生成」类别需要 */}
            {isScheduled && (
              <ProFormSelect
                name="standardItems"
                label={t('standardItems')}
                fieldProps={{
                  mode: 'tags',
                  tokenSeparators: [',', '，'],
                  open: false,
                  suffixIcon: null,
                }}
                colProps={{ span: 24 }}
                placeholder={t('standardItemsPlaceholder')}
                extra={t('standardItemsExtra')}
              />
            )}

            {profile.specialFields?.includes('temperatureRequirement') && (
              <ProFormSelect
                name="temperatureRequirement"
                label={t('temperatureRequirement')}
                options={TEMPERATURE_OPTIONS}
                allowClear
                colProps={{ span: 12 }}
                placeholder={t('temperatureRequirementPlaceholder')}
                extra={t('temperatureRequirementExtra')}
              />
            )}

            {profile.specialFields?.includes('requiresApproval') && (
              <ProFormRadio.Group
                name="requiresApproval"
                label={t('requiresApproval')}
                initialValue={false}
                options={[
                  { label: t('requiresApprovalNo'), value: false },
                  { label: t('requiresApprovalYes'), value: true },
                ]}
                extra={t('requiresApprovalExtra')}
              />
            )}

            {profile.specialFields?.includes('cargoTypes') && (
              <ProFormSelect
                name="cargoTypes"
                label={t('cargoTypes')}
                options={CARGO_TYPE_SUGGESTIONS}
                fieldProps={{
                  mode: 'tags',
                  tokenSeparators: [',', '，'],
                  filterOption: true,
                  suffixIcon: null,
                }}
                colProps={{ span: 24 }}
                placeholder={t('cargoTypesPlaceholder')}
                extra={t('cargoTypesExtra')}
              />
            )}
          </>
        )}
      </ProForm>

      {/* 周期计划管理（仅编辑态且解析后为「计划生成」）：类别已保存才有 id，
          计划挂类别之下；由「计划生成」切回按需时后端拒绝（须先删计划） */}
      {mode === 'edit' && data?.id != null && isScheduled && (
        <PeriodicPlanPanel
          serviceTypeId={data.id}
          serviceTypeLabel={`${data.code ?? ''} ${data.name ?? ''}`.trim()}
          standardItems={data.standardItems ?? []}
        />
      )}

      {/* 新建态选了「计划生成」：保存后才能配置计划（计划需要类别 id） */}
      {mode === 'create' && showConfig && isScheduled && (
        <Alert
          type="info"
          showIcon
          style={{ marginTop: 8 }}
          message={t('plan.createAfterSaveHint')}
        />
      )}

      {/* 常用运送路线管理（仅编辑态、0106 子树）：与触发方式无关（按需押运也需要路线模板） */}
      {mode === 'edit' && data?.id != null && isTransportSubtree && (
        <TransportRoutePanel
          serviceTypeId={data.id}
          serviceTypeLabel={`${data.code ?? ''} ${data.name ?? ''}`.trim()}
        />
      )}
    </div>
  );
}
