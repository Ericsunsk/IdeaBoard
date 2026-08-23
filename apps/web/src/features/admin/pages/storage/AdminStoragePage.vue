<template>
  <AdminResourceGridFrame
    :empty-description="
      hasStorageSearch ? '调整搜索关键词后重试。' : '点击右上角新增存储开始配置。'
    "
    :empty-title="emptyStorageMessage"
    :is-empty="filteredStorageCards.length === 0"
    :is-loading="loading && storages.length === 0"
  >
    <template #toolbar>
      <AdminResourceToolbar>
        <template #search>
          <AdminToolbarSearch v-model="searchKeyword" input-label="搜索存储对象" />
        </template>

        <template #metrics="{ metricClass }">
          <span :class="metricClass">
            <span class="h-2.5 w-2.5 shrink-0 rounded-full bg-admin-success"></span>
            <span>{{ storageCountLabel }}</span>
          </span>
          <Tag
            v-if="currentStorage"
            :value="`当前 ${currentStorage.name}`"
            severity="success"
            rounded
          />
          <Tag
            :value="`${configuredStorageCount}/${storages.length} 已配置`"
            severity="info"
            rounded
          />
        </template>

        <template #actions="{ iconButtonClass }">
          <Button
            text
            :class="iconButtonClass"
            aria-label="刷新存储"
            :loading="loading"
            @click="refreshData"
          >
            <template #icon>
              <Refresh class="size-4 text-admin-text-2" />
            </template>
          </Button>
          <Button
            label="新增存储"
            :class="adminActionButtonClass"
            :disabled="creatableProviders.length === 0"
            @click="handleOpenCreate"
          >
            <template #icon>
              <Plus class="size-4" />
            </template>
          </Button>
        </template>
      </AdminResourceToolbar>
    </template>

    <!-- 存储卡片列表 -->
    <Card
      v-for="card in filteredStorageCards"
      :key="card.storage.id"
      :class="[adminMasonryCardClass, 'group relative overflow-hidden transition-colors']"
      :pt="adminCardPt"
    >
      <template #title>
        <div class="flex items-start justify-between gap-3">
          <!-- 左侧：Logo 按钮 + 名称 + 供应商副标 -->
          <div class="flex min-w-0 items-center gap-3">
            <button
              type="button"
              :class="[adminLogoButtonClass, 'size-10 p-2 shrink-0 rounded-xl shadow-xs']"
              :data-control-state="activeLogoTarget?.id === card.storage.id ? 'selected' : 'idle'"
              :aria-expanded="activeLogoTarget?.id === card.storage.id ? 'true' : 'false'"
              title="点击切换存储 Logo"
              aria-label="切换存储 Logo"
              @click="openLogoPicker($event, card)"
            >
              <img
                v-if="card.vendorLogo"
                :src="card.vendorLogo"
                :alt="card.storage.name"
                class="size-full object-contain transition-transform group-hover:scale-105"
              />
              <component
                :is="card.vendorIcon"
                v-else-if="card.vendorIcon"
                class="size-5 text-admin-text-2"
              />
              <span v-else class="text-xs font-bold text-admin-accent uppercase select-none">
                {{ card.avatarLabel }}
              </span>
            </button>
            <div class="min-w-0">
              <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                {{ card.storage.name }}
              </div>
              <div class="truncate text-admin-meta font-medium text-admin-subtitle-color">
                {{ card.subtitle || card.providerLabel }}
              </div>
            </div>
          </div>

          <!-- 右上角：使用中徽章 / 状态徽章 (严格统一尺寸与字阶) -->
          <div class="flex shrink-0 items-center gap-1.5">
            <AdminStatusPill v-if="card.storage.isCurrent" label="使用中" variant="success" />
            <AdminStatusPill
              v-else
              :label="card.statusText"
              :variant="card.statusText === '已配置' ? 'success' : 'neutral'"
              :pulse="false"
            />
          </div>
        </div>
      </template>

      <template #content>
        <div class="flex h-full flex-col justify-between gap-3.5">
          <!-- 核心参数扁平极简网格 -->
          <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div
              v-for="item in card.metaItems"
              :key="`${card.storage.id}-${item.label}`"
              class="flex min-w-0 flex-col gap-0.5"
            >
              <span class="text-admin-caption text-admin-text-3">{{ item.label }}</span>
              <span
                class="truncate font-mono text-xs font-semibold text-admin-text-1"
                :title="item.value"
              >
                {{ item.value }}
              </span>
            </div>
          </div>

          <!-- 复用现有原生尺寸小按钮，支持灵活自适应折行 -->
          <div class="mt-auto flex flex-wrap gap-2 max-sm:flex-col max-sm:items-stretch">
            <Button
              size="small"
              label="编辑"
              severity="contrast"
              @click="handleOpenEdit(card.storage.id)"
            />
            <Button
              v-if="card.supportsTest"
              size="small"
              label="测试连接"
              severity="contrast"
              outlined
              :loading="testingId === card.storage.id"
              :disabled="!card.canTest || testingId !== null"
              @click="handleTest(card.storage)"
            />
            <Button
              v-if="!card.storage.isCurrent"
              size="small"
              label="设为当前"
              severity="contrast"
              :loading="applyingId === card.storage.id"
              :disabled="!card.canApply || applyingId !== null"
              @click="applyStorage(card.storage)"
            />
            <Button
              v-if="!card.storage.isCurrent && !card.storage.builtin"
              size="small"
              label="删除"
              severity="contrast"
              outlined
              @click="requestRemoveProfile(card.storage)"
            />
          </div>
        </div>
      </template>
    </Card>
  </AdminResourceGridFrame>

  <!-- 存储配置独立弹窗（紧凑表单） -->
  <AdminDialogShell
    :visible="editorOpen"
    :aria-label="editorMode === 'create' ? '添加对象存储' : form.name || '存储配置'"
    size="form"
    content-layout="scrollable"
    :busy="saving || savingAndApplying"
    @update:visible="handleEditorDialogVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full min-w-0 items-center justify-between gap-3 pr-6">
        <div class="flex items-center gap-3">
          <button
            type="button"
            :class="[adminLogoButtonClass, 'size-11 p-2 shrink-0 rounded-xl shadow-xs']"
            :title="editorMode === 'edit' ? '点击切换存储 Logo' : undefined"
            :aria-label="editorMode === 'edit' ? '切换存储 Logo' : undefined"
            :data-control-state="activeLogoTarget?.id === form.id ? 'selected' : 'idle'"
            :aria-expanded="activeLogoTarget?.id === form.id ? 'true' : 'false'"
            @click="editorMode === 'edit' && openLogoPickerForEditing($event)"
          >
            <img
              v-if="getDialogVendorLogo()"
              :src="getDialogVendorLogo()"
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
              {{ editorMode === 'create' ? '添加对象存储' : form.name }}
            </h2>
            <p class="m-0 text-admin-caption text-admin-subtitle-color">
              {{
                editorMode === 'create'
                  ? '选择云存储厂商预设或自定义 S3 兼容接入'
                  : `供应商：${providerLabel(form.provider)}`
              }}
            </p>
          </div>
        </div>

        <Tag v-if="form.isCurrent" value="当前生效存储" severity="success" rounded />
      </div>
    </template>

    <div class="flex flex-col gap-3.5 py-1">
      <!-- 1. 新增模式：顶部极简厂商 Logo 预设卡片选择 -->
      <div v-if="editorMode === 'create'" class="flex items-center gap-2.5 pt-0.5">
        <button
          v-for="vendor in vendorPresetCatalog"
          :key="vendor.key"
          v-tooltip.top="vendor.label"
          type="button"
          class="group relative flex size-12 items-center justify-center rounded-xl border p-2 cursor-pointer transition-colors"
          :class="
            selectedVendorKey === vendor.key
              ? 'border-admin-accent bg-white shadow-xs'
              : 'border-admin-border-soft bg-white hover:border-admin-accent/60'
          "
          :aria-label="vendor.label"
          @click="handleVendorSelect(vendor.key)"
        >
          <img
            :src="vendor.logoSrc"
            :alt="vendor.label"
            class="size-full object-contain transition-transform group-hover:scale-105"
          />
        </button>
      </div>

      <!-- 2. 云厂商多字段分页导航 Tab (复用全局 SelectButton 规范) -->
      <div v-if="textFields.length > 0">
        <SelectButton
          v-model="activeConfigTab"
          :options="configTabOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
          size="small"
          aria-label="存储配置分页"
        />
      </div>

      <!-- 3. Tab 1: 基础与凭据 (或本地存储默认视图) -->
      <template v-if="textFields.length === 0 || activeConfigTab === 'basic'">
        <!-- 启用状态开关卡片 -->
        <div
          class="flex items-center justify-between rounded-xl border border-admin-border-soft bg-admin-surface-muted/50 px-4 py-3"
        >
          <span class="text-admin-body font-semibold text-admin-title-color">启用该存储配置</span>
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
              {{ form.enabled ? '已启用' : '已关闭' }}
            </span>
            <ToggleSwitch v-model="form.enabled" size="small" aria-label="存储启用状态" />
          </div>
        </div>

        <!-- 存储名称 -->
        <AdminFormField label="存储配置名称" for="admin-storage-name" required>
          <InputText
            id="admin-storage-name"
            v-model="form.name"
            class="w-full !text-xs"
            placeholder="例如：生产环境主存储 / 阿里云 OSS"
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
              <InputText
                :id="`admin-storage-${field.key}`"
                :model-value="configTextValue(field.key)"
                class="w-full !text-xs font-mono"
                :type="field.type === 'password' ? 'password' : 'text'"
                :placeholder="
                  field.secret && form.secretConfigured && !configTextValue(field.key)
                    ? '已配置，留空则保持不变'
                    : field.placeholder || `请输入${field.label}`
                "
                @update:model-value="updateConfigValue(field.key, $event ?? '')"
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
              <Button
                v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
                text
                size="small"
                label="一键自动探测域名"
                :class="[adminTextButtonClass, '!text-xs font-medium']"
                :loading="fetching"
                @click="fetchBucketDomains"
              />
            </div>
            <InputText
              :id="`admin-storage-${field.key}`"
              :model-value="configTextValue(field.key)"
              class="w-full !text-xs font-mono"
              :type="field.type === 'password' ? 'password' : 'text'"
              :placeholder="
                field.secret && form.secretConfigured && !configTextValue(field.key)
                  ? '已配置，留空则保持不变'
                  : field.placeholder || `请输入${field.label}`
              "
              @update:model-value="updateConfigValue(field.key, $event ?? '')"
            />

            <!-- 探测到的加速域名一键填入 -->
            <div
              v-if="field.key === 'access_domain' && bucketDomains.length > 0"
              class="flex items-center gap-1.5 flex-wrap pt-1"
            >
              <span class="text-[11px] font-semibold text-admin-text-3">探测到域名：</span>
              <button
                v-for="domain in bucketDomains"
                :key="domain"
                type="button"
                class="cursor-pointer rounded-md border border-admin-border-soft bg-white px-2 py-0.5 text-[11px] font-mono font-medium text-admin-accent hover:border-admin-accent transition-colors"
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
              <span v-if="field.hint" class="text-[11px] text-admin-text-3">{{ field.hint }}</span>
            </div>
            <ToggleSwitch
              :model-value="configBoolValue(field.key)"
              size="small"
              @update:model-value="updateConfigValue(field.key, Boolean($event))"
            />
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <Button
        label="取消"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :disabled="saving || savingAndApplying"
        @click="closeEditor"
      />
      <Button
        v-if="!form.isCurrent"
        label="保存"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :loading="saving"
        :disabled="savingAndApplying"
        @click="saveDraft"
      />
      <Button
        :label="form.isCurrent ? '保存' : '设为当前'"
        :class="adminActionButtonClass"
        severity="contrast"
        :loading="savingAndApplying"
        :disabled="saving"
        @click="saveAndApply"
      />
    </template>
  </AdminDialogShell>

  <!-- 存储 Logo 选择弹窗 -->
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
import { DatabaseExport, Plus, Refresh } from '@iconoir/vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminFormField from '../../shared/components/AdminFormField.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminStatusPill from '../../shared/components/AdminStatusPill.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import {
  adminActionButtonClass,
  adminLogoButtonClass,
  adminMasonryCardClass,
  adminTextButtonClass,
} from '../../shared/lib/adminUiClasses'
import { storageVendorLogo, vendorIconMap, vendorPresetCatalog } from '../../storage/storageMeta'
import { useAdminStorageContext } from '../../storage/useAdminStorageContext'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'
import AdminLogoPickerPopover from '../../provider-config/AdminLogoPickerPopover.vue'

