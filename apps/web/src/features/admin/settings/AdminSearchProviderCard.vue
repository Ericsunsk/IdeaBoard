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
            :value="
              config.api_key_configured
                ? SETTINGS_MESSAGES.searchApiKeyConfigured
                : SETTINGS_MESSAGES.searchApiKeyMissing
            "
            :severity="config.api_key_configured ? 'success' : 'secondary'"
            rounded
          />
          <AdminBadge :value="testStateLabel" :severity="testStateSeverity" rounded />
          <AdminBadge
            :value="SETTINGS_MESSAGES.searchEnableState(config.enabled)"
            :severity="config.enabled ? 'success' : 'secondary'"
            rounded
          />
        </div>

        <AdminAlert v-if="operationError" severity="error">
          {{ operationError }}
        </AdminAlert>

        <AdminFormField
          :label="SETTINGS_MESSAGES.searchApiKeyLabel(providerLabel)"
          :for="`admin-search-${provider}-api-key`"
          :hint="SETTINGS_MESSAGES.searchApiKeyHint"
        >
          <AdminInput
            :id="`admin-search-${provider}-api-key`"
            v-model="form.values.apiKey"
            type="password"
            :aria-label="SETTINGS_MESSAGES.searchApiKeyLabel(providerLabel)"
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
            <span v-if="config.failure_category">{{
              SETTINGS_MESSAGES.searchFailureCategory(config.failure_category)
            }}</span>
            <span v-if="config.tested_at">
              {{
                SETTINGS_MESSAGES.searchTestedAt(
                  Boolean(config.failure_category),
                  formatAdminDateTime(config.tested_at),
                )
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
          {{ SETTINGS_MESSAGES.searchLastTestPassed(formatAdminDateTime(config.tested_at)) }}
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
            {{ SETTINGS_MESSAGES.searchTestConnection }}
          </AdminButton>
          <AdminButton
            :class="adminActionButtonClass"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="saveKey"
          >
            {{ SETTINGS_MESSAGES.searchSaveKey }}
          </AdminButton>
          <AdminButton
            v-if="config.enabled"
            :class="adminActionButtonClass"
            variant="destructive"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="setEnabled(false)"
          >
            {{ SETTINGS_MESSAGES.searchDisable }}
          </AdminButton>
          <AdminButton
            v-else
            :class="adminActionButtonClass"
            :disabled="!canEnable || controlsDisabled"
            :loading="saving"
            @click="setEnabled(true)"
          >
            {{ SETTINGS_MESSAGES.searchEnable }}
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
import { SETTINGS_MESSAGES } from './pages/messages'
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
    apiKey: v.pipe(v.string(), v.trim(), v.minLength(1, SETTINGS_MESSAGES.searchApiKeyRequired)),
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
  if (props.config.enabled) return SETTINGS_MESSAGES.providerEnabled
  if (props.config.api_key_configured) return SETTINGS_MESSAGES.providerDisabled
  return SETTINGS_MESSAGES.providerUnconfigured
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
    return SETTINGS_MESSAGES.searchSequenceApiKeyMissing
  }
  if (form.values.apiKey.trim() !== '') {
    return SETTINGS_MESSAGES.searchSequenceKeyChanged
  }
  if (props.config.test_state !== 'passed') {
    return SETTINGS_MESSAGES.searchSequenceTestFirst
  }
  return SETTINGS_MESSAGES.searchSequenceReady
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
    title: SETTINGS_MESSAGES.searchSaveKeyTitle(providerLabel.value),
    message: SETTINGS_MESSAGES.searchSaveKeyMessage,
    confirmLabel: SETTINGS_MESSAGES.searchSaveKey,
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
    operationError.value = formatAdminDisplayError(err, SETTINGS_MESSAGES.searchSaveKeyFailed)
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
    operationError.value = formatAdminDisplayError(err, SETTINGS_MESSAGES.searchTestFailed)
  } finally {
    testing.value = false
  }
}

async function setEnabled(enabled: boolean) {
  const operatorReason = await confirmSettingsSave(confirm, {
    title: SETTINGS_MESSAGES.searchToggleTitle(enabled, providerLabel.value),
    message: enabled
      ? SETTINGS_MESSAGES.searchToggleEnableMessage
      : SETTINGS_MESSAGES.searchToggleDisableMessage,
    confirmLabel: SETTINGS_MESSAGES.searchEnableState(enabled),
  })
  if (!operatorReason) return
  saving.value = true
  operationError.value = ''
  try {
    await resource.value.update(props.provider, { enabled }, { operatorReason })
    emit('change')
    feedback.toggled(providerLabel.value, enabled)
  } catch (err) {
    operationError.value = formatAdminDisplayError(err, SETTINGS_MESSAGES.searchToggleFailed(enabled))
  } finally {
    saving.value = false
  }
}
</script>
