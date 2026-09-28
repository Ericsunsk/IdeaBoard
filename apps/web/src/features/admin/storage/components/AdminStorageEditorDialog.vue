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
    :busy="saving || savingAndApplying"
    @update:visible="emit('update:visible', $event)"
  >
    <template #header>
      <div class="flex w-full min-w-0 items-center justify-between gap-3 pr-6">
        <div class="flex items-center gap-3">
          <button
            type="button"
            :class="[adminLogoButtonClass, 'size-11 p-2 shrink-0 rounded-xl shadow-xs']"
            :title="editorMode === 'edit' ? STORAGE_MESSAGES.switchLogoTitle : undefined"
            :aria-label="editorMode === 'edit' ? STORAGE_MESSAGES.switchLogoAria : undefined"
            :data-control-state="activeLogoTargetId === form.id ? 'selected' : 'idle'"
            :aria-expanded="activeLogoTargetId === form.id ? 'true' : 'false'"
            @click="editorMode === 'edit' && emit('open-logo-picker', $event)"
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
              class="size-5 text-admin-text-2"
            />
          </button>
          <div>
            <h2 class="m-0 text-admin-title-sm font-bold text-admin-title-color">
              {{ editorMode === 'create' ? STORAGE_MESSAGES.editorCreateTitle : form.name }}
            </h2>
            <p class="m-0 text-admin-caption text-admin-subtitle-color">
              {{
                editorMode === 'create'
                  ? STORAGE_MESSAGES.editorCreateSubtitle
                  : STORAGE_MESSAGES.editorProviderSubtitle(providerLabel(form.provider))
              }}
            </p>
          </div>
        </div>

        <AdminBadge
          v-if="form.isCurrent"
          :value="STORAGE_MESSAGES.editorCurrentBadge"
          severity="success"
          rounded
        />
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
          class="group relative flex size-12 items-center justify-center rounded-xl border p-2 cursor-pointer transition-colors"
          :class="
            selectedVendorKey === vendor.key
              ? 'border-admin-accent bg-white shadow-xs'
              : 'border-admin-border-soft bg-white hover:border-admin-accent/60'
          "
          :aria-label="vendor.label"
          @click="emit('select-vendor', vendor.key)"
        >
          <img
            :src="vendor.logoSrc"
            :alt="vendor.label"
            class="size-full object-contain transition-transform group-hover:scale-105"
          />
        </button>
      </div>

      <!-- 2. 云厂商多字段分页导航 Tab -->
      <div v-if="textFields.length > 0">
        <nav
          :aria-label="STORAGE_MESSAGES.editorTabsAria"
          class="inline-flex h-9 items-center justify-center rounded-lg bg-admin-surface-muted p-1 text-admin-text-2"
        >
          <button
            v-for="tab in configTabOptions"
            :key="tab.value"
            type="button"
            :class="[
              'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md px-3 py-1 text-admin-caption font-medium transition-colors cursor-pointer',
              activeConfigTab === tab.value
                ? 'bg-white text-admin-text-1 shadow-xs font-semibold'
                : 'hover:text-admin-text-1 text-admin-text-2',
            ]"
            @click="activeConfigTab = tab.value"
          >
            {{ tab.label }}
          </button>
        </nav>
      </div>

      <!-- 3. Tab 1: 基础与凭据 (或本地存储默认视图) -->
      <template v-if="textFields.length === 0 || activeConfigTab === 'basic'">
        <!-- 启用状态开关卡片 -->
        <div
          class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-4 py-3"
        >
          <span class="text-admin-body font-semibold text-admin-title-color">{{
            STORAGE_MESSAGES.editorEnableLabel
          }}</span>
          <div class="flex items-center gap-2.5">
            <span
              class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-semibold"
              :class="
                form.enabled
                  ? 'bg-admin-success-soft text-admin-success'
                  : 'bg-admin-surface-muted text-admin-text-3 border border-admin-border-soft'
              "
            >
              <span
                class="size-1.5 rounded-full"
                :class="form.enabled ? 'bg-admin-success animate-pulse' : 'bg-admin-text-3'"
              />
              {{ STORAGE_MESSAGES.editorEnableState(form.enabled) }}
            </span>
            <AdminSwitch
              :model-value="form.enabled"
              :aria-label="STORAGE_MESSAGES.editorEnableAria"
              @update:model-value="emit('update-form-field', 'enabled', Boolean($event))"
            />
          </div>
        </div>

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
                class="text-admin-meta font-[680] text-admin-text-2"
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
                class="text-admin-meta font-[680] text-admin-text-2"
              >
                {{ field.label }}
              </label>
              <AdminButton
                v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
                text
                size="small"
                :class="[adminTextButtonClass, '!text-xs font-medium']"
                :loading="fetching"
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
              @update:model-value="updateConfigValue(field.key, String($event ?? ''))"
            />

            <!-- 探测到的加速域名一键填入 -->
            <div
              v-if="field.key === 'access_domain' && bucketDomains.length > 0"
              class="flex items-center gap-1.5 flex-wrap pt-1"
            >
              <span class="text-admin-caption font-semibold text-admin-text-3">{{
                STORAGE_MESSAGES.editorDomainsFound
              }}</span>
              <button
                v-for="domain in bucketDomains"
                :key="domain"
                type="button"
                class="cursor-pointer rounded-md border border-admin-border-soft bg-white px-2 py-0.5 text-admin-caption font-mono font-medium text-admin-accent hover:border-admin-accent transition-colors"
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
            class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 p-3"
          >
            <div class="flex flex-col">
              <span class="text-xs font-semibold text-admin-title-color">{{ field.label }}</span>
              <span v-if="field.hint" class="text-admin-caption text-admin-text-3">{{
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
      <AdminButton
        :class="adminActionButtonClass"
        variant="outline"
        :disabled="saving || savingAndApplying"
        @click="emit('cancel')"
      >
        {{ STORAGE_MESSAGES.cancel }}
      </AdminButton>
      <AdminButton
        v-if="!form.isCurrent"
        :class="adminActionButtonClass"
        variant="outline"
        :loading="saving"
        :disabled="savingAndApplying"
        @click="emit('save-draft')"
      >
        {{ STORAGE_MESSAGES.save }}
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="savingAndApplying"
        :disabled="saving"
        @click="emit('save-and-apply')"
      >
        {{ STORAGE_MESSAGES.editorSaveApply(form.isCurrent) }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { DatabaseExport } from '@iconoir/vue'
import AdminBadge from '../../shared/components/AdminBadge.vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminInput from '../../shared/components/AdminInput.vue'
import AdminSwitch from '../../shared/components/AdminSwitch.vue'
import {
  adminActionButtonClass,
  adminLogoButtonClass,
  adminTextButtonClass,
} from '../../shared/lib/uiClasses'
import { storageVendorLogo, vendorIconMap, vendorPresetCatalog } from '../meta'
import { STORAGE_MESSAGES } from '../pages/messages'
import type {
  StorageCard,
  StorageProvider,
  StorageProviderField,
  StorageProviderMeta,
} from '../profiles'

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
  savingAndApplying: boolean
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
  'save-and-apply': []
  cancel: []
  'open-logo-picker': [event: Event]
  'update-form-field': [field: 'name' | 'enabled', value: string | boolean]
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
