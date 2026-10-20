<template>
  <AdminDialogShell
    :visible="visible"
    :aria-label="
      editorMode === 'create'
        ? STORAGE_MESSAGES.editorCreateTitle
        : form.name || STORAGE_MESSAGES.editorFallbackTitle
    "
    :description="STORAGE_MESSAGES.editorDescription"
    size="form"
    content-layout="scrollable"
    :busy="saving"
    @update:visible="emit('update:visible', $event)"
  >
    <template #header>
      <div v-if="editorMode === 'create'">
        <h2 class="m-0 text-admin-title-sm font-bold text-foreground">
          {{ STORAGE_MESSAGES.editorCreateTitle }}
        </h2>
      </div>

      <div v-else class="flex w-full min-w-0 items-center gap-3 pr-6">
        <div class="flex items-center gap-3">
          <button
            type="button"
            :class="[adminLogoButtonClass, 'size-11 p-2 shrink-0 shadow-raised']"
            :title="STORAGE_MESSAGES.switchLogoTitle"
            :aria-label="STORAGE_MESSAGES.switchLogoAria"
            :data-control-state="activeLogoTargetId === form.id ? 'selected' : 'idle'"
            :aria-expanded="activeLogoTargetId === form.id ? 'true' : 'false'"
            @click="emit('open-logo-picker', $event)"
          >
            <img
              v-if="dialogVendorLogo"
              :src="dialogVendorLogo"
              :alt="form.name"
              class="size-full object-contain"
            />
            <component
              :is="vendorIconMap[form.provider] || DatabaseExport"
              v-else
              class="size-5 text-muted-foreground"
            />
          </button>
          <div>
            <h2 class="m-0 text-admin-title-sm font-bold text-foreground">
              {{ form.name }}
            </h2>
            <p class="m-0 text-admin-caption text-muted-foreground">
              {{ STORAGE_MESSAGES.editorProviderSubtitle(providerLabel(form.provider)) }}
            </p>
          </div>
        </div>
      </div>
    </template>

    <div class="flex flex-col gap-3.5 py-1">
      <!-- 1. 新增模式：顶部极简厂商 Logo 预设卡片选择 (阿里云 OSS, 腾讯云 COS, Amazon S3, Cloudflare R2, MinIO) -->
      <div v-if="editorMode === 'create'" class="flex items-center gap-2.5 pt-0.5">
        <button
          v-for="vendor in vendorPresetCatalog"
          :key="vendor.key"
          :title="vendor.label"
          type="button"
          class="group relative flex size-12 items-center justify-center rounded-control border p-2 cursor-pointer transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
          :class="
            selectedVendorKey === vendor.key
              ? 'border-primary bg-card shadow-raised'
              : 'border-border bg-card hover:border-primary-line-bold'
          "
          :aria-label="vendor.label"
          @click="emit('select-vendor', vendor.key)"
        >
          <img
            :src="vendor.logoSrc"
            :alt="vendor.label"
            class="size-full object-contain transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] group-hover:scale-105"
          />
        </button>
      </div>

      <!-- 2. 云厂商多字段分页导航 Tab -->
      <div v-if="textFields.length > 0">
        <nav
          :aria-label="STORAGE_MESSAGES.editorTabsAria"
          class="inline-flex h-9 items-center justify-center rounded-control bg-muted p-1 text-muted-foreground"
        >
          <button
            v-for="tab in configTabOptions"
            :key="tab.value"
            type="button"
            :class="[
              'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-control px-3 py-1 text-admin-caption font-medium transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] cursor-pointer',
              activeConfigTab === tab.value
                ? 'bg-card text-foreground shadow-raised font-semibold'
                : 'hover:text-foreground text-muted-foreground',
            ]"
            @click="activeConfigTab = tab.value"
          >
            {{ tab.label }}
          </button>
        </nav>
      </div>

      <!-- 3. Tab 1: 基础与凭据 (或本地存储默认视图) -->
      <template v-if="textFields.length === 0 || activeConfigTab === 'basic'">
        <!-- 存储名称 -->
        <AdminFormField :label="STORAGE_MESSAGES.editorNameLabel" for="admin-storage-name" required>
          <AdminInput
            id="admin-storage-name"
            :model-value="form.name"
            class="w-full !text-xs"
            :placeholder="STORAGE_MESSAGES.editorNamePlaceholder"
            @update:model-value="emit('update-form-field', 'name', String($event ?? ''))"
          />
        </AdminFormField>

        <!-- Access Key 与 Secret Access Key -->
        <template v-if="textFields.length > 0">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div v-for="field in authFields" :key="field.key" class="flex min-w-0 flex-col gap-1">
              <label
                :for="`admin-storage-${field.key}`"
                class="text-admin-meta font-admin-emphasis text-muted-foreground"
              >
                {{ field.label }}
              </label>
              <AdminInput
                :id="`admin-storage-${field.key}`"
                :model-value="configTextValue(field.key)"
                class="w-full !text-xs font-mono"
                :type="field.type === 'password' ? 'password' : 'text'"
                :placeholder="
                  field.secret && form.secretConfigured && !configTextValue(field.key)
                    ? STORAGE_MESSAGES.secretKeptPlaceholder
                    : field.placeholder || STORAGE_MESSAGES.fieldPlaceholder(field.label)
                "
                @update:model-value="updateConfigValue(field.key, String($event ?? ''))"
              />
            </div>
          </div>
        </template>
      </template>

      <!-- 4. Tab 2: 端点与网络 -->
      <template v-else-if="activeConfigTab === 'endpoint'">
        <div class="flex flex-col gap-3">
          <div v-for="field in endpointFields" :key="field.key" class="flex min-w-0 flex-col gap-1">
            <div class="flex items-center justify-between">
              <label
                :for="`admin-storage-${field.key}`"
                class="text-admin-meta font-admin-emphasis text-muted-foreground"
              >
                {{ field.label }}
              </label>
              <AdminButton
                v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
                text
                size="sm"
                :class="[adminTextButtonClass, '!text-xs font-medium']"
                :disabled="fetching"
                @click="emit('fetch-bucket-domains')"
              >
                {{ STORAGE_MESSAGES.editorDetectDomains }}
              </AdminButton>
            </div>
            <AdminInput
              :id="`admin-storage-${field.key}`"
              :model-value="configTextValue(field.key)"
              class="w-full !text-xs font-mono"
              :type="field.type === 'password' ? 'password' : 'text'"
              :placeholder="
                field.secret && form.secretConfigured && !configTextValue(field.key)
                  ? STORAGE_MESSAGES.secretKeptPlaceholder
                  : field.placeholder || STORAGE_MESSAGES.fieldPlaceholder(field.label)
              "
              :loading="field.key === 'access_domain' && fetching"
              loading-label="Fetching…"
              @update:model-value="updateConfigValue(field.key, String($event ?? ''))"
            />

            <!-- 探测到的加速域名一键填入 -->
            <div
              v-if="field.key === 'access_domain' && bucketDomains.length > 0"
              class="flex items-center gap-1.5 flex-wrap pt-1"
            >
              <span class="text-admin-caption font-semibold text-muted-foreground">{{
                STORAGE_MESSAGES.editorDomainsFound
              }}</span>
              <button
                v-for="domain in bucketDomains"
                :key="domain"
                type="button"
                class="cursor-pointer rounded-control border border-border bg-card px-2 py-0.5 text-admin-caption font-mono font-medium text-primary hover:border-primary transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
                @click="updateConfigValue(field.key, `https://${domain}`)"
              >
                https://{{ domain }}
              </button>
            </div>
          </div>
        </div>
      </template>

      <!-- 5. Tab 3: 高级选项 -->
      <template v-else-if="activeConfigTab === 'advanced'">
        <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
          <div
            v-for="field in switchFields"
            :key="field.key"
            class="flex items-center justify-between rounded-panel border border-border bg-muted/50 p-3"
          >
            <div class="flex flex-col">
              <span class="text-xs font-semibold text-foreground">{{ field.label }}</span>
              <span v-if="field.hint" class="text-admin-caption text-muted-foreground">{{
                field.hint
              }}</span>
            </div>
            <AdminSwitch
              :model-value="configBoolValue(field.key)"
              @update:model-value="updateConfigValue(field.key, Boolean($event))"
            />
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <AdminDialogActions
        :confirm-label="STORAGE_MESSAGES.save"
        :loading="saving"
        :cancel-disabled="saving"
        confirm-variant="primary"
        @cancel="emit('cancel')"
        @confirm="emit('save-draft')"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { DatabaseExport } from '@iconoir/vue'
