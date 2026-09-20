<template>
  <AdminSettingsDialogFrame
    :visible="visible"
    header="Stripe 设置"
    description="配置 Stripe 密钥与税务编码后保存。"
    :busy="isBusy"
    :loading="isLoading && !summary"
    loading-label="加载 Stripe 设置"
    :error="loadError"
    @reset="resetAndLoad"
    @update:visible="emit('update:visible', $event)"
    @retry="load"
  >
    <div
      v-if="summary"
      class="grid grid-cols-1 gap-2 rounded-xl border border-admin-border bg-admin-surface-muted p-3 sm:grid-cols-2"
    >
      <div class="min-w-0">
        <div class="text-admin-caption text-admin-text-3">账户</div>
        <div class="truncate text-admin-body font-semibold text-admin-title-color">
          {{ summary.accountId || '尚未配置' }}
        </div>
      </div>
      <div class="min-w-0">
        <div class="text-admin-caption text-admin-text-3">模式</div>
        <div class="text-admin-body font-semibold text-admin-title-color">
          {{ modeLabel }}
        </div>
      </div>
      <div class="flex items-center gap-1.5 text-admin-caption text-admin-text-2">
        <span
          class="size-1.5 rounded-full"
          :class="summary.productReady ? 'bg-admin-success' : 'bg-admin-warning'"
        />
        {{ summary.productReady ? 'Product 已就绪' : 'Product 未就绪' }}
      </div>
      <div class="flex items-center gap-1.5 text-admin-caption text-admin-text-2">
        <span
          class="size-1.5 rounded-full"
          :class="summary.webhookReady ? 'bg-admin-success' : 'bg-admin-warning'"
        />
        {{ summary.webhookReady ? 'Webhook 已就绪' : 'Webhook 未就绪' }}
      </div>
    </div>

    <form class="flex flex-col gap-4" novalidate @submit.prevent="submit">
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Payment Method Configuration ID"
        for="admin-stripe-payment-method-configuration-id"
        :error="errors.paymentMethodConfigurationId"
        required
      >
        <AdminInput
          id="admin-stripe-payment-method-configuration-id"
          v-model="form.paymentMethodConfigurationId"
          class="w-full"
          placeholder="pmc_..."
          maxlength="255"
          aria-label="Payment Method Configuration ID"
          :disabled="isBusy"
          :class="invalid ? 'border-admin-danger focus-visible:ring-admin-danger/20' : undefined"
          :aria-describedby="describedbyId"
          @update:model-value="clearError('paymentMethodConfigurationId')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Stripe API Key"
        for="admin-stripe-api-key"
        :error="errors.apiKey"
        hint="支持 sk_ 或 rk_ 格式。"
        :required="!summary?.apiKeyConfigured"
      >
        <AdminInput
          id="admin-stripe-api-key"
          v-model="form.apiKey"
          class="w-full"
          type="password"
          autocomplete="off"
          aria-label="Stripe API Key"
          :placeholder="apiKeyPlaceholder"
          :disabled="isBusy"
          :class="invalid ? 'border-admin-danger focus-visible:ring-admin-danger/20' : undefined"
          :aria-describedby="describedbyId"
          @update:model-value="clearError('apiKey')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Product Tax Code"
        for="admin-stripe-product-tax-code"
        :error="errors.productTaxCode"
        required
      >
        <AdminInput
          id="admin-stripe-product-tax-code"
          v-model="form.productTaxCode"
          class="w-full"
          placeholder="例如：txcd_10105001"
          aria-label="Product Tax Code"
          :disabled="isBusy"
          :class="invalid ? 'border-admin-danger focus-visible:ring-admin-danger/20' : undefined"
          :aria-describedby="describedbyId"
          @update:model-value="clearError('productTaxCode')"
        />
      </AdminFormField>

      <AdminFormField label="充值开放" for="admin-stripe-purchases-enabled">
        <div
          class="flex min-h-10 items-center justify-between rounded-xl border border-admin-border bg-admin-surface-muted px-3"
        >
          <span class="text-admin-body text-admin-text-2">
            {{ form.purchasesEnabled ? '已开放' : '已关闭' }}
          </span>
          <AdminSwitch
            id="admin-stripe-purchases-enabled"
            v-model="form.purchasesEnabled"
            aria-label="充值开放"
            :disabled="isBusy"
          />
        </div>
      </AdminFormField>

      <AdminAlert v-if="saveError" severity="error">{{ saveError }}</AdminAlert>
    </form>

    <template #footer="{ requestClose }">
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="isBusy"
        @click="requestClose"
      >
        取消
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="isSaving"
        :disabled="isLoading || Boolean(loadError)"
        @click="submit"
      >
        保存设置
      </AdminButton>
    </template>
  </AdminSettingsDialogFrame>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminSettingsDialogFrame from '../../shared/components/AdminSettingsDialogFrame.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/toast'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { createAdminStripeSettingsResource } from './resource'
