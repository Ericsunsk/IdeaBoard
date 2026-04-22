<template>
  <div class="admin-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">工作流系统提示词</h1>
        <p class="page-desc">按节点分组管理工作流提示词。列表页只做总览，具体内容在弹窗内编辑。</p>
      </div>
    </div>

    <div class="tip-bar">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      <span>支持模板变量：在提示词中插入 <code v-pre>{{变量名}}</code>，系统会自动替换为实际值。页面按节点分组展示，但每个功能块仍单独保存。</span>
    </div>

    <div class="node-grid">
      <section v-for="group in groupedFeatureGroups" :key="group.key" class="node-card">
        <div class="node-card-header">
          <div class="node-card-copy">
            <div class="node-title-row">
              <h2 class="node-card-title">{{ group.title }}</h2>
              <span class="node-card-count">{{ group.items.length }} 项功能</span>
            </div>
            <p class="node-card-desc">{{ group.desc }}</p>
          </div>
        </div>

        <div class="node-card-meta">
          <span class="meta-chip">{{ getGroupVariableCount(group) }} 个变量</span>
          <span class="meta-chip">{{ getGroupFilledCount(group) }}/{{ group.items.length }} 已配置</span>
        </div>

        <div class="node-feature-preview">
          <span v-for="item in group.items" :key="item.slug" class="feature-pill">{{ item.title }}</span>
        </div>

        <div class="node-card-actions">
          <n-button size="small" type="primary" @click="openEditor(group.key)">编辑</n-button>
        </div>
      </section>
    </div>
  </div>

  <AdminEditorModal
    :show="Boolean(activeGroup)"
    :eyebrow="activeGroup?.title || ''"
    :title="activeGroup ? `编辑 ${activeGroup.title} 节点提示词` : ''"
    :description="activeGroup?.desc || ''"
    @close="closeEditor"
  >
    <template #header-aside>
      <div v-if="activeGroup" class="editor-meta">
        <span class="meta-chip">{{ activeGroup.items.length }} 项功能</span>
        <span class="meta-chip">{{ getGroupVariableCount(activeGroup) }} 个变量</span>
      </div>
    </template>

    <div v-if="activeGroup" class="editor-feature-stack">
      <article v-for="item in activeGroup.items" :key="item.slug" class="feature-block">
        <div class="feature-header">
          <div class="feature-title-row">
            <span class="feature-title">{{ item.title }}</span>
            <span class="feature-slug">{{ item.slug }}</span>
          </div>
        </div>

        <div v-if="getVarsForSlug(item.slug).length > 0" class="var-buttons">
          <span class="var-buttons-label">插入变量</span>
          <button
            v-for="v in getVarsForSlug(item.slug)"
            :key="v.key"
            :class="['var-btn', `is-${getVarTone(v.key)}`]"
            @click="insertVar(item.slug, v.key)"
          >
            <span class="var-btn-key">{{ v.key }}</span>
            <span class="var-btn-desc">{{ v.desc }}</span>
          </button>
        </div>

        <div v-if="getFeatureNote(item.slug)" class="var-note">
          {{ getFeatureNote(item.slug) }}
        </div>

        <n-input
          :ref="(el: any) => { if (el) textareaRefs[item.slug] = el }"
          v-model:value="promptMap[item.slug]"
          type="textarea"
          :placeholder="`为「${item.title}」设置系统提示词，可使用上方变量按钮插入模板变量...`"
          :autosize="{ minRows: 4, maxRows: 10 }"
          :loading="saving[item.slug]"
        />

        <div class="feature-actions">
          <span class="feature-count">{{ (promptMap[item.slug] || '').length }} 字</span>
          <n-button size="small" type="primary" :loading="saving[item.slug]" @click="savePrompt(item.slug)">
            保存
          </n-button>
        </div>
      </article>
    </div>
  </AdminEditorModal>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { NInput, NButton, useMessage } from 'naive-ui'
