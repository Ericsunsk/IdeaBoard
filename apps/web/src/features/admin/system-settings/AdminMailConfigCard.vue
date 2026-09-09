<template>
  <SettingEditorCard title="邮件发送" :status="statusLabel" :status-severity="statusSeverity">
    <template #default>
      <div class="flex flex-col gap-5">
        <Message v-if="error" severity="error" :closable="false">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span>{{ error }}</span>
            <Button
              label="重试"
              :class="adminActionButtonClass"
              severity="secondary"
              size="small"
              outlined
              @click="load"
            />
          </div>
        </Message>
        <Message v-else-if="loading" severity="secondary" :closable="false"
          >正在加载邮件配置…</Message
        >

        <div class="grid gap-4 md:grid-cols-2">
          <AdminFormField label="发件人名称" for="admin-mail-from-name" required>
            <InputText
              id="admin-mail-from-name"
              v-model="form.from_name"
              :disabled="controlsDisabled"
              fluid
            />
          </AdminFormField>
          <AdminFormField label="发件人 Email" for="admin-mail-from-email" required>
            <InputText
              id="admin-mail-from-email"
              v-model="form.from_email"
              type="email"
              :disabled="controlsDisabled"
              fluid
            />
          </AdminFormField>
        </div>
        <AdminFormField
          label="Resend API Key"
          for="admin-mail-api-key"
          hint="已配置时留空即可保持不变。"
        >
          <InputText
            id="admin-mail-api-key"
            v-model="form.api_key"
            type="password"
            aria-label="Resend API Key"
            autocomplete="off"
            :disabled="controlsDisabled"
            fluid
          />
        </AdminFormField>
        <AdminFormField label="Reply-To Email" for="admin-mail-reply-to">
          <InputText
            id="admin-mail-reply-to"
            v-model="form.reply_to"
            type="email"
            :disabled="controlsDisabled"
            fluid
          />
        </AdminFormField>
        <Message v-if="validationError" severity="error" :closable="false">{{
          validationError
        }}</Message>

        <div
          class="grid gap-3 rounded-lg border border-admin-border bg-admin-surface-muted p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end"
        >
          <AdminFormField
            label="测试收件地址"
            for="admin-mail-test-recipient"
            hint="保存后发送一封真实 Test Email 以启用邮件发送。"
          >
            <InputText
              id="admin-mail-test-recipient"
              v-model="recipient"
              type="email"
              aria-label="测试收件地址"
              :disabled="controlsDisabled || testing || !summary?.configured"
              fluid
            />
          </AdminFormField>
          <Button
            label="发送 Test Email"
            :class="adminActionButtonClass"
            severity="secondary"
            outlined
            :loading="testing"
            :disabled="testDisabled"
            @click="sendTest"
          />
        </div>
        <Message v-if="testError" severity="error" :closable="false">{{ testError }}</Message>
      </div>
    </template>
    <template #actions>
      <Button
        :class="adminActionButtonClass"
        label="保存邮件配置"
        severity="contrast"
        :disabled="controlsDisabled"
        :loading="saving"
        @click="save"
      />
      <Button
        :class="adminActionButtonClass"
        label="删除配置"
        severity="danger"
        outlined
        :disabled="controlsDisabled || !summary?.configured"
        @click="clear"
      />
      <Button
        :class="adminActionButtonClass"
        label="重置"
        severity="secondary"
        outlined
        :disabled="controlsDisabled"
        @click="reset"
      />
    </template>
  </SettingEditorCard>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { formatAdminDisplayError, useAdminToast } from '../shared/lib/toast'
import SettingEditorCard from './SettingEditorCard.vue'
import { createAdminMailConfigResource } from './mailConfigResource'
import type { AdminMailConfigResource } from './mailConfigResource'
import type { MailConfigSummary } from './mailConfigTypes'

