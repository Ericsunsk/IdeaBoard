<template>
  <div class="admin-page storage-page">
    <div class="admin-card-grid is-2">
      <article
        v-for="storage in storages"
        :key="storage.id"
        class="storage-card admin-card-surface admin-card-interactive"
      >
        <AdminCardHeader
          :provider="providerLabel(storage.provider)"
          :status="statusText(storage)"
          :status-variant="toChipVariant(statusClass(storage))"
          :title="storage.name"
          :subtitle="storageSubtitle(storage)"
        />

        <div class="storage-meta-grid">
          <div
            v-for="item in storageMetaItems(storage)"
            :key="`${storage.id}-${item.label}`"
            class="meta-item"
          >
            <span class="meta-label">{{ item.label }}</span>
            <span class="meta-value">{{ item.value }}</span>
          </div>
        </div>

        <div class="feature-chips">
          <AdminChip
            v-for="chip in storageFeatureChips(storage)"
            :key="`${storage.id}-${chip}`"
            :label="chip"
          />
        </div>

        <div class="storage-card-actions">
          <n-button size="small" @click="openEdit(storage.id)">编辑</n-button>
          <n-button
            v-if="supportsTest(storage)"
            size="small"
            :loading="testingId === storage.id"
            :disabled="!canTest(storage)"
            @click="handleTest(storage)"
          >
            测试连接
          </n-button>
          <n-button
            v-if="!storage.isCurrent"
            size="small"
            type="primary"
            :loading="applyingId === storage.id"
            :disabled="!canApply(storage)"
            @click="applyStorage(storage.id)"
          >
            设为当前
          </n-button>
          <n-button
            v-if="!storage.isCurrent && !storage.builtin"
            size="small"
            tertiary
            type="error"
            @click="removeProfile(storage.id)"
          >
            删除
          </n-button>
        </div>
      </article>

      <button
        class="admin-add-card"
        type="button"
        :disabled="creatableProviders.length === 0"
        @click="openCreate"
      >
        <n-icon class="add-mark"><Plus /></n-icon>
        <span class="add-title">添加存储供应商</span>
      </button>
    </div>
  </div>

  <AdminEditorModal
    :show="editorOpen"
    :eyebrow="form.isCurrent ? '当前存储' : '存储配置'"
    :title="editorMode === 'create' ? '添加存储' : '编辑存储'"
    :description="
      editorMode === 'create'
        ? ''
        : selectedProviderMeta?.description || '维护上传存储配置，并切换系统当前生效的上传链路。'
    "
    :width="920"
    @close="closeEditor"
  >
    <template #header-copy-extra>
      <div v-if="editorMode === 'create'" class="editor-provider-stack">
        <div class="field vendor-select-field">
          <div class="field-label-row">
            <label>供应商</label>
            <span class="field-hint-inline">仅已接入供应商可直接创建</span>
          </div>
          <n-select
            :value="selectedVendorKey"
            :options="vendorSelectOptions"
            :render-label="renderVendorLabel"
            :menu-props="{ class: 'admin-storage-vendor-menu' }"
            :show-checkmark="false"
            :theme-overrides="selectThemeOverrides"
            placeholder="选择供应商"
            @update:value="handleVendorSelect"
          />
        </div>
        <div v-if="activeVendorPreset" class="editor-vendor-preview">
          <img
            :src="activeVendorPreset.logoSrc"
            :alt="activeVendorPreset.label"
            class="vendor-preview-logo"
          />
          <div class="editor-vendor-copy">
            <div class="editor-vendor-row">
              <span class="editor-vendor-name">{{ activeVendorPreset.label }}</span>
            </div>
            <p class="editor-desc-tight">
              {{ activeVendorPreset.description }}
            </p>
          </div>
        </div>
      </div>
    </template>

    <template #header-aside>
      <div v-if="editorMode !== 'create'" class="admin-pill is-compact">
        {{ providerLabel(form.provider) }}
      </div>
    </template>

    <div class="editor-section">
      <div class="field-group">
        <div class="field-row">
          <div class="field">
            <label>存储名称</label>
            <n-input v-model:value="form.name" placeholder="例如：生产存储 / 备用存储" />
          </div>
          <div class="field">
            <label>启用状态</label>
            <div class="switch-inline">
              <span class="switch-text">{{ form.enabled ? '已启用' : '已关闭' }}</span>
              <n-switch v-model:value="form.enabled" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="editor-section">
      <div class="section-title">基础配置</div>
      <div v-if="textFields.length > 0" class="field-group">
        <div class="field-row config-field-row">
          <div
            v-for="field in textFields"
            :key="field.key"
            :class="['field', { 'field-region': field.key === 'region_id' }]"
          >
            <div class="field-label-row">
              <label>{{ field.label }}</label>
              <n-button
                v-if="field.key === 'access_domain' && selectedProviderMeta?.supports_bucket_info"
                text
                type="primary"
                size="tiny"
                :loading="fetching"
                @click="fetchBucketDomains"
              >
                自动获取
              </n-button>
            </div>
            <n-input
              :value="configTextValue(field.key)"
              :type="field.type === 'password' ? 'password' : 'text'"
              :show-password-on="field.type === 'password' ? 'click' : undefined"
              :placeholder="
                field.secret && form.secretConfigured && !configTextValue(field.key)
                  ? '已配置，留空则保持不变'
                  : field.placeholder || `请输入${field.label}`
              "
              @update:value="updateConfigValue(field.key, $event)"
            />
            <span v-if="field.hint" class="field-hint">{{ field.hint }}</span>
            <div
              v-if="field.key === 'access_domain' && bucketDomains.length > 0"
              class="domain-list"
            >
              <span class="domain-label">检测到的域名：</span>
              <span
                v-for="domain in bucketDomains"
                :key="domain"
                class="admin-pill is-clickable"
                @click="updateConfigValue(field.key, `https://${domain}`)"
              >
                {{ domain }}
              </span>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="empty-config-note">当前存储类型无需额外密钥配置，保存后即可切换使用。</div>
    </div>

    <div v-if="switchFields.length > 0" class="editor-section">
      <div class="section-title">高级选项</div>
      <div class="toggle-grid">
        <div v-for="field in switchFields" :key="field.key" class="toggle-card">
          <div>
            <div class="toggle-title">{{ field.label }}</div>
            <div v-if="field.hint" class="toggle-desc">
              {{ field.hint }}
            </div>
          </div>
          <n-switch
            :value="configBoolValue(field.key)"
            @update:value="updateConfigValue(field.key, $event)"
          />
        </div>
      </div>
    </div>

    <template #footer-note>
      {{
        form.isCurrent
          ? '保存将直接更新系统当前使用中的上传存储。'
          : '保存只更新这张卡片；点击“保存并设为当前”后才会切换系统上传链路。'
      }}
    </template>

    <template #footer-actions>
      <n-button @click="closeEditor">取消</n-button>
      <n-button :loading="saving" @click="saveDraft">仅保存卡片</n-button>
      <n-button type="primary" :loading="savingAndApplying" @click="saveAndApply">
        {{ form.isCurrent ? '保存当前配置' : '保存并设为当前' }}
      </n-button>
    </template>
  </AdminEditorModal>
