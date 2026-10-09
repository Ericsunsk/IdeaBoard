<template>
  <SettingEditorCard
    :icon="SETTINGS_SECTIONS.mail.icon"
    :title="SETTINGS_SECTIONS.mail.title"
    :description="SETTINGS_MESSAGES.mailSubtitle"
    :status="props.plain ? undefined : statusLabel"
    :status-severity="props.plain ? undefined : statusSeverity"
    :plain="props.plain"
    :class="props.class"
  >
    <template #header-actions>
      <AdminSwitch
        id="admin-mail-enabled"
        v-model="enabled"
        :aria-label="SETTINGS_MESSAGES.mailEnabledLabel"
        :disabled="switchDisabled"
        @update:model-value="toggleEnabled"
      />
    </template>
    <template #default>
      <div class="flex flex-col gap-5">
        <AdminAlert v-if="error" severity="error">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ error }}</span>
            <ActionButton variant="outline" @click="load">
              {{ SETTINGS_MESSAGES.retry }}
            </ActionButton>
          </div>
        </AdminAlert>
        <AdminAlert v-else-if="loading" severity="secondary">{{
          SETTINGS_MESSAGES.mailLoading
        }}</AdminAlert>

        <div class="grid gap-4 md:grid-cols-2">
          <AdminFormField
            :label="SETTINGS_MESSAGES.mailFromNameLabel"
            for="admin-mail-from-name"
            required
          >
            <AdminInput
              id="admin-mail-from-name"
              v-model="form.values.from_name"
              :aria-label="SETTINGS_MESSAGES.mailFromNameLabel"
              :disabled="controlsDisabled"
              class="w-full"
            />
          </AdminFormField>
          <AdminFormField
            :label="SETTINGS_MESSAGES.mailFromEmailLabel"
            for="admin-mail-from-email"
            required
          >
            <AdminInput
              id="admin-mail-from-email"
              v-model="form.values.from_email"
              type="email"
              :aria-label="SETTINGS_MESSAGES.mailFromEmailLabel"
              :disabled="controlsDisabled"
              class="w-full"
            />
          </AdminFormField>
        </div>
        <AdminFormField
          label="邮件服务商 API Key"
          for="admin-mail-api-key"
          :hint="SETTINGS_MESSAGES.mailApiKeyHint"
        >
          <AdminInput
            id="admin-mail-api-key"
            v-model="form.values.api_key"
            type="password"
            aria-label="邮件服务商 API Key"
            autocomplete="off"
            :disabled="controlsDisabled"
            class="w-full"
          />
        </AdminFormField>
        <AdminFormField label="Reply-To Email" for="admin-mail-reply-to">
          <AdminInput
            id="admin-mail-reply-to"
            v-model="form.values.reply_to"
            type="email"
            aria-label="Reply-To Email"
            :disabled="controlsDisabled"
            class="w-full"
          />
        </AdminFormField>
        <AdminAlert v-if="firstValidationError" severity="error">
          {{ firstValidationError }}
        </AdminAlert>

        <div
          class="grid gap-3 rounded-lg border border-border bg-muted/50 p-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end"
        >
          <AdminFormField
            :label="SETTINGS_MESSAGES.mailTestRecipientLabel"
            for="admin-mail-test-recipient"
            :hint="SETTINGS_MESSAGES.mailTestHint"
          >
            <AdminInput
              id="admin-mail-test-recipient"
              v-model="recipient"
              type="email"
              :aria-label="SETTINGS_MESSAGES.mailTestRecipientLabel"
              :disabled="controlsDisabled || testing || !summary?.configured"
              class="w-full"
            />
          </AdminFormField>
          <ActionButton
            variant="outline"
            :loading="testing"
            :disabled="testDisabled"
            @click="sendTest"
          >
            {{ SETTINGS_MESSAGES.mailSendTest }}
          </ActionButton>
        </div>
        <AdminAlert v-if="testError" severity="error">
          {{ testError }}
        </AdminAlert>
        <div
          class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/40 p-4"
          role="group"
          :aria-label="SETTINGS_MESSAGES.mailDangerZoneTitle"
        >
          <div class="min-w-0 flex-1">
            <p class="text-admin-body-sm font-bold text-destructive">
              {{ SETTINGS_MESSAGES.mailDangerZoneTitle }}
            </p>
            <p class="mt-0.5 text-admin-caption text-muted-foreground">
              {{ SETTINGS_MESSAGES.mailDangerZoneDescription }}
            </p>
          </div>
          <ActionButton
            variant="destructive"
            :disabled="controlsDisabled || !summary?.configured"
            @click="clear"
          >
            {{ SETTINGS_MESSAGES.mailDelete }}
          </ActionButton>
        </div>
      </div>
    </template>
    <template #actions>
      <AdminEditorActions
        :save-disabled="controlsDisabled"
        :reset-disabled="controlsDisabled"
        :saving="saving"
        @save="save"
        @reset="reset"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, type HTMLAttributes } from 'vue'
