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
      <!-- 错误警告提示 -->
      <AdminAlert v-if="error || form.errors.value.base_url" severity="error">
        {{ error || form.errors.value.base_url }}
      </AdminAlert>

      <!-- 网关 Base URL -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="gateway-base-url-input"
          >
            {{ MODELS_MESSAGES.gatewayBaseUrlLabel }}
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

      <!-- CF 身份：官方域名可空（后端从 URL 派生）；自定义域名如需模型探测请填写 -->
      <div class="flex flex-col gap-1.5">
        <label
          class="text-admin-caption font-semibold text-foreground"
          for="gateway-account-id-input"
        >
          Account ID
          <span class="font-normal text-muted-foreground">{{
            MODELS_MESSAGES.gatewayCustomDomainHint
          }}</span>
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
          <span class="font-normal text-muted-foreground">{{
            MODELS_MESSAGES.gatewayCustomDomainHint
          }}</span>
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

      <!-- 连接测试目标：provider + path（#847 起必填，后端不再回退模型映射） -->
      <div class="grid grid-cols-2 gap-3">
        <div class="flex flex-col gap-1.5">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="gateway-probe-provider-input"
          >
            {{ MODELS_MESSAGES.gatewayProbeProviderLabel }}
          </label>
          <AdminInput
            id="gateway-probe-provider-input"
            v-model="probeProvider"
            :placeholder="MODELS_MESSAGES.gatewayProbeProviderPlaceholder"
            autocomplete="off"
            :disabled="loading || saving || testing"
            class="w-full font-mono"
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <label
            class="text-admin-caption font-semibold text-foreground"
            for="gateway-probe-path-input"
          >
            {{ MODELS_MESSAGES.gatewayProbePathLabel }}
          </label>
          <AdminInput
            id="gateway-probe-path-input"
            v-model="probePath"
            :placeholder="MODELS_MESSAGES.gatewayProbePathPlaceholder"
            autocomplete="off"
            :disabled="loading || saving || testing"
            class="w-full font-mono"
          />
        </div>
      </div>
    </div>

    <!-- 弹窗底栏：取消 + 第三动作（测试连接，经 extra-actions 位） + 保存 -->
    <template #footer>
      <AdminDialogActions
        :cancel-label="MODELS_MESSAGES.cancel"
        :confirm-label="MODELS_MESSAGES.gatewayHeader"
        :loading="saving"
        :disabled="loading || saving || testing"
        :cancel-disabled="saving || testing"
        @cancel="emit('update:visible', false)"
        @confirm="handleSave"
      >
        <template #extra-actions>
          <ActionButton
            variant="outline"
            :loading="testing"
            :disabled="loading || saving || testing"
            @click="handleTestConnection"
          >
            {{ MODELS_MESSAGES.gatewayTest }}
          </ActionButton>
        </template>
      </AdminDialogActions>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import * as v from 'valibot'
import { useForm } from '@/components/ui/form'
import { Check, ShieldCheck } from '@iconoir/vue'
import AdminAlert from '../../../shared/components/AdminAlert.vue'
import ActionButton from '../../../../../shared/components/ActionButton.vue'
import AdminDialogActions from '../../../shared/components/AdminDialogActions.vue'
import AdminDialogShell from '../../../shared/components/AdminDialogShell.vue'
import AdminInput from '../../../shared/components/AdminInput.vue'
import { formatAdminDisplayError } from '../../../shared/lib/toast'
import { useAdminGatewayConfigFeedback } from '../../application/useAdminModelCatalogFeedback'
import { useAdminConfirm } from '../../../shared/composables/useAdminConfirm'
import {
  ADMIN_PROBE_OPERATOR_REASON,
  confirmSettingsSave,
} from '../../../shared/lib/confirmSettingsSave'
import { MODELS_MESSAGES } from '../pages/messages'
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
const feedback = useAdminGatewayConfigFeedback()
const adminConfirm = useAdminConfirm()

const summary = ref<GatewayConnectionSummary | null>(null)
const probeProvider = ref('')
const probePath = ref('')
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
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
  try {
    const data = await resource.value.get()
    summary.value = data
    form.resetForm({
      base_url: data.base_url ?? '',
      token: '',
      account_id: data.account_id ?? '',
      gateway_id: data.gateway_id ?? '',
    })
  } catch (err) {
    error.value = formatAdminDisplayError(err, MODELS_MESSAGES.gatewayLoadFailed)
  } finally {
    loading.value = false
  }
}

const handleTestConnection = form.handleSubmit(async (values) => {
  // Server-side probes skip the confirm dialog (backend POST read-guard:
  // no reason ceremony); the fixed probe note still lands on the audit record.
  testing.value = true
  error.value = ''

  try {
    const finalBaseUrl = values.base_url.trim().replace(/\/+$/, '')

    // testConnection 是 connection interface 必需成员：直调服务端探活，无假成功回退。
    const provider = probeProvider.value.trim()
    const path = probePath.value.trim().replace(/^\/+/, '')
    if (!provider || !path) {
      error.value = MODELS_MESSAGES.gatewayProbeRequired
      return
    }
    const res = await resource.value.testConnection(
      {
        base_url: finalBaseUrl,
        token: values.token.trim() || undefined,
        provider,
        path,
      },
      { operatorReason: ADMIN_PROBE_OPERATOR_REASON },
    )
    if (res.success) {
      feedback.testPassed(res.message)
    } else {
      error.value = res.message || MODELS_MESSAGES.gatewayTestFailed
      feedback.testFailed(res.message)
    }
  } catch (err) {
    const msg = formatAdminDisplayError(err, MODELS_MESSAGES.gatewayTestFailed)
    error.value = msg
    feedback.testFailed(msg)
  } finally {
    testing.value = false
  }
})

const handleSave = form.handleSubmit(async (values) => {
  // Control writes collect the operator reason through the shared confirm
  // shell (reversible tier: note optional, empty falls back to a default);
  // a cancelled dialog aborts the save silently.
  const operatorReason = await confirmSettingsSave(adminConfirm, {
    title: MODELS_MESSAGES.gatewaySaveTitle,
    message: MODELS_MESSAGES.gatewaySaveMessage,
    confirmLabel: MODELS_MESSAGES.gatewaySaveConfirm,
  })
  if (!operatorReason) return
  saving.value = true
  error.value = ''

  try {
    const finalBaseUrl = values.base_url.trim().replace(/\/+$/, '')

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
    // 身份字段显式透传（含清空）：自定义域名场景靠它们建 control-plane 身份。
    payload.account_id = values.account_id.trim().toLowerCase()
    payload.gateway_id = values.gateway_id.trim().toLowerCase()

    await resource.value.update(payload, { operatorReason })

    feedback.saved()
    // saved 必须是服务端确认的真相：重读失败就报错、不关弹窗、不 emit。
    try {
      emit('saved', await resource.value.get())
    } catch (err) {
      error.value = formatAdminDisplayError(err, MODELS_MESSAGES.gatewayRereadFailed)
      return
    }
    emit('update:visible', false)
  } catch (err) {
    error.value = formatAdminDisplayError(err, MODELS_MESSAGES.gatewaySaveFailed)
  } finally {
    saving.value = false
  }
})
</script>
