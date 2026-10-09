<template>
  <SettingEditorCard
    :icon="SETTINGS_SECTIONS.mail.icon"
    :title="SETTINGS_SECTIONS.mail.title"
    :description="SETTINGS_MESSAGES.mailSubtitle"
    :plain="props.plain"
    :class="props.class"
  >
    <template #header-actions>
      <!-- 防抖占位收口到 SwitchRow：无状态（加载中/加载失败）时保留与最长胶囊
        （Unverified 10 字符）同宽的空槽，开关 x 坐标不动；失败回滚逻辑
        （toggleEnabled）不受影响。占位为空 span（无文案），加载失败“无胶囊”
        的 toast 语义不变；有状态时胶囊同样锁宽，短长文案不推挤开关。
        Switch 永不绑状态 key。 -->
      <SwitchRow
        v-if="!props.plain"
        :model-value="enabled"
        :pill="statusLabel"
        :pill-variant="statusVariant"
        status-min-width="104px"
        pill-size="medium"
        gap-class="gap-2"
        :switch-attrs="{
          id: 'admin-mail-enabled',
          disabled: switchDisabled,
          'aria-label': SETTINGS_MESSAGES.mailEnabledLabel,
        }"
        @update:model-value="toggleEnabled"
      />
      <!-- plain 嵌入模式（设置总览页）沿用旧语义：无 status 槽，只保留开关。 -->
      <AdminSwitch
        v-else
        id="admin-mail-enabled"
        :model-value="enabled"
        :aria-label="SETTINGS_MESSAGES.mailEnabledLabel"
        :disabled="switchDisabled"
        @update:model-value="toggleEnabled"
      />
    </template>
    <template #default>
      <div class="flex flex-col gap-5">
        <div v-if="loading" class="flex items-center gap-2" role="status">
          <AdminSpinner class="size-4" :aria-label="SETTINGS_MESSAGES.mailLoading" />
          <span class="text-admin-body text-muted-foreground">{{
            SETTINGS_MESSAGES.mailLoading
          }}</span>
        </div>

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
              :loading="testing"
              class="w-full"
            />
          </AdminFormField>
          <ActionButton variant="outline" :disabled="testDisabled" @click="sendTest">
            {{ SETTINGS_MESSAGES.mailSendTest }}
          </ActionButton>
        </div>
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
import AdminSpinner from '../../shared/components/AdminSpinner.vue'
import SwitchRow from '../../shared/components/SwitchRow.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import { useOptimisticToggle } from '../../shared/composables/useOptimisticToggle'
import { secretField } from '../../shared/lib/secretSchema'
import { confirmSettingsSave } from '../../shared/lib/confirmSettingsSave'
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
// 启用开关乐观收口到共享 useOptimisticToggle：intent 同 tick 翻转，
// pending 只压开关；成功后等回填 reconcile，失败回滚。toast 语义不变。
const MAIL_TOGGLE_KEY = 'mail' as const
const mailToggle = useOptimisticToggle<string>()
// 启用真相源是 summary.enabled，显示值走乐观覆盖；无 summary 时默认 true。
const enabled = computed(() =>
  mailToggle.effective(MAIL_TOGGLE_KEY, summary.value?.enabled ?? true),
)
const error = ref('')
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
  () =>
    loading.value ||
    saving.value ||
    testing.value ||
    mailToggle.isPending(MAIL_TOGGLE_KEY) ||
    Boolean(error.value),
)
const switchDisabled = computed(() => controlsDisabled.value || !summary.value)
const testDisabled = computed(
  () => controlsDisabled.value || !summary.value?.configured || !recipient.value.trim(),
)
// 健康三态（方案 A）：Ready/异常/未验证，载体统一 AdminStatusPill。
// - ready → Ready(success)
// - unavailable → 异常(danger)
// - not_tested / not_configured → 未验证(neutral)：not_tested 只是“保存后待一次
//   Test Email”的中间态，已有输入区 hint 与发送按钮引导，不属于阻塞性待行动，
//   故不用 warning，避免黄牌疲劳；warning 仅保留给阻塞性待行动。
// - 加载中/加载失败不进 Status：只走 toast + spinner，无胶囊。
const MAIL_STATUS_READY = 'Ready'
const MAIL_STATUS_ABNORMAL = 'Error'
const MAIL_STATUS_UNVERIFIED = 'Unverified'
const statusLabel = computed(() => {
  if (error.value) return undefined
  if (loading.value && !summary.value) return undefined
  const readiness = summary.value?.readiness
  if (!readiness) return undefined
  if (readiness === 'ready') return MAIL_STATUS_READY
  if (readiness === 'unavailable') return MAIL_STATUS_ABNORMAL
  return MAIL_STATUS_UNVERIFIED
})
const statusVariant = computed(() => {
  if (error.value) return 'neutral' as const
  const readiness = summary.value?.readiness
  if (readiness === 'ready') return 'success' as const
  if (readiness === 'unavailable') return 'danger' as const
  return 'neutral' as const
})

function populate(data: MailConfigSummary) {
  summary.value = data
  mailToggle.reconcile(MAIL_TOGGLE_KEY, data.enabled)
  form.resetForm({
    from_name: data.from_name,
    from_email: data.from_email,
    api_key: '',
    reply_to: data.reply_to ?? '',
  })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await resource.value.get()
    populate(data)
  } catch (err) {
    error.value = formatAdminDisplayError(err, SETTINGS_MESSAGES.mailLoadFailed)
    feedback.loadFailed(err)
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
  // Routine config write (backend ReasonOptional): save directly without
  // the confirm dialog; the audit records an empty reason.
  saving.value = true
  try {
    const updated = await resource.value.update(buildUpdatePayload(values, enabled.value), {
      operatorReason: '',
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
  const committed = summary.value
  if (!committed || mailToggle.isPending(MAIL_TOGGLE_KEY)) return
  // Routine config write (backend ReasonOptional): persist directly without
  // the confirm dialog; the audit records an empty reason. 乐观翻转与
  // pending/回滚收口到共享 composable，失败 toast 由 feedback 传入。
  await mailToggle.toggle(
    MAIL_TOGGLE_KEY,
    next,
    async (_, nextVal) => {
      const updated = await resource.value.update(
        {
          from_name: committed.from_name,
          from_email: committed.from_email,
          reply_to: committed.reply_to,
          enabled: nextVal,
        },
        { operatorReason: '' },
      )
      summary.value = updated
      mailToggle.reconcile(MAIL_TOGGLE_KEY, updated.enabled)
    },
    {
      onSuccess: () => {
        feedback.saved()
      },
      onFailure: (_, __, err) => {
        feedback.saveFailed(err)
      },
    },
  )
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
    feedback.testFailed(new Error(SETTINGS_MESSAGES.mailTestRecipientInvalid))
    return
  }
  // Routine probe (backend ReasonOptional): send directly without
  // interactive confirmation; the audit records an empty reason.
  testing.value = true
  try {
    await resource.value.test(target, { operatorReason: '' })
    await load()
    feedback.testSent()
  } catch (err) {
    feedback.testFailed(err)
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  void load()
})
</script>