const props = defineProps<{ resource?: AdminMailConfigResource }>()
const defaultResource = createAdminMailConfigResource()
const toast = useAdminToast()
const summary = ref<MailConfigSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const testError = ref('')
const validationError = ref('')
const recipient = ref('')
const form = reactive({ api_key: '', from_name: '', from_email: '', reply_to: '' })
const controlsDisabled = computed(() => loading.value || saving.value || Boolean(error.value))
const testDisabled = computed(
  () =>
    controlsDisabled.value ||
    testing.value ||
    !summary.value?.configured ||
    !recipient.value.trim(),
)
const statusLabel = computed(() => {
  if (error.value) return '加载失败'
  if (loading.value || !summary.value) return '加载中'
  if (summary.value.readiness === 'ready') return 'Ready'
  if (summary.value.readiness === 'not_tested') return '待测试'
  if (summary.value.readiness === 'unavailable') return '不可用'
  return '未配置'
})
const statusSeverity = computed(() => {
  if (error.value || summary.value?.readiness === 'unavailable') return 'danger' as const
  if (summary.value?.readiness === 'ready') return 'success' as const
  if (summary.value?.readiness === 'not_tested') return 'warn' as const
  return 'secondary' as const
})

function activeResource() {
  return props.resource ?? defaultResource
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const next = await activeResource().get()
    summary.value = next
    form.from_name = next.from_name
    form.from_email = next.from_email
    form.reply_to = next.reply_to
  } catch (cause) {
    error.value = formatAdminDisplayError(cause, '加载邮件配置失败，请重试')
  } finally {
    loading.value = false
  }
}

async function save() {
  validationError.value = validate()
  if (validationError.value || controlsDisabled.value) return
  saving.value = true
  try {
    summary.value = await activeResource().update({
      ...(form.api_key.trim() ? { api_key: form.api_key.trim() } : {}),
      from_name: form.from_name.trim(),
      from_email: form.from_email.trim(),
      ...(form.reply_to.trim() ? { reply_to: form.reply_to.trim() } : {}),
    })
    form.api_key = ''
    toast.success('邮件配置已保存，请发送 Test Email')
  } catch (cause) {
    toast.error(formatAdminDisplayError(cause, '保存邮件配置失败'))
  } finally {
    saving.value = false
  }
}

async function sendTest() {
  if (testDisabled.value) return
  testing.value = true
  testError.value = ''
  try {
    await activeResource().test(recipient.value.trim())
    await load()
    toast.success('Test Email 已发送')
  } catch (cause) {
    testError.value = formatAdminDisplayError(cause, '测试邮件发送失败，请检查配置')
  } finally {
    testing.value = false
  }
}

async function clear() {
  if (controlsDisabled.value || !summary.value?.configured) return
  if (
    typeof window !== 'undefined' &&
    !window.confirm('确认删除邮件配置？删除后将停止发送 Email。')
  )
    return
  try {
    await activeResource().clear()
    form.api_key = ''
    form.from_name = ''
    form.from_email = ''
    form.reply_to = ''
    await load()
    toast.success('邮件配置已删除')
  } catch (cause) {
    toast.error(formatAdminDisplayError(cause, '删除邮件配置失败'))
  }
}

function reset() {
  if (!summary.value) return
  form.api_key = ''
  form.from_name = summary.value.from_name
  form.from_email = summary.value.from_email
  form.reply_to = summary.value.reply_to
  validationError.value = ''
}

function validate() {
  if (!form.from_name.trim()) return '请输入发件人名称'
  if (!/^\S+@\S+\.\S+$/.test(form.from_email.trim())) return '请输入有效的发件人 Email'
  if (form.reply_to.trim() && !/^\S+@\S+\.\S+$/.test(form.reply_to.trim()))
    return '请输入有效的 Reply-To Email'
  if (!summary.value?.api_key_configured && !form.api_key.trim()) return '请输入 Resend API Key'
  return ''
}

onMounted(load)
</script>
