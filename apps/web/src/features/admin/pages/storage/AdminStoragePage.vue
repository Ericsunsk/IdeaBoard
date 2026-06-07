<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0 min-[1081px]:-mb-[17px]">
    <div
      class="flex min-w-0 flex-col overflow-hidden rounded-2xl border border-admin-border-soft bg-admin-card-strong p-0 min-[1081px]:h-[calc(100dvh-117px)]"
    >
      <div
        class="flex shrink-0 flex-wrap items-center justify-between gap-4 bg-transparent px-6 py-4"
      >
        <div class="flex flex-wrap items-center gap-3">
          <IconField class="relative w-full sm:w-[260px]">
            <InputIcon>
              <Search class="size-4 text-admin-text-3" />
            </InputIcon>
            <InputText
              v-model="searchKeyword"
              placeholder="Search"
              fluid
              class="!h-[38px] !w-full !rounded-lg !bg-white !text-[15px] !text-admin-text-1 !shadow-[0_3px_10px_rgba(45,64,89,0.05)] placeholder:!text-[#7a8798]"
            />
            <Button
              v-if="searchKeyword"
              text
              rounded
              class="absolute top-1/2 right-1 !h-8 !w-8 -translate-y-1/2"
              aria-label="清空搜索"
              @click="searchKeyword = ''"
            >
              <template #icon>
                <Xmark />
              </template>
            </Button>
          </IconField>
          <span
            class="flex shrink-0 items-center gap-2 whitespace-nowrap rounded-full border border-admin-border bg-white px-3 py-1.5 text-[13px] font-bold text-admin-text-1 shadow-[0_2px_8px_rgba(45,64,89,0.02)]"
          >
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
        </div>

        <div class="flex flex-wrap items-center justify-end gap-3">
          <Button
            text
            :class="framedIconButtonClass"
            aria-label="刷新存储"
            :loading="loading"
            @click="loadData"
          >
            <template #icon>
              <Refresh class="size-4 text-admin-text-2" />
            </template>
          </Button>
          <Button
            label="新增存储"
            class="!h-9 !rounded-lg"
            :disabled="creatableProviders.length === 0"
            @click="openCreate"
          >
            <template #icon>
              <Plus class="size-4" />
            </template>
          </Button>
        </div>
      </div>

      <div
        class="app-scrollbar min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 pb-6"
        :class="{ 'is-scrolling': isStoragePanelScrolling }"
        @scroll.passive="handleStoragePanelScroll"
      >
        <div
          v-if="loading && storages.length === 0"
          class="flex min-h-[260px] items-center justify-center text-admin-content text-admin-text-3"
        >
          <ProgressSpinner class="!h-9 !w-9 text-admin-accent" stroke-width="4" />
        </div>

        <div
          v-else-if="filteredStorages.length === 0"
          class="flex min-h-[260px] flex-col items-center justify-center rounded-lg border border-admin-border-soft bg-admin-card text-center text-admin-text-2"
        >
          <div class="text-admin-title font-semibold text-admin-title-color">
            {{ emptyStorageMessage }}
          </div>
          <div class="mt-2 text-admin-meta text-admin-subtitle-color">
            {{ hasStorageSearch ? '调整搜索关键词后重试。' : '点击右上角新增存储开始配置。' }}
          </div>
        </div>

        <div v-else class="columns-1 gap-3.5 md:columns-2 xl:columns-3">
          <Card
            v-for="storage in filteredStorages"
            :key="storage.id"
            class="mb-3.5 break-inside-avoid !rounded-lg"
          >
            <template #title>
              <div class="flex items-start gap-3">
                <div class="flex min-w-0 items-center gap-3">
                  <Avatar
                    :image="storageVendorLogo(storage)"
                    :label="
                      storageVendorLogo(storage)
                        ? undefined
                        : providerLabel(storage.provider).slice(0, 2).toUpperCase()
                    "
                    shape="circle"
                    :class="[
                      '!h-10 !w-10 !shrink-0 !text-sm !font-bold',
                      storageVendorLogo(storage)
                        ? '!bg-admin-surface-muted !p-1'
                        : '!bg-admin-accent !text-admin-text-inverse',
                    ]"
                  />
                  <div class="min-w-0">
                    <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                      {{ storage.name }}
                    </div>
                    <div class="truncate text-admin-meta font-medium text-admin-subtitle-color">
                      {{ storageSubtitle(storage) }}
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <template #content>
              <div class="flex h-full min-h-[148px] flex-col gap-3.5">
                <div class="flex flex-wrap items-center gap-1.5">
                  <Tag
                    :value="providerLabel(storage.provider)"
                    severity="info"
                    :class="storageCardTagClass"
                    rounded
                  />
                  <Tag
                    :value="statusText(storage)"
                    :severity="getStorageStatusSeverity(statusClass(storage))"
                    :class="storageCardTagClass"
                    rounded
                  />
                </div>

                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div
                    v-for="item in storageMetaItems(storage)"
                    :key="`${storage.id}-${item.label}`"
                    class="flex min-w-0 flex-col gap-1"
                  >
                    <span class="text-admin-caption text-admin-text-3">{{ item.label }}</span>
                    <span class="break-all text-admin-body leading-admin-body text-admin-text-1">
                      {{ item.value }}
                    </span>
                  </div>
                </div>

                <div class="flex flex-wrap gap-1.5">
                  <Tag
                    v-for="chip in storageFeatureChips(storage)"
                    :key="`${storage.id}-${chip}`"
                    :value="chip"
                    severity="secondary"
                    :class="storageCardTagClass"
                    rounded
                  />
                </div>

                <div class="mt-auto flex flex-wrap gap-2 max-sm:flex-col max-sm:items-stretch">
                  <Button
                    size="small"
                    label="编辑"
                    severity="contrast"
                    @click="openEdit(storage.id)"
                  />
                  <Button
                    v-if="supportsTest(storage)"
                    size="small"
                    label="测试连接"
                    severity="contrast"
                    outlined
                    :loading="testingId === storage.id"
                    :disabled="!canTest(storage)"
                    @click="handleTest(storage)"
                  />
                  <Button
                    v-if="!storage.isCurrent"
                    size="small"
                    label="设为当前"
                    severity="contrast"
                    :loading="applyingId === storage.id"
                    :disabled="!canApply(storage)"
                    @click="applyStorage(storage.id)"
                  />
                  <Button
                    v-if="!storage.isCurrent && !storage.builtin"
                    size="small"
                    label="删除"
                    severity="contrast"
                    outlined
                    @click="removeProfile(storage.id)"
                  />
                </div>
              </div>
            </template>
          </Card>
        </div>
      </div>
    </div>
  </div>

  <Dialog
    :visible="editorOpen"
    modal
    dismissable-mask
    :draggable="false"
    :style="{ width: '920px', maxWidth: 'calc(100vw - 24px)' }"
    @update:visible="handleEditorDialogVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full flex-col gap-4 pr-4 md:flex-row md:items-start md:justify-between">
        <div class="min-w-0 flex-1">
          <div class="mb-3 flex flex-wrap gap-2">
            <Tag :value="form.isCurrent ? '当前存储' : '存储配置'" severity="secondary" rounded />
          </div>
          <h2 class="m-0 text-admin-modal-title leading-[1.15] font-bold text-admin-title-color">
            {{ editorMode === 'create' ? '添加存储' : '编辑存储' }}
          </h2>
        </div>

        <Tag
          v-if="editorMode !== 'create'"
          :value="providerLabel(form.provider)"
          severity="contrast"
          rounded
          class="mt-1"
        />
      </div>
    </template>

    <div
      class="app-scrollbar max-h-[min(68vh,640px)] overflow-y-auto overscroll-contain pr-3"
      :class="{ 'is-scrolling': isStorageDialogScrolling }"
      @scroll.passive="handleStorageDialogScroll"
    >
      <Tabs value="basic" class="min-w-0 pr-3">
        <TabList>
          <Tab value="basic">基本信息</Tab>
          <Tab value="connection">连接配置</Tab>
          <Tab value="advanced">高级选项</Tab>
        </TabList>

        <TabPanels>
          <TabPanel value="basic">
            <div class="flex flex-col gap-3.5 pt-1">
              <div
                v-if="editorMode === 'create'"
                class="grid items-end gap-3 md:grid-cols-[minmax(280px,360px)_minmax(0,1fr)]"
              >
                <div class="flex min-w-0 flex-1 flex-col gap-1.5">
                  <div class="flex items-center justify-between gap-2">
                    <label class="text-admin-meta font-[680] text-admin-text-2">供应商</label>
                    <span class="text-admin-meta text-admin-text-3">仅已接入供应商可直接创建</span>
                  </div>
                  <Select
                    :model-value="selectedVendorKey"
                    :options="vendorSelectOptions"
                    option-label="label"
                    option-value="value"
                    option-disabled="disabled"
                    class="w-full"
                    placeholder="选择供应商"
                    @update:model-value="handleVendorSelect"
                  >
                    <template #value>
                      <div v-if="activeVendorPreset" class="flex min-w-0 items-center gap-2 py-1">
                        <Avatar
                          :image="activeVendorPreset.logoSrc"
                          :label="
                            activeVendorPreset.logoSrc
                              ? undefined
                              : activeVendorPreset.label.slice(0, 1)
                          "
                          shape="circle"
                          class="!h-8 !w-8 !shrink-0 !bg-admin-surface-muted !p-1"
                        />
                        <span
                          class="truncate text-admin-content font-semibold text-admin-title-color"
                        >
                          {{ activeVendorPreset.label }}
                        </span>
                      </div>
                      <span v-else class="text-admin-text-3">选择供应商</span>
                    </template>
                    <template #option="{ option }">
                      <div class="flex min-w-0 items-center gap-2">
                        <Avatar
                          :image="option.vendor.logoSrc"
                          :label="
                            option.vendor.logoSrc ? undefined : option.vendor.label.slice(0, 1)
                          "
                          shape="circle"
                          class="!h-8 !w-8 !shrink-0 !bg-admin-surface-muted !p-1"
                        />
                        <span class="truncate text-admin-body font-semibold text-admin-title-color">
                          {{ option.vendor.label }}
                        </span>
                      </div>
                    </template>
                  </Select>
                </div>
                <Message v-if="activeVendorPreset" severity="secondary" :closable="false">
                  {{ activeVendorPreset.description }}
                </Message>
              </div>

              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div class="flex min-w-0 flex-col gap-1.5">
                  <label class="text-admin-meta font-[680] text-admin-text-2">存储名称</label>
                  <InputText v-model="form.name" placeholder="例如：生产存储 / 备用存储" />
                </div>
                <div class="flex min-w-0 flex-col gap-1.5">
                  <label class="text-admin-meta font-[680] text-admin-text-2">启用状态</label>
                  <div
                    class="flex min-h-10 items-center justify-between rounded-xl border border-admin-border bg-admin-surface-muted px-3"
                  >
                    <span class="text-admin-body text-admin-text-2">
                      {{ form.enabled ? '已启用' : '已关闭' }}
                    </span>
                    <ToggleSwitch v-model="form.enabled" />
                  </div>
                </div>
              </div>
            </div>
          </TabPanel>

          <TabPanel value="connection">
            <div v-if="textFields.length > 0" class="grid grid-cols-1 gap-3 pt-1 lg:grid-cols-2">
              <div
                v-for="field in textFields"
                :key="field.key"
                class="flex min-w-0 flex-1 flex-col gap-1.5"
              >
                <div class="flex items-center justify-between gap-2">
                  <label class="text-admin-meta font-[680] text-admin-text-2">
                    {{ field.label }}
                  </label>
                  <Button
                    v-if="
                      field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info
                    "
                    text
                    size="tiny"
                    label="自动获取"
                    :loading="fetching"
                    @click="fetchBucketDomains"
                  />
                </div>
                <InputText
                  :model-value="configTextValue(field.key)"
                  :class="['w-full', { 'lg:!max-w-[220px]': field.key === 'region_id' }]"
                  :type="field.type === 'password' ? 'password' : 'text'"
                  :placeholder="
                    field.secret && form.secretConfigured && !configTextValue(field.key)
                      ? '已配置，留空则保持不变'
                      : field.placeholder || `请输入${field.label}`
                  "
                  @update:model-value="updateConfigValue(field.key, $event ?? '')"
                />
                <span v-if="field.hint" class="text-admin-caption text-admin-text-3">
                  {{ field.hint }}
                </span>
                <div
                  v-if="field.key === 'access_domain' && bucketDomains.length > 0"
                  class="mt-2 flex flex-wrap gap-2"
                >
                  <Tag value="检测到的域名" severity="secondary" rounded />
                  <Button
                    v-for="domain in bucketDomains"
                    :key="domain"
                    :label="domain"
                    size="small"
                    severity="secondary"
                    text
                    rounded
                    @click="updateConfigValue(field.key, `https://${domain}`)"
                  />
                </div>
              </div>
            </div>
            <Message v-else severity="secondary" :closable="false">
              当前存储类型无需额外密钥配置，保存后即可切换使用。
            </Message>
          </TabPanel>

          <TabPanel value="advanced">
            <div v-if="switchFields.length > 0" class="grid grid-cols-1 gap-3 pt-1 lg:grid-cols-2">
              <div
                v-for="field in switchFields"
                :key="field.key"
                class="flex items-center justify-between gap-3.5 rounded-[14px] border border-admin-border bg-admin-surface-muted px-4 py-3.5"
              >
                <div>
                  <div class="text-admin-body font-semibold text-admin-title-color">
                    {{ field.label }}
                  </div>
                  <div
                    v-if="field.hint"
                    class="mt-1 text-admin-meta leading-[1.6] text-admin-subtitle-color"
                  >
                    {{ field.hint }}
                  </div>
                </div>
                <ToggleSwitch
                  :value="configBoolValue(field.key)"
                  @update:model-value="updateConfigValue(field.key, Boolean($event))"
                />
              </div>
            </div>
            <Message v-else severity="secondary" :closable="false">
              当前存储类型没有高级选项。
            </Message>
          </TabPanel>
        </TabPanels>
      </Tabs>
    </div>

    <template #footer>
      <div class="flex w-full flex-wrap justify-end gap-2">
        <Button label="取消" severity="secondary" outlined @click="closeEditor" />
        <Button label="保存" severity="secondary" :loading="saving" @click="saveDraft" />
        <Button
          :label="form.isCurrent ? '保存当前配置' : '保存并设为当前'"
          :loading="savingAndApplying"
          @click="saveAndApply"
        />
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Plus, Refresh, Search, Xmark } from '@iconoir/vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { storageVendorLogo } from './storageMeta'
import { useStorageManager } from './useStorageManager'

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
  activeVendorPreset,
  vendorSelectOptions,
  configTextValue,
  configBoolValue,
  updateConfigValue,
  providerLabel,
  supportsTest,
  canTest,
  canApply,
  statusText,
  statusClass,
  storageSubtitle,
  storageMetaItems,
  storageFeatureChips,
  handleVendorSelect,
  openCreate,
  openEdit,
  closeEditor,
  saveDraft,
  saveAndApply,
  applyStorage,
  handleTest,
  fetchBucketDomains,
  removeProfile,
  loadData,
} = useStorageManager()
const { isScrolling: isStoragePanelScrolling, handleScroll: handleStoragePanelScroll } =
  useTransientScrolling()
