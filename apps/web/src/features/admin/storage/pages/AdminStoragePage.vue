<template>
  <div class="flex flex-col gap-4">
    <!-- 头部：板块名 + 副标题 + 操作栏 -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-1">
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <Database class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <h3 class="text-admin-title-sm font-bold text-foreground">
            {{ STORAGE_MESSAGES.storageTitle }}
          </h3>
        </div>
        <p class="text-admin-caption text-muted-foreground">
          {{ STORAGE_MESSAGES.storageSubtitle }}
        </p>
      </div>

      <!-- 右侧：新增操作 -->
      <div class="flex shrink-0 items-center gap-2">
        <ActionButton :disabled="creatableProviders.length === 0" @click="handleOpenCreate">
          <Plus class="size-4" />
          {{ STORAGE_MESSAGES.create }}
        </ActionButton>
      </div>
    </div>

    <!-- 1. 存储列表表格与空状态/加载态 -->
    <AdminEmptyState v-if="loadFailed" :title="STORAGE_MESSAGES.tableLoadFailed">
      <template #actions>
        <ActionButton :loading="loading" @click="loadData">
          {{ STORAGE_MESSAGES.retry }}
        </ActionButton>
      </template>
    </AdminEmptyState>
    <AdminStorageTable
      v-else
      :rows="allStorageRows"
      :loading="loading"
      :applying-id="applyingId"
      :testing-id="testingId"
      :toggling-id="togglingId"
      :active-logo-target-id="activeLogoTarget?.id"
      @open-logo-picker="openLogoPicker"
      @apply-storage="applyStorage"
      @configure-preset="handleConfigurePreset"
      @open-edit="handleOpenEdit"
      @test-storage="handleTest"
      @remove-storage="requestRemoveProfile"
      @toggle-enabled="toggleEnabled"
    />
  </div>

  <!-- 3. 存储配置独立弹窗（含厂商预设选择与多 Tab 表单） -->
  <AdminStorageEditorDialog
    :visible="editorOpen"
    :editor-mode="editorMode"
    :selected-vendor-key="selectedVendorKey"
    :form="form"
    :selected-provider-meta="selectedProviderMeta"
    :text-fields="textFields"
    :switch-fields="switchFields"
    :bucket-domains="bucketDomains"
    :saving="saving"
    :fetching="fetching"
    :active-logo-target-id="activeLogoTarget?.id"
    :config-text-value="configTextValue"
    :config-bool-value="configBoolValue"
    :provider-label="providerLabel"
    @update:visible="handleEditorDialogVisibleUpdate"
    @select-vendor="handleVendorSelect"
    @update-config-value="updateConfigValue"
    @fetch-bucket-domains="fetchBucketDomains"
    @save-draft="saveDraft"
    @cancel="closeEditor"
    @open-logo-picker="openLogoPickerForEditing"
    @update-form-field="handleUpdateFormField"
  />

  <!-- 4. 存储 Logo 选择弹窗 -->
  <AdminLogoPickerPopover
    ref="logoPickerPopoverRef"
    scope="storage-profile"
    :target-id="activeLogoTarget?.id"
    :target-name="activeLogoTarget?.name"
    :target-base-url="activeLogoTarget?.endpoint"
    initial-category="storage"
    @hide="activeLogoTarget = null"
  />
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Database, Plus } from '@iconoir/vue'
import ActionButton from '../../../../shared/components/ActionButton.vue'
import AdminLogoPickerPopover from '../../shared/components/AdminLogoPickerPopover.vue'
import AdminEmptyState from '../../shared/components/AdminEmptyState.vue'
import { vendorPresetCatalog } from '../meta'
import { STORAGE_MESSAGES } from './messages'
import { useStorageContext } from '../useStorageContext'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import type { StorageCard } from '../profiles'
import AdminStorageTable, { type StorageTableRow } from '../components/AdminStorageTable.vue'
import AdminStorageEditorDialog from '../components/AdminStorageEditorDialog.vue'

const adminConfirm = useAdminConfirm()
const {
  saving,
  fetching,
  loading,
  loadFailed,
  applyingId,
  testingId,
  togglingId,
  editorOpen,
  editorMode,
  selectedVendorKey,
  bucketDomains,
  form,
  creatableProviders,
  selectedProviderMeta,
  textFields,
  switchFields,
  cards,
  configTextValue,
  configBoolValue,
  updateConfigValue,
  providerLabel,
  handleVendorSelect,
  openCreate,
  openEdit,
  closeEditor,
  saveDraft,
  applyStorage,
  handleTest,
  toggleEnabled,
  fetchBucketDomains,
  requestRemoveProfile,
  loadData,
} = useStorageContext({ confirm: adminConfirm })

function getBucketDisplay(card: (typeof cards.items.value)[number]): string {
  if (card.storage.provider === 'local') return '/uploads'
  const val = card.storage.config?.bucket_name
  return typeof val === 'string' && val.trim() ? val.trim() : '—'
}