type StorageConfigTab = 'basic' | 'endpoint' | 'advanced'
const activeConfigTab = ref<StorageConfigTab>('basic')
const configTabOptions = computed<{ label: string; value: StorageConfigTab }[]>(() => {
  const options: { label: string; value: StorageConfigTab }[] = [
    { label: '基础与凭据', value: 'basic' },
    { label: '端点与网络', value: 'endpoint' },
  ]
  if (switchFields.value.length > 0) {
    options.push({ label: '高级选项', value: 'advanced' })
  }
  return options
})
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
  storages,
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
} = useAdminStorageContext({ confirm: adminConfirm })
const searchKeyword = cards.searchKeyword
const hasStorageSearch = cards.hasSearch
const filteredStorageCards = cards.items
const currentStorage = cards.currentStorage
const configuredStorageCount = cards.configuredStorageCount
const storageCountLabel = cards.storageCountLabel
const emptyStorageMessage = cards.emptyMessage

// 分类字段 (按信息密度结构化分类)
const authFields = computed(() =>
  textFields.value.filter(
    (f) => f.key === 'access_key_id' || f.key === 'secret_access_key' || f.secret,
  ),
)
const endpointFields = computed(() =>
  textFields.value.filter(
    (f) => f.key !== 'access_key_id' && f.key !== 'secret_access_key' && !f.secret,
  ),
)

const activeLogoTarget = ref<{ id: string; name: string; endpoint: string } | null>(null)
const logoPickerPopoverRef = ref<InstanceType<typeof AdminLogoPickerPopover> | null>(null)

function getDialogVendorLogo(): string | undefined {
  if (editorMode.value === 'create') {
    return vendorPresetCatalog.find((v) => v.key === selectedVendorKey.value)?.logoSrc
  }
  return storageVendorLogo(form.value)
}

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
  activeConfigTab.value = 'basic'
  openCreate()
}

function handleOpenEdit(id: string) {
  activeConfigTab.value = 'basic'
  openEdit(id)
}

function openLogoPicker(
  event: Event,
  card: { storage: { id: string; name: string; config?: Record<string, unknown> } },
) {
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

onMounted(() => {
  void loadData()
})
</script>
