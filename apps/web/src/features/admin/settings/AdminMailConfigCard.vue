<template>
  <SettingEditorCard
    title="邮件发送"
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
              重试
            </AdminButton>
          </div>
        </AdminAlert>
        <AdminAlert v-else-if="loading" severity="secondary"> 正在加载邮件配置… </AdminAlert>

        <div class="grid gap-4 md:grid-cols-2">
          <AdminFormField label="发件人名称" for="admin-mail-from-name" required>
            <AdminInput
              id="admin-mail-from-name"
              v-model="form.values.from_name"
              aria-label="发件人名称"
              :disabled="controlsDisabled"
              class="w-full"
            />
          </AdminFormField>
          <AdminFormField label="发件人 Email" for="admin-mail-from-email" required>
            <AdminInput
              id="admin-mail-from-email"
              v-model="form.values.from_email"
              type="email"
              aria-label="发件人 Email"
              :disabled="controlsDisabled"
              class="w-full"
            />
          </AdminFormField>
        </div>
        <AdminFormField
          label="Resend API Key"
          for="admin-mail-api-key"
          hint="已配置时留空即可保持不变。"
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
          class="grid gap-3 rounded-lg border border-admin-border-soft bg-admin-surface-muted/50 p-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-end"
        >
          <AdminFormField
            label="测试收件地址"
            for="admin-mail-test-recipient"
            hint="保存后发送一封真实 Test Email 以启用邮件发送。"
          >
            <AdminInput
              id="admin-mail-test-recipient"
              v-model="recipient"
              type="email"
              aria-label="测试收件地址"
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
            发送 Test Email
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
          删除配置
        </AdminButton>
        <div class="flex items-center gap-2">
          <AdminButton
            :class="adminActionButtonClass"
            variant="outline"
            :disabled="controlsDisabled"
            @click="reset"
          >
            重置
          </AdminButton>
          <AdminButton
            :class="adminActionButtonClass"
            :disabled="controlsDisabled"
            :loading="saving"
            @click="save"
          >
            保存邮件配置
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
import { confirmSettingsSave } from '../shared/lib/confirmSettingsSave'
import { useAdminConfirm } from '../shared/composables/useAdminConfirm'
import { formatAdminDisplayError } from '../shared/lib/toast'
import { useAdminMailSettingsFeedback } from './useSettingsFeedback'
import SettingEditorCard from './SettingEditorCard.vue'
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
      from_name: v.pipe(v.string(), v.trim(), v.minLength(1, '发件人名称不能为空')),
      from_email: v.pipe(
        v.string(),
        v.trim(),
        v.check((value) => value.includes('@'), '请输入有效的发件人 Email'),
      ),
      api_key: secretField(!summary.value?.api_key_configured, '请输入 Resend API Key'),
      reply_to: v.pipe(
        v.string(),
        v.trim(),
        v.check((value) => value === '' || value.includes('@'), '请输入有效的 Reply-To Email'),
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
  if (error.value) return '加载失败'
  if (loading.value && !summary.value) return '加载中'
  if (summary.value?.readiness === 'ready') return 'Ready'
  if (summary.value?.readiness === 'not_tested') return '未测试'
  return '未配置'
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
    error.value = formatAdminDisplayError(err, '加载邮件配置失败')
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
    title: '保存邮件配置',
    message: '确定保存邮件发送配置吗？该操作将记录操作人与原因。',
    confirmLabel: '保存邮件配置',
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
    title: '删除邮件配置',
    message: '确定删除邮件发送配置吗？删除后事务邮件将不可用。该操作将记录操作人与原因。',
    confirmLabel: '删除配置',
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
    testError.value = '请输入有效的测试收件地址'
    return
  }
  const operatorReason = await confirmSettingsSave(confirm, {
    title: '发送 Test Email',
    message: `将向 ${target} 发送一封真实 Test Email。该操作将记录操作人与原因。`,
    confirmLabel: '发送 Test Email',
  })
  if (!operatorReason) return
  testError.value = ''
  testing.value = true
  try {
    await resource.value.test(target, { operatorReason })
    await load()
    feedback.testSent()
  } catch (err) {
    testError.value = formatAdminDisplayError(err, '发送 Test Email 失败')
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  void load()
})
</script>
