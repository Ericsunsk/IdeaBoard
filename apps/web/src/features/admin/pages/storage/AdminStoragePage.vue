<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0">
    <div class="grid grid-cols-1 gap-3.5 lg:grid-cols-2">
      <Card
        v-for="storage in storages"
        :key="storage.id"
      >
        <template #content>
          <div class="flex h-full min-h-[148px] flex-col gap-3.5">
            <div class="min-w-0">
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <Tag :value="providerLabel(storage.provider)" severity="contrast" rounded />
                <Tag
                  :value="statusText(storage)"
                  :severity="getStorageStatusSeverity(statusClass(storage))"
                  rounded
                />
              </div>
              <div class="truncate text-admin-heading font-bold text-admin-title-color">
                {{ storage.name }}
              </div>
              <div class="truncate text-admin-meta text-admin-subtitle-color">
                {{ storageSubtitle(storage) }}
              </div>
            </div>

            <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
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

            <div class="flex flex-wrap gap-2">
              <Tag
                v-for="chip in storageFeatureChips(storage)"
                :key="`${storage.id}-${chip}`"
                :value="chip"
                severity="secondary"
                rounded
              />
            </div>

            <div class="mt-auto flex flex-wrap gap-2 max-sm:flex-col max-sm:items-stretch">
              <Button
                size="small"
                label="编辑"
                severity="secondary"
                outlined
                @click="openEdit(storage.id)"
              />
              <Button
                v-if="supportsTest(storage)"
                size="small"
                label="测试连接"
                severity="secondary"
                outlined
                :loading="testingId === storage.id"
                :disabled="!canTest(storage)"
                @click="handleTest(storage)"
              />
              <Button
                v-if="!storage.isCurrent"
                size="small"
                label="设为当前"
                :loading="applyingId === storage.id"
                :disabled="!canApply(storage)"
                @click="applyStorage(storage.id)"
              />
              <Button
                v-if="!storage.isCurrent && !storage.builtin"
                size="small"
                label="删除"
                severity="danger"
                text
                @click="removeProfile(storage.id)"
              />
            </div>
          </div>
        </template>
      </Card>

      <Card>
        <template #content>
          <Button
            severity="secondary"
            outlined
            class="!flex !min-h-[148px] !w-full !flex-col !items-center !justify-center !gap-2.5"
            :disabled="creatableProviders.length === 0"
            @click="openCreate"
          >
            <template #icon>
              <Plus />
            </template>
            <span>添加存储供应商</span>
          </Button>
        </template>
      </Card>
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
          <p
            v-if="editorMode !== 'create'"
            class="mt-2.5 mb-0 text-admin-content leading-7 text-admin-subtitle-color"
          >
            {{
              selectedProviderMeta?.description ||
              '维护上传存储配置，并切换系统当前生效的上传链路。'
            }}
          </p>

          <div
            v-if="editorMode === 'create'"
            class="mt-3.5 grid items-end gap-3 md:grid-cols-[minmax(280px,360px)_minmax(0,1fr)]"
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
                    <div
                      class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[10px] bg-gradient-to-b from-[#fcfcfd] to-[#ededf0] shadow-[inset_0_0_0_1px_rgba(17,17,17,0.06)]"
                    >
                      <img
                        :src="activeVendorPreset.logoSrc"
                        :alt="activeVendorPreset.label"
                        class="h-[18px] w-[18px] object-contain"
                      />
                    </div>
                    <span class="truncate text-admin-content font-semibold text-admin-title-color">
                      {{ activeVendorPreset.label }}
                    </span>
                  </div>
                  <span v-else class="text-admin-text-3">选择供应商</span>
                </template>
                <template #option="{ option }">
                  <div class="flex min-w-0 items-center gap-2">
                    <div
                      class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-[10px] bg-gradient-to-b from-[#fcfcfd] to-[#ededf0] shadow-[inset_0_0_0_1px_rgba(17,17,17,0.06)]"
                    >
                      <img
                        :src="option.vendor.logoSrc"
                        :alt="option.vendor.label"
                        class="h-[18px] w-[18px] object-contain"
                      />
                    </div>
                    <span class="truncate text-admin-body font-semibold text-admin-title-color">
                      {{ option.vendor.label }}
                    </span>
                  </div>
                </template>
              </Select>
            </div>
            <div
              v-if="activeVendorPreset"
              class="flex min-h-[68px] items-center gap-3.5 rounded-2xl border border-admin-border bg-admin-surface-muted px-3.5 py-3"
            >
              <img
                :src="activeVendorPreset.logoSrc"
                :alt="activeVendorPreset.label"
                class="h-9 w-9 shrink-0 rounded-[10px] bg-admin-card object-contain p-1.5"
              />
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2.5">
                  <span class="text-admin-content font-semibold text-admin-title-color">
                    {{ activeVendorPreset.label }}
                  </span>
                </div>
                <p class="mt-1 text-admin-body leading-[1.55] text-admin-subtitle-color">
                  {{ activeVendorPreset.description }}
                </p>
              </div>
            </div>
          </div>
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

    <div class="mt-4 border-t border-admin-divider pt-4">
      <div class="flex flex-col gap-[14px]">
        <div class="flex flex-col gap-3 md:flex-row">
          <div class="flex flex-1 flex-col gap-1.5">
            <label class="text-admin-meta font-[680] text-admin-text-2">存储名称</label>
            <InputText v-model="form.name" placeholder="例如：生产存储 / 备用存储" />
          </div>
          <div class="flex flex-1 flex-col gap-1.5">
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
    </div>

    <div class="mt-4 border-t border-admin-divider pt-4">
      <div class="mb-3 text-admin-body font-semibold text-admin-title-color">基础配置</div>
      <div v-if="textFields.length > 0" class="flex flex-col gap-[14px]">
        <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <div
            v-for="field in textFields"
            :key="field.key"
            class="flex min-w-0 flex-1 flex-col gap-1.5"
          >
            <div class="flex items-center justify-between gap-2">
              <label class="text-admin-meta font-[680] text-admin-text-2">{{ field.label }}</label>
              <Button
                v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
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
              <span class="text-admin-caption text-admin-text-3">检测到的域名：</span>
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
      </div>
      <Message v-else severity="secondary" :closable="false">
        当前存储类型无需额外密钥配置，保存后即可切换使用。
      </Message>
    </div>

    <div v-if="switchFields.length > 0" class="mt-4 border-t border-admin-divider pt-4">
      <div class="mb-3 text-admin-body font-semibold text-admin-title-color">高级选项</div>
      <div class="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <div
          v-for="field in switchFields"
          :key="field.key"
          class="flex items-center justify-between gap-3.5 rounded-[14px] border border-admin-border bg-admin-surface-muted px-4 py-3.5"
        >
          <div>
            <div class="text-admin-body font-semibold text-admin-title-color">{{ field.label }}</div>
            <div v-if="field.hint" class="mt-1 text-admin-meta leading-[1.6] text-admin-subtitle-color">
              {{ field.hint }}
            </div>
          </div>
          <ToggleSwitch
            :value="configBoolValue(field.key)"
            @update:model-value="updateConfigValue(field.key, Boolean($event))"
          />
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex w-full flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div class="max-w-[480px] text-admin-meta leading-6 text-admin-text-3">
          {{
            form.isCurrent
              ? '保存将直接更新系统当前使用中的上传存储。'
              : '保存只更新这张卡片；点击“保存并设为当前”后才会切换系统上传链路。'
          }}
        </div>
        <div class="flex flex-wrap justify-end gap-2">
          <Button label="取消" severity="secondary" outlined @click="closeEditor" />
          <Button label="仅保存卡片" severity="secondary" :loading="saving" @click="saveDraft" />
          <Button
            :label="form.isCurrent ? '保存当前配置' : '保存并设为当前'"
            :loading="savingAndApplying"
            @click="saveAndApply"
          />
        </div>
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { Plus } from '@iconoir/vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useStorageManager } from './useStorageManager'

const {
  saving,
  savingAndApplying,
  fetching,
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
} = useStorageManager()

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
