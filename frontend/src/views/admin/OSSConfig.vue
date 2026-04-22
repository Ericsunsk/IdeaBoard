<template>
  <div class="admin-page storage-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">存储</h2>
      </div>
      <div class="header-actions">
        <n-button type="primary" :disabled="creatableProviders.length === 0" @click="openCreate">
          添加存储
        </n-button>
      </div>
    </div>

    <div class="storage-grid">
      <article v-for="storage in storages" :key="storage.id" class="storage-card" :class="{ current: storage.isCurrent }">
        <AdminCardHeader
          :provider="providerLabel(storage.provider)"
          :status="statusText(storage)"
          :status-variant="toChipVariant(statusClass(storage))"
          :title="storage.name"
          :subtitle="storageSubtitle(storage)"
        />

        <div class="storage-meta-grid">
          <div v-for="item in storageMetaItems(storage)" :key="`${storage.id}-${item.label}`" class="meta-item">
            <span class="meta-label">{{ item.label }}</span>
            <span class="meta-value">{{ item.value }}</span>
          </div>
        </div>

        <div class="feature-chips">
          <AdminChip v-for="chip in storageFeatureChips(storage)" :key="`${storage.id}-${chip}`" :label="chip" />
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
          <n-button v-if="!storage.isCurrent && !storage.builtin" size="small" tertiary type="error" @click="removeProfile(storage.id)">
            删除
          </n-button>
        </div>
      </article>

      <button class="storage-add-card" type="button" :disabled="creatableProviders.length === 0" @click="openCreate">
        <span class="add-mark">+</span>
        <span class="add-title">添加存储</span>
        <span class="add-desc">先选供应商，再创建新的存储卡片。保存后可直接切换为当前上传链路。</span>
      </button>
    </div>
  </div>

  <AdminEditorModal
    :show="editorOpen"
    :eyebrow="form.isCurrent ? '当前存储' : '存储配置'"
    :title="editorMode === 'create' ? '添加存储' : '编辑存储'"
    :description="editorMode === 'create'
      ? ''
      : selectedProviderMeta?.description || '维护上传存储配置，并切换系统当前生效的上传链路。'"
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
            :render-option="renderVendorOption"
            placeholder="选择供应商"
            @update:value="handleVendorSelect"
          />
        </div>
        <div v-if="activeVendorPreset" class="editor-vendor-preview">
          <img :src="activeVendorPreset.logoSrc" :alt="activeVendorPreset.label" class="vendor-preview-logo" />
          <div class="editor-vendor-copy">
            <div class="editor-vendor-row">
              <span class="editor-vendor-name">{{ activeVendorPreset.label }}</span>
              <span class="editor-provider-status" :class="{ supported: activeVendorPreset.supported }">
                {{ activeVendorPreset.supported ? '已接入' : '即将支持' }}
              </span>
            </div>
            <p class="editor-desc-tight">{{ activeVendorPreset.description }}</p>
          </div>
        </div>
      </div>
    </template>

    <template #header-aside>
      <div v-if="editorMode !== 'create'" class="editor-provider">{{ providerLabel(form.provider) }}</div>
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
        <div class="field-row">
          <div v-for="field in textFields" :key="field.key" class="field">
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
              :placeholder="field.secret && form.secretConfigured && !configTextValue(field.key)
                ? '已配置，留空则保持不变'
                : field.placeholder || `请输入${field.label}`"
              @update:value="updateConfigValue(field.key, $event)"
            />
            <span v-if="field.hint" class="field-hint">{{ field.hint }}</span>
            <div v-if="field.key === 'access_domain' && bucketDomains.length > 0" class="domain-list">
              <span class="domain-label">检测到的域名：</span>
              <span
                v-for="domain in bucketDomains"
                :key="domain"
                class="domain-tag"
                @click="updateConfigValue(field.key, `https://${domain}`)"
              >
                {{ domain }}
              </span>
            </div>
          </div>
        </div>
      </div>
      <div v-else class="empty-config-note">
        当前存储类型无需额外密钥配置，保存后即可切换使用。
      </div>
    </div>

    <div v-if="switchFields.length > 0" class="editor-section">
      <div class="section-title">高级选项</div>
      <div class="toggle-grid">
        <div v-for="field in switchFields" :key="field.key" class="toggle-card">
          <div>
            <div class="toggle-title">{{ field.label }}</div>
            <div v-if="field.hint" class="toggle-desc">{{ field.hint }}</div>
          </div>
          <n-switch :value="configBoolValue(field.key)" @update:value="updateConfigValue(field.key, $event)" />
        </div>
      </div>
    </div>

    <template #footer-note>
      {{ form.isCurrent
        ? '保存将直接更新系统当前使用中的上传存储。'
        : '保存只更新这张卡片；点击“保存并设为当前”后才会切换系统上传链路。' }}
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
import { NButton, NInput, NSelect, NSwitch } from 'naive-ui'
import AdminEditorModal from './components/AdminEditorModal.vue'
import AdminCardHeader from './components/AdminCardHeader.vue'
import AdminChip from './components/AdminChip.vue'
import { useStorageManager } from './useStorageManager'