</template>

<script setup lang="ts">
import { Plus } from '@iconoir/vue'
import { type GlobalThemeOverrides } from 'naive-ui'
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
  toChipVariant,
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
  renderVendorLabel,
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

const selectThemeOverrides: NonNullable<GlobalThemeOverrides['Select']> = {
  peers: {
    InternalSelection: {
      heightMedium: '60px',
      borderRadius: '12px',
      color: 'var(--admin-card)',
      colorActive: 'var(--admin-card)',
      border: '1px solid var(--admin-border)',
      borderHover: '1px solid var(--admin-border)',
      borderActive: '1px solid var(--admin-border)',
      borderFocus: '1px solid var(--admin-border)',
      boxShadowHover: 'none',
      boxShadowActive: 'none',
      boxShadowFocus: 'none',
      textColor: 'var(--admin-text-1)',
      placeholderColor: 'var(--admin-text-3)',
      arrowColor: 'var(--admin-text-3)',
      paddingSingle: '0 14px',
    },
  },
}
</script>

<style scoped>
@import '../../shared/styles/adminCommon.css';

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.storage-card {
  min-height: 180px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.storage-meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.meta-item {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.meta-label {
  color: var(--admin-text-3);
  font-size: var(--admin-font-size-caption);
}

.meta-value {
  color: var(--admin-text-1);
  font-size: var(--admin-font-size-body);
  line-height: 1.5;
  word-break: break-all;
}

.feature-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.storage-card-actions {
  margin-top: auto;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

/* Add card styles now global */

.editor-provider-stack {
  margin-top: 14px;
  display: grid;
  grid-template-columns: minmax(280px, 360px) minmax(0, 1fr);
  gap: 12px 16px;
  align-items: end;
}

.vendor-select-field {
  min-width: 0;
}

.field-hint-inline {
  color: var(--admin-text-3);
  font-size: var(--admin-font-size-meta);
}

.editor-vendor-preview {
  border-radius: 16px;
  border: 1px solid var(--admin-border);
  background: var(--admin-surface-muted);
  min-height: 68px;
  padding: 12px 14px;
  display: flex;
  align-items: center;
  gap: 14px;
}

.vendor-preview-logo {
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  object-fit: contain;
  border-radius: 10px;
  background: var(--admin-card);
  padding: 5px;
}

.editor-vendor-copy {
  min-width: 0;
  flex: 1;
}

.editor-vendor-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.editor-vendor-name {
  color: var(--admin-title-color);
  font-size: var(--admin-font-size-content);
  font-weight: 600;
}

.editor-desc-tight {
  margin-top: 4px;
  color: var(--admin-subtitle-color);
  font-size: var(--admin-font-size-body);
  line-height: 1.55;
}

:deep(.vendor-select-entry) {
  min-width: 0;
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
}

:deep(.vendor-select-entry-brand) {
  width: 34px;
  height: 34px;
  flex: 0 0 34px;
  border-radius: 11px;
  background: linear-gradient(180deg, #fcfcfd 0%, #ededf0 100%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-shadow: inset 0 0 0 1px rgba(17, 17, 17, 0.06);
}

:deep(.vendor-select-entry-logo) {
  width: 22px;
  height: 22px;
  object-fit: contain;
}

:deep(.vendor-select-entry-copy) {
  min-width: 0;
  flex: 1;
}

:deep(.vendor-select-entry-top) {
  min-width: 0;
  display: flex;
  align-items: center;
}

:deep(.vendor-select-entry-name) {
  display: block;
  color: var(--admin-title-color);
  font-size: var(--admin-font-size-content);
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

:deep(.vendor-select-entry-family) {
  display: none;
}

.editor-section {
  border-top: 1px solid var(--admin-divider);
  padding-top: 16px;
  margin-top: 16px;
}

.section-title {
  color: var(--admin-title-color);
  font-size: var(--admin-font-size-body);
  font-weight: 600;
  margin-bottom: 12px;
}

.switch-inline {
  min-height: 40px;
  padding: 0 12px;
  border-radius: 12px;
  border: 1px solid var(--admin-border);
  background: var(--admin-surface-muted);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.switch-text {
  color: var(--admin-text-2);
  font-size: var(--admin-font-size-body);
}

.toggle-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.toggle-card {
  border-radius: 14px;
  border: 1px solid var(--admin-border);
  background: var(--admin-surface-muted);
  padding: 14px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.toggle-title {
  color: var(--admin-title-color);
  font-size: var(--admin-font-size-body);
  font-weight: 600;
}

.toggle-desc {
  color: var(--admin-subtitle-color);
  font-size: var(--admin-font-size-meta);
  line-height: 1.6;
  margin-top: 4px;
}

.field-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.config-field-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.config-field-row > .field {
  min-width: 0;
}

.config-field-row .field-region :deep(.n-input) {
  width: 100%;
  max-width: 220px;
}

.empty-config-note {
  min-height: 84px;
  border-radius: 14px;
  border: 1px dashed var(--admin-border);
  background: var(--admin-surface-muted);
  color: var(--admin-text-2);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  text-align: center;
  font-size: var(--admin-font-size-body);
  line-height: 1.7;
}

.domain-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.domain-label {
  font-size: var(--admin-font-size-caption);
  color: var(--admin-text-3);
}

@media (max-width: 980px) {
  .storage-meta-grid,
  .toggle-grid {
    grid-template-columns: 1fr;
  }

  .editor-provider-stack {
    grid-template-columns: 1fr;
  }

  .config-field-row {
    grid-template-columns: 1fr;
  }

  .config-field-row .field-region :deep(.n-input) {
    max-width: none;
  }
}

@media (max-width: 720px) {
  .storage-card {
    min-height: unset;
  }

  .storage-card-actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>

<style>
.admin-storage-vendor-menu {
  --n-height: min(360px, calc(100vh - 160px));
  padding: 10px;
  border-radius: 20px;
  border: 1px solid var(--admin-border);
  background: var(--admin-card);
  box-shadow: 0 16px 36px rgba(15, 23, 42, 0.22);
}

.admin-storage-vendor-menu .n-base-select-option {
  min-height: auto;
  padding: 0;
  background: transparent;
  color: inherit;
}

.admin-storage-vendor-menu .n-base-select-option::before {
  display: none;
}

.admin-storage-vendor-menu .n-base-select-option + .n-base-select-option {
  margin-top: 6px;
}

.admin-storage-vendor-menu .n-base-select-option__content {
  width: 100%;
  overflow: visible;
}

.admin-storage-vendor-menu .n-base-select-option:hover,
.admin-storage-vendor-menu .n-base-select-option.n-base-select-option--selected {
  color: inherit;
}

.admin-storage-vendor-menu .n-base-select-option--disabled {
  opacity: 1;
}

.admin-storage-vendor-menu .n-scrollbar-rail--vertical,
.admin-storage-vendor-menu .n-scrollbar-rail--horizontal {
  display: none !important;
}

.admin-storage-vendor-menu .vendor-select-entry {
  width: 100%;
  min-width: 0;
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  align-items: center;
  gap: 10px;
  padding: 6px 10px;
  border-radius: 14px;
  border: 1px solid var(--admin-border);
  background: var(--admin-surface-muted);
  transition:
    border-color 0.18s ease,
    background 0.18s ease,
    box-shadow 0.18s ease;
}

.admin-storage-vendor-menu
  .n-base-select-option:not(.n-base-select-option--disabled):hover
  .vendor-select-entry,
.admin-storage-vendor-menu .n-base-select-option--pending .vendor-select-entry {
  border-color: var(--admin-border);
  background: var(--admin-card);
  box-shadow: inset 0 0 0 1px var(--admin-border-soft);
}

.admin-storage-vendor-menu .n-base-select-option--selected .vendor-select-entry {
  border-color: var(--admin-border);
  background: var(--admin-card-strong);
  box-shadow: inset 0 0 0 1px var(--admin-border-soft);
}

.admin-storage-vendor-menu .n-base-select-option--disabled .vendor-select-entry {
  opacity: 0.64;
}

.admin-storage-vendor-menu .vendor-select-entry-brand {
  width: 32px;
  height: 32px;
  border-radius: 11px;
  background: linear-gradient(180deg, #fcfcfd 0%, #ededf0 100%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-shadow: inset 0 0 0 1px rgba(17, 17, 17, 0.06);
}

.admin-storage-vendor-menu .vendor-select-entry-logo {
  width: 20px;
  height: 20px;
  object-fit: contain;
}

.admin-storage-vendor-menu .vendor-select-entry-copy {
  min-width: 0;
}

.admin-storage-vendor-menu .vendor-select-entry-top {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: nowrap;
}

.admin-storage-vendor-menu .vendor-select-entry-name {
  color: var(--admin-title-color);
  min-width: 0;
  font-size: var(--admin-font-size-body);
  font-weight: 600;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.admin-storage-vendor-menu .vendor-select-entry-family {
  min-height: 18px;
  padding: 0 6px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: var(--admin-surface-muted);
  color: var(--admin-text-3);
  font-size: var(--admin-font-size-micro);
  line-height: 1;
  letter-spacing: 0.01em;
  flex: 0 0 auto;
}
</style>
