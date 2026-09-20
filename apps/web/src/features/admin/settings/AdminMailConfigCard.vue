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
              v-model="form.from_name"
              aria-label="发件人名称"
              :disabled="controlsDisabled"
              class="w-full"
            />
          </AdminFormField>
          <AdminFormField label="发件人 Email" for="admin-mail-from-email" required>
            <AdminInput
              id="admin-mail-from-email"
              v-model="form.from_email"
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
            v-model="form.api_key"
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
            v-model="form.reply_to"
            type="email"
            aria-label="Reply-To Email"
            :disabled="controlsDisabled"
            class="w-full"
          />
        </AdminFormField>
        <AdminAlert v-if="validationError" severity="error">
          {{ validationError }}
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
import { computed, onMounted, reactive, ref, type HTMLAttributes } from 'vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminInput from '../shared/components/AdminInput.vue'
import AdminAlert from '../shared/components/AdminAlert.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { formatAdminDisplayError, useAdminToast } from '../shared/lib/toast'
import SettingEditorCard from './SettingEditorCard.vue'
import { createAdminMailConfigResource } from './mailConfigResource'
import type { AdminMailConfigResource } from './mailConfigResource'
import type { MailConfigSummary } from './mailConfigTypes'

const props = defineProps<{
  class?: HTMLAttributes['class']
  resource?: AdminMailConfigResource
}>()
const defaultResource = createAdminMailConfigResource()
const toast = useAdminToast()
const summary = ref<MailConfigSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const validationError = ref('')
const testError = ref('')
const recipient = ref('')
const form = reactive({
  from_name: '',
  from_email: '',
  api_key: '',
  reply_to: '',
})

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
  form.from_name = data.from_name
  form.from_email = data.from_email
  form.api_key = ''
  form.reply_to = data.reply_to ?? ''
  validationError.value = ''
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
    form.from_name = ''
    form.from_email = ''
    form.api_key = ''
    form.reply_to = ''
    validationError.value = ''
  }
}

function validate(): boolean {
  validationError.value = ''
  if (!summary.value?.api_key_configured && !form.api_key.trim()) {
    validationError.value = '请输入 Resend API Key'
    return false
  }
  if (!form.from_name.trim()) {
    validationError.value = '发件人名称不能为空'
    return false
  }
  const email = form.from_email.trim()
  if (!email || !email.includes('@')) {
    validationError.value = '请输入有效的发件人 Email'
    return false
  }
  const replyTo = form.reply_to.trim()
  if (replyTo && !replyTo.includes('@')) {
    validationError.value = '请输入有效的 Reply-To Email'
    return false
  }
  return true
}

async function save() {
  if (!validate()) return
  saving.value = true
  try {
    const payload: {
      from_name: string
      from_email: string
      api_key?: string
      reply_to?: string
    } = {
      from_name: form.from_name.trim(),
      from_email: form.from_email.trim(),
    }
    if (form.api_key.trim()) {
      payload.api_key = form.api_key.trim()
    }
    if (form.reply_to.trim()) {
      payload.reply_to = form.reply_to.trim()
    }
    const updated = await resource.value.update(payload)
    populate(updated)
    form.api_key = ''
    toast.success('邮件配置已保存，请发送 Test Email')
  } catch (err) {
    toast.error(formatAdminDisplayError(err, '保存邮件配置失败'))
  } finally {
    saving.value = false
  }
}

async function clear() {
  saving.value = true
  try {
    await resource.value.clear()
    await load()
    toast.success('邮件配置已删除')
  } catch (err) {
    toast.error(formatAdminDisplayError(err, '删除邮件配置失败'))
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
  testError.value = ''
  testing.value = true
  try {
    await resource.value.test(target)
    await load()
    toast.success('Test Email 已发送')
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
