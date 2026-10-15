<template>
  <ProModal
    v-model:visible="visible"
    :title="runTitle"
    :loading="running"
    :config="{
      component: 'drawer',
      drawer: { size: 520, closeOnClickModal: false },
    }"
  >
    <ElForm ref="formRef" :model="runForm" label-width="120px" class="drawer-form">
      <ElFormItem
        v-for="v in inputVars"
        :key="v.name"
        :label="v.label || v.name"
        :prop="v.name"
        :rules="v.required ? [{ required: true, message: t('pages.ai_agent.runs.requiredInput') }] : []"
      >
        <ElInput v-model="runForm[v.name!]" type="textarea" :rows="2" />
      </ElFormItem>
      <div v-if="inputVars.length === 0" class="field-tip">
        {{ t("pages.ai_agent.runs.noInputs") }}
      </div>
    </ElForm>

    <template #footer>
      <div class="drawer-footer">
        <ElButton @click="visible = false">{{ $t("common.button.cancel") }}</ElButton>
        <ElButton type="primary" :loading="running" @click="handleRun">
          {{ t("pages.ai_agent.runs.run") }}
        </ElButton>
      </div>
    </template>
  </ProModal>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from "vue";
import { ElButton, ElForm, ElFormItem, ElInput, ElMessage } from "element-plus";

import ProModal from "@/components/Pro/ProModal/index.vue";
import { useI18n } from "@/core/i18n";
import { runAiAgentApp } from "@/api/composables";

const emit = defineEmits<{
  success: [];
}>();

const { t } = useI18n();

const formRef = ref();
const running = ref(false);
const app = ref<any>();
const runForm = reactive<Record<string, string>>({});

const visible = ref(false);

const runTitle = computed(() => t("pages.ai_agent.runs.testTitle", { name: app.value?.name || "" }));

// start 节点 inputVars 声明解析（graph JSON 里取）
const inputVars = computed(() => {
  if (!app.value?.graph) return [];
  try {
    const g = JSON.parse(app.value.graph);
    const start = (g.nodes ?? []).find((n: any) => n.type === "start");
    return (start?.data?.inputVars ?? []) as { name: string; label?: string; required?: boolean }[];
  } catch {
    return [];
  }
});

function open(row: any) {
  app.value = row;
  Object.keys(runForm).forEach((k) => delete runForm[k]);
  visible.value = true;
}

async function handleRun() {
  const valid = await formRef.value
    ?.validate()
    .then(() => true, () => false);
  if (!valid) return;
  running.value = true;
  try {
    await runAiAgentApp(app.value.id, { ...runForm });
    ElMessage.success(t("pages.ai_agent.runs.testSent"));
    visible.value = false;
    emit("success");
  } catch (error: any) {
    console.error("agent run failed:", error);
    ElMessage.error(error?.message || t("pages.ai_agent.runs.testFailed"));
  } finally {
    running.value = false;
  }
}

defineExpose({ open });
</script>

<style lang="scss" scoped>
.drawer-form {
  padding-right: 10px;
}

.field-tip {
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
