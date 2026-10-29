<template>
  <AdminSettingsDialogFrame
    :visible="isVisible"
    :header="OAUTH_MESSAGES.oauthSettings"
    :description="OAUTH_MESSAGES.oauthDescription"
    :busy="isSaving"
    :loading="isLoading && !summary"
    :loading-label="OAUTH_MESSAGES.oauthLoading"
    :error="null"
    @reset="resetAndLoad"
    @update:visible="emit('update:isVisible', $event)"
  >
    <form v-if="summary" class="flex flex-col gap-3.5 py-1" novalidate @submit.prevent="submit">
      <div class="grid grid-cols-2 gap-3" role="group" :aria-label="OAUTH_MESSAGES.oauthGroupAria">
        <div
          v-for="provider in oauthLoginProviders"
          :key="provider.value"
          class="group relative flex items-center justify-between rounded-panel border p-3 cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] select-none"
          :class="
            selectedProvider === provider.value
              ? 'border-primary bg-primary-faint shadow-raised'
              : 'border-border bg-muted/40 hover:border-border hover:bg-muted'
          "
          @click="selectProvider(provider.value)"
        >
          <div class="flex items-center gap-2.5 min-w-0">
            <button
              type="button"
              class="flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-control border border-border bg-card p-1.5 shadow-raised"
              :aria-label="OAUTH_MESSAGES.oauthSelectLogoAria(provider.label)"
              @click.stop="openLogoPicker($event, provider.value)"
            >
              <img
                :src="oauthProviderLogo(provider.value)"
                class="size-full object-contain"
                alt=""
                aria-hidden="true"
              />
            </button>
            <span class="text-admin-body font-bold text-foreground truncate">
              {{ provider.label }}
            </span>
          </div>

          <div class="shrink-0 pl-2" @click.stop>
            <AdminSwitch
              :id="`admin-oauth-enabled-${provider.value}`"
              :name="`oauth_enabled_${provider.value}`"
              :model-value="providerEnabled[provider.value] ?? false"
              :aria-label="OAUTH_MESSAGES.oauthProviderLoginAria(provider.label)"
              :disabled="isBusy"
              @update:model-value="setProviderEnabled(provider.value, $event)"
            />
          </div>
        </div>
      </div>

      <!-- 语义化 Provider 切换器（可访问性与测试合约） -->
      <AdminFormField label="OAuth Provider" for="admin-oauth-provider" class="sr-only">
        <AdminSelect
          id="admin-oauth-provider"
          v-model="selectedProvider"
          name="oauth_provider"
          aria-label="OAuth Provider"
          :options="oauthLoginProviders"
          option-label="label"
          option-value="value"
          class="w-full"
          :disabled="isBusy"
        />
      </AdminFormField>

      <div
        class="flex flex-col gap-3.5 rounded-panel border border-border bg-card p-4 shadow-raised"
      >
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-control border border-border bg-muted p-1"
            :aria-label="OAUTH_MESSAGES.oauthSelectLogoAria(providerLabel)"
            @click="openLogoPicker($event, selectedProvider)"
          >
            <img
              :src="oauthProviderLogo(selectedProvider)"
              class="size-full object-contain"
              alt=""
              aria-hidden="true"
            />
          </button>
          <h3 class="m-0 text-admin-body font-bold text-foreground">
            {{ OAUTH_MESSAGES.oauthCredentialsTitle(providerLabel) }}
          </h3>
        </div>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="Client ID"
          for="admin-oauth-client-id"
          :error="form.errors.value.clientId"
          :required="form.values.enabled"
        >
          <AdminInput
            id="admin-oauth-client-id"
            v-model="form.values.clientId"
            name="oauth_client_id"
            class="w-full"
            autocomplete="off"
            aria-label="Client ID"
            :placeholder="OAUTH_MESSAGES.oauthClientIdPlaceholder"
            :disabled="isBusy"
            :is-invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="form.clearErrors('clientId')"
          />
        </AdminFormField>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="Client Secret"
          for="admin-oauth-client-secret"
          :error="form.errors.value.clientSecret"
          :required="form.values.enabled && !summary.clientSecretConfigured"
        >
          <AdminInput
            id="admin-oauth-client-secret"
            v-model="form.values.clientSecret"
            name="oauth_client_secret"
            type="password"
            autocomplete="new-password"
            class="w-full"
            aria-label="Client Secret"
            :placeholder="secretPlaceholder"
            :disabled="isBusy"
            :is-invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="form.clearErrors('clientSecret')"
          />
        </AdminFormField>

        <AdminFormField label="Callback URL" for="admin-oauth-callback-url">
          <div class="relative flex items-center">
            <AdminInput
              id="admin-oauth-callback-url"
              name="oauth_callback_url"
              :model-value="summary.callbackUrl"
              class="w-full font-mono text-admin-caption pr-16"
              aria-label="Callback URL"
              readonly
            />
            <AdminButton
              type="button"
              size="sm"
              variant="ghost"
              class="!absolute right-1 !h-7 !px-2.5 text-admin-caption font-semibold"
              @click="copyCallbackUrl"
            >
              {{ OAUTH_MESSAGES.oauthCopyCallback }}
            </AdminButton>
          </div>
          <p class="m-0 mt-1 text-admin-micro text-muted-foreground">
            {{
              OAUTH_MESSAGES.oauthCallbackHint(
                providerLabel === 'Google' ? 'Google Cloud Console' : 'Discord Developer Portal',
              )
            }}
          </p>
        </AdminFormField>
      </div>
    </form>

    <template #footer="{ requestClose }">
      <AdminDialogActions
        :confirm-label="OAUTH_MESSAGES.oauthSave"
        :loading="isSaving"
        :disabled="isLoading || !summary"
        :cancel-disabled="isBusy"
        confirm-variant="default"
        @cancel="requestClose"
        @confirm="submit"
      />
    </template>
  </AdminSettingsDialogFrame>

  <AdminLogoPickerPopover
    ref="logoPickerPopoverRef"
    scope="oauth-provider"
    :target-id="activeLogoProvider || undefined"
    :target-name="activeLogoProvider ? getOAuthProviderLabel(activeLogoProvider) : undefined"
    initial-category="auth"
    @hide="activeLogoProvider = null"
  />
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import * as v from 'valibot'
import { useForm } from '@/components/ui/form'
import { AdminButton } from '../../../../shared/index'
import { AdminDialogActions } from '../../../../shared/index'
import { AdminInput } from '../../../../shared/index'
import { AdminSelect } from '../../../../shared/index'
import { AdminSwitch } from '../../../../shared/index'
import { AdminSettingsDialogFrame } from '../../../../shared/index'
import { AdminFormField } from '../../../../shared/index'
import { AdminLogoPickerPopover } from '../../../../shared/index'
import { formatAdminDisplayError, useAdminToast } from '../../../../shared/index'
import { useAppClipboard } from '@/core/composables/useAppClipboard'
import { secretField } from '../../../../shared/index'
import {
  createOAuthProviderSettingsResource,
  type OAuthProviderSettingsResource,
  type OAuthProviderSettingsSummary,
} from '../../../session/oauth/resource'
import { getOAuthProviderLabel, type OAuthProviderCode } from '@/core/auth/oauthProviders'
import { resolveOAuthProviderLogo } from './logo'
import { OAUTH_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{
    isVisible: boolean
    resource?: OAuthProviderSettingsResource | undefined
  }>(),
  { resource: undefined },
)
const emit = defineEmits<{
  'update:isVisible': [isVisible: boolean]
}>()

