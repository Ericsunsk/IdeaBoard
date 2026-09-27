<template>
  <SettingEditorCard
    :title="providerLabel"
    :status="statusLabel"
    :status-severity="statusSeverity"
    :class="props.class"
  >
    <template #default>
      <div class="flex flex-col gap-5">
        <div class="flex flex-wrap items-center gap-2">
          <AdminBadge
            :value="config.api_key_configured ? 'API Key 已配置' : 'API Key 未配置'"
            :severity="config.api_key_configured ? 'success' : 'secondary'"
            rounded
          />
          <AdminBadge :value="testStateLabel" :severity="testStateSeverity" rounded />
          <AdminBadge
            :value="config.enabled ? '已启用' : '已停用'"
            :severity="config.enabled ? 'success' : 'secondary'"
            rounded
          />
        </div>

        <AdminAlert v-if="operationError" severity="error">
          {{ operationError }}
        </AdminAlert>

        <AdminFormField
          :label="`${providerLabel} API Key`"
          :for="`admin-search-${provider}-api-key`"
          hint="已配置时留空即可保持不变，输入新 Key 后需重新保存并测试。"
        >
          <AdminInput
            :id="`admin-search-${provider}-api-key`"
            v-model="form.values.apiKey"
            type="password"
            :aria-label="`${providerLabel} API Key`"
            :invalid="Boolean(form.errors.value.apiKey)"
            autocomplete="off"
            :disabled="controlsDisabled"
            class="w-full"
          />
        </AdminFormField>
        <AdminAlert v-if="form.errors.value.apiKey" severity="error">
          {{ form.errors.value.apiKey }}
        </AdminAlert>

        <div
          v-if="config.test_state === 'failed' || config.status_message"
          class="rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 p-4"
        >
          <div class="text-admin-caption text-admin-text-3">
            <span v-if="config.failure_category">失败分类：{{ config.failure_category }}</span>
            <span v-if="config.tested_at">
              {{ config.failure_category ? ' · ' : '' }}测试时间：{{
                formatAdminDateTime(config.tested_at)
              }}
            </span>
          </div>
          <div v-if="config.status_message" class="mt-1 text-admin-body text-admin-text-1">
            {{ config.status_message }}
          </div>
        </div>
        <div
          v-else-if="config.test_state === 'passed' && config.tested_at"
          class="text-admin-caption text-admin-text-3"
        >
          上次测试通过：{{ formatAdminDateTime(config.tested_at) }}
        </div>
      </div>
    </template>
    <template #actions>
      <div class="flex w-full flex-wrap items-center justify-between gap-2">
        <div class="text-admin-caption text-admin-text-3">
          {{ sequenceHint }}
        </div>
        <div class="flex items-center gap-2">
          <AdminButton
            :class="adminActionButtonClass"
            variant="outline"
            :disabled="testDisabled"
            :loading="testing"
            @click="runTest"
          >
            测试连接
          </AdminButton>
          <AdminButton
            :class="adminActionButtonClass"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="saveKey"
          >
            保存 Key
          </AdminButton>
          <AdminButton
            v-if="config.enabled"
            :class="adminActionButtonClass"
            variant="destructive"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="setEnabled(false)"
          >
            停用
          </AdminButton>
          <AdminButton
            v-else
            :class="adminActionButtonClass"
            :disabled="!canEnable || controlsDisabled"
            :loading="saving"
            @click="setEnabled(true)"
          >
            启用
          </AdminButton>
        </div>
      </div>
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, ref, type HTMLAttributes } from 'vue'
import * as v from 'valibot'
import { useForm } from '@/components/ui/form'
import { formatAdminDateTime } from '../../../shared/utils/dateTime'
import AdminAlert from '../shared/components/AdminAlert.vue'
import AdminBadge from '../shared/components/AdminBadge.vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import AdminInput from '../shared/components/AdminInput.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import {
  ADMIN_PROBE_OPERATOR_REASON,
  confirmSettingsSave,
} from '../shared/lib/confirmSettingsSave'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import { formatAdminDisplayError } from '../shared/lib/toast'
import SettingEditorCard from './SettingEditorCard.vue'
import { createAdminSearchProviderResource } from './searchProviderResource'
import type { AdminSearchProviderResource } from './searchProviderResource'
import { useAdminSearchProviderFeedback } from './useSettingsFeedback'
import {
  SEARCH_PROVIDER_LABELS,
  type SearchProviderConfig,
  type SearchProviderId,
  type SearchProviderTestState,
} from './searchProviderTypes'

const props = defineProps<{
  provider: SearchProviderId
  config: SearchProviderConfig
  resource?: AdminSearchProviderResource
  class?: HTMLAttributes['class']
}>()
const emit = defineEmits<{
  change: []
}>()

const defaultResource = createAdminSearchProviderResource()
const feedback = useAdminSearchProviderFeedback()
const confirm = useAdminConfirm()
const resource = computed(() => props.resource ?? defaultResource)

