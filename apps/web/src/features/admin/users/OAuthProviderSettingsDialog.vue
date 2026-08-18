<template>
  <AdminDialogShell
    :visible="visible"
    header="OAuth Provider 设置"
    size="form"
    :busy="isSaving"
    @reset="resetAndLoad"
    @update:visible="emit('update:visible', $event)"
  >
    <div v-if="isLoading && !summary" class="flex min-h-36 items-center justify-center">
      <ProgressSpinner aria-label="加载 OAuth Provider 设置" stroke-width="3" />
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
            :class="adminTextButtonClass"
            :disabled="isBusy"
            @click="load"
          />
        </div>
      </Message>

      <form
        v-if="summary"
        class="flex min-h-[26rem] flex-col gap-4"
        novalidate
        @submit.prevent="submit"
      >
        <div class="grid grid-cols-2 gap-5" role="group" aria-label="OAuth 登录方式">
          <div
            v-for="provider in oauthLoginProviders"
            :key="provider.value"
            class="flex min-w-0 items-center justify-between"
          >
            <img
              :src="provider.value === 'google' ? googleLogoUrl : discordLogoUrl"
              class="size-6 shrink-0"
              alt=""
              aria-hidden="true"
            />
            <ToggleSwitch
              :model-value="providerEnabled[provider.value] ?? false"
              :input-id="`admin-oauth-enabled-${provider.value}`"
              :name="`oauth_enabled_${provider.value}`"
              :aria-label="`${provider.label} 登录`"
              :disabled="isBusy"
              @update:model-value="setProviderEnabled(provider.value, $event)"
            />
          </div>
        </div>

        <AdminFormField label="OAuth Provider" for="admin-oauth-provider">
          <Select
            v-model="selectedProvider"
            input-id="admin-oauth-provider"
            name="oauth_provider"
            aria-label="OAuth Provider"
            :options="oauthLoginProviders"
            option-label="label"
            option-value="value"
            class="w-full"
            :disabled="isBusy"
          />
        </AdminFormField>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="Client ID"
          for="admin-oauth-client-id"
          :error="errors.clientId"
          :required="form.enabled"
        >
          <InputText
            id="admin-oauth-client-id"
            v-model="form.clientId"
            name="oauth_client_id"
            class="w-full"
            autocomplete="off"
            aria-label="Client ID"
            :disabled="isBusy"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="errors.clientId = ''"
          />
        </AdminFormField>

        <AdminFormField
          v-slot="{ describedbyId, invalid }"
          label="Client Secret"
          for="admin-oauth-client-secret"
          :error="errors.clientSecret"
          :required="form.enabled && !summary.clientSecretConfigured"
        >
          <InputText
            id="admin-oauth-client-secret"
            v-model="form.clientSecret"
            name="oauth_client_secret"
            type="password"
            autocomplete="new-password"
            class="w-full"
            aria-label="Client Secret"
            :placeholder="secretPlaceholder"
            :disabled="isBusy"
            :invalid="invalid"
            :aria-describedby="describedbyId"
            @update:model-value="errors.clientSecret = ''"
          />
        </AdminFormField>

        <AdminFormField label="Callback URL" for="admin-oauth-callback-url">
          <InputText
            id="admin-oauth-callback-url"
            name="oauth_callback_url"
            :model-value="summary.callbackUrl"
            class="w-full font-mono text-admin-caption"
            aria-label="Callback URL"
            readonly
          />
        </AdminFormField>

        <Message v-if="saveError" severity="error" :closable="false">{{ saveError }}</Message>
      </form>
    </template>

    <template #footer="{ requestClose }">
      <Button
        label="取消"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :disabled="isBusy"
        @click="requestClose"
      />
      <Button
        label="保存设置"
        :class="adminActionButtonClass"
        severity="contrast"
        :loading="isSaving"
        :disabled="isLoading || Boolean(loadError) || !summary"
        @click="submit"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { formatAdminDisplayError, useAdminToast } from '../shared/lib/adminToast'
