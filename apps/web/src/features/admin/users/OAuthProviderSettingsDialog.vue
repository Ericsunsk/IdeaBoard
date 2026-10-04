<template>
  <AdminSettingsDialogFrame
    :visible="visible"
    :header="USERS_MESSAGES.oauthSettings"
    :description="USERS_MESSAGES.oauthDescription"
    :busy="isSaving"
    :loading="isLoading && !summary"
    :loading-label="USERS_MESSAGES.oauthLoading"
    :error="loadError"
    @reset="resetAndLoad"
    @update:visible="emit('update:visible', $event)"
    @retry="load"
  >
    <form v-if="summary" class="flex flex-col gap-3.5 py-1" novalidate @submit.prevent="submit">
      <!-- 1. 顶部 Provider 选择与启用卡片网格 -->
      <div class="grid grid-cols-2 gap-3" role="group" :aria-label="USERS_MESSAGES.oauthGroupAria">
        <div
          v-for="provider in oauthLoginProviders"
          :key="provider.value"
          class="group relative flex items-center justify-between rounded-xl border p-3 cursor-pointer transition-colors select-none"
          :class="
            selectedProvider === provider.value
              ? 'border-primary bg-primary/10 shadow-2xs'
              : 'border-border bg-muted/40 hover:border-border hover:bg-muted'
          "
          @click="selectedProvider = provider.value"
        >
          <div class="flex items-center gap-2.5 min-w-0">
            <button
              type="button"
              class="flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-border bg-card p-1.5 shadow-2xs"
              :aria-label="USERS_MESSAGES.oauthSelectLogoAria(provider.label)"
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
              :aria-label="USERS_MESSAGES.oauthProviderLoginAria(provider.label)"
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
      <div class="flex flex-col gap-3.5 rounded-xl border border-border bg-card p-4 shadow-2xs">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md border border-border bg-muted p-1"
              :aria-label="USERS_MESSAGES.oauthSelectLogoAria(providerLabel)"
              @click="openLogoPicker($event, selectedProvider)"
            >
              <img
                :src="oauthProviderLogo(selectedProvider)"
                class="size-full object-contain"
                alt=""
                aria-hidden="true"
              />
            </button>
            <h3 class="m-0 text-admin-body-sm font-bold text-foreground">
              {{ USERS_MESSAGES.oauthCredentialsTitle(providerLabel) }}
            </h3>
          </div>
          <AdminStatusPill
            :label="USERS_MESSAGES.oauthEnabledLabel(form.values.enabled)"
            :variant="form.values.enabled ? 'success' : 'neutral'"
          />
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
            :placeholder="USERS_MESSAGES.oauthClientIdPlaceholder"
            :disabled="isBusy"
            :invalid="invalid"
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
            :invalid="invalid"
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
              {{ USERS_MESSAGES.oauthCopyCallback }}
            </AdminButton>
          </div>
          <p class="m-0 mt-1 text-admin-micro text-muted-foreground">
            {{
              USERS_MESSAGES.oauthCallbackHint(
                providerLabel === 'Google' ? 'Google Cloud Console' : 'Discord Developer Portal',
              )
            }}
          </p>
        </AdminFormField>
      </div>

      <AdminAlert v-if="saveError" severity="error">{{ saveError }}</AdminAlert>
    </form>

    <template #footer="{ requestClose }">
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="isBusy"
        @click="requestClose"
      >
        {{ USERS_MESSAGES.cancel }}
      </AdminButton>
      <AdminButton
        variant="default"
        :class="adminActionButtonClass"
        :loading="isSaving"
        :disabled="isLoading || Boolean(loadError) || !summary"
        @click="submit"
      >
        {{ USERS_MESSAGES.oauthSave }}
      </AdminButton>
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
import AdminAlert from '../shared/components/AdminAlert.vue'
import AdminButton from '../shared/components/AdminButton.vue'
import AdminInput from '../shared/components/AdminInput.vue'
import AdminSelect from '../shared/components/AdminSelect.vue'
import AdminSwitch from '../shared/components/AdminSwitch.vue'
import AdminSettingsDialogFrame from '../shared/components/AdminSettingsDialogFrame.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import AdminStatusPill from '../shared/components/AdminStatusPill.vue'
import AdminLogoPickerPopover from '../shared/components/AdminLogoPickerPopover.vue'
import { formatAdminDisplayError, useAdminToast } from '../shared/lib/toast'
import { useAppClipboard } from '../../../shared/composables/useAppClipboard'
import { adminActionButtonClass } from '../shared/lib/uiClasses'
import { secretField } from '../shared/lib/secretSchema'
import googleLogoUrl from '../../../shared/assets/oauth/google.svg'
import discordLogoUrl from '../../../shared/assets/oauth/discord.svg'
import { getAdminBrandLogo } from '../shared/logos/brandLogos'
import {
  createOAuthProviderSettingsResource,
  type OAuthProviderSettingsResource,
  type OAuthProviderSettingsSummary,
} from './oauthProviderSettingsResource'
import { getOAuthProviderLabel, type OAuthProviderCode } from '../../../shared/auth/oauthProviders'
import { USERS_MESSAGES } from './messages'

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
const saveError = ref('')
const form = useForm({
  initialValues: { enabled: false, clientId: '', clientSecret: '' },
  schema: () =>
    v.object({
      enabled: v.boolean(),
      clientId: secretField(form.values.enabled, USERS_MESSAGES.oauthClientIdRequired),
      clientSecret: secretField(
        form.values.enabled && !summary.value?.clientSecretConfigured,
        USERS_MESSAGES.oauthClientSecretRequired,
      ),
    }),
})
let providerLoadEpoch = 0
const isBusy = computed(() => isSaving.value || (isLoading.value && !summary.value))
const providerLabel = computed(() => getOAuthProviderLabel(selectedProvider.value))
const secretPlaceholder = computed(() =>
  USERS_MESSAGES.oauthSecretPlaceholder(Boolean(summary.value?.clientSecretConfigured)),
)

