<script lang="ts" setup>
import { reactive, ref } from 'vue';

import { useVbenDrawer } from '@vben/common-ui';
import { $t } from '@vben/locales';

import { message } from 'ant-design-vue';

import { runAiAgentApp } from '#/api';

const emit = defineEmits<{ success: [] }>();

const [Drawer, drawerApi] = useVbenDrawer({
  class: 'w-[520px]',
  onConfirm: async () => {
    for (const v of inputVars.value) {
      if (v.required && !runForm[v.name!]) {
        message.error(`${v.label ?? v.name} ${$t('page.aiAgent.runs.requiredInput')}`);
        return;
      }
    }
    drawerApi.setState({ loading: true });
    try {
      await runAiAgentApp(app.value.id, { ...runForm });
      message.success($t('page.aiAgent.runs.testSent'));
      drawerApi.close();
      emit('success');
    } catch (error: any) {
      console.error('[ai-agent] run failed', error);
      message.error(error?.message || $t('page.aiAgent.runs.testFailed'));
    } finally {
      drawerApi.setState({ loading: false });
    }
  },
  onOpenChange(isOpen) {
    if (!isOpen) return;
    const data = drawerApi.getData<{ row: any }>() ?? { row: {} };
    app.value = data.row ?? {};
    Object.keys(runForm).forEach((k) => delete runForm[k]);
    // start 节点 inputVars 声明解析（graph JSON 里取）
    try {
      const g = app.value.graph ? JSON.parse(app.value.graph) : {};
      const start = (g.nodes ?? []).find((n: any) => n.type === 'start');
      inputVars.value = start?.data?.inputVars ?? [];
    } catch {
      inputVars.value = [];
    }
  },
});

const app = ref<any>({});
const runForm = reactive<Record<string, string>>({});
const inputVars = ref<Array<{ name?: string; label?: string; required?: boolean }>>([]);
</script>

<template>
  <Drawer :title="$t('page.aiAgent.runs.testTitle', { name: app?.name || '' })">
    <a-form layout="vertical" class="pt-2">
      <a-form-item
        v-for="v in inputVars"
        :key="v.name"
        :label="v.label || v.name"
        :required="v.required"
      >
        <a-textarea v-model:value="runForm[v.name!]" :rows="2" />
      </a-form-item>
      <div v-if="inputVars.length === 0" class="text-xs text-muted-foreground pb-2">
        {{ $t('page.aiAgent.runs.noInputs') }}
      </div>
    </a-form>
  </Drawer>
</template>
