<template>
  <div class="admin-page image-processing-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">{{ copy.pageTitle }}</h2>
        <p class="page-desc">{{ copy.pageDesc }}</p>
      </div>
    </div>

    <div class="processor-grid">
      <article class="processor-card" :class="{ current: currentCard.enabled && isConfigured }">
        <AdminCardHeader
          :provider="copy.cardProvider"
          :status="statusText"
          :status-variant="toChipVariant(statusClass)"
          :title="copy.cardTitle"
          :subtitle="copy.cardSubtitle"
        />

        <div class="processor-meta-grid">
          <div v-for="item in cardMetaItems" :key="item.label" class="meta-item">
            <span class="meta-label">{{ item.label }}</span>
            <span class="meta-value">{{ item.value }}</span>
          </div>
        </div>

        <div class="feature-chips">
          <AdminChip v-for="chip in cardFeatureChips" :key="chip" :label="chip" />
        </div>

        <div class="processor-card-actions">
          <n-button size="small" type="primary" @click="openEditor">{{ copy.cardEditButton }}</n-button>
        </div>
      </article>
    </div>
  </div>

  <AdminEditorModal
    :show="editorOpen"
    :eyebrow="copy.editorEyebrow"
    :title="copy.editorTitle"
    :description="copy.editorDesc"
    width="860px"
    @close="closeEditor"
  >
    <template #header-aside>
      <div class="editor-provider">{{ copy.editorProvider }}</div>
    </template>

    <div class="editor-section">
      <div class="field-group">
        <div class="field-row">
          <div class="field">
            <label>{{ copy.labelEnabled }}</label>
            <div class="switch-inline">
              <span class="switch-text">{{ draft.enabled ? copy.enabledOn : copy.enabledOff }}</span>
              <n-switch v-model:value="draft.enabled" />
            </div>
          </div>
          <div class="field">
            <label>{{ copy.labelRegionId }}</label>
            <n-input v-model:value="draft.region_id" :placeholder="copy.regionPlaceholder" />
            <span class="field-hint">{{ copy.regionHint }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="editor-section">
      <div class="section-title">{{ copy.sectionSecret }}</div>
      <div class="field-group">
        <div class="field-row">
          <div class="field">
            <label>{{ copy.labelAccessKeyId }}</label>
            <n-input v-model:value="draft.access_key_id" :placeholder="copy.accessKeyPlaceholder" />
            <span class="field-hint">{{ copy.accessKeyHintPrefix }}<a href="https://ram.console.aliyun.com/manage/ak" target="_blank">{{ copy.accessKeyHintLink }}</a>{{ copy.accessKeyHintSuffix }}</span>
          </div>
          <div class="field">
            <label>{{ copy.labelAccessKeySecret }}</label>
            <n-input
              v-model:value="draft.access_key_secret"
              type="password"
              show-password-on="click"
              :placeholder="secretConfigured ? copy.secretPlaceholderConfigured : copy.secretPlaceholderEmpty"
            />
            <span class="field-hint">{{ copy.secretHintPrefix }}<a href="https://common-buy.aliyun.com/?commodityCode=viapi_imageseg_public_cn" target="_blank">{{ copy.secretHintLink }}</a>{{ copy.secretHintSuffix }}</span>
          </div>
        </div>
      </div>
    </div>

    <template #footer-note>
      {{ copy.editorNote }}
    </template>

    <template #footer-actions>
      <n-button @click="closeEditor">{{ copy.cancelButton }}</n-button>
      <n-button type="primary" :loading="saving" @click="handleSave">{{ copy.saveButton }}</n-button>
    </template>
  </AdminEditorModal>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NButton, NInput, NSwitch, useMessage } from 'naive-ui'
import { adminSegApi } from '../../api/index'
import AdminEditorModal from './components/AdminEditorModal.vue'
import AdminCardHeader from './components/AdminCardHeader.vue'
import AdminChip from './components/AdminChip.vue'
import {
  buildSegSavePayload,
  createDefaultSegForm,
  isSegConfigured,
  normalizeSegConfig,
  SEG_CONFIG_COPY,
  segFeatureChips,
  segMetaItems,
  segStatusClass,
  segStatusText,
  type SegConfigForm,
} from './segConfigMeta'

const message = useMessage()
const copy = SEG_CONFIG_COPY
const loading = ref(false)
const saving = ref(false)
const editorOpen = ref(false)
const secretConfigured = ref(false)

const currentCard = ref<SegConfigForm>(createDefaultSegForm())
const draft = ref<SegConfigForm>(createDefaultSegForm())

const isConfigured = computed(() => isSegConfigured(currentCard.value, secretConfigured.value))

const statusText = computed(() => segStatusText(currentCard.value.enabled, isConfigured.value))
const statusClass = computed(() => segStatusClass(currentCard.value.enabled, isConfigured.value))
const cardMetaItems = computed(() => segMetaItems(currentCard.value, secretConfigured.value))
const cardFeatureChips = computed(() => segFeatureChips(currentCard.value))

function toChipVariant(status: string): 'neutral' | 'current' | 'enabled' | 'empty' {
  switch (status) {
    case 'is-current':
      return 'current'
    case 'is-enabled':
      return 'enabled'
    case 'is-empty':
      return 'empty'
    default:
      return 'neutral'
  }
}

function errorMessage(error: unknown, fallback: string) {
  if (typeof error === 'object' && error && 'msg' in error && typeof (error as { msg?: unknown }).msg === 'string') {
    return (error as { msg: string }).msg
  }
  return fallback
}

async function loadConfig() {
  loading.value = true
  try {
    const res = await adminSegApi.getConfig()
    if (res.code === 0 && res.data?.config) {
      const normalized = normalizeSegConfig(res.data.config)
      secretConfigured.value = normalized.secretConfigured
      currentCard.value = normalized.card
    }
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

function openEditor() {
  draft.value = {
    ...currentCard.value,
  }
  editorOpen.value = true
}

function closeEditor() {
  if (saving.value) return
  editorOpen.value = false
}

async function handleSave() {
  saving.value = true
  try {
    const { payload, nextSecret } = buildSegSavePayload(draft.value)

    const res = await adminSegApi.saveConfig(payload)
    if (res.code === 0) {
      currentCard.value = {
        enabled: draft.value.enabled,
        access_key_id: draft.value.access_key_id,
        access_key_secret: '',
        region_id: draft.value.region_id,
      }
      draft.value.access_key_secret = ''
      secretConfigured.value = Boolean(nextSecret) || secretConfigured.value
      message.success(copy.saveSuccess)
      closeEditor()
    } else {
      message.error(res.msg || copy.saveFailed)
    }
  } catch (error) {
    message.error(errorMessage(error, copy.saveFailed))
  } finally {
    saving.value = false
  }
}

onMounted(loadConfig)
</script>

<style scoped>
@import './admin-common.css';

.image-processing-page {
  gap: 14px;
}

.processor-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}

.processor-card {
  min-height: 248px;
  border-radius: 18px;
  border: 1px solid var(--admin-border);
  background: var(--admin-card);
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.processor-card:hover {
  border-color: #2e3034;
  background: #141416;
}

.processor-card.current {
  border-color: rgba(244, 244, 245, 0.24);
}

.processor-meta-grid {
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

.processor-card-actions {
  margin-top: auto;
  display: flex;
  gap: 8px;
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

@media (max-width: 980px) {
  .processor-grid,
  .processor-meta-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .processor-card {
    min-height: unset;
  }
}
</style>
