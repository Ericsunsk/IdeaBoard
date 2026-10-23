<template>
  <AdminSettingsDialogFrame
    :visible="visible"
    :header="OAUTH_MESSAGES.oauthSettings"
    :description="OAUTH_MESSAGES.oauthDescription"
    :busy="isSaving"
    :loading="isLoading && !summary"
    :loading-label="OAUTH_MESSAGES.oauthLoading"
    :error="loadError"
    @reset="resetAndLoad"
    @update:visible="emit('update:visible', $event)"
    @retry="load"
  >
    <form v-if="summary" class="flex flex-col gap-3.5 py-1" novalidate @submit.prevent="submit">
      <!-- 1. 顶部 Provider 选择与启用卡片网格 -->
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
          @click="selectedProvider = provider.value"
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

      <!-- 2. 当前选中 Provider 的凭据与回调配置区 -->
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
        :disabled="isLoading || Boolean(loadError) || !summary"
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
import {
  getOAuthProviderLabel,
  type OAuthProviderCode,
} from '@/core/auth/oauthProviders'
import { resolveOAuthProviderLogo } from './logo'
import { OAUTH_MESSAGES } from './messages'

const props = withDefaults(
  defineProps<{
    visible: boolean
    resource?: OAuthProviderSettingsResource | undefined
  }>(),
  { resource: undefined },
)
const emit = defineEmits<{
  'update:visible': [visible: boolean]
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
const providerSwitching = ref(false)
const loadError = ref('')
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
let providerLoadEpoch = 0
const isBusy = computed(() => isSaving.value || (isLoading.value && !summary.value))
const providerLabel = computed(() => getOAuthProviderLabel(selectedProvider.value))
const secretPlaceholder = computed(() =>
  OAUTH_MESSAGES.oauthSecretPlaceholder(Boolean(summary.value?.clientSecretConfigured)),
)

function oauthProviderLogo(provider: OAuthProviderCode): string {
  return resolveOAuthProviderLogo(provider)
}

function openLogoPicker(event: Event, provider: OAuthProviderCode) {
  selectedProvider.value = provider
  activeLogoProvider.value = provider
  logoPickerPopoverRef.value?.toggle(event)
}

watch(selectedProvider, (provider) => {
  if (!props.visible) return
  clearSensitiveState()
  loadError.value = ''
  const cachedSummary = providerSummaries.value[provider]
  if (cachedSummary) applySummary(cachedSummary)
  void loadSelectedProvider(provider)
})

function activeResource() {
  return props.resource ?? defaultResource
}

function resetAndLoad() {
  clearSensitiveState()
  loadError.value = ''
  void load()
}

async function load() {
  if (isBusy.value) return
  isLoading.value = true
  loadError.value = ''
  try {
    const state = await fetchProviderState()
    applySummary(state.summaries[selectedProvider.value] ?? state.firstSummary)
    applyProviderSummaries(state.summaries)
  } catch (error) {
    loadError.value = formatAdminDisplayError(error, OAUTH_MESSAGES.oauthLoadFailed)
  } finally {
    isLoading.value = false
  }
}

async function loadSelectedProvider(provider: OAuthProviderCode) {
  const epoch = ++providerLoadEpoch
  providerSwitching.value = true
  try {
    const nextSummary = await activeResource().get(provider)
    providerSummaries.value[provider] = nextSummary
    if (epoch !== providerLoadEpoch) return
    if (selectedProvider.value === provider) applySummary(nextSummary)
    providerEnabled[provider] = nextSummary.enabled
    initialProviderEnabled[provider] = nextSummary.enabled
  } catch (error) {
    if (epoch === providerLoadEpoch) {
      loadError.value = formatAdminDisplayError(error, OAUTH_MESSAGES.oauthLoadFailed)
    }
  } finally {
    if (epoch === providerLoadEpoch) providerSwitching.value = false
  }
}

function setProviderEnabled(provider: OAuthProviderCode, enabled: boolean) {
  providerEnabled[provider] = enabled
  if (provider === selectedProvider.value) form.values.enabled = enabled
}

const submit = form.handleSubmit(async (values) => {
  if (isBusy.value || providerSwitching.value || !summary.value) return
  const changedProviders = oauthLoginProviders
    .map((provider) => provider.value)
    .filter(
      (provider) =>
        provider !== selectedProvider.value &&
        providerEnabled[provider] !== initialProviderEnabled[provider],
    )
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
        enabled: values.enabled,
        clientId: values.clientId,
        clientSecret: values.clientSecret,
      }),
      ...changedProviders.map((provider) =>
        activeResource().save(provider, {
          enabled: providerEnabled[provider] ?? false,
          clientId: providerSummaries.value[provider]?.clientId ?? '',
          clientSecret: '',
        }),
      ),
    ]
    const [savedSummary] = await Promise.all(saveRequests)
    clearSensitiveState()
    if (savedSummary) applySummary(savedSummary)
    toast.success(OAUTH_MESSAGES.oauthSaved(providerLabel.value))
    try {
      const state = await fetchProviderState()
      applySummary(state.summaries[selectedProvider.value] ?? state.firstSummary)
      applyProviderSummaries(state.summaries)
      emit('update:visible', false)
    } catch {
      loadError.value = OAUTH_MESSAGES.oauthRefreshFailed
    }
  } catch (error) {
    clearSensitiveState()
    toast.error(formatAdminDisplayError(error, OAUTH_MESSAGES.oauthSaveFailed))
  } finally {
    isSaving.value = false
  }
})

async function fetchProviderState() {
  const entries = await Promise.all(
    oauthLoginProviders.map(
      async (provider) => [provider.value, await activeResource().get(provider.value)] as const,
    ),
  )
  const summaries = Object.fromEntries(entries)
  const firstSummary = summaries[selectedProvider.value] ?? entries[0]?.[1]
  if (!firstSummary) throw new Error(OAUTH_MESSAGES.oauthConfigMissing)
  return { summaries, firstSummary }
}

function applyProviderSummaries(nextSummaries: Record<string, OAuthProviderSettingsSummary>) {
  providerSummaries.value = nextSummaries
  for (const provider of oauthLoginProviders) {
    const enabled = nextSummaries[provider.value]?.enabled ?? false
    providerEnabled[provider.value] = enabled
    initialProviderEnabled[provider.value] = enabled
  }
}

function applySummary(nextSummary: OAuthProviderSettingsSummary) {
  summary.value = nextSummary
  form.resetForm({
    enabled: nextSummary.enabled,
    clientId: nextSummary.clientId,
    clientSecret: '',
  })
  providerEnabled[selectedProvider.value] = nextSummary.enabled
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
