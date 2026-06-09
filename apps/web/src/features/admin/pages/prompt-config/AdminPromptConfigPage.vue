<template>
  <AdminManagementFrame
    :content-class="promptPanelContentClass"
    :is-scrolling="isPromptPanelScrolling"
    @scroll="handlePromptPanelScroll"
  >
    <template v-if="featureGroups.length > 0">
      <Card v-for="group in featureGroups" :key="group.key" class="!rounded-lg">
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
    </template>
    <div
      v-else
      class="flex min-h-[260px] flex-col items-center justify-center rounded-lg border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <div class="text-admin-title font-semibold text-admin-title-color">未加载到节点配置</div>
      <div class="mt-2 text-admin-meta text-admin-subtitle-color">
        请检查功能分组定义或刷新页面重试。
      </div>
    </div>
  </AdminManagementFrame>

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

    <div
      v-if="activeGroup"
      class="app-scrollbar h-[min(66vh,620px)] overflow-y-auto overscroll-contain pr-3"
      :class="{ 'is-scrolling': isPromptDialogScrolling }"
      @scroll.passive="handlePromptDialogScroll"
    >
      <div class="flex flex-col gap-3 pr-3">
        <Panel v-for="item in activeGroup.items" :key="item.slug" :pt="adminPanelPt">
          <template #header>
            <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2.5">
              <span class="text-admin-content font-semibold text-admin-title-color">
                {{ item.title }}
              </span>
              <Tag
                :value="item.slug"
                severity="secondary"
                rounded
                class="!min-w-0 max-w-full font-mono !text-admin-caption [&_.p-tag-label]:truncate"
              />
            </div>
          </template>

          <div class="flex flex-col gap-3">
            <div class="flex items-center justify-between gap-2.5">
              <Tag
                :value="enabledMap[item.slug] ? '已启用' : '已停用'"
                :severity="enabledMap[item.slug] ? 'success' : 'secondary'"
                rounded
              />
              <ToggleSwitch v-model="enabledMap[item.slug]" :disabled="saving[item.slug]" />
            </div>

            <div
              v-if="getVarsForSlug(item.slug).length > 0"
              class="flex min-w-0 flex-wrap items-center gap-1.5"
            >
              <Tag value="插入变量" severity="secondary" rounded />
              <Button
                v-for="v in getVarsForSlug(item.slug)"
                :key="v.key"
                type="button"
                size="small"
                rounded
                :label="v.key"
                :class="getVariableTokenButtonClass(v.tone)"
                @click="insertVar(item.slug, v.key)"
              />
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
              <Tag
                :value="`${(promptMap[item.slug] || '').length} 字`"
                severity="secondary"
                rounded
              />
              <Button
                size="small"
                label="保存"
                :loading="saving[item.slug]"
                @click="savePrompt(item.slug)"
              />
            </div>
          </div>
        </Panel>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <Button label="关闭" severity="secondary" outlined @click="closeEditor" />
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import Panel from 'primevue/panel'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useAppToast } from '../../../../shared/composables/useAppToast'
import { formatDisplayErrorMessage } from '../../../../shared/utils/errorMessages'
import AdminManagementFrame from '../../shared/components/AdminManagementFrame.vue'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminPanelPt } from '../../shared/lib/adminPanelPt'
import { saveFeaturePrompt } from '../../shared/queries/adminActions'
import {
  getListAdminFeaturePromptsQueryKey,
  useListAdminFeaturePrompts,
} from '../../../../shared/api/generated-query/admin-feature-prompts/admin-feature-prompts'
import type {
  FeaturePrompt,
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from '../../../../shared/api/generated-query/model'
import {
  featureMap,
  groupedFeatureGroups,
  getGroupFilledCount as calcGroupFilledCount,
  type FeatureGroup,
  type FeatureSlug,
} from './featurePromptMeta'

const appToast = useAppToast()
const queryClient = useQueryClient()
const { isScrolling: isPromptPanelScrolling, handleScroll: handlePromptPanelScroll } =
  useTransientScrolling()
const { isScrolling: isPromptDialogScrolling, handleScroll: handlePromptDialogScroll } =
  useTransientScrolling()

// textarea 引用 + 插入逻辑
type TextareaRefLike = HTMLTextAreaElement | { $el?: Element }
const textareaRefs = reactive<Record<string, TextareaRefLike>>({})
const activeGroupKey = ref<string | null>(null)
const featureGroups = computed<FeatureGroup[]>(() =>
  groupedFeatureGroups.filter((group) => group.items.length > 0),
)
const promptPanelContentClass = computed(() =>
  featureGroups.value.length > 0
    ? 'grid auto-rows-max grid-cols-1 content-start gap-3.5 p-6 lg:grid-cols-2 2xl:grid-cols-3'
    : 'p-6',
)
const activeGroup = computed<FeatureGroup | null>(() => {
  return featureGroups.value.find((group) => group.key === activeGroupKey.value) || null
})
const variableTokenButtonClass =
  'max-w-full !h-7 !min-h-0 !rounded-full !border-transparent !px-2.5 !py-0 font-mono !text-[11px] !font-[760] !leading-4 [&_.p-button-label]:break-all [&_.p-button-label]:leading-4'
const variableTokenButtonToneClass: Record<FeaturePromptVariableTone, string> = {
  prompt: '!bg-[rgba(71,117,168,0.16)] !text-admin-info hover:!bg-[rgba(71,117,168,0.22)]',
  control: '!bg-[rgba(212,136,48,0.16)] !text-admin-warning hover:!bg-[rgba(212,136,48,0.22)]',
  context: '!bg-admin-surface-muted !text-admin-text-2-strong hover:!bg-admin-surface-hover',
}

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
  return new Set(group.items.flatMap((item) => getVarsForSlug(item.slug).map((v) => v.key))).size
}