const form = useForm({
  initialValues: { apiKey: '' },
  schema: v.object({
    apiKey: v.pipe(v.string(), v.trim(), v.minLength(1, '请输入 API Key 后再保存')),
  }),
})
const saving = ref(false)
const testing = ref(false)
const operationError = ref('')

const providerLabel = computed(() => SEARCH_PROVIDER_LABELS[props.provider])
const controlsDisabled = computed(() => saving.value || testing.value)

const TEST_STATE_META: Record<
  SearchProviderTestState,
  { label: string; severity: 'success' | 'danger' | 'secondary' }
> = {
  passed: { label: '连接正常', severity: 'success' },
  failed: { label: '测试失败', severity: 'danger' },
  never: { label: '未测试', severity: 'secondary' },
}

const testStateLabel = computed(() => TEST_STATE_META[props.config.test_state].label)
const testStateSeverity = computed(() => TEST_STATE_META[props.config.test_state].severity)
const statusLabel = computed(() => {
  if (props.config.enabled) return '已启用'
  if (props.config.api_key_configured) return '已停用'
  return '未配置'
})
const statusSeverity = computed(() => (props.config.enabled ? 'success' : 'secondary'))

// 启用要求当前 Key 已通过测试；输入了新 Key 但尚未保存时测试状态已过期，必须先保存再测试。
const canEnable = computed(
  () => props.config.test_state === 'passed' && form.values.apiKey.trim() === '',
)
const testDisabled = computed(
  () =>
    controlsDisabled.value || form.values.apiKey.trim() !== '' || !props.config.api_key_configured,
)
const sequenceHint = computed(() => {
  if (!props.config.api_key_configured && form.values.apiKey.trim() === '') {
    return '先输入 API Key 并保存。'
  }
  if (form.values.apiKey.trim() !== '') {
    return 'Key 已修改，先保存再测试。'
  }
  if (props.config.test_state !== 'passed') {
    return '保存后请先测试连接，通过后才能启用。'
  }
  return '连接正常，可以启用或停用。'
})

function resetDraft() {
  form.resetForm()
  operationError.value = ''
}

const saveKey = form.handleSubmit(async (values) => {
  const key = values.apiKey.trim()
  // Control Plane writes require an operator reason (audit); a cancelled
  // dialog aborts the save silently.
  const operatorReason = await confirmSettingsSave(confirm, {
    title: `保存 ${providerLabel.value} API Key`,
    message: '保存新 Key 会重置测试状态并停用该供应商，保存后需重新测试。该操作将记录操作人与原因。',
    confirmLabel: '保存 Key',
  })
  if (!operatorReason) return
  saving.value = true
  operationError.value = ''
  try {
    // 保存新 Key 会重置测试状态（后端 saveLifecycle），落盘后必须重新测试；
    // 若之前已启用会一并停用，避免未经测试的 Key 生效。
    const wasEnabled = props.config.enabled
    await resource.value.update(
      props.provider,
      { api_key: key, enabled: false },
      { operatorReason },
    )
    resetDraft()
    emit('change')
    feedback.keySaved(providerLabel.value, wasEnabled)
  } catch (err) {
    operationError.value = formatAdminDisplayError(err, '保存 API Key 失败')
  } finally {
    saving.value = false
  }
})

async function runTest() {
  // Connectivity probes skip the confirm dialog (backend POST read-guard:
  // no reason ceremony); the fixed probe note still lands on the audit record.
  testing.value = true
  operationError.value = ''
  try {
    await resource.value.test(props.provider, { operatorReason: ADMIN_PROBE_OPERATOR_REASON })
    emit('change')
    feedback.testPassed(providerLabel.value)
  } catch (err) {
    // 测试失败状态已由后端落盘，刷新列表以展示失败分类与时间。
    emit('change')
    operationError.value = formatAdminDisplayError(err, '连接测试失败')
  } finally {
    testing.value = false
  }
}

async function setEnabled(enabled: boolean) {
  const operatorReason = await confirmSettingsSave(confirm, {
    title: `${enabled ? '启用' : '停用'}${providerLabel.value}`,
    message: enabled
      ? '启用后该供应商将承接图片搜索流量。该操作将记录操作人与原因。'
      : '停用后该供应商不再承接图片搜索流量。该操作将记录操作人与原因。',
    confirmLabel: enabled ? '启用' : '停用',
  })
  if (!operatorReason) return
  saving.value = true
  operationError.value = ''
  try {
    await resource.value.update(props.provider, { enabled }, { operatorReason })
    emit('change')
    feedback.toggled(providerLabel.value, enabled)
  } catch (err) {
    operationError.value = formatAdminDisplayError(err, enabled ? '启用失败' : '停用失败')
  } finally {
    saving.value = false
  }
}
</script>