const {
  saving,
  savingAndApplying,
  fetching,
  applyingId,
  testingId,
  editorOpen,
  editorMode,
  editingId,
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
  renderVendorOption,
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
</script>

<style scoped>
@import './admin-common.css';

.storage-page {
  gap: 14px;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.storage-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.storage-card,
.storage-add-card {
  min-height: 252px;
  border-radius: 18px;
  border: 1px solid var(--admin-border);
  background: var(--admin-card);
  padding: 18px;
}

.storage-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  transition: border-color 0.18s ease, background 0.18s ease, transform 0.18s ease;
}

.storage-card:hover,
.storage-add-card:hover {
  border-color: #2e3034;
  background: #141416;
}

.storage-card.current {
  border-color: rgba(244, 244, 245, 0.24);
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
  font-size: 11px;
}

.meta-value {
  color: var(--admin-text-1);
  font-size: 13px;
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

.storage-add-card {
  appearance: none;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: flex-start;
  gap: 10px;
  text-align: left;
  transition: border-color 0.18s ease, background 0.18s ease, transform 0.18s ease;
}

.storage-add-card:hover {
  transform: translateY(-1px);
}

.storage-add-card:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}

.add-mark {
  width: 42px;
  height: 42px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 12px;
  background: #17181a;
  border: 1px solid var(--admin-border);
  color: var(--admin-text-1);
  font-size: 24px;
  line-height: 1;
}

.add-title {
  color: var(--admin-text-1);
  font-size: 18px;
  font-weight: 600;
}

.add-desc {
  color: var(--admin-text-2);
  font-size: 13px;
  line-height: 1.6;
}

.editor-provider {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 0 10px;
  border-radius: 999px;
  background: #17181a;
  border: 1px solid var(--admin-border);
  color: var(--admin-text-2);
  font-size: 11px;
}

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
  font-size: 12px;
}

.editor-provider-status {
  min-height: 24px;
  padding: 0 8px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: #1b1c1f;
  color: var(--admin-text-3);
  font-size: 11px;
  border: 1px solid var(--admin-border);
}

.editor-provider-status.supported {
  background: rgba(16, 185, 129, 0.12);
  border-color: rgba(16, 185, 129, 0.24);
  color: #d1fae5;
}

.editor-vendor-preview {
  border-radius: 16px;
  border: 1px solid var(--admin-border);
  background: #151618;
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
  background: #ffffff;
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
  color: var(--admin-text-1);
  font-size: 14px;
  font-weight: 600;
}

.editor-desc-tight {
  margin-top: 4px;
  font-size: 13px;
  line-height: 1.55;
}

:deep(.n-base-selection) {
  min-height: 48px;
}

:deep(.vendor-select-option) {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

:deep(.vendor-select-logo) {
  width: 28px;
  height: 28px;
  flex: 0 0 28px;
  object-fit: contain;
  border-radius: 8px;
  background: #ffffff;
  padding: 4px;
}

:deep(.vendor-select-copy) {
  min-width: 0;
  flex: 1;
  display: grid;
  gap: 2px;
}

:deep(.vendor-select-row) {
  display: flex;
  align-items: center;
  gap: 8px;
}

:deep(.vendor-select-name) {
  color: var(--admin-text-1);
  font-size: 13px;
  font-weight: 600;
}

:deep(.vendor-select-status) {
  min-height: 20px;
  padding: 0 6px;
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  background: #1b1c1f;
  color: var(--admin-text-3);
  font-size: 10px;
  border: 1px solid var(--admin-border);
}

:deep(.vendor-select-status.is-supported) {
  background: rgba(16, 185, 129, 0.12);
  border-color: rgba(16, 185, 129, 0.24);
  color: #d1fae5;
}

:deep(.vendor-select-desc) {
  color: var(--admin-text-3);
  font-size: 11px;
  line-height: 1.5;
}

.editor-section {
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  padding-top: 16px;
  margin-top: 16px;
}

.section-title {
  color: var(--admin-text-1);
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 12px;
}

.switch-inline {
  min-height: 40px;
  padding: 0 12px;
  border-radius: 12px;
  border: 1px solid var(--admin-border);
  background: #151618;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.switch-text {
  color: var(--admin-text-2);
  font-size: 13px;
}

.toggle-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.toggle-card {
  border-radius: 14px;
  border: 1px solid var(--admin-border);
  background: #151618;
  padding: 14px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.toggle-title {
  color: var(--admin-text-1);
  font-size: 13px;
  font-weight: 600;
}

.toggle-desc {
  color: var(--admin-text-3);
  font-size: 12px;
  line-height: 1.6;
  margin-top: 4px;
}

.field-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.empty-config-note {
  min-height: 84px;
  border-radius: 14px;
  border: 1px dashed var(--admin-border);
  background: #151618;
  color: var(--admin-text-2);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 16px;
  text-align: center;
  font-size: 13px;
  line-height: 1.7;
}

.domain-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.domain-label {
  font-size: 11px;
  color: var(--admin-text-3);
}

.domain-tag {
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(59, 130, 246, 0.12);
  color: #bfdbfe;
  cursor: pointer;
}

@media (max-width: 980px) {
  .storage-grid {
    grid-template-columns: 1fr;
  }

  .storage-meta-grid,
  .toggle-grid {
    grid-template-columns: 1fr;
  }

  .editor-provider-stack {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .storage-card,
  .storage-add-card {
    min-height: unset;
  }

  .storage-card-actions {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
