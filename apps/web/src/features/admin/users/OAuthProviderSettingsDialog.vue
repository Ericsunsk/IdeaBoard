<template>
  <AdminDialogShell
    :visible="visible"
    header="OAuth Provider 设置"
    size="form"
    :busy="isBusy"
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
            :disabled="isBusy"
            @click="load"
          />
        </div>
      </Message>

      <form v-if="summary" class="flex flex-col gap-4" novalidate @submit.prevent="submit">
        <AdminFormField label="OAuth Provider" for="admin-oauth-provider">
          <Select
            v-model="selectedProvider"
            input-id="admin-oauth-provider"
            name="oauth_provider"
            aria-label="OAuth Provider"
            :options="providerOptions"
            option-label="label"
            option-value="value"
            class="w-full"
            :disabled="isBusy"
          />
        </AdminFormField>

        <AdminFormField label="Google 登录" for="admin-oauth-enabled">
          <div class="flex min-h-10 items-center justify-between border-y border-admin-border py-2">
            <div class="flex min-w-0 items-center gap-2">
              <span class="text-admin-body text-admin-text-2">
                {{ form.enabled ? '已启用' : '已停用' }}
              </span>
              <Tag
                :value="runtimeAvailable ? 'Runtime 可用' : 'Runtime 未就绪'"
                :severity="runtimeAvailable ? 'success' : 'secondary'"
              />
            </div>
            <ToggleSwitch
              v-model="form.enabled"
              input-id="admin-oauth-enabled"
              name="oauth_enabled"
              aria-label="Google 登录"
              :disabled="isBusy"
            />
          </div>
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
          :hint="secretHint"
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
      <Button label="取消" severity="secondary" outlined :disabled="isBusy" @click="requestClose" />
      <Button
        label="保存设置"
        :loading="isSaving"
        :disabled="isLoading || Boolean(loadError) || !summary"
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
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import AdminDialogShell from '../shared/components/AdminDialogShell.vue'
import AdminFormField from '../shared/components/AdminFormField.vue'
import { formatAdminDisplayError, useAdminToast } from '../shared/lib/adminToast'
import {
  createOAuthProviderSettingsResource,
  type OAuthCapabilities,
  type OAuthProviderCode,
  type OAuthProviderSettingsResource,
  type OAuthProviderSettingsSummary,
} from './oauthProviderSettingsResource'

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
const providerOptions: Array<{ label: string; value: OAuthProviderCode }> = [
  { label: 'Google', value: 'google' },
]
const selectedProvider = ref<OAuthProviderCode>('google')
const summary = ref<OAuthProviderSettingsSummary | null>(null)
const capabilities = ref<OAuthCapabilities | null>(null)
const isLoading = ref(false)
const isSaving = ref(false)
const loadError = ref('')
const saveError = ref('')
const form = reactive({ enabled: false, clientId: '', clientSecret: '' })
const errors = reactive({ clientId: '', clientSecret: '' })
const isBusy = computed(() => isLoading.value || isSaving.value)
const runtimeAvailable = computed(
  () => capabilities.value?.social[selectedProvider.value] ?? summary.value?.available ?? false,
)
const secretPlaceholder = computed(() =>
  summary.value?.clientSecretConfigured ? '已配置，留空则保持不变' : '输入 Client Secret',
)
const secretHint = computed(() =>
  summary.value?.clientSecretConfigured ? '当前 Secret 不会回显。' : undefined,
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
    const [nextSummary, nextCapabilities] = await Promise.all([
      activeResource().get(selectedProvider.value),
      activeResource().getCapabilities(),
    ])
    applyLoadedState(nextSummary, nextCapabilities)
  } catch (error) {
    loadError.value = formatAdminDisplayError(error, '加载 OAuth Provider 设置失败，请重试')
  } finally {
    isLoading.value = false
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

async function submit() {
  if (isBusy.value || !summary.value || !validate()) return
  isSaving.value = true
  saveError.value = ''
  try {
    const savedSummary = await activeResource().save(selectedProvider.value, {
      enabled: form.enabled,
      clientId: form.clientId,
      clientSecret: form.clientSecret,
    })
    clearSensitiveState()
    applySummary(savedSummary)
    toast.success('Google OAuth 设置已保存')
    try {
      const [nextSummary, nextCapabilities] = await Promise.all([
        activeResource().get(selectedProvider.value),
        activeResource().getCapabilities(),
      ])
      applyLoadedState(nextSummary, nextCapabilities)
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

function applyLoadedState(
  nextSummary: OAuthProviderSettingsSummary,
  nextCapabilities: OAuthCapabilities,
) {
  applySummary(nextSummary)
  capabilities.value = nextCapabilities
}

function applySummary(nextSummary: OAuthProviderSettingsSummary) {
  summary.value = nextSummary
  form.enabled = nextSummary.enabled
  form.clientId = nextSummary.clientId
  clearSensitiveState()
}

function clearSensitiveState() {
  form.clientSecret = ''
  errors.clientSecret = ''
}
</script>