import { featurePromptApi } from '../../api'
import AdminEditorModal from './components/AdminEditorModal.vue'
import {
  featureMap,
  groupedFeatureGroups,
  getFeatureNote,
  getGroupFilledCount as calcGroupFilledCount,
  getGroupVariableCount as calcGroupVariableCount,
  getVarTone,
  getVarsForSlug,
  type FeatureGroup,
  type FeatureSlug,
} from './featurePromptMeta'

const message = useMessage()

// textarea 引用 + 插入逻辑
const textareaRefs = reactive<Record<string, any>>({})
const activeGroupKey = ref<string | null>(null)
const activeGroup = computed<FeatureGroup | null>(() => {
  return groupedFeatureGroups.find((group) => group.key === activeGroupKey.value) || null
})

function openEditor(groupKey: string) {
  const group = groupedFeatureGroups.find((item) => item.key === groupKey)
  if (group) {
    resetGroupDraftFromSaved(group)
  }
  activeGroupKey.value = groupKey
}

function closeEditor() {
  if (activeGroup.value) {
    resetGroupDraftFromSaved(activeGroup.value)
  }
  activeGroupKey.value = null
}

function getGroupVariableCount(group: FeatureGroup) {
  return calcGroupVariableCount(group)
}

function getGroupFilledCount(group: FeatureGroup) {
  return calcGroupFilledCount(group, savedPromptMap)
}

function insertVar(slug: FeatureSlug, varKey: string) {
  const current = promptMap[slug] || ''
  const ref = textareaRefs[slug]
  let pos = current.length
  const textareaEl = ref?.textareaElRef ?? ref?.$el?.querySelector?.('textarea')
  if (textareaEl instanceof HTMLTextAreaElement) {
    pos = textareaEl.selectionStart ?? current.length
  }
  promptMap[slug] = current.slice(0, pos) + varKey + current.slice(pos)
}

const promptMap = reactive<Record<string, string>>({})
const savedPromptMap = reactive<Record<string, string>>({})
const saving = reactive<Record<string, boolean>>({})
const loading = ref(false)

function resetGroupDraftFromSaved(group: FeatureGroup) {
  for (const item of group.items) {
    promptMap[item.slug] = savedPromptMap[item.slug] || ''
  }
}

async function loadPrompts() {
  loading.value = true
  try {
    const res: any = await featurePromptApi.adminList()
    if (res.code === 0 && res.data?.prompts) {
      for (const [slug, prompt] of Object.entries(res.data.prompts)) {
        const val = (prompt as string) || ''
        savedPromptMap[slug] = val
        promptMap[slug] = val
      }
    }
  } catch (error) {
    console.error('[FeaturePromptConfig] Failed to load prompts', error)
    message.error('加载提示词失败')
  }
  loading.value = false
}

async function savePrompt(slug: FeatureSlug) {
  saving[slug] = true
  try {
    const res: any = await featurePromptApi.adminSet(slug, promptMap[slug] || '')
    if (res.code === 0) {
      savedPromptMap[slug] = promptMap[slug] || ''
      message.success(`「${featureMap[slug].title}」提示词已保存`)
    } else {
      message.error(res.msg || '保存失败')
    }
  } catch {
    message.error('保存失败')
  }
  saving[slug] = false
}

onMounted(loadPrompts)
</script>

<style scoped>
@import './admin-common.css';

.node-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.node-card {
  background: var(--admin-card);
  border: 1px solid var(--admin-border);
  border-radius: 16px;
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 244px;
  transition: border-color 0.18s ease, background 0.18s ease;
}

.node-card:hover {
  border-color: #2e3034;
  background: #141416;
}

.node-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.node-card-copy {
  min-width: 0;
}

.node-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.node-card-title {
  margin: 0;
  font-size: 16px;
  font-weight: 650;
  color: var(--admin-text-1);
}

