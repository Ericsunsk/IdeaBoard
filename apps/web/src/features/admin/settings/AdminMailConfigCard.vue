<template>
  <SettingEditorCard
    :title="SETTINGS_MESSAGES.mailRowTitle"
    :status="statusLabel"
    :status-severity="statusSeverity"
    :class="props.class"
  >
    <template #default>
      <div class="flex flex-col gap-5">
        <AdminAlert v-if="error" severity="error">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ error }}</span>
            <AdminButton
              :class="adminActionButtonClass"
              variant="outline"
              size="small"
              @click="load"
            >
              {{ SETTINGS_MESSAGES.retry }}
            </AdminButton>
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
          label="Resend API Key"
          for="admin-mail-api-key"
          :hint="SETTINGS_MESSAGES.mailApiKeyHint"
        >
          <AdminInput
            id="admin-mail-api-key"
            v-model="form.values.api_key"
            type="password"
            aria-label="Resend API Key"
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
          <AdminButton
            :class="adminActionButtonClass"
            variant="outline"
            :loading="testing"
            :disabled="testDisabled"
            @click="sendTest"
          >
            {{ SETTINGS_MESSAGES.mailSendTest }}
          </AdminButton>
        </div>
        <AdminAlert v-if="testError" severity="error">
          {{ testError }}
        </AdminAlert>
      </div>
    </template>
    <template #actions>
      <div class="flex w-full flex-wrap items-center justify-between gap-2">
        <AdminButton
          :class="adminActionButtonClass"
          variant="destructive"
          :disabled="controlsDisabled || !summary?.configured"
          @click="clear"
        >
          {{ SETTINGS_MESSAGES.mailDelete }}
        </AdminButton>
        <div class="flex items-center gap-2">
          <AdminButton
            :class="adminActionButtonClass"
            variant="outline"
            :disabled="controlsDisabled"
            @click="reset"
          >
            {{ SETTINGS_MESSAGES.reset }}
          </AdminButton>
          <AdminButton
            :class="adminActionButtonClass"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="save"
          >
            {{ SETTINGS_MESSAGES.mailSave }}
          </AdminButton>
        </div>
      </div>
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, type HTMLAttributes } from 'vue'
import * as v from 'valibot'
import { useForm, firstFormError } from '@/components/ui/form'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminInput from '../shared/components/AdminInput.vue'
import AdminAlert from '../shared/components/AdminAlert.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { secretField } from '../shared/lib/secretSchema'
import { ADMIN_PROBE_OPERATOR_REASON, confirmSettingsSave } from '../shared/lib/confirmSettingsSave'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import { formatAdminDisplayError } from '../shared/lib/toast'
import { useAdminMailSettingsFeedback } from './useSettingsFeedback'
import SettingEditorCard from './SettingEditorCard.vue'
import { SETTINGS_MESSAGES } from './pages/messages'
import { createAdminMailConfigResource } from './mailConfigResource'
import type { AdminMailConfigResource } from './mailConfigResource'
import type { MailConfigSummary } from './mailConfigTypes'

const props = defineProps<{
  class?: HTMLAttributes['class']
  resource?: AdminMailConfigResource
}>()
const defaultResource = createAdminMailConfigResource()
const feedback = useAdminMailSettingsFeedback()
const confirm = useAdminConfirm()
const summary = ref<MailConfigSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
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
  () => loading.value || saving.value || testing.value || Boolean(error.value),
)
const testDisabled = computed(
  () => controlsDisabled.value || !summary.value?.configured || !recipient.value.trim(),
)
const statusLabel = computed(() => {
  if (error.value) return SETTINGS_MESSAGES.providerLoadFailed
  if (loading.value && !summary.value) return SETTINGS_MESSAGES.providerLoading
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
    form.resetForm({ from_name: '', from_email: '', api_key: '', reply_to: '' })
  }
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
    const payload: {
      from_name: string
      from_email: string
      api_key?: string
      reply_to?: string
    } = {
      from_name: values.from_name.trim(),
      from_email: values.from_email.trim(),
    }
    if (values.api_key.trim()) {
      payload.api_key = values.api_key.trim()
    }
    if (values.reply_to.trim()) {
      payload.reply_to = values.reply_to.trim()
    }
    const updated = await resource.value.update(payload, { operatorReason })
    populate(updated)
    feedback.saved()
  } catch (err) {
    feedback.saveFailed(err)
  } finally {
    saving.value = false
  }
})

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
  // Connectivity probes skip the confirm dialog (backend POST read-guard:
  // no reason ceremony); the fixed probe note still lands on the audit record.
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
