<template>
  <div class="admin-page prompt-config-page">
    <div v-if="featureGroups.length > 0" class="admin-card-grid is-3">
      <section
        v-for="group in featureGroups"
        :key="group.key"
        class="node-card admin-card-surface admin-card-interactive"
      >
        <AdminCardHeader :title="group.title" size="small">
          <template #extra>
            <span class="admin-pill is-compact">{{ group.items.length }} 项配置</span>
          </template>
        </AdminCardHeader>

        <div class="node-card-meta">
          <span class="admin-pill is-compact">{{ getGroupVariableCount(group) }} 个变量</span>
          <span class="admin-pill is-compact"
            >{{ getGroupFilledCount(group) }}/{{ group.items.length }} 已配置</span
          >
        </div>

        <div class="node-feature-preview">
          <span v-for="item in group.items" :key="item.slug" class="admin-pill">{{
            item.title
          }}</span>
        </div>

        <div class="node-card-actions">
          <n-button size="small" type="primary" @click="openEditor(group.key)">编辑</n-button>
        </div>
      </section>
    </div>
    <div v-else class="node-empty-state">
      <div class="node-empty-title">未加载到节点配置</div>
      <div class="node-empty-desc">请检查功能分组定义或刷新页面重试。</div>
    </div>
  </div>

  <AdminEditorModal
    :show="Boolean(activeGroup)"
    :eyebrow="activeGroup?.title || ''"
    :title="activeGroup ? `编辑 ${activeGroup.title} 节点提示词` : ''"
    :description="activeGroupDescription"
    @close="closeEditor"
  >
    <template #header-aside>
      <div v-if="activeGroup" class="editor-meta">
        <span class="admin-pill is-compact">{{ activeGroup.items.length }} 项配置</span>
        <span class="admin-pill is-compact">{{ getGroupVariableCount(activeGroup) }} 个变量</span>
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
            :class="['var-btn', `is-${v.tone}`]"
            @click="insertVar(item.slug, v.key)"
          >
            <span class="var-btn-key">{{ v.key }}</span>
          </button>
        </div>

        <div v-if="getFeatureNote(item.slug)" class="var-note">
          {{ getFeatureNote(item.slug) }}
        </div>

        <n-input
          :ref="(el: unknown) => bindTextareaRef(item.slug, el)"
          v-model:value="promptMap[item.slug]"
          type="textarea"
          :placeholder="`为「${item.title}」设置系统提示词，可使用上方变量按钮插入模板变量...`"
          :autosize="{ minRows: 4, maxRows: 10 }"
          :loading="saving[item.slug]"
        />

        <div class="feature-actions">
          <span class="feature-count">{{ (promptMap[item.slug] || '').length }} 字</span>
          <n-button
            size="small"
            type="primary"
            :loading="saving[item.slug]"
            @click="savePrompt(item.slug)"
          >
            保存
          </n-button>
        </div>
      </article>
    </div>
  </AdminEditorModal>
</template>

<script setup lang="ts">
import { NButton, NInput } from 'naive-ui'
import { computed, reactive, ref, watch } from 'vue'
import AdminCardHeader from '../../shared/components/AdminCardHeader.vue'
import AdminEditorModal from '../../shared/components/AdminEditorModal.vue'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import { saveFeaturePrompt } from '../../shared/queries/adminActions'
import {
  getListAdminFeaturePromptsQueryKey,
  useListAdminFeaturePrompts,
} from '../../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import {
  featureMap,
  groupedFeatureGroups,
  getFeatureNote,
  getGroupFilledCount as calcGroupFilledCount,
  getGroupVariableCount as calcGroupVariableCount,
  getVarsForSlug,
  type FeatureGroup,
  type FeatureSlug,
} from './featurePromptMeta'

const message = useMessage()
const queryClient = useQueryClient()