import * as v from 'valibot'
import { useForm, firstFormError } from '@/components/ui/form'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminEditorActions from '../../shared/components/AdminEditorActions.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import { secretField } from '../../shared/lib/secretSchema'
import {
  ADMIN_PROBE_OPERATOR_REASON,
  confirmSettingsSave,
} from '../../shared/lib/confirmSettingsSave'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import { useAdminMailSettingsFeedback } from '../shared/useSettingsFeedback'
import SettingEditorCard from '../shared/SettingEditorCard.vue'
import { SETTINGS_SECTIONS } from '../shared/sections'
import { SETTINGS_MESSAGES } from '../pages/messages'
import { createAdminMailConfigResource } from './resource'
import type { AdminMailConfigResource } from './resource'
import type { MailConfigSummary, MailConfigUpdate } from './types'

const props = withDefaults(
  defineProps<{
    class?: HTMLAttributes['class'] | undefined
    plain?: boolean
    resource?: AdminMailConfigResource | undefined
  }>(),
  { class: undefined, plain: false, resource: undefined },
)
const defaultResource = createAdminMailConfigResource()
const feedback = useAdminMailSettingsFeedback()
const confirm = useAdminConfirm()
const summary = ref<MailConfigSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const toggling = ref(false)
// 启用开关即改即存：状态来自 summary.enabled，v-model 先行
// 翻转 UI，toggleEnabled 走保存通道确认并持久化，取消/失败回滚。
const enabled = ref(true)
const error = ref('')
const testError = ref('')
const recipient = ref('')
// Key 是否必填取决于服务端状态（首次配置必须、已有可留空保持），schema 以函数
// 形式传入，读取时按当前 summary 重建。
const form = useForm({
  initialValues: { from_name: '', from_email: '', api_key: '', reply_to: '' },
  schema: () =>
    v.object({
      from_name: v.pipe(
        v.string(),
        v.trim(),
        v.minLength(1, SETTINGS_MESSAGES.mailFromNameRequired),
      ),
      from_email: v.pipe(
        v.string(),
        v.trim(),
        v.check((value) => value.includes('@'), SETTINGS_MESSAGES.mailFromEmailInvalid),
      ),
      api_key: secretField(
        !summary.value?.api_key_configured,
        SETTINGS_MESSAGES.mailApiKeyRequired,
      ),
      reply_to: v.pipe(
        v.string(),
        v.trim(),
        v.check(
          (value) => value === '' || value.includes('@'),
          SETTINGS_MESSAGES.mailReplyToInvalid,
        ),
      ),
    }),
})
const firstValidationError = computed(() =>
  firstFormError(form.errors.value, ['from_name', 'from_email', 'api_key', 'reply_to']),
)

const resource = computed(() => props.resource ?? defaultResource)
const controlsDisabled = computed(
  () => loading.value || saving.value || testing.value || toggling.value || Boolean(error.value),
)
const switchDisabled = computed(() => controlsDisabled.value || !summary.value)
const testDisabled = computed(
  () => controlsDisabled.value || !summary.value?.configured || !recipient.value.trim(),
)
const statusLabel = computed(() => {
  if (error.value) return SETTINGS_MESSAGES.settingsLoadFailed
  if (loading.value && !summary.value) return SETTINGS_MESSAGES.settingsLoading
  if (summary.value?.readiness === 'ready') return 'Ready'
  if (summary.value?.readiness === 'not_tested') return SETTINGS_MESSAGES.mailStatusUntested
  return SETTINGS_MESSAGES.mailStatusUnconfigured
})
const statusSeverity = computed(() => {
  if (error.value) return 'danger'
  if (summary.value?.readiness === 'ready') return 'success'
  if (summary.value?.readiness === 'not_tested') return 'warn'
  return 'secondary'
})

