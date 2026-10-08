<template>
  <AdminSettingsDialogFrame
    :visible="visible"
    :header="PAYMENTS_MESSAGES.stripeHeader"
    :description="PAYMENTS_MESSAGES.stripeDescription"
    :busy="isBusy"
    :loading="isLoading && !summary"
    :loading-label="PAYMENTS_MESSAGES.stripeLoading"
    :error="loadError"
    @reset="resetAndLoad"
    @update:visible="emit('update:visible', $event)"
    @retry="load"
  >
    <div
      v-if="summary"
      class="grid grid-cols-1 gap-2 rounded-xl border border-border bg-muted p-3 sm:grid-cols-2"
    >
      <div class="min-w-0">
        <div class="text-admin-caption text-muted-foreground">
          {{ PAYMENTS_MESSAGES.accountLabel }}
        </div>
        <div class="truncate text-admin-body font-semibold text-foreground">
          {{ summary.accountId || PAYMENTS_MESSAGES.accountUnset }}
        </div>
      </div>
      <div class="min-w-0">
        <div class="text-admin-caption text-muted-foreground">
          {{ PAYMENTS_MESSAGES.modeLabel }}
        </div>
        <div class="text-admin-body font-semibold text-foreground">
          {{ modeLabel }}
        </div>
      </div>
      <div class="flex items-center gap-1.5 text-admin-caption text-muted-foreground">
        <span
          class="size-1.5 rounded-full"
          :class="summary.productReady ? 'bg-success' : 'bg-warning'"
        />
        {{ PAYMENTS_MESSAGES.productReadyState(summary.productReady) }}
      </div>
      <div class="flex items-center gap-1.5 text-admin-caption text-muted-foreground">
        <span
          class="size-1.5 rounded-full"
          :class="summary.webhookReady ? 'bg-success' : 'bg-warning'"
        />
        {{ PAYMENTS_MESSAGES.webhookReadyState(summary.webhookReady) }}
      </div>
    </div>

    <form class="flex flex-col gap-4" novalidate @submit.prevent="submit">
      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Payment Method Configuration ID"
        for="admin-stripe-payment-method-configuration-id"
        :error="form.errors.value.paymentMethodConfigurationId"
        required
      >
        <AdminInput
          id="admin-stripe-payment-method-configuration-id"
          v-model="form.values.paymentMethodConfigurationId"
          class="w-full"
          placeholder="pmc_..."
          maxlength="255"
          aria-label="Payment Method Configuration ID"
          :disabled="isBusy"
          :class="invalid ? '!border-destructive focus-visible:ring-destructive/20' : undefined"
          :aria-describedby="describedbyId"
          @update:model-value="clearError('paymentMethodConfigurationId')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Stripe API Key"
        for="admin-stripe-api-key"
        :error="form.errors.value.apiKey"
        :hint="PAYMENTS_MESSAGES.apiKeyHint"
        :required="!summary?.apiKeyConfigured"
      >
        <AdminInput
          id="admin-stripe-api-key"
          v-model="form.values.apiKey"
          class="w-full"
          type="password"
          autocomplete="off"
          aria-label="Stripe API Key"
          :placeholder="apiKeyPlaceholder"
          :disabled="isBusy"
          :class="invalid ? '!border-destructive focus-visible:ring-destructive/20' : undefined"
          :aria-describedby="describedbyId"
          @update:model-value="clearError('apiKey')"
        />
      </AdminFormField>

      <AdminFormField
        v-slot="{ describedbyId, invalid }"
        label="Product Tax Code"
        for="admin-stripe-product-tax-code"
        :error="form.errors.value.productTaxCode"
        required
      >
        <AdminInput
          id="admin-stripe-product-tax-code"
          v-model="form.values.productTaxCode"
          class="w-full"
          :placeholder="PAYMENTS_MESSAGES.taxCodePlaceholder"
          aria-label="Product Tax Code"
          :disabled="isBusy"
          :class="invalid ? '!border-destructive focus-visible:ring-destructive/20' : undefined"
          :aria-describedby="describedbyId"
          @update:model-value="clearError('productTaxCode')"
        />
      </AdminFormField>

      <AdminFormField
        :label="PAYMENTS_MESSAGES.purchasesLabel"
        for="admin-stripe-purchases-enabled"
      >
        <div
          class="flex min-h-10 items-center justify-between rounded-xl border border-border bg-muted px-3"
        >
          <span class="text-admin-body text-muted-foreground">
            {{ PAYMENTS_MESSAGES.purchasesState(form.values.purchasesEnabled) }}
          </span>
          <AdminSwitch
            id="admin-stripe-purchases-enabled"
            v-model="form.values.purchasesEnabled"
            :aria-label="PAYMENTS_MESSAGES.purchasesLabel"
            :disabled="isBusy"
          />
        </div>
      </AdminFormField>

      <AdminAlert v-if="saveError" severity="error">{{ saveError }}</AdminAlert>
    </form>

    <template #footer="{ requestClose }">
      <AdminDialogActions
        :cancel-label="PAYMENTS_MESSAGES.cancel"
        :confirm-label="PAYMENTS_MESSAGES.saveSettings"
        :loading="isSaving"
        :disabled="isLoading || Boolean(loadError)"
        :cancel-disabled="isBusy"
        @cancel="requestClose"
        @confirm="submit"
      />
    </template>
  </AdminSettingsDialogFrame>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import * as v from 'valibot'