function getEndpointDisplay(card: (typeof cards.items.value)[number]): string {
  if (card.storage.provider === 'local') return STORAGE_MESSAGES.localEndpoint
  const domain = card.storage.config?.access_domain
  if (typeof domain === 'string' && domain.trim()) return domain.trim()
  const ep = card.storage.config?.endpoint
  if (typeof ep === 'string' && ep.trim()) return ep.trim()
  return '—'
}

const PRESET_MATCHERS: Array<{ key: string; keywords: string[] }> = [
  { key: 'aliyun-oss', keywords: ['aliyuncs.com', 'aliyun', '阿里', 'oss'] },
  { key: 'tencent-cos', keywords: ['myqcloud.com', 'tencent', '腾讯', 'cos'] },
  { key: 'cloudflare-r2', keywords: ['r2.cloudflarestorage.com', 'cloudflare', 'r2'] },
  { key: 'minio', keywords: ['9000', 'minio'] },
  { key: 'aws-s3', keywords: ['amazonaws.com', 's3', 'amazon'] },
]

function matchProfileToPreset(storage: StorageCard): string | null {
  const text = `${storage.config?.endpoint ?? ''} ${storage.name}`.toLowerCase()
  const matched = PRESET_MATCHERS.find((m) => m.keywords.some((k) => text.includes(k)))
  return matched?.key ?? null
}

function buildConfiguredRow(
  card: (typeof cards.items.value)[number],
  presetKey?: string,
): StorageTableRow {
  const isCur = card.storage.isCurrent
  const isConf = card.statusText === STORAGE_MESSAGES.configured
  return {
    id: card.storage.id,
    isPresetOnly: false,
    presetKey,
    storage: card.storage,
    name: card.storage.name,
    vendorLogo: card.vendorLogo,
    vendorIcon: card.vendorIcon,
    avatarLabel: card.avatarLabel,
    isCurrent: isCur,
    enabled: card.storage.enabled,
    statusType: isCur ? 'current' : isConf ? 'configured' : 'unconfigured',
    bucket: getBucketDisplay(card),
    endpoint: getEndpointDisplay(card),
    supportsTest: card.supportsTest,
    canTest: card.canTest,
    canApply: card.canApply,
  }
}

function buildUnconfiguredPresetRow(preset: (typeof vendorPresetCatalog)[number]): StorageTableRow {
  return {
    id: `preset-${preset.key}`,
    isPresetOnly: true,
    presetKey: preset.key,
    name: preset.label,
    vendorLogo: preset.logoSrc,
    avatarLabel: preset.label.slice(0, 2),
    isCurrent: false,
    enabled: false,
    statusType: 'unconfigured',
    bucket: '—',
    endpoint: String(preset.defaultConfig?.endpoint ?? '—'),
    supportsTest: false,
    canTest: false,
    canApply: false,
  }
}

const allStorageRows = computed<StorageTableRow[]>(() => {
  const configuredCards = cards.items.value
  const rows: StorageTableRow[] = []
  const claimedProfileIds = new Set<string>()

  // 1. 遍历预设 catalog 中的各大厂商（阿里云 OSS、腾讯云 COS、Amazon S3、Cloudflare R2、MinIO）
  for (const preset of vendorPresetCatalog) {
    const matched = configuredCards.filter(
      (card) =>
        !claimedProfileIds.has(card.storage.id) &&
        matchProfileToPreset(card.storage) === preset.key,
    )

    if (matched.length > 0) {
      for (const card of matched) {
        claimedProfileIds.add(card.storage.id)
        rows.push(buildConfiguredRow(card, preset.key))
      }
    } else {
      rows.push(buildUnconfiguredPresetRow(preset))
    }
  }

  // 2. 将剩余未匹配到特定预设的已配置 profile 追加进来
  for (const card of configuredCards) {
    if (!claimedProfileIds.has(card.storage.id)) {
      rows.push(buildConfiguredRow(card))
    }
  }

  return rows
})

const activeLogoTarget = ref<{ id: string; name: string; endpoint: string } | null>(null)
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)

function openLogoPickerForEditing(event: Event) {
  if (!form.value.id) return
  activeLogoTarget.value = {
    id: form.value.id,
    name: form.value.name,
    endpoint: String(form.value.config?.endpoint || ''),
  }
  logoPickerPopoverRef.value?.toggle(event)
}

function handleOpenCreate() {
  openCreate()
}

function handleConfigurePreset(presetKey?: string) {
  openCreate()
  if (presetKey) handleVendorSelect(presetKey)
}

function handleOpenEdit(id: string) {
  openEdit(id)
}

function openLogoPicker(
  event: Event,
  card: { storage?: { id: string; name: string; config?: Record<string, unknown> } },
) {
  if (!card.storage) return
  activeLogoTarget.value = {
    id: card.storage.id,
    name: card.storage.name,
    endpoint: String(card.storage.config?.endpoint || ''),
  }
  logoPickerPopoverRef.value?.toggle(event)
}

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}

function handleUpdateFormField(field: 'name', value: string | boolean) {
  if (field === 'name') form.value.name = String(value)
}

onMounted(() => {
  void loadData()
})
</script>