function populate(data: MailConfigSummary) {
  summary.value = data
  enabled.value = data.enabled
  form.resetForm({
    from_name: data.from_name,
    from_email: data.from_email,
    api_key: '',
    reply_to: data.reply_to ?? '',
  })
  testError.value = ''
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await resource.value.get()
    populate(data)
  } catch (err) {
    error.value = formatAdminDisplayError(err, SETTINGS_MESSAGES.mailLoadFailed)
  } finally {
    loading.value = false
  }
}

function reset() {
  if (summary.value) {
    populate(summary.value)
  } else {
    enabled.value = true
    form.resetForm({ from_name: '', from_email: '', api_key: '', reply_to: '' })
  }
}

type MailFormValues = {
  from_name: string
  from_email: string
  api_key: string
  reply_to: string
}

function buildUpdatePayload(values: MailFormValues, enabledValue: boolean): MailConfigUpdate {
  const payload: MailConfigUpdate = {
    from_name: values.from_name.trim(),
    from_email: values.from_email.trim(),
    enabled: enabledValue,
  }
  if (values.api_key.trim()) {
    payload.api_key = values.api_key.trim()
  }
  if (values.reply_to.trim()) {
    payload.reply_to = values.reply_to.trim()
  }
  return payload
}

const save = form.handleSubmit(async (values) => {
  // Control Plane writes require an operator reason (audit); a cancelled
  // dialog aborts the save silently.
  const operatorReason = await confirmSettingsSave(confirm, {
    title: SETTINGS_MESSAGES.mailSave,
    message: SETTINGS_MESSAGES.mailSaveMessage,
    confirmLabel: SETTINGS_MESSAGES.mailSave,
  })
  if (!operatorReason) return
  saving.value = true
  try {
    const updated = await resource.value.update(buildUpdatePayload(values, enabled.value), {
      operatorReason,
    })
    populate(updated)
    feedback.saved()
  } catch (err) {
    feedback.saveFailed(err)
  } finally {
    saving.value = false
  }
})

async function toggleEnabled(next: boolean) {
  // v-model 已先行翻转 UI；已提交状态（summary）是回滚基准。
  const committed = summary.value
  const previous = committed?.enabled ?? !next
  if (!committed || toggling.value) {
    enabled.value = previous
    return
  }
  const operatorReason = await confirmSettingsSave(confirm, {
    title: next ? SETTINGS_MESSAGES.mailEnableTitle : SETTINGS_MESSAGES.mailDisableTitle,
    message: next ? SETTINGS_MESSAGES.mailEnableMessage : SETTINGS_MESSAGES.mailDisableMessage,
    confirmLabel: next ? SETTINGS_MESSAGES.mailEnableTitle : SETTINGS_MESSAGES.mailDisableTitle,
  })
  if (!operatorReason) {
    enabled.value = previous
    return
  }
  toggling.value = true
  try {
    const updated = await resource.value.update(
      {
        from_name: committed.from_name,
        from_email: committed.from_email,
        reply_to: committed.reply_to,
        enabled: next,
      },
      { operatorReason },
    )
    summary.value = updated
    enabled.value = updated.enabled
    testError.value = ''
    feedback.saved()
  } catch (err) {
    enabled.value = previous
    feedback.saveFailed(err)
  } finally {
    toggling.value = false
  }
}

async function clear() {
  const operatorReason = await confirmSettingsSave(confirm, {
    title: SETTINGS_MESSAGES.mailDeleteTitle,
    message: SETTINGS_MESSAGES.mailDeleteMessage,
    confirmLabel: SETTINGS_MESSAGES.mailDelete,
    risk: 'destructive',
  })
  if (!operatorReason) return
  saving.value = true
  try {
    await resource.value.clear({ operatorReason })
    await load()
    feedback.cleared()
  } catch (err) {
    feedback.clearFailed(err)
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  const target = recipient.value.trim()
  if (!target || !target.includes('@')) {
    testError.value = SETTINGS_MESSAGES.mailTestRecipientInvalid
    return
  }
  // Connectivity probes use a fixed operator reason and skip interactive confirmation.
  testError.value = ''
  testing.value = true
  try {
    await resource.value.test(target, { operatorReason: ADMIN_PROBE_OPERATOR_REASON })
    await load()
    feedback.testSent()
  } catch (err) {
    testError.value = formatAdminDisplayError(err, SETTINGS_MESSAGES.mailTestFailed)
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  void load()
})
</script>
