<template>
  <div class="admin-page">
    <div class="node-grid">
      <section v-for="group in groupedFeatureGroups" :key="group.key" class="node-card">
        <div class="node-card-header">
          <div class="node-card-copy">
            <div class="node-title-row">
              <h2 class="node-card-title">{{ group.title }}</h2>
              <span class="node-card-count">{{ group.items.length }} 项功能</span>
            </div>
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
import { ref, reactive, computed, watch } from 'vue'
import { useMutation, useQuery } from '@tanstack/vue-query'
import { NInput, NButton, useMessage } from 'naive-ui'
import AdminEditorModal from '../shared/components/AdminEditorModal.vue'
import { queryKeys } from '../../../shared/query/keys'
import {
  fetchFeaturePrompts,
  invalidateFeaturePrompts,
  saveFeaturePrompt,
} from '../../../shared/query/adminQueries'
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
const promptsQuery = useQuery({
  queryKey: queryKeys.admin.featurePrompts(),
  queryFn: fetchFeaturePrompts,
})
const savePromptMutation = useMutation({
  mutationFn: saveFeaturePrompt,
  onSuccess: async ({ slug, prompt }) => {
    savedPromptMap[slug] = prompt
    promptMap[slug] = prompt
    await invalidateFeaturePrompts()
  },
})
watch(
  () => promptsQuery.error.value,
  (error) => {
    if (error) message.error(error instanceof Error ? error.message : '加载提示词失败')
  },
)

function resetGroupDraftFromSaved(group: FeatureGroup) {
  for (const item of group.items) {
    promptMap[item.slug] = savedPromptMap[item.slug] || ''
  }
}

watch(
  () => promptsQuery.data.value,
  (prompts) => {
    if (!prompts) return
    for (const [slug, prompt] of Object.entries(prompts)) {
      const val = prompt || ''
      const hadDraft = Object.prototype.hasOwnProperty.call(promptMap, slug)
      const currentDraft = promptMap[slug] || ''
      const previousSaved = savedPromptMap[slug] || ''
      savedPromptMap[slug] = val
      if (!hadDraft || currentDraft === previousSaved) {
        promptMap[slug] = val
      }
    }
  },
  { immediate: true },
)

async function savePrompt(slug: FeatureSlug) {
  saving[slug] = true
  try {
    await savePromptMutation.mutateAsync({ slug, prompt: promptMap[slug] || '' })
    message.success(`「${featureMap[slug].title}」提示词已保存`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '保存失败')
  } finally {
    saving[slug] = false
  }
}
</script>

<style scoped>
@import '../shared/adminCommon.css';

.node-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.node-card {
  background: var(--admin-card-strong);
  border: 1px solid var(--admin-border-soft);
  border-radius: 16px;
  padding: 16px;
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
  border-top: 1px solid var(--admin-divider);
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
  --var-bg: #115e59;
  --var-bg-hover: #0f766e;
  --var-key: #ccfbf1;
  appearance: none;
  display: inline-flex;
  align-items: center;
  gap: 0;
  padding: 8px 12px;
  border-radius: 10px;
  border: 0;
  background: var(--var-bg);
  cursor: pointer;
  box-shadow: none;
  transition: background 0.15s ease;
}

.var-btn.is-prompt {
  --var-bg: #115e59;
  --var-bg-hover: #0f766e;
  --var-key: #ccfbf1;
}

.var-btn.is-control {
  --var-bg: #92400e;
  --var-bg-hover: #b45309;
  --var-key: #fef3c7;
}

.var-btn.is-asset {
  --var-bg: #1d4ed8;
  --var-bg-hover: #2563eb;
  --var-key: #dbeafe;
}

.var-btn.is-context {
  --var-bg: #6d28d9;
  --var-bg-hover: #7c3aed;
  --var-key: #ede9fe;
}

.var-btn:hover {
  background: var(--var-bg-hover);
}

.var-btn-key {
  color: var(--var-key);
  font-size: 11px;
  font-weight: 600;
  font-family: monospace;
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
