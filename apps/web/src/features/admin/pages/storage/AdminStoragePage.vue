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
            @click="openCreate"
          >
            <template #icon>
              <Plus class="size-4" />
            </template>
          </Button>
        </template>
      </AdminResourceToolbar>
    </template>

    <Card
      v-for="card in filteredStorageCards"
      :key="card.storage.id"
      :class="adminMasonryCardClass"
      :pt="adminCardPt"
    >
      <template #title>
        <div class="flex items-start gap-3">
          <div class="flex min-w-0 items-center gap-3">
            <Avatar
              :image="card.vendorLogo"
              :label="card.avatarLabel"
              shape="circle"
              :class="[
                '!h-10 !w-10 !shrink-0 !text-sm !font-bold',
                card.vendorLogo || card.vendorIcon
                  ? '!bg-admin-surface-muted !p-1 !text-admin-text-2'
                  : '!bg-admin-accent !text-admin-text-inverse',
              ]"
            >
              <template v-if="card.vendorIcon" #icon>
                <component :is="card.vendorIcon" class="size-5" />
              </template>
            </Avatar>
            <div class="min-w-0">
              <div class="truncate text-admin-title-sm font-bold text-admin-title-color">
                {{ card.storage.name }}
              </div>
              <div
                v-if="card.subtitle"
                class="truncate text-admin-meta font-medium text-admin-subtitle-color"
              >
                {{ card.subtitle }}
              </div>
            </div>
          </div>
        </div>
      </template>

      <template #content>
        <div class="flex h-full min-h-[148px] flex-col gap-3.5">
          <div class="flex flex-wrap items-center gap-1.5">
            <Tag
              :value="card.providerLabel"
              severity="info"
              :class="adminCompactTagClass"
              rounded
            />
            <Tag
              :value="card.statusText"
              :severity="card.statusSeverity"
              :class="adminCompactTagClass"
              rounded
            />
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div
              v-for="item in card.metaItems"
              :key="`${card.storage.id}-${item.label}`"
              class="flex min-w-0 flex-col gap-1"
            >
              <span class="text-admin-caption text-admin-text-3">{{ item.label }}</span>
              <span class="break-all text-admin-body leading-admin-body text-admin-text-1">
                {{ item.value }}
              </span>
            </div>
          </div>

          <div class="mt-auto flex flex-wrap gap-2 max-sm:flex-col max-sm:items-stretch">
            <Button
              size="small"
              label="编辑"
              :class="adminActionButtonClass"
              severity="contrast"
              @click="openEdit(card.storage.id)"
            />
            <Button
              v-if="card.supportsTest"
              size="small"
              label="测试连接"
              :class="adminActionButtonClass"
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
              :class="adminActionButtonClass"
              severity="contrast"
              :loading="applyingId === card.storage.id"
              :disabled="!card.canApply || applyingId !== null"
              @click="applyStorage(card.storage)"
            />
            <Button
              v-if="!card.storage.isCurrent && !card.storage.builtin"
              size="small"
              label="删除"
              :class="adminActionButtonClass"
              severity="contrast"
              outlined
              @click="requestRemoveProfile(card.storage)"
            />
          </div>
        </div>
      </template>
    </Card>
  </AdminResourceGridFrame>

  <AdminDialogShell
    :visible="editorOpen"
    :aria-label="editorMode === 'create' ? '添加存储' : '编辑存储'"
    size="wide"
    content-layout="scrollable"
    :busy="saving || savingAndApplying"
    @update:visible="handleEditorDialogVisibleUpdate"
  >
    <template #header>
      <div :class="adminDialogHeaderSplitClass">
        <div :class="[adminDialogHeaderContentClass, 'flex-1']">
          <div class="mb-3 flex flex-wrap gap-2">
            <Tag :value="form.isCurrent ? '当前存储' : '存储配置'" severity="secondary" rounded />
          </div>
          <h2 :class="adminDialogTitleClass">
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

    <Tabs value="basic" class="min-w-0">
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
                <label class="text-admin-meta font-[680] text-admin-text-2">供应商</label>
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
                        :label="option.vendor.logoSrc ? undefined : option.vendor.label.slice(0, 1)"
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
                  <ToggleSwitch v-model="form.enabled" aria-label="存储启用状态" />
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
                  v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
                  text
                  size="tiny"
                  label="自动获取"
                  :class="adminTextButtonClass"
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
                  :class="adminTextButtonClass"
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
        label="保存"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :loading="saving"
        @click="saveDraft"
      />
      <Button
        :label="form.isCurrent ? '保存当前配置' : '保存并设为当前'"
        :class="adminActionButtonClass"
        severity="contrast"
        :loading="savingAndApplying"
        @click="saveAndApply"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { Plus, Refresh } from '@iconoir/vue'
import Avatar from 'primevue/avatar'
import Button from 'primevue/button'
import Card from 'primevue/card'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tab from 'primevue/tab'
import TabList from 'primevue/tablist'
import TabPanel from 'primevue/tabpanel'
import TabPanels from 'primevue/tabpanels'
import Tabs from 'primevue/tabs'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminResourceGridFrame from '../../shared/components/AdminResourceGridFrame.vue'
import AdminResourceToolbar from '../../shared/components/AdminResourceToolbar.vue'
import AdminToolbarSearch from '../../shared/components/AdminToolbarSearch.vue'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import {
  adminActionButtonClass,
  adminCompactTagClass,
  adminDialogHeaderContentClass,
  adminDialogHeaderSplitClass,
  adminDialogTitleClass,
  adminMasonryCardClass,
  adminTextButtonClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminStorageContext } from '../../storage/useAdminStorageContext'
import { useAdminConfirm } from '../../shared/composables/useAdminConfirm'

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
  activeVendorPreset,
  vendorSelectOptions,
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

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}

onMounted(() => {
  void loadData()
})
</script>