// textarea 引用 + 插入逻辑
type TextareaRefLike = { textareaElRef?: HTMLTextAreaElement; $el?: Element }
const textareaRefs = reactive<Record<string, TextareaRefLike>>({})
const activeGroupKey = ref<string | null>(null)
const featureGroups = computed<FeatureGroup[]>(() =>
  groupedFeatureGroups.filter((group) => group.items.length > 0),
)
const activeGroup = computed<FeatureGroup | null>(() => {
  return featureGroups.value.find((group) => group.key === activeGroupKey.value) || null
})
const activeGroupDescription = computed(() => {
  if (!activeGroup.value) return ''
  const desc = activeGroup.value.desc?.trim()
  return desc ? `编辑当前节点的提示词配置。${desc}` : '编辑当前节点的提示词配置。'
})

function openEditor(groupKey: string) {
  const group = featureGroups.value.find((item) => item.key === groupKey)
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

function bindTextareaRef(slug: FeatureSlug, el: unknown) {
  if (el) textareaRefs[slug] = el as TextareaRefLike
}

const promptMap = reactive<Record<string, string>>({})
const savedPromptMap = reactive<Record<string, string>>({})
const saving = reactive<Record<string, boolean>>({})
const promptsQuery = useListAdminFeaturePrompts()
const savePromptMutation = useMutation({
  mutationFn: saveFeaturePrompt,
  onSuccess: async ({ slug, prompt }) => {
    savedPromptMap[slug] = prompt
    promptMap[slug] = prompt
    await queryClient.invalidateQueries({ queryKey: getListAdminFeaturePromptsQueryKey() })
  },
})
watch(
  () => promptsQuery.error.value,
  (error) => {
    if (error) message.error(formatDisplayErrorMessage(error, '加载提示词失败'))
  },
)

function resetGroupDraftFromSaved(group: FeatureGroup) {
  for (const item of group.items) {
    promptMap[item.slug] = savedPromptMap[item.slug] || ''
  }
}

watch(
  () => promptsQuery.data.value,
  (response) => {
    const prompts = response?.data?.prompts
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
    message.error(formatDisplayErrorMessage(error, '保存失败'))
  } finally {
    saving[slug] = false
  }
}
</script>

<style scoped>
@import '../../shared/styles/adminCommon.css';

.prompt-config-page {
  min-width: 0;
}

.node-empty-state {
  display: flex;
  min-height: 260px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--admin-border-soft);
  border-radius: 16px;
  background: var(--admin-card);
  color: var(--admin-text-2);
}

.node-empty-title {
  color: var(--admin-title-color);
  font-size: var(--admin-font-size-title);
  font-weight: 650;
}

.node-empty-desc {
  margin-top: 8px;
  color: var(--admin-subtitle-color);
  font-size: var(--admin-font-size-meta);
}

.node-card {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-height: 244px;
}

.node-card-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.node-feature-preview {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-content: flex-start;
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
  font-size: var(--admin-font-size-content);
  font-weight: 600;
  color: var(--admin-title-color);
}

.feature-slug {
  max-width: 100%;
  overflow-wrap: anywhere;
  font-size: var(--admin-font-size-caption);
  color: var(--admin-text-3);
  background: var(--admin-surface-muted);
  border: 1px solid var(--admin-border);
  padding: 2px 8px;
  border-radius: 6px;
  font-family: monospace;
}

.feature-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

.feature-count {
  font-size: var(--admin-font-size-caption);
  color: var(--admin-text-3);
}

.var-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  min-width: 0;
}

.var-buttons-label {
  font-size: var(--admin-font-size-caption);
  color: var(--admin-text-2);
  flex-shrink: 0;
}

.var-note {
  font-size: var(--admin-font-size-caption);
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
  max-width: 100%;
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
  font-size: var(--admin-font-size-caption);
  font-weight: 600;
  font-family: monospace;
  overflow-wrap: anywhere;
}

@media (max-width: 720px) {
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
</style>
