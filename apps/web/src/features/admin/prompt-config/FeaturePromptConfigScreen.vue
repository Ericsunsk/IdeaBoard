<template>
  <div class="admin-page">
    <div class="node-grid">
      <section v-for="group in groupedFeatureGroups" :key="group.key" class="node-card">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="m-0 text-base font-[650] text-[var(--admin-text-1)]">
                {{ group.title }}
              </h2>
              <span
                class="rounded-md border border-[var(--admin-border)] bg-[#17181a] px-2 py-[3px] text-[11px] text-[var(--admin-text-3)]"
              >
                {{ group.items.length }} 项功能
              </span>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap gap-2">
          <span class="meta-chip">{{ getGroupVariableCount(group) }} 个变量</span>
          <span class="meta-chip"
            >{{ getGroupFilledCount(group) }}/{{ group.items.length }} 已配置</span
          >
        </div>

        <div class="flex flex-wrap content-start gap-2">
          <span v-for="item in group.items" :key="item.slug" class="feature-pill">{{
            item.title
          }}</span>
        </div>

        <div class="mt-auto flex justify-start">
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
      <div v-if="activeGroup" class="flex flex-wrap gap-2">
        <span class="meta-chip">{{ activeGroup.items.length }} 项功能</span>
        <span class="meta-chip">{{ getGroupVariableCount(activeGroup) }} 个变量</span>
      </div>
    </template>

    <div v-if="activeGroup" class="flex flex-col">
      <article v-for="item in activeGroup.items" :key="item.slug" class="feature-block">
        <div class="flex flex-wrap items-center gap-2.5">
          <div class="flex flex-wrap items-center gap-2.5">
            <span class="text-sm font-semibold text-[var(--admin-text-1)]">{{ item.title }}</span>
            <span
              class="rounded-md border border-[var(--admin-border)] bg-[#17181a] px-2 py-0.5 font-mono text-[11px] text-[var(--admin-text-3)]"
            >
              {{ item.slug }}
            </span>
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

        <div v-if="getFeatureNote(item.slug)" class="text-[11px] text-[var(--admin-text-3)]">
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

        <div class="flex items-center justify-between max-[720px]:gap-2.5">
          <span class="text-[11px] text-[var(--admin-text-3)] max-[720px]:shrink-0"
            >{{ (promptMap[item.slug] || '').length }} 字</span
          >
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
import { NButton, NInput, useMessage } from 'naive-ui'
import { formatDisplayErrorMessage } from '../../../shared/utils/errorMessages'
import { saveFeaturePrompt } from '../../../shared/query/adminQueries'
import {
  getListAdminFeaturePromptsQueryKey,
  useListAdminFeaturePrompts,
} from '../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
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
const queryClient = useQueryClient()

// textarea 引用 + 插入逻辑
type TextareaRefLike = { textareaElRef?: HTMLTextAreaElement; $el?: Element }
const textareaRefs = reactive<Record<string, TextareaRefLike>>({})
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
  const textareaRef = textareaRefs[slug]
  let pos = current.length
  const textareaEl = textareaRef?.textareaElRef ?? textareaRef?.$el?.querySelector?.('textarea')
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
  transition:
    border-color 0.18s ease,
    background 0.18s ease;
}

.node-card:hover {
  border-color: #2e3034;
  background: #141416;
}

.node-card-desc {
  margin: 6px 0 0;
  color: var(--admin-text-2);
  font-size: 12px;
  line-height: 1.6;
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
}

@media (min-width: 721px) and (max-width: 1120px) {
  .node-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
