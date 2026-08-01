<template>
  <AdminDialogShell
    :visible="visible"
    header="Stripe 设置"
    size="form"
    :busy="isBusy"
    @reset="resetAndLoad"
    @update:visible="emit('update:visible', $event)"
  >
    <div v-if="isLoading && !summary" class="flex min-h-36 items-center justify-center">
      <ProgressSpinner aria-label="加载 Stripe 设置" stroke-width="3" />
    </div>

    <template v-else>
      <Message v-if="loadError" severity="error" :closable="false">
        <div class="flex flex-wrap items-center gap-2">
          <span>{{ loadError }}</span>
          <Button
            label="重新加载"
            severity="secondary"
            size="small"
            text
            :disabled="isBusy"
            @click="load"
          />
        </div>
      </Message>

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
          label="Stripe API Key"
          for="admin-stripe-api-key"
          :error="errors.apiKey"
          :hint="apiKeyHint"
          :required="!summary?.apiKeyConfigured"
        >
          <InputText
            id="admin-stripe-api-key"
            v-model="form.apiKey"
            class="w-full"
            type="password"
            autocomplete="off"
            aria-label="Stripe API Key"
            :placeholder="apiKeyPlaceholder"
            :disabled="isBusy"
            :invalid="invalid"
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
          <InputText
            id="admin-stripe-product-tax-code"
            v-model="form.productTaxCode"
            class="w-full"
            placeholder="例如：txcd_10105001"
            aria-label="Product Tax Code"
            :disabled="isBusy"
            :invalid="invalid"
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
            <ToggleSwitch
              v-model="form.purchasesEnabled"
              input-id="admin-stripe-purchases-enabled"
              aria-label="充值开放"
              :disabled="isBusy"
            />
          </div>
        </AdminFormField>

        <Message v-if="saveError" severity="error" :closable="false">{{ saveError }}</Message>
      </form>
    </template>

    <template #footer="{ requestClose }">
      <Button label="取消" severity="secondary" outlined :disabled="isBusy" @click="requestClose" />
      <Button
        label="保存设置"
        :loading="isSaving"
        :disabled="isLoading || Boolean(loadError)"
        @click="submit"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import { formatAdminDisplayError, useAdminToast } from '../../shared/lib/adminToast'
import { createAdminStripeSettingsResource } from './stripeSettingsResource'
import type { AdminStripeSettingsResource, StripeConfigurationSummary } from './stripeSettingsTypes'

const props = withDefaults(
  defineProps<{
    visible: boolean
    resource?: AdminStripeSettingsResource
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
  productTaxCode: '',
  purchasesEnabled: false,
})
const errors = reactive({ apiKey: '', productTaxCode: '' })
const isBusy = computed(() => isLoading.value || isSaving.value)
const apiKeyPlaceholder = computed(() =>
  summary.value?.apiKeyConfigured ? '已配置，留空则保持不变' : '输入 Stripe API Key',
)
const apiKeyHint = computed(() =>
  summary.value?.apiKeyConfigured ? '为保护 Secret，不会回显当前 API Key。' : undefined,
)
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
  errors.productTaxCode = form.productTaxCode.trim() ? '' : '请输入 Product Tax Code'
  return !errors.apiKey && !errors.productTaxCode
}

function clearError(field: 'apiKey' | 'productTaxCode') {
  errors[field] = ''
}

async function submit() {
  if (isBusy.value || !validate()) return
  isSaving.value = true
  saveError.value = ''
  try {
    const nextSummary = await activeResource().save({
      ...(form.apiKey.trim() ? { apiKey: form.apiKey.trim() } : {}),
      productTaxCode: form.productTaxCode.trim(),
      purchasesEnabled: form.purchasesEnabled,
    })
    summary.value = nextSummary
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
  errors.productTaxCode = ''
}
</script>