import { AdminButton } from '../../../../shared/index'
import { AdminDialogActions } from '../../../../shared/index'
import { AdminDialogShell } from '../../../../shared/index'
import { AdminFormField } from '../../../../shared/index'
import { AdminInput } from '../../../../shared/index'
import { AdminSwitch } from '../../../../shared/index'
import { adminLogoButtonClass, adminTextButtonClass } from '../../../../shared/index'
import { storageVendorLogo, vendorIconMap, vendorPresetCatalog } from '../../../core/storage/meta'
import { STORAGE_MESSAGES } from './messages'
import type {
  StorageCard,
  StorageProvider,
  StorageProviderField,
  StorageProviderMeta,
} from '../../../core/storage/profiles'

type StorageConfigTab = 'basic' | 'endpoint' | 'advanced'

interface FormState {
  id?: string
  name: string
  provider: StorageProvider
  enabled: boolean
  isCurrent: boolean
  secretConfigured: boolean
  config: Record<string, unknown>
}

const props = defineProps<{
  visible: boolean
  editorMode: 'create' | 'edit'
  selectedVendorKey: string
  form: FormState
  selectedProviderMeta?: StorageProviderMeta | null | undefined
  textFields: StorageProviderField[]
  switchFields: StorageProviderField[]
  bucketDomains: string[]
  saving: boolean
  fetching: boolean
  activeLogoTargetId?: string | null | undefined
  configTextValue: (key: string) => string
  configBoolValue: (key: string) => boolean
  providerLabel: (provider: StorageProvider) => string
}>()

