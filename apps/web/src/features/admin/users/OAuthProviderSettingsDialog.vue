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

      <form v-if="summary" class="flex flex-col gap-3.5 py-1" novalidate @submit.prevent="submit">
        <!-- 1. 顶部 Provider 选择与启用卡片网格 -->
        <div class="grid grid-cols-2 gap-3" role="group" aria-label="OAuth 登录方式">
          <div
            v-for="provider in oauthLoginProviders"
            :key="provider.value"
            class="group relative flex items-center justify-between rounded-xl border p-3 cursor-pointer transition-colors select-none"
            :class="
              selectedProvider === provider.value
                ? 'border-admin-accent bg-admin-accent-soft/30 shadow-2xs'
                : 'border-admin-border-soft bg-admin-surface-muted/40 hover:border-admin-border hover:bg-admin-surface-muted'
            "
            @click="selectedProvider = provider.value"
          >
            <div class="flex items-center gap-2.5 min-w-0">
              <button
                type="button"
                class="flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-lg border border-admin-border-soft bg-white p-1.5 shadow-2xs"
                :aria-label="`选择 ${provider.label} Logo`"
                @click.stop="openLogoPicker($event, provider.value)"
              >
                <img
                  :src="oauthProviderLogo(provider.value)"
                  class="size-full object-contain"
                  alt=""
                  aria-hidden="true"
                />
              </button>
              <span class="text-admin-body font-bold text-admin-title-color truncate">
                {{ provider.label }}
              </span>
            </div>

            <div class="shrink-0 pl-2" @click.stop>
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
        </div>

        <!-- 语义化 Provider 切换器（可访问性与测试合约） -->
        <AdminFormField label="OAuth Provider" for="admin-oauth-provider" class="sr-only">
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

        <!-- 2. 当前选中 Provider 的凭据与回调配置区 -->
        <div
          class="flex flex-col gap-3.5 rounded-xl border border-admin-border-soft bg-white p-4 shadow-2xs"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="flex size-6 shrink-0 cursor-pointer items-center justify-center rounded-md border border-admin-border-soft bg-admin-surface-muted p-1"
                :aria-label="`选择 ${providerLabel} Logo`"
                @click="openLogoPicker($event, selectedProvider)"
              >
                <img
                  :src="oauthProviderLogo(selectedProvider)"
                  class="size-full object-contain"
                  alt=""
                  aria-hidden="true"
                />
              </button>
              <h3 class="m-0 text-admin-body-sm font-bold text-admin-title-color">
                {{ providerLabel }} 凭据配置
              </h3>
            </div>
            <AdminStatusPill
              :label="form.enabled ? '已启用' : '未启用'"
              :variant="form.enabled ? 'success' : 'neutral'"
            />
          </div>

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
              placeholder="请输入 OAuth Client ID"
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
            <div class="relative flex items-center">
              <InputText
                id="admin-oauth-callback-url"
                name="oauth_callback_url"
                :model-value="summary.callbackUrl"
                class="w-full font-mono text-admin-caption pr-16"
                aria-label="Callback URL"
                readonly
              />
              <Button
                label="复制"
                type="button"
                size="small"
                severity="secondary"
                text
                class="!absolute right-1 !h-7 !px-2.5 text-admin-caption font-semibold"
                @click="copyCallbackUrl"
              />
            </div>
            <p class="m-0 mt-1 text-admin-micro text-admin-text-3">
              请在
              {{
                providerLabel === 'Google' ? 'Google Cloud Console' : 'Discord Developer Portal'
              }}
              将此 URL 添加为重定向白名单 URI
            </p>
          </AdminFormField>
        </div>

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
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import AdminStatusPill from '../shared/components/AdminStatusPill.vue'
import AdminLogoPickerPopover from '../provider-config/AdminLogoPickerPopover.vue'
import { formatAdminDisplayError, useAdminToast } from '../shared/lib/adminToast'
import { adminActionButtonClass, adminTextButtonClass } from '../shared/lib/adminUiClasses'
import googleLogoUrl from '../../../shared/assets/oauth/google.svg'
import discordLogoUrl from '../../../shared/assets/oauth/discord.svg'
import { getAdminBrandLogo } from '../shared/logos/adminBrandLogos'
import {
  createOAuthProviderSettingsResource,
  type OAuthProviderSettingsResource,
  type OAuthProviderSettingsSummary,
} from './oauthProviderSettingsResource'
import { getOAuthProviderLabel, type OAuthProviderCode } from '../../../shared/auth/oauthProviders'

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
const form = reactive({ enabled: false, clientId: '', clientSecret: '' })
const errors = reactive({ clientId: '', clientSecret: '' })
let providerLoadEpoch = 0
const isBusy = computed(() => isSaving.value || (isLoading.value && !summary.value))
const providerLabel = computed(() => getOAuthProviderLabel(selectedProvider.value))
const secretPlaceholder = computed(() =>
  summary.value?.clientSecretConfigured ? '已配置，留空则保持不变' : '输入 Client Secret',
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
    oauthLoginProviders.map(
      async (provider) => [provider.value, await activeResource().get(provider.value)] as const,
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

async function copyCallbackUrl() {
  if (!summary.value?.callbackUrl) return
  try {
    await navigator.clipboard.writeText(summary.value.callbackUrl)
    toast.success('Callback URL 已复制到剪贴板')
  } catch {
    toast.error('复制失败，请手动选择复制')
  }
}
</script>