import { useForm } from '@/components/ui/form'
import AdminAlert from '../../shared/components/AdminAlert.vue'
import AdminDialogActions from '../../shared/components/AdminDialogActions.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import AdminSettingsDialogFrame from '../../shared/components/AdminSettingsDialogFrame.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import { formatAdminDisplayError } from '../../shared/lib/toast'
import { useStripeSettingsFeedback } from '../useStripeSettingsFeedback'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import { confirmSettingsSave } from '../../shared/lib/confirmSettingsSave'
import { secretField } from '../../shared/lib/secretSchema'
import { createAdminStripeSettingsResource } from './resource'
import { PAYMENTS_MESSAGES } from '../messages'
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
const feedback = useStripeSettingsFeedback()
const adminConfirm = useAdminConfirm()
const summary = ref<StripeConfigurationSummary | null>(null)
const isLoading = ref(false)
const isSaving = ref(false)
const loadError = ref('')
const saveError = ref('')
const form = useForm({
  initialValues: {
    apiKey: '',
    paymentMethodConfigurationId: '',
    productTaxCode: '',
    purchasesEnabled: false,
  },
  schema: () =>
    v.object({
      apiKey: secretField(!summary.value?.apiKeyConfigured, PAYMENTS_MESSAGES.apiKeyRequired),
      paymentMethodConfigurationId: v.pipe(
        v.string(),
        v.trim(),
        v.check((value) => value.length > 0, PAYMENTS_MESSAGES.pmcRequired),
        v.check(
          (value) => value.length === 0 || paymentMethodConfigurationIdPattern.test(value),
          PAYMENTS_MESSAGES.pmcInvalid,
        ),
      ),
      productTaxCode: v.pipe(
        v.string(),
        v.trim(),
        v.minLength(1, PAYMENTS_MESSAGES.productTaxCodeRequired),
      ),
    }),
})
const isBusy = computed(() => isLoading.value || isSaving.value)
const apiKeyPlaceholder = computed(() =>
  PAYMENTS_MESSAGES.apiKeyPlaceholder(Boolean(summary.value?.apiKeyConfigured)),
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
    form.resetForm({
      apiKey: '',
      paymentMethodConfigurationId: nextSummary.paymentMethodConfigurationId,
      productTaxCode: nextSummary.productTaxCode,
      purchasesEnabled: nextSummary.purchasesEnabled,
    })
  } catch (error) {
    loadError.value = formatAdminDisplayError(error, PAYMENTS_MESSAGES.stripeLoadFailed)
  } finally {
    isLoading.value = false
  }
}

function clearError(field: 'apiKey' | 'paymentMethodConfigurationId' | 'productTaxCode') {
  form.clearErrors(field)
}

const submit = form.handleSubmit(async (values) => {
  if (isBusy.value) return
  // Control writes collect the operator reason through the shared confirm
  // shell (reversible tier: note optional, empty falls back to a default);
  // a cancelled dialog aborts the save silently.
  const operatorReason = await confirmSettingsSave(adminConfirm, {
    title: PAYMENTS_MESSAGES.stripeSaveTitle,
    message: PAYMENTS_MESSAGES.stripeSaveMessage,
    confirmLabel: PAYMENTS_MESSAGES.stripeSaveConfirm,
  })
  if (!operatorReason) return
  isSaving.value = true
  saveError.value = ''
  try {
    const nextSummary = await activeResource().save(
      {
        ...(values.apiKey.trim() ? { apiKey: values.apiKey.trim() } : {}),
        paymentMethodConfigurationId: values.paymentMethodConfigurationId.trim(),
        productTaxCode: values.productTaxCode.trim(),
        purchasesEnabled: values.purchasesEnabled,
      },
      { operatorReason },
    )
    summary.value = nextSummary
    form.resetForm({
      apiKey: '',
      paymentMethodConfigurationId: nextSummary.paymentMethodConfigurationId,
      productTaxCode: nextSummary.productTaxCode,
      purchasesEnabled: nextSummary.purchasesEnabled,
    })
    clearSensitiveState()
    feedback.saved()
    emit('update:visible', false)
  } catch (error) {
    clearSensitiveState()
    saveError.value = formatAdminDisplayError(error, PAYMENTS_MESSAGES.stripeSaveFailed)
  } finally {
    isSaving.value = false
  }
})

function clearSensitiveState() {
  form.values.apiKey = ''
  form.clearErrors()
}
</script>