const defaultResource = createOAuthProviderSettingsResource()
const toast = useAdminToast()
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)
const activeLogoProvider = ref<OAuthProviderCode | null>(null)
const oauthLoginProviders = [
  { label: 'Google', value: 'google' as OAuthProviderCode },
  { label: 'Discord', value: 'discord' as OAuthProviderCode },
]
const selectedProvider = ref<OAuthProviderCode>('google')
const summary = ref<OAuthProviderSettingsSummary | null>(null)
const providerSummaries = ref<Record<string, OAuthProviderSettingsSummary>>({})
const providerEnabled = reactive<Record<string, boolean>>({})
const initialProviderEnabled = reactive<Record<string, boolean>>({})
const isLoading = ref(false)
const isSaving = ref(false)
const form = useForm({
  initialValues: { enabled: false, clientId: '', clientSecret: '' },
  schema: () =>
    v.object({
      enabled: v.boolean(),
      clientId: secretField(form.values.enabled, OAUTH_MESSAGES.oauthClientIdRequired),
      clientSecret: secretField(
        form.values.enabled && !summary.value?.clientSecretConfigured,
        OAUTH_MESSAGES.oauthClientSecretRequired,
      ),
    }),
})
const isBusy = computed(() => isSaving.value || isLoading.value)
const providerLabel = computed(() => getOAuthProviderLabel(selectedProvider.value))
const secretPlaceholder = computed(() =>
  OAUTH_MESSAGES.oauthSecretPlaceholder(Boolean(summary.value?.clientSecretConfigured)),
)

function oauthProviderLogo(provider: OAuthProviderCode): string {
  return resolveOAuthProviderLogo(provider)
}

function openLogoPicker(event: Event, provider: OAuthProviderCode) {
  if (isBusy.value) return
  selectedProvider.value = provider
  activeLogoProvider.value = provider
  logoPickerPopoverRef.value?.toggle(event)
}

function selectProvider(provider: OAuthProviderCode) {
  if (!isBusy.value) selectedProvider.value = provider
}

watch(selectedProvider, (provider) => {
  if (!props.isVisible) return
  clearSensitiveState()
  const cachedSummary = providerSummaries.value[provider]
  // 按需单 provider GET：已缓存直接复用，不再每次切换都全量往返。
  if (cachedSummary) {
    applySummary(cachedSummary)
    return
  }
  void loadSelectedProvider(provider)
})

function activeResource() {
  return props.resource ?? defaultResource
}

