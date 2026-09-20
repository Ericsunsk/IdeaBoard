<template>
  <div class="flex flex-col gap-4">
    <!-- 1. 顶栏操作与搜索过滤工具条 -->
    <div class="flex w-full flex-col gap-3">
      <div class="flex w-full flex-wrap items-center justify-between gap-2.5">
        <!-- 左侧：搜索输入框与状态筛选器 -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <AdminToolbarSearch
            v-model="searchKeyword"
            fluid
            input-label="搜索存储对象"
            placeholder="搜索存储名称、供应商或存储桶..."
          />

          <AdminOptionFilter
            v-model="statusFilter"
            :options="statusFilterOptions"
            all-value="all"
            label="筛选存储状态"
          />
        </div>

        <!-- 右侧：刷新同步与新增操作 -->
        <div class="flex shrink-0 items-center gap-2">
          <AdminRefreshButton :loading="loading" aria-label="刷新存储" @click="refreshData" />

          <AdminButton
            :class="adminActionButtonClass"
            :disabled="creatableProviders.length === 0"
            @click="handleOpenCreate"
          >
            <Plus class="size-4" />
            新增存储
          </AdminButton>
        </div>
      </div>

      <!-- 次级统计提示条：共 X 个对象存储 -->
      <div v-if="!loading" class="flex items-center justify-between pt-1">
        <div class="flex items-center gap-2 text-sm text-admin-text-2">
          <span class="font-medium text-admin-text-1">
            共 {{ filteredStorageRows.length }} 个存储服务
          </span>
          <span v-if="hasActiveFilter" class="text-xs text-admin-text-3">
            (共 {{ allStorageRows.length }} 个)
          </span>
        </div>
      </div>
    </div>

    <!-- 2. 存储列表表格与空状态/加载态 -->
    <AdminStorageTable
      :rows="filteredStorageRows"
      :loading="loading"
      :has-active-filter="hasActiveFilter"
      :applying-id="applyingId"
      :testing-id="testingId"
      :active-logo-target-id="activeLogoTarget?.id"
      @clear-filters="clearFilters"
      @open-logo-picker="openLogoPicker"
      @apply-storage="applyStorage"
      @configure-preset="handleConfigurePreset"
      @open-edit="handleOpenEdit"
      @test-storage="handleTest"
      @remove-storage="requestRemoveProfile"
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
    :saving-and-applying="savingAndApplying"
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
    @save-and-apply="saveAndApply"
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
import { Plus } from '@iconoir/vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminLogoPickerPopover from '../../shared/components/AdminLogoPickerPopover.vue'
import AdminOptionFilter from '../../shared/components/AdminOptionFilter.vue'
import AdminRefreshButton from '../../shared/components/AdminRefreshButton.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import { vendorPresetCatalog } from '../meta'
import { useStorageContext } from '../useStorageContext'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import type { StorageCard } from '../profiles'
import AdminStorageTable, { type StorageTableRow } from '../components/AdminStorageTable.vue'
import AdminStorageEditorDialog from '../components/AdminStorageEditorDialog.vue'

const adminConfirm = useAdminConfirm()
const {
  saving,
  savingAndApplying,
  fetching,
  loading,
  applyingId,
  testingId,
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
  saveAndApply,
  applyStorage,
  handleTest,
  fetchBucketDomains,
  requestRemoveProfile,
  loadData,
  refreshData,
} = useStorageContext({ confirm: adminConfirm })

const searchKeyword = ref('')

type StatusFilterValue = 'all' | 'current' | 'configured' | 'unconfigured'
const statusFilter = ref<StatusFilterValue>('all')
const statusFilterOptions: { label: string; value: StatusFilterValue }[] = [
  { label: '全部状态', value: 'all' },
  { label: '使用中', value: 'current' },
  { label: '已配置', value: 'configured' },
  { label: '未配置', value: 'unconfigured' },
]

const hasActiveFilter = computed(() => {
  return Boolean(searchKeyword.value.trim()) || statusFilter.value !== 'all'
})

function clearFilters() {
  searchKeyword.value = ''
  statusFilter.value = 'all'
}

function getBucketDisplay(card: (typeof cards.items.value)[number]): string {
  if (card.storage.provider === 'local') return '/uploads'
  const val = card.storage.config?.bucket_name
  return typeof val === 'string' && val.trim() ? val.trim() : '—'
}

function getEndpointDisplay(card: (typeof cards.items.value)[number]): string {
  if (card.storage.provider === 'local') return '服务器本地'
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
  const isConf = card.statusText === '已配置'
  return {
    id: card.storage.id,
    isPresetOnly: false,
    presetKey,
    storage: card.storage,
    name: card.storage.name,
    subtitle: card.subtitle || card.providerLabel,
    providerLabel: card.providerLabel,
    vendorLogo: card.vendorLogo,
    vendorIcon: card.vendorIcon,
    avatarLabel: card.avatarLabel,
    isCurrent: isCur,
    statusText: isCur ? '使用中' : (card.statusText as '使用中' | '已配置' | '未配置'),
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
    subtitle: preset.description,
    providerLabel: preset.label,
    vendorLogo: preset.logoSrc,
    avatarLabel: preset.label.slice(0, 2),
    isCurrent: false,
    statusText: '未配置',
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

const filteredStorageRows = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  let list = allStorageRows.value

  if (keyword) {
    list = list.filter((r) => {
      const matchName = r.name.toLowerCase().includes(keyword)
      const matchSubtitle = r.subtitle.toLowerCase().includes(keyword)
      const matchBucket = r.bucket.toLowerCase().includes(keyword)
      const matchEndpoint = r.endpoint.toLowerCase().includes(keyword)
      return matchName || matchSubtitle || matchBucket || matchEndpoint
    })
  }

  if (statusFilter.value === 'current') {
    list = list.filter((r) => r.isCurrent)
  } else if (statusFilter.value === 'configured') {
    list = list.filter((r) => r.statusType === 'configured')
  } else if (statusFilter.value === 'unconfigured') {
    list = list.filter((r) => r.statusType === 'unconfigured')
  }

  return list
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

function handleUpdateFormField(field: 'name' | 'enabled', value: string | boolean) {
  if (field === 'name') form.value.name = String(value)
  if (field === 'enabled') form.value.enabled = Boolean(value)
}

onMounted(() => {
  void loadData()
})
</script>
