<template>
  <div class="flex flex-col gap-4">
    <!-- 头部：板块名 + 副标题 + 操作栏 -->
    <div class="flex flex-wrap items-center justify-between gap-3 pb-1">
      <div class="flex min-w-0 flex-col gap-0.5">
        <div class="flex items-center gap-2">
          <component
            :is="SETTINGS_SECTIONS.storage.icon"
            class="size-4 shrink-0 text-muted-foreground"
            aria-hidden="true"
          />
          <h3 class="text-admin-title-sm font-bold text-foreground">
            {{ SETTINGS_SECTIONS.storage.title }}
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

    <!-- 1. 存储列表表格（方案 A：加载失败只 toast，不进 Status/空态；表格恒显预设行占位） -->
    <AdminStorageTable
      :rows="allStorageRows"
      :loading="loading"
      :applying-id="applyingId"
      :testing-id="testingId"
      :is-toggling="isStorageToggling"
      :active-logo-target-id="activeLogoTarget?.id"
      @open-logo-picker="openLogoPicker"
      @apply-storage="applyStorage"
      @configure-preset="handleConfigurePreset"
      @open-edit="handleOpenEdit"
      @test-storage="handleTest"
      @remove-storage="requestRemoveProfile"
      @toggle-enabled="toggleEnabled"
    >
      <template #footer>
        <slot name="footer" />
      </template>
    </AdminStorageTable>
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
import { computed, onMounted, ref, watch } from 'vue'
import { Plus } from '@iconoir/vue'
import ActionButton from '@/core/components/ActionButton.vue'
import { AdminLogoPickerPopover } from '../../../../shared/index'
import { vendorPresetCatalog } from '../../../core/storage/meta'
import { STORAGE_MESSAGES } from './messages'
import { SETTINGS_SECTIONS } from '../../pages/sections'
import { useStorageContext } from '../../../session/storage/useStorageContext'
import { useAdminConfirm } from '../../../../shared/index'
import { reconcileStableRows } from '../../../../shared/index'
import type { StorageCard } from '../../../core/storage/profiles'
import AdminStorageTable, {
  type StorageHealth,
  type StorageTableRow,
} from './AdminStorageTable.vue'
import AdminStorageEditorDialog from './AdminStorageEditorDialog.vue'

const adminConfirm = useAdminConfirm()
const {
  saving,
  fetching,
  loading,
  applyingId,
  testingId,
  toggle: storageToggle,
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

function resolveStorageHealth(card: (typeof cards.items.value)[number]): StorageHealth {
  // 健康三态（方案 A）：当前生效→正常(success)；启用但缺关键配置→异常(danger)；
  // 其余（含 standby 已配置、未配置预设）→未验证(neutral)。
  // standby 已配置不蹭健康色；配置缺失只占位“—”，启用真相源是行内开关。
  // 缺关键配置以后端卡片 canApply 为准（enabled && !canApply）。
  if (card.storage.isCurrent) return 'normal'
  if (card.storage.enabled && !card.canApply) return 'abnormal'
  return 'unverified'
}

function buildConfiguredRow(
  card: (typeof cards.items.value)[number],
  presetKey?: string,
): StorageTableRow {
  const isCur = card.storage.isCurrent
  // 开关乐观走共享 toggle.effective 即时翻转；健康胶囊仍取服务端
  // enabled（resolveStorageHealth 读 card.storage），不随开关预翻。
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
    enabled: storageToggle.effective(card.storage.id, card.storage.enabled),
    health: resolveStorageHealth(card),
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
    health: 'unverified',
    bucket: '—',
    endpoint: String(preset.defaultConfig?.endpoint ?? '—'),
    supportsTest: false,
    canTest: false,
    canApply: false,
  }
}

const storageRowCache = new Map<string, StorageTableRow>()

function isSameStorageCore(cached: StorageTableRow, incoming: StorageTableRow): boolean {
  return (
    cached.id === incoming.id &&
    cached.isPresetOnly === incoming.isPresetOnly &&
    cached.presetKey === incoming.presetKey &&
    cached.name === incoming.name &&
    cached.isCurrent === incoming.isCurrent &&
    cached.enabled === incoming.enabled &&
    cached.health === incoming.health &&
    cached.bucket === incoming.bucket &&
    cached.endpoint === incoming.endpoint
  )
}

function isSameStorageFlags(cached: StorageTableRow, incoming: StorageTableRow): boolean {
  return (
    cached.supportsTest === incoming.supportsTest &&
    cached.canTest === incoming.canTest &&
    cached.canApply === incoming.canApply &&
    cached.storage?.id === incoming.storage?.id &&
    cached.storage?.enabled === incoming.storage?.enabled &&
    cached.storage?.isCurrent === incoming.storage?.isCurrent
  )
}

function isSameStorageRow(cached: StorageTableRow, incoming: StorageTableRow): boolean {
  return isSameStorageCore(cached, incoming) && isSameStorageFlags(cached, incoming)
}

function isStorageToggling(id: string): boolean {
  return storageToggle.isPending(id)
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

  // 行引用稳定：先把乐观覆盖投影进 enabled，再经共享 helper 对账；内容
  // 一致即复用旧引用，只有真正变化的行才返回新对象。
  return reconcileStableRows(storageRowCache, rows, (row) => row.id, isSameStorageRow)
})

// 服务端追上乐观值后清掉覆盖，避免长期遮挡外部变更；不等则保留。
watch(
  () => cards.items.value,
  (items) => {
    storageToggle.reconcileAll(
      items.map((card) => ({ key: card.storage.id, enabled: card.storage.enabled })),
    )
  },
  { deep: true },
)

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