.node-card-count {
  font-size: 11px;
  color: var(--admin-text-3);
  background: #17181a;
  border: 1px solid var(--admin-border);
  padding: 3px 8px;
  border-radius: 6px;
}

.node-card-desc {
  margin: 6px 0 0;
  color: var(--admin-text-2);
  font-size: 12px;
  line-height: 1.6;
}

.node-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.meta-chip {
  display: inline-flex;
  align-items: center;
  height: 26px;
  padding: 0 10px;
  border-radius: 999px;
  border: 1px solid var(--admin-border);
  background: #17181a;
  color: var(--admin-text-2);
  font-size: 11px;
}

.node-feature-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-content: flex-start;
}

.feature-pill {
  display: inline-flex;
  align-items: center;
  min-height: 28px;
  padding: 4px 10px;
  border-radius: 999px;
  background: #17181a;
  border: 1px solid var(--admin-border);
  color: var(--admin-text-2);
  font-size: 11px;
  line-height: 1.4;
}

.node-card-actions {
  margin-top: auto;
  display: flex;
  justify-content: flex-start;
}

.editor-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.editor-feature-stack {
  display: flex;
  flex-direction: column;
}

.feature-block {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 0 0;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
}

.feature-block:first-child {
  padding-top: 0;
  border-top: 0;
}

.feature-header,
.feature-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.feature-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--admin-text-1);
}

.feature-slug {
  font-size: 11px;
  color: var(--admin-text-3);
  background: #17181a;
  border: 1px solid var(--admin-border);
  padding: 2px 8px;
  border-radius: 6px;
  font-family: monospace;
}

.feature-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.feature-count {
  font-size: 11px;
  color: var(--admin-text-3);
}

.var-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.var-buttons-label {
  font-size: 11px;
  color: var(--admin-text-2);
  flex-shrink: 0;
}

.var-note {
  font-size: 11px;
  color: var(--admin-text-3);
}

.var-btn {
  --var-accent: #5eead4;
  --var-bg: #115e59;
  --var-bg-hover: #0f766e;
  --var-key: #ccfbf1;
  --var-desc-color: #99f6e4;
  appearance: none;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-radius: 10px;
  border: 0;
  background: var(--var-bg);
  cursor: pointer;
  box-shadow: none;
  transition: transform 0.15s ease, background 0.15s ease;
}

.var-btn.is-prompt {
  --var-accent: #5eead4;
  --var-bg: #115e59;
  --var-bg-hover: #0f766e;
  --var-key: #ccfbf1;
  --var-desc-color: #99f6e4;
}

.var-btn.is-control {
  --var-accent: #fbbf24;
  --var-bg: #92400e;
  --var-bg-hover: #b45309;
  --var-key: #fef3c7;
  --var-desc-color: #fde68a;
}

.var-btn.is-asset {
  --var-accent: #60a5fa;
  --var-bg: #1d4ed8;
  --var-bg-hover: #2563eb;
  --var-key: #dbeafe;
  --var-desc-color: #bfdbfe;
}

.var-btn.is-context {
  --var-accent: #a78bfa;
  --var-bg: #6d28d9;
  --var-bg-hover: #7c3aed;
  --var-key: #ede9fe;
  --var-desc-color: #ddd6fe;
}

.var-btn:hover {
  transform: translateY(-1px);
  background: var(--var-bg-hover);
}

.var-btn:active {
  transform: translateY(0);
}

.var-btn-key {
  color: var(--var-key);
  font-size: 11px;
  font-weight: 600;
  font-family: monospace;
}

.var-btn-desc {
  color: var(--var-desc-color);
  font-size: 10px;
}

@media (max-width: 720px) {
  .node-grid {
    grid-template-columns: 1fr;
  }

  .node-card {
    padding: 14px;
    min-height: unset;
  }

  .feature-actions {
    gap: 10px;
  }

  .feature-count {
    flex-shrink: 0;
  }
}

@media (min-width: 721px) and (max-width: 1120px) {
  .node-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
