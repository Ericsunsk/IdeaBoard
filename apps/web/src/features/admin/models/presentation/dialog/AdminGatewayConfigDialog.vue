<template>
  <AdminDialogShell
    :visible="visible"
    header="添加网关"
    description="填写网关连接配置后保存。"
    size="compact"
    content-layout="plain"
    @update:visible="emit('update:visible', $event)"
    @close="emit('update:visible', false)"
  >
    <div class="flex flex-col gap-4">
      <!-- 错误警告提示 -->
      <AdminAlert v-if="error || validationError" severity="error">
        {{ error || validationError }}
      </AdminAlert>

      <!-- 网关 Base URL -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-admin-text-1"
            for="gateway-base-url-input"
          >
            网关 Base URL
          </label>
          <span
            v-if="summary?.is_first_party"
            class="inline-flex items-center gap-1 text-admin-caption text-admin-accent font-medium"
          >
            <Check class="size-3.5" />
            已识别 Cloudflare 网关
          </span>
        </div>
        <AdminInput
          id="gateway-base-url-input"
          v-model="form.base_url"
          placeholder="例如: https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway_id}"
          :disabled="loading || saving"
          class="w-full"
        />
      </div>

      <!-- API 密钥 -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-admin-text-1"
            for="gateway-token-input"
          >
            API 密钥
          </label>
          <span
            v-if="isTokenConfigured"
            class="inline-flex items-center gap-1 text-admin-caption text-admin-success font-medium"
          >
            <ShieldCheck class="size-3.5" />
            已配置 (留空保持不变)
          </span>
        </div>
        <AdminInput
          id="gateway-token-input"
          v-model="form.token"
          type="password"
          placeholder="输入网关鉴权 Token（若开启鉴权）"
          autocomplete="off"
          :disabled="loading || saving"
          class="w-full"
        />
      </div>

      <!-- CF 身份：官方域名可空（后端从 URL 派生）；自定义域名如需模型探测请填写 -->
      <div class="flex flex-col gap-1.5">
        <label
          class="text-admin-caption font-semibold text-admin-text-1"
          for="gateway-account-id-input"
        >
          Account ID <span class="font-normal text-admin-text-3">（自定义域名探测选填）</span>
        </label>
        <AdminInput
          id="gateway-account-id-input"
          v-model="form.account_id"
          placeholder="32 位小写十六进制"
          autocomplete="off"
          :disabled="loading || saving"
          class="w-full font-mono"
        />
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-admin-text-1" for="gateway-id-input">
          Gateway ID <span class="font-normal text-admin-text-3">（自定义域名探测选填）</span>
        </label>
        <AdminInput
          id="gateway-id-input"
          v-model="form.gateway_id"
          placeholder="网关标识"
          autocomplete="off"
          :disabled="loading || saving"
          class="w-full font-mono"
        />
      </div>
    </div>

    <!-- 弹窗底栏 -->
    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="saving || testing"
        @click="emit('update:visible', false)"
      >
        取消
      </AdminButton>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :loading="testing"
        :disabled="loading || saving || testing"
        @click="handleTestConnection"
      >
        测试连接
      </AdminButton>
      <AdminButton
        variant="default"
        :class="adminActionButtonClass"
        :loading="saving"
        :disabled="loading || saving || testing"
        @click="handleSave"
      >
        添加网关
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Check, ShieldCheck } from '@iconoir/vue'
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../../shared/components/AdminDialogShell.vue'
import AdminInput from '../../../shared/components/AdminInput.vue'
import { formatAdminDisplayError, useAdminToast } from '../../../shared/lib/toast'
import { adminActionButtonClass } from '../../../shared/lib/uiClasses'
import {
  createAdminGatewayResource,
  type AdminGatewayConnection,
  type GatewayConnectionSummary,
} from '../../infrastructure/adminGatewayResource'

const props = defineProps<{
  visible: boolean
  resource?: AdminGatewayConnection
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  saved: [data: GatewayConnectionSummary]
}>()

const defaultResource = createAdminGatewayResource().connection
const resource = computed(() => props.resource ?? defaultResource)
const toast = useAdminToast()

const summary = ref<GatewayConnectionSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const validationError = ref('')

const form = reactive({
  base_url: '',
  token: '',
  account_id: '',
  gateway_id: '',
})

const isTokenConfigured = computed(() => Boolean(summary.value?.token_configured))

watch(
  () => props.visible,
  (open) => {
    if (open) {
      loadGatewayConfig()
    }
  },
  { immediate: true },
)

async function loadGatewayConfig() {
  loading.value = true
  error.value = ''
  validationError.value = ''
  try {
    const data = await resource.value.get()
    summary.value = data
    form.base_url = data.base_url ?? ''
    form.account_id = data.account_id ?? ''
    form.gateway_id = data.gateway_id ?? ''
    form.token = ''
  } catch (err) {
    error.value = formatAdminDisplayError(err, '读取网关配置失败')
  } finally {
    loading.value = false
  }
}

function validate(): boolean {
  validationError.value = ''
  const baseUrl = form.base_url.trim()

  if (!baseUrl) {
    validationError.value = '请输入网关 Base URL'
    return false
  }

  if (!/^https?:\/\//i.test(baseUrl)) {
    validationError.value = '网关 Base URL 必须以 http:// 或 https:// 开头'
    return false
  }

  return true
}

async function handleTestConnection() {
  if (!validate()) return

  testing.value = true
  error.value = ''
  validationError.value = ''

  try {
    const finalBaseUrl = form.base_url.trim().replace(/\/+$/, '')

    // testConnection 是 connection interface 必需成员：直调服务端探活，无假成功回退。
    const res = await resource.value.testConnection({
      base_url: finalBaseUrl,
      token: form.token.trim() || undefined,
    })
    if (res.success) {
      toast.success(res.message || '网关连接测试成功')
    } else {
      error.value = res.message || '网关连接测试失败'
      toast.error(res.message || '网关连接测试失败')
    }
  } catch (err) {
    const msg = formatAdminDisplayError(err, '网关连接测试失败')
    error.value = msg
    toast.error(msg)
  } finally {
    testing.value = false
  }
}

async function handleSave() {
  if (!validate()) return

  saving.value = true
  error.value = ''
  validationError.value = ''

  try {
    const finalBaseUrl = form.base_url.trim().replace(/\/+$/, '')

    const payload: {
      base_url?: string
      token?: string
      account_id?: string
      gateway_id?: string
    } = {
      base_url: finalBaseUrl,
    }
    if (form.token.trim()) {
      payload.token = form.token.trim()
    }
    // 身份字段显式透传（含清空）：自定义域名场景靠它们建 control-plane 身份。
    payload.account_id = form.account_id.trim().toLowerCase()
    payload.gateway_id = form.gateway_id.trim().toLowerCase()

    await resource.value.update(payload)

    toast.success('AI 网关配置已保存')
    // saved 必须是服务端确认的真相：重读失败就报错、不关弹窗、不 emit。
    try {
      emit('saved', await resource.value.get())
    } catch (err) {
      error.value = formatAdminDisplayError(err, '配置已保存，但重读网关状态失败，请重试')
      return
    }
    emit('update:visible', false)
  } catch (err) {
    error.value = formatAdminDisplayError(err, '保存网关配置失败')
  } finally {
    saving.value = false
  }
}
</script>
