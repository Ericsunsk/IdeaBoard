<template>
  <div class="flex max-w-full min-w-0 flex-col gap-4 p-0">
    <div
      v-if="featureGroups.length > 0"
      class="grid grid-cols-1 gap-3.5 lg:grid-cols-2 2xl:grid-cols-3"
    >
      <Card
        v-for="group in featureGroups"
        :key="group.key"
      >
        <template #content>
          <div class="flex min-h-[208px] flex-col gap-3.5">
            <div class="flex items-start justify-between gap-3">
              <span class="min-w-0 truncate text-admin-title-sm font-bold text-admin-title-color">
                {{ group.title }}
              </span>
              <Tag :value="`${group.items.length} 项配置`" severity="secondary" rounded />
            </div>

            <div class="flex flex-wrap gap-2">
              <Tag :value="`${getGroupVariableCount(group)} 个变量`" severity="secondary" rounded />
              <Tag
                :value="`${getGroupFilledCount(group)}/${group.items.length} 已配置`"
                severity="secondary"
                rounded
              />
            </div>

            <div class="flex flex-wrap content-start gap-2">
              <Tag
                v-for="item in group.items"
                :key="item.slug"
                :value="item.title"
                severity="secondary"
                rounded
              />
            </div>

            <div class="mt-auto flex justify-start">
              <Button size="small" label="编辑" @click="openEditor(group.key)" />
            </div>
          </div>
        </template>
      </Card>
    </div>
    <div
      v-else
      class="flex min-h-[260px] flex-col items-center justify-center rounded-2xl border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <div class="text-admin-title font-semibold text-admin-title-color">未加载到节点配置</div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">请检查功能分组定义或刷新页面重试。</div>
    </div>
  </div>

  <Dialog
    :visible="Boolean(activeGroup)"
    modal
    dismissable-mask
    :draggable="false"
    :style="{ width: '980px', maxWidth: 'calc(100vw - 24px)' }"
    @update:visible="handleEditorDialogVisibleUpdate"
  >
    <template #header>
      <div class="flex w-full flex-col gap-4 pr-4 md:flex-row md:items-start md:justify-between">
        <div class="min-w-0">
          <div v-if="activeGroup" class="mb-3 flex flex-wrap gap-2">
            <Tag :value="activeGroup.title" severity="secondary" rounded />
          </div>
          <h2 class="m-0 text-admin-modal-title leading-[1.15] font-bold text-admin-title-color">
            {{ activeGroup ? `编辑 ${activeGroup.title} 节点提示词` : '' }}
          </h2>
          <p class="mt-2.5 mb-0 text-admin-content leading-7 text-admin-subtitle-color">
            {{ activeGroupDescription }}
          </p>
        </div>

        <div v-if="activeGroup" class="flex flex-wrap gap-2">
          <Tag :value="`${activeGroup.items.length} 项配置`" severity="secondary" rounded />
          <Tag
            :value="`${getGroupVariableCount(activeGroup)} 个变量`"
            severity="secondary"
            rounded
          />
        </div>
      </div>
    </template>

    <div v-if="activeGroup" class="flex flex-col">
      <article
        v-for="item in activeGroup.items"
        :key="item.slug"
        class="flex flex-col gap-2.5 border-t border-admin-divider pt-3.5 first:border-t-0 first:pt-0"
      >
        <div class="flex flex-wrap items-center gap-2.5">
          <span class="text-admin-content font-semibold text-admin-title-color">
            {{ item.title }}
          </span>
          <span
            class="max-w-full rounded-md border border-admin-border bg-admin-surface-muted px-2 py-0.5 font-mono text-admin-caption text-admin-text-3 [overflow-wrap:anywhere]"
          >
            {{ item.slug }}
          </span>
        </div>

        <div
          v-if="getVarsForSlug(item.slug).length > 0"
          class="flex min-w-0 flex-wrap items-center gap-1.5"
        >
          <span class="shrink-0 text-admin-caption text-admin-text-2">插入变量</span>
          <Button
            v-for="v in getVarsForSlug(item.slug)"
            :key="v.key"
            type="button"
            size="small"
            outlined
            rounded
            :severity="getVariableButtonSeverity(v.tone)"
            :label="v.key"
            class="max-w-full font-mono [&_.p-button-label]:break-all [&_.p-button-label]:text-admin-caption"
            @click="insertVar(item.slug, v.key)"
          />
        </div>

        <div v-if="getFeatureNote(item.slug)" class="text-admin-caption text-admin-text-3">
          {{ getFeatureNote(item.slug) }}
        </div>

        <Textarea
          :ref="(el: unknown) => bindTextareaRef(item.slug, el)"
          v-model="promptMap[item.slug]"
          auto-resize
          class="w-full"
          rows="4"
          :placeholder="`为「${item.title}」设置系统提示词，可使用上方变量按钮插入模板变量...`"
          :disabled="saving[item.slug]"
        />

        <div class="flex items-center justify-between gap-2.5">
          <span class="shrink-0 text-admin-caption text-admin-text-3">
            {{ (promptMap[item.slug] || '').length }} 字
          </span>
          <Button
            size="small"
            label="保存"
            :loading="saving[item.slug]"
            @click="savePrompt(item.slug)"
          />
        </div>
      </article>
    </div>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useAppToast } from '../../../../shared/composables/useAppToast'
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
  type VarTone,
} from './featurePromptMeta'

const appToast = useAppToast()
const queryClient = useQueryClient()

// textarea 引用 + 插入逻辑
type TextareaRefLike = HTMLTextAreaElement | { $el?: Element }
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

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}

function getGroupVariableCount(group: FeatureGroup) {
  return calcGroupVariableCount(group)
}

function getGroupFilledCount(group: FeatureGroup) {
  return calcGroupFilledCount(group, savedPromptMap)
}

function getVariableButtonSeverity(tone: VarTone) {
  if (tone === 'control') return 'warn'
  if (tone === 'context') return 'secondary'
  return 'info'
}

function insertVar(slug: FeatureSlug, varKey: string) {
  const current = promptMap[slug] || ''
  const ref = textareaRefs[slug]
  let pos = current.length
  const textareaEl = resolveTextareaElement(ref)
  if (textareaEl instanceof HTMLTextAreaElement) {
    pos = textareaEl.selectionStart ?? current.length
  }
  promptMap[slug] = current.slice(0, pos) + varKey + current.slice(pos)
}

function resolveTextareaElement(ref: TextareaRefLike | undefined) {
  if (ref instanceof HTMLTextAreaElement) return ref
  const root = ref?.$el
  if (root instanceof HTMLTextAreaElement) return root
  return root?.querySelector?.('textarea') ?? null
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
    if (error) appToast.error(formatDisplayErrorMessage(error, '加载提示词失败'))
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
    appToast.success(`「${featureMap[slug].title}」提示词已保存`)
  } catch (error) {
    appToast.error(formatDisplayErrorMessage(error, '保存失败'))
  } finally {
    saving[slug] = false
  }
}
</script>