import type { AdminStripeSettingsResource, StripeConfigurationSummary } from './types'

const props = withDefaults(
  defineProps<{
    visible: boolean
    resource?: AdminStripeSettingsResource | undefined
  }>(),
  { resource: undefined },
)
const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const defaultResource = createAdminStripeSettingsResource()
const toast = useAdminToast()
const summary = ref<StripeConfigurationSummary | null>(null)
const isLoading = ref(false)
const isSaving = ref(false)
const loadError = ref('')
const saveError = ref('')
const form = reactive({
  apiKey: '',
  paymentMethodConfigurationId: '',
  productTaxCode: '',
  purchasesEnabled: false,
})
const errors = reactive({ apiKey: '', paymentMethodConfigurationId: '', productTaxCode: '' })
const isBusy = computed(() => isLoading.value || isSaving.value)
const apiKeyPlaceholder = computed(() =>
  summary.value?.apiKeyConfigured ? '已配置，留空则保持不变' : '输入 Stripe API Key',
)
const paymentMethodConfigurationIdPattern = /^pmc_[A-Za-z0-9]+$/
const modeLabel = computed(() =>
  summary.value?.mode === 'live' ? 'Live Mode' : summary.value?.mode === 'test' ? 'Test Mode' : '—',
)

function activeResource() {
  return props.resource ?? defaultResource
}

function resetAndLoad() {
  clearSensitiveState()
  saveError.value = ''
  loadError.value = ''
  void load()
}

async function load() {
  if (isBusy.value) return
  isLoading.value = true
  loadError.value = ''
  try {
    const nextSummary = await activeResource().get()
    summary.value = nextSummary
    form.paymentMethodConfigurationId = nextSummary.paymentMethodConfigurationId
    form.productTaxCode = nextSummary.productTaxCode
    form.purchasesEnabled = nextSummary.purchasesEnabled
  } catch (error) {
    loadError.value = formatAdminDisplayError(error, '加载 Stripe 设置失败，请重试')
  } finally {
    isLoading.value = false
  }
}

function validate() {
  errors.apiKey =
    !summary.value?.apiKeyConfigured && !form.apiKey.trim() ? '请输入 Stripe API Key' : ''
  const paymentMethodConfigurationId = form.paymentMethodConfigurationId.trim()
  errors.paymentMethodConfigurationId = !paymentMethodConfigurationId
    ? '请输入 Payment Method Configuration ID'
    : paymentMethodConfigurationIdPattern.test(paymentMethodConfigurationId)
      ? ''
      : '请输入有效的 pmc_... ID'
  errors.productTaxCode = form.productTaxCode.trim() ? '' : '请输入 Product Tax Code'
  return !errors.apiKey && !errors.paymentMethodConfigurationId && !errors.productTaxCode
}

function clearError(field: 'apiKey' | 'paymentMethodConfigurationId' | 'productTaxCode') {
  errors[field] = ''
}

async function submit() {
  if (isBusy.value || !validate()) return
  isSaving.value = true
  saveError.value = ''
  try {
    const nextSummary = await activeResource().save({
      ...(form.apiKey.trim() ? { apiKey: form.apiKey.trim() } : {}),
      paymentMethodConfigurationId: form.paymentMethodConfigurationId.trim(),
      productTaxCode: form.productTaxCode.trim(),
      purchasesEnabled: form.purchasesEnabled,
    })
    summary.value = nextSummary
    form.paymentMethodConfigurationId = nextSummary.paymentMethodConfigurationId
    form.productTaxCode = nextSummary.productTaxCode
    form.purchasesEnabled = nextSummary.purchasesEnabled
    clearSensitiveState()
    toast.success('Stripe 设置已保存')
    emit('update:visible', false)
  } catch (error) {
    clearSensitiveState()
    saveError.value = formatAdminDisplayError(error, '保存 Stripe 设置失败，请重试')
  } finally {
    isSaving.value = false
  }
}

function clearSensitiveState() {
  form.apiKey = ''
  errors.apiKey = ''
  errors.paymentMethodConfigurationId = ''
  errors.productTaxCode = ''
}
</script>