function resetAndLoad() {
  clearSensitiveState()
  void loadSelectedProvider(selectedProvider.value)
}

// Opening and uncached provider switches share the same guarded load.
async function loadSelectedProvider(provider: OAuthProviderCode) {
  if (isBusy.value) return
  summary.value = null
  isLoading.value = true
  try {
    const nextSummary = await activeResource().get(provider)
    providerSummaries.value[provider] = nextSummary
    if (selectedProvider.value === provider) applySummary(nextSummary)
    providerEnabled[provider] = nextSummary.isEnabled
    initialProviderEnabled[provider] = nextSummary.isEnabled
  } catch (error) {
    toast.error(formatAdminDisplayError(error, OAUTH_MESSAGES.oauthLoadFailed))
  } finally {
    isLoading.value = false
  }
}

function setProviderEnabled(provider: OAuthProviderCode, enabled: boolean) {
  providerEnabled[provider] = enabled
  if (provider === selectedProvider.value) form.values.enabled = enabled
}

// Load unvisited providers before validating their pending enablement changes.
async function ensureChangedProviderSummaries(providers: OAuthProviderCode[]) {
  await Promise.all(
    providers.map(async (provider) => {
      if (providerSummaries.value[provider]) return
      const nextSummary = await activeResource().get(provider)
      providerSummaries.value[provider] = nextSummary
      // 不覆盖用户的待保存拨动：providerEnabled 保持开关状态，initial
      // 记后端真相源供对比；若拨动与后端一致，重算后自动消为未变更。
      if (initialProviderEnabled[provider] === undefined) {
        initialProviderEnabled[provider] = nextSummary.isEnabled
      }
    }),
  )
}

function changedProvidersOf(): OAuthProviderCode[] {
  return oauthLoginProviders
    .map((provider) => provider.value)
    .filter(
      (provider) =>
        provider !== selectedProvider.value &&
        providerEnabled[provider] !== initialProviderEnabled[provider],
    )
}

const submit = form.handleSubmit(async (values) => {
  if (isBusy.value || !summary.value) return

  const candidates = changedProvidersOf()
  try {
    await ensureChangedProviderSummaries(candidates)
  } catch (error) {
    toast.error(formatAdminDisplayError(error, OAUTH_MESSAGES.oauthLoadFailed))
    return
  }
  const changedProviders = changedProvidersOf()
  const incompleteProvider = changedProviders.find(
    (provider) => providerEnabled[provider] && !providerSummaries.value[provider]?.configured,
  )
  if (incompleteProvider) {
    toast.error(OAUTH_MESSAGES.oauthIncompleteProvider(getOAuthProviderLabel(incompleteProvider)))
    return
  }

  isSaving.value = true
  try {
    const currentProvider = selectedProvider.value
    const saveRequests = [
      activeResource().save(currentProvider, {
        isEnabled: values.enabled,
        clientId: values.clientId,
        clientSecret: values.clientSecret,
      }),
      ...changedProviders.map((provider) =>
        activeResource().save(provider, {
          isEnabled: providerEnabled[provider] ?? false,
          clientId: providerSummaries.value[provider]?.clientId ?? '',
          clientSecret: '',
        }),
      ),
    ]
    const savedSummaries = await Promise.all(saveRequests)
    const [savedSummary, ...savedChanged] = savedSummaries
    // Save responses refresh the dialog cache; the parent refetches on close.
    savedChanged.forEach((nextSummary, index) => {
      const provider = changedProviders[index]!
      providerSummaries.value[provider] = nextSummary
      providerEnabled[provider] = nextSummary.isEnabled
      initialProviderEnabled[provider] = nextSummary.isEnabled
    })
    clearSensitiveState()
    if (savedSummary) {
      providerSummaries.value[currentProvider] = savedSummary
      providerEnabled[currentProvider] = savedSummary.isEnabled
      initialProviderEnabled[currentProvider] = savedSummary.isEnabled
      applySummary(savedSummary)
    }
    toast.success(OAUTH_MESSAGES.oauthSaved(providerLabel.value))
    emit('update:isVisible', false)
  } catch (error) {
    clearSensitiveState()
    toast.error(formatAdminDisplayError(error, OAUTH_MESSAGES.oauthSaveFailed))
  } finally {
    isSaving.value = false
  }
})

function applySummary(nextSummary: OAuthProviderSettingsSummary) {
  summary.value = nextSummary
  form.resetForm({
    enabled: nextSummary.isEnabled,
    clientId: nextSummary.clientId,
    clientSecret: '',
  })
  providerEnabled[selectedProvider.value] = nextSummary.isEnabled
  clearSensitiveState()
}

function clearSensitiveState() {
  form.values.clientSecret = ''
  form.clearErrors('clientSecret')
}

const { copyText } = useAppClipboard()

async function copyCallbackUrl() {
  if (!summary.value?.callbackUrl) return
  const ok = await copyText(summary.value.callbackUrl)
  if (ok) toast.success(OAUTH_MESSAGES.oauthCallbackCopied)
  else toast.error(OAUTH_MESSAGES.oauthCopyFailed)
}
</script>