const emit = defineEmits<{
  'update:visible': [visible: boolean]
  'select-vendor': [key: string]
  'update-config-value': [key: string, value: string | boolean]
  'fetch-bucket-domains': []
  'save-draft': []
  cancel: []
  'open-logo-picker': [event: Event]
  'update-form-field': [field: 'name', value: string | boolean]
}>()

const activeConfigTab = ref<StorageConfigTab>('basic')

watch(
  () => props.visible,
  (val) => {
    if (val) {
      activeConfigTab.value = 'basic'
    }
  },
)

const configTabOptions = computed<{ label: string; value: StorageConfigTab }[]>(() => {
  const options: { label: string; value: StorageConfigTab }[] = [
    { label: STORAGE_MESSAGES.editorTabBasic, value: 'basic' },
    { label: STORAGE_MESSAGES.editorTabEndpoint, value: 'endpoint' },
  ]
  if (props.switchFields.length > 0) {
    options.push({ label: STORAGE_MESSAGES.editorTabAdvanced, value: 'advanced' })
  }
  return options
})

const authFields = computed(() =>
  props.textFields.filter(
    (f) => f.key === 'access_key_id' || f.key === 'secret_access_key' || f.secret,
  ),
)

const endpointFields = computed(() =>
  props.textFields.filter(
    (f) => f.key !== 'access_key_id' && f.key !== 'secret_access_key' && !f.secret,
  ),
)

const dialogVendorLogo = computed<string | undefined>(() => {
  if (props.editorMode === 'create') {
    return vendorPresetCatalog.find((v) => v.key === props.selectedVendorKey)?.logoSrc
  }
  return storageVendorLogo(props.form as unknown as StorageCard)
})

function updateConfigValue(key: string, value: string | boolean) {
  emit('update-config-value', key, value)
}
</script>