function oauthProviderLogo(provider: OAuthProviderCode): string {
  return (
    getAdminBrandLogo({ name: provider }, 'oauth-provider', provider) ??
    (provider === 'google' ? googleLogoUrl : discordLogoUrl)
  )
}

function openLogoPicker(event: Event, provider: OAuthProviderCode) {
  selectedProvider.value = provider
  activeLogoProvider.value = provider
  logoPickerPopoverRef.value?.toggle(event)
}

watch(selectedProvider, (provider) => {
  if (!props.visible) return
  clearSensitiveState()
  saveError.value = ''
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
  saveError.value = ''
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
    loadError.value = formatAdminDisplayError(error, USERS_MESSAGES.oauthLoadFailed)
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
      loadError.value = formatAdminDisplayError(error, USERS_MESSAGES.oauthLoadFailed)
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
    saveError.value = USERS_MESSAGES.oauthIncompleteProvider(
      getOAuthProviderLabel(incompleteProvider),
    )
    return
  }
  isSaving.value = true
  saveError.value = ''
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
    toast.success(USERS_MESSAGES.oauthSaved(providerLabel.value))
    try {
      const state = await fetchProviderState()
      applySummary(state.summaries[selectedProvider.value] ?? state.firstSummary)
      applyProviderSummaries(state.summaries)
      emit('update:visible', false)
    } catch {
      loadError.value = USERS_MESSAGES.oauthRefreshFailed
    }
  } catch (error) {
    clearSensitiveState()
    saveError.value = formatAdminDisplayError(error, USERS_MESSAGES.oauthSaveFailed)
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
  if (!firstSummary) throw new Error(USERS_MESSAGES.oauthConfigMissing)
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
  if (ok) toast.success(USERS_MESSAGES.oauthCallbackCopied)
  else toast.error(USERS_MESSAGES.oauthCopyFailed)
}
</script>