function getGroupFilledCount(group: FeatureGroup) {
  return calcGroupFilledCount(group, savedPromptMap)
}

function getVariableTokenButtonClass(tone?: FeaturePromptVariableTone) {
  return [variableTokenButtonClass, variableTokenButtonToneClass[tone || 'context']]
}

function getVarsForSlug(slug: FeatureSlug): FeaturePromptVariable[] {
  return promptConfigMap[slug]?.variables ?? []
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
  if (el) {
    textareaRefs[slug] = el as TextareaRefLike
    return
  }
  delete textareaRefs[slug]
}

const promptMap = reactive<Record<string, string>>({})
const savedPromptMap = reactive<Record<string, string>>({})
const enabledMap = reactive<Record<string, boolean>>({})
const promptConfigMap = reactive<Record<string, FeaturePrompt>>({})
const saving = reactive<Record<string, boolean>>({})
const promptsQuery = useListAdminFeaturePrompts()
const savePromptMutation = useMutation({
  mutationFn: saveFeaturePrompt,
  onSuccess: async (config) => {
    promptConfigMap[config.slug] = config
    savedPromptMap[config.slug] = config.prompt
    promptMap[config.slug] = config.prompt
    enabledMap[config.slug] = config.enabled
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
    enabledMap[item.slug] = promptConfigMap[item.slug]?.enabled ?? true
  }
}

watch(
  () => promptsQuery.data.value,
  (response) => {
    const prompts = response?.data?.prompts
    if (!prompts) return
    for (const config of prompts) {
      const slug = config.slug
      promptConfigMap[slug] = config
      const val = config.prompt || ''
      const hadDraft = Object.prototype.hasOwnProperty.call(promptMap, slug)
      const currentDraft = promptMap[slug] || ''
      const previousSaved = savedPromptMap[slug] || ''
      savedPromptMap[slug] = val
      if (!hadDraft || currentDraft === previousSaved) {
        promptMap[slug] = val
        enabledMap[slug] = config.enabled
      }
    }
  },
  { immediate: true },
)

async function savePrompt(slug: FeatureSlug) {
  saving[slug] = true
  try {
    await savePromptMutation.mutateAsync({
      slug,
      prompt: promptMap[slug] || '',
      enabled: enabledMap[slug] ?? true,
    })
    appToast.success(`「${featureMap[slug].title}」提示词已保存`)
  } catch (error) {
    appToast.error(formatDisplayErrorMessage(error, '保存失败'))
  } finally {
    saving[slug] = false
  }
}
</script>
