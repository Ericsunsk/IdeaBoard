<template>
  <AdminDialogShell
    :visible="visible"
    :header="MODELS_MESSAGES.gatewayHeader"
    :description="MODELS_MESSAGES.gatewayDescription"
    size="compact"
    content-layout="plain"
    @update:visible="emit('update:visible', $event)"
    @close="emit('update:visible', false)"
  >
    <div class="flex flex-col gap-4">
      <!-- 字段级校验提示保留（定位到具体输入）；加载/保存失败只走 toast。 -->
      <AdminAlert v-if="form.errors.value.base_url" severity="error">
        {{ form.errors.value.base_url }}
      </AdminAlert>

      <!-- 网关 Base URL -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="gateway-base-url-input"
          >
            {{ MODELS_MESSAGES.gatewayBaseUrlLabel }}
            <span aria-hidden="true" class="text-destructive">*</span>
          </label>
          <span
            v-if="summary?.is_first_party"
            class="inline-flex items-center gap-1 text-admin-caption text-primary font-medium"
          >
            <Check class="size-3.5" />
            {{ MODELS_MESSAGES.gatewayRecognized }}
          </span>
        </div>
        <AdminInput
          id="gateway-base-url-input"
          v-model="form.values.base_url"
          :placeholder="MODELS_MESSAGES.gatewayBaseUrlPlaceholder"
          :disabled="loading || saving"
          class="w-full"
        />
      </div>

      <!-- API 密钥 -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label class="text-admin-caption font-semibold text-foreground" for="gateway-token-input">
            {{ MODELS_MESSAGES.gatewayTokenLabel }}
          </label>
          <span
            v-if="isTokenConfigured"
            class="inline-flex items-center gap-1 text-admin-caption text-success font-medium"
          >
            <ShieldCheck class="size-3.5" />
            {{ MODELS_MESSAGES.gatewayTokenConfigured }}
          </span>
        </div>
        <AdminInput
          id="gateway-token-input"
          v-model="form.values.token"
          type="password"
          :placeholder="MODELS_MESSAGES.gatewayTokenPlaceholder"
          autocomplete="off"
          :disabled="loading || saving"
          class="w-full"
        />
      </div>

      <!-- CF 身份：官方域名可空（后端从 URL 派生）；自定义域名不带身份路径，必填 -->
      <div class="flex flex-col gap-1.5">
        <label
          class="text-admin-caption font-semibold text-foreground"
          for="gateway-account-id-input"
        >
          Account ID
          <span v-if="isCustomDomain" aria-hidden="true" class="text-destructive">*</span>
        </label>
        <AdminInput
          id="gateway-account-id-input"
          v-model="form.values.account_id"
          :placeholder="MODELS_MESSAGES.gatewayAccountPlaceholder"
          autocomplete="off"
          :disabled="loading || saving"
          class="w-full font-mono"
        />
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-admin-caption font-semibold text-foreground" for="gateway-id-input">
          Gateway ID
          <span v-if="isCustomDomain" aria-hidden="true" class="text-destructive">*</span>
        </label>
        <AdminInput
          id="gateway-id-input"
          v-model="form.values.gateway_id"
          :placeholder="MODELS_MESSAGES.gatewayIdPlaceholder"
          autocomplete="off"
          :disabled="loading || saving"
          class="w-full font-mono"
        />
      </div>
    </div>

    <template #footer>
      <AdminDialogActions
        :cancel-label="MODELS_MESSAGES.cancel"
        :confirm-label="MODELS_MESSAGES.gatewaySave"
        :loading="saving"
        :disabled="loading || saving"
        :cancel-disabled="saving"
        @cancel="emit('update:visible', false)"
        @confirm="handleSave"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import * as v from 'valibot'
import { useForm } from '@/components/ui/form'
import { Check, ShieldCheck } from '@iconoir/vue'
import { AdminAlert } from '../../../shared'
import { AdminDialogActions } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { AdminInput } from '../../../shared'
import { useAdminGatewayConfigFeedback } from '../../session/useAdminModelCatalogFeedback'
import { MODELS_MESSAGES } from '../pages/messages'
import {
  createAdminGatewayResource,
  type AdminGatewayConnection,
  type GatewayConnectionSummary,
} from '../../resource'

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
const feedback = useAdminGatewayConfigFeedback()

// 非默认主机视为自定义域名：必须显式填写 Account ID 与 Gateway ID。
const DEFAULT_GATEWAY_HOST = 'gateway.ai.cloudflare.com'

function gatewayHostnameOf(raw: string): string {
  try {
    return new URL(raw.trim()).hostname.toLowerCase().replace(/\.$/, '')
  } catch {
    return ''
  }
}

const summary = ref<GatewayConnectionSummary | null>(null)
const loading = ref(false)
const saving = ref(false)
const form = useForm({
  initialValues: { base_url: '', token: '', account_id: '', gateway_id: '' },
  schema: v.object({
    base_url: v.pipe(
      v.string(),
      v.trim(),
      v.minLength(1, MODELS_MESSAGES.gatewayBaseUrlRequired),
      v.check((value) => /^https?:\/\//i.test(value), MODELS_MESSAGES.gatewayBaseUrlScheme),
    ),
  }),
})

const isTokenConfigured = computed(() => Boolean(summary.value?.token_configured))

const isCustomDomain = computed(() => {
  const host = gatewayHostnameOf(form.values.base_url ?? '')
  return host !== '' && host !== DEFAULT_GATEWAY_HOST
})

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
  try {
    const data = await resource.value.get()
    summary.value = data
    form.resetForm({
      base_url: data.base_url ?? '',
      token: '',
      account_id: data.account_id ?? '',
      gateway_id: data.gateway_id ?? '',
    })
  } catch (caughtError) {
    feedback.loadFailed(caughtError)
  } finally {
    loading.value = false
  }
}

const handleSave = form.handleSubmit(async (values) => {
  saving.value = true

  try {
    const finalBaseUrl = values.base_url.trim().replace(/\/+$/, '')

    const host = gatewayHostnameOf(finalBaseUrl)
    const accountId = values.account_id.trim().toLowerCase()
    const gatewayId = values.gateway_id.trim().toLowerCase()
    if (host !== '' && host !== DEFAULT_GATEWAY_HOST && (!accountId || !gatewayId)) {
      feedback.saveFailed(new Error(MODELS_MESSAGES.gatewayCustomIdentityRequired))
      return
    }

    const payload: {
      base_url?: string
      token?: string
      account_id?: string
      gateway_id?: string
    } = {
      base_url: finalBaseUrl,
    }
    if (values.token.trim()) {
      payload.token = values.token.trim()
    }
    // 身份字段显式透传（含清空）。
    payload.account_id = accountId
    payload.gateway_id = gatewayId

    await resource.value.update(payload)

    feedback.saved()
    // saved 必须是服务端确认的真相：重读失败就报错、不关弹窗、不 emit（toast 通知）。
    try {
      emit('saved', await resource.value.get())
    } catch (caughtError) {
      feedback.rereadFailed(caughtError)
      return
    }
    emit('update:visible', false)
  } catch (caughtError) {
    feedback.saveFailed(caughtError)
  } finally {
    saving.value = false
  }
})
</script>