const { isScrolling: isStorageDialogScrolling, handleScroll: handleStorageDialogScroll } =
  useTransientScrolling()
const searchKeyword = ref('')
const currentStorage = computed(() => storages.value.find((storage) => storage.isCurrent) || null)
const hasStorageSearch = computed(() => Boolean(searchKeyword.value.trim()))
const filteredStorages = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return storages.value

  return storages.value.filter((storage) => {
    const metaText = storageMetaItems(storage)
      .flatMap((item) => [item.label, item.value])
      .join(' ')
    const featureText = storageFeatureChips(storage).join(' ')
    const searchableText = [
      storage.name,
      providerLabel(storage.provider),
      storageSubtitle(storage),
      statusText(storage),
      metaText,
      featureText,
    ]
      .join(' ')
      .toLowerCase()

    return searchableText.includes(keyword)
  })
})
const configuredStorageCount = computed(
  () => storages.value.filter((storage) => statusClass(storage) !== 'is-empty').length,
)
const storageCountLabel = computed(() =>
  hasStorageSearch.value
    ? `${filteredStorages.value.length} 匹配存储`
    : `${storages.value.length} 存储`,
)
const emptyStorageMessage = computed(() => (hasStorageSearch.value ? '无匹配存储' : '暂无存储配置'))
const framedIconButtonClass =
  '!h-9 !w-9 !rounded-lg !border !border-admin-border !bg-white hover:!bg-admin-surface-muted transition-colors flex items-center justify-center p-0'
const storageCardTagClass = '!px-2 !py-0.5 !text-[11px] !leading-4'

function getStorageStatusSeverity(status: string) {
  switch (status) {
    case 'is-current':
      return 'success'
    case 'is-enabled':
      return 'info'
    case 'is-empty':
      return 'warn'
    default:
      return 'secondary'
  }
}

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}
</script>