import { adminActionButtonClass, adminTextButtonClass } from '../shared/lib/adminUiClasses'
import googleLogoUrl from '../../../shared/assets/oauth/google.svg'
import discordLogoUrl from '../../../shared/assets/oauth/discord.svg'
import {
  createOAuthProviderSettingsResource,
  type OAuthProviderSettingsResource,
  type OAuthProviderSettingsSummary,
} from './oauthProviderSettingsResource'
import {
  getOAuthProviderLabel,
  type OAuthProviderCode,
} from '../../../shared/auth/oauthProviders'

const props = withDefaults(
  defineProps<{
    visible: boolean
    resource?: OAuthProviderSettingsResource
  }>(),
  { resource: undefined },
)
const emit = defineEmits<{
  'update:visible': [visible: boolean]
}>()

const defaultResource = createOAuthProviderSettingsResource()
const toast = useAdminToast()
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
const form = reactive({ enabled: false, clientId: '', clientSecret: '' })
const errors = reactive({ clientId: '', clientSecret: '' })
let providerLoadEpoch = 0
const isBusy = computed(() => isSaving.value || (isLoading.value && !summary.value))
const providerLabel = computed(() => getOAuthProviderLabel(selectedProvider.value))
const secretPlaceholder = computed(() =>
  summary.value?.clientSecretConfigured ? '已配置，留空则保持不变' : '输入 Client Secret',
)

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
    loadError.value = formatAdminDisplayError(error, '加载 OAuth Provider 设置失败，请重试')
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
      loadError.value = formatAdminDisplayError(error, '加载 OAuth Provider 设置失败，请重试')
    }
  } finally {
    if (epoch === providerLoadEpoch) providerSwitching.value = false
  }
}

function validate() {
  errors.clientId = form.enabled && !form.clientId.trim() ? '请输入 Client ID' : ''
  errors.clientSecret =
    form.enabled && !summary.value?.clientSecretConfigured && !form.clientSecret.trim()
      ? '请输入 Client Secret'
      : ''
  return !errors.clientId && !errors.clientSecret
}

function setProviderEnabled(provider: OAuthProviderCode, enabled: boolean) {
  providerEnabled[provider] = enabled
  if (provider === selectedProvider.value) form.enabled = enabled
}

async function submit() {
  if (isBusy.value || providerSwitching.value || !summary.value || !validate()) return
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
    saveError.value = `请先配置 ${getOAuthProviderLabel(incompleteProvider)} 的 Client ID 和 Client Secret`
    return
  }
  isSaving.value = true
  saveError.value = ''
  try {
    const currentProvider = selectedProvider.value
    const saveRequests = [
      activeResource().save(currentProvider, {
        enabled: form.enabled,
        clientId: form.clientId,
        clientSecret: form.clientSecret,
      }),
      ...changedProviders.map((provider) =>
        activeResource().save(provider, {
          enabled: providerEnabled[provider],
          clientId: providerSummaries.value[provider]?.clientId ?? '',
          clientSecret: '',
        }),
      ),
    ]
    const [savedSummary] = await Promise.all(saveRequests)
    clearSensitiveState()
    applySummary(savedSummary)
    toast.success(`${providerLabel.value} OAuth 设置已保存`)
    try {
      const state = await fetchProviderState()
      applySummary(state.summaries[selectedProvider.value] ?? state.firstSummary)
      applyProviderSummaries(state.summaries)
      emit('update:visible', false)
    } catch {
      loadError.value = '设置已保存，但状态刷新失败，请重新加载'
    }
  } catch (error) {
    clearSensitiveState()
    saveError.value = formatAdminDisplayError(error, '保存 OAuth Provider 设置失败，请重试')
  } finally {
    isSaving.value = false
  }
}

async function fetchProviderState() {
  const entries = await Promise.all(
    oauthLoginProviders.map(async (provider) =>
      [provider.value, await activeResource().get(provider.value)] as const,
    ),
  )
  const summaries = Object.fromEntries(entries)
  const firstSummary = summaries[selectedProvider.value] ?? entries[0]?.[1]
  if (!firstSummary) throw new Error('未找到 OAuth Provider 配置')
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
  form.enabled = nextSummary.enabled
  form.clientId = nextSummary.clientId
  providerEnabled[selectedProvider.value] = nextSummary.enabled
  clearSensitiveState()
}

function clearSensitiveState() {
  form.clientSecret = ''
  errors.clientSecret = ''
}
</script>
