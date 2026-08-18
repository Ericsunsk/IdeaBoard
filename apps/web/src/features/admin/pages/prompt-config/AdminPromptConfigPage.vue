<template>
  <AdminSurfaceFrame viewport-height scroll-content :content-class="promptPanelContentClass">
    <div
      v-if="loading && prompts.length === 0"
      class="flex min-h-[260px] items-center justify-center rounded-lg border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <span class="text-admin-title font-semibold text-admin-title-color">正在加载系统提示词</span>
    </div>
    <div
      v-else-if="loadFailed && prompts.length === 0"
      class="flex min-h-[260px] flex-col items-center justify-center gap-3 rounded-lg border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <span class="text-admin-title font-semibold text-admin-title-color">系统提示词加载失败</span>
      <Button
        :class="adminActionButtonClass"
        label="重新加载"
        severity="secondary"
        outlined
        @click="loadPrompts()"
      />
    </div>
    <template v-else-if="featureGroups.length > 0">
      <Card v-for="group in featureGroups" :key="group.key" class="!rounded-lg" :pt="adminCardPt">
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
              <Button
                size="small"
                label="编辑"
                :class="adminActionButtonClass"
                severity="contrast"
                :disabled="loading"
                :loading="loading"
                @click="openEditor(group.key)"
              />
            </div>
          </div>
        </template>
      </Card>
    </template>
    <div
      v-else
      class="flex min-h-[260px] items-center justify-center rounded-lg border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <span class="text-admin-title font-semibold text-admin-title-color">未加载到节点配置</span>
    </div>
  </AdminSurfaceFrame>

  <AdminDialogShell
    :visible="Boolean(activeGroup)"
    aria-label="编辑节点提示词"
    size="wide"
    content-layout="scrollable"
    :busy="isEditorBusy"
    @update:visible="handleEditorDialogVisibleUpdate"
  >
    <template #header>
      <div :class="adminDialogHeaderSplitClass">
        <div :class="adminDialogHeaderContentClass">
          <div v-if="activeGroup" class="mb-3 flex flex-wrap gap-2">
            <Tag :value="activeGroup.title" severity="secondary" rounded />
          </div>
          <h2 :class="adminDialogTitleClass">
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

    <div v-if="activeGroup" class="flex flex-col gap-3">
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
              class="!min-w-0 max-w-full font-mono !text-admin-caption"
              :pt="workflowSlugTagPt"
            />
          </div>
        </template>

        <div class="flex flex-col gap-3">
          <div class="flex flex-col gap-2">
            <div class="flex min-w-0 flex-wrap items-center justify-between gap-2">
              <span class="text-admin-caption font-bold text-admin-text-2">System Prompt</span>
              <div
                v-if="getVarsForSlug(item.slug).length > 0"
                class="flex min-w-0 flex-wrap items-center justify-end gap-1.5"
              >
                <span class="text-admin-caption font-medium text-admin-text-3">插入变量</span>
                <Button
                  v-for="variable in getVarsForSlug(item.slug)"
                  :key="variable.key"
                  type="button"
                  size="small"
                  rounded
                  :label="variable.key"
                  :disabled="saving[item.slug]"
                  :pt="variableTokenButtonPt"
                  :class="getVariableTokenButtonClass(variable.tone)"
                  @click="insertVariable(item.slug, variable.key)"
                />
              </div>
            </div>

            <Textarea
              :ref="(element: unknown) => bindTextareaRef(item.slug, element)"
              v-model="promptMap[item.slug]"
              auto-resize
              class="w-full"
              rows="4"
              placeholder="可使用上方变量插入模板"
              :disabled="saving[item.slug]"
            />
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2.5">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <Tag
                :value="`${(promptMap[item.slug] || '').length} 字`"
                severity="secondary"
                rounded
              />
            </div>
            <Button
              size="small"
              label="保存"
              :class="adminActionButtonClass"
              severity="contrast"
              :loading="saving[item.slug]"
              :disabled="
                !getDraftValidation(item.slug).canSave || !getDraftValidation(item.slug).isDirty
              "
              @click="savePrompt(item.slug)"
            />
          </div>
        </div>
      </Panel>
    </div>

    <template #footer>
      <Button
        label="关闭"
        :class="adminActionButtonClass"
        severity="secondary"
        outlined
        :disabled="isEditorBusy"
        @click="closeEditor"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import Button from 'primevue/button'
import type { ButtonPassThroughOptions } from 'primevue/button'
import Card from 'primevue/card'
import Panel from 'primevue/panel'
import Tag from 'primevue/tag'
import type { TagPassThroughOptions } from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useAdminPromptConfigFeedback } from '../../prompt-config/useAdminPromptConfigFeedback'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminSurfaceFrame from '../../shared/components/AdminSurfaceFrame.vue'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminPanelPt } from '../../shared/lib/adminPanelPt'
import {
  adminDialogHeaderContentClass,
  adminDialogHeaderSplitClass,
  adminDialogTitleClass,
  adminActionButtonClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminPromptConfigWorkspace } from '../../prompt-config/promptConfigWorkspace'
import type {
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from '../../prompt-config/promptConfigWorkspace'
import {
  featureMap,
  groupedFeatureGroups,
  type FeatureGroup,
  type FeatureSlug,
} from './featurePromptMeta'

const feedback = useAdminPromptConfigFeedback()
const promptConfigWorkspace = useAdminPromptConfigWorkspace({
  feedback,
  getFeatureTitle: (slug) => featureMap[slug as FeatureSlug]?.title ?? slug,
})
const {
  getDraftValidation,
  getPromptVariables,
  getSavedPrompt,
  insertVariableIntoDraft,
  loadFailed,
  loadPrompts,
  loading,
  promptMap,
  prompts,
  resetDrafts,
  savePrompt,
  saving,
} = promptConfigWorkspace

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
const isEditorBusy = computed(
  () => activeGroup.value?.items.some((item) => saving[item.slug]) ?? false,
)
const variableTokenButtonClass =
  'max-w-full !h-7 !min-h-0 !rounded-full !border-transparent !px-2.5 !py-0 font-mono !text-admin-caption !font-[760] !leading-4'
const variableTokenButtonPt = {
  label: { class: 'break-all leading-4' },
} satisfies ButtonPassThroughOptions
const workflowSlugTagPt = {
  label: { class: 'truncate' },
} satisfies TagPassThroughOptions
const variableTokenButtonToneClass: Record<FeaturePromptVariableTone, string> = {
  prompt: '!bg-[rgba(71,117,168,0.16)] !text-admin-info hover:!bg-[rgba(71,117,168,0.22)]',
  control: '!bg-[rgba(212,136,48,0.16)] !text-admin-warning hover:!bg-[rgba(212,136,48,0.22)]',
  context: '!bg-admin-surface-muted !text-admin-text-2-strong hover:!bg-admin-surface-hover',
}

function openEditor(groupKey: string) {
  const group = featureGroups.value.find((item) => item.key === groupKey)
  if (group) resetGroupDraftFromSaved(group)
  activeGroupKey.value = groupKey
}

function closeEditor() {
  if (activeGroup.value) resetGroupDraftFromSaved(activeGroup.value)
  activeGroupKey.value = null
}

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}

function getGroupVariableCount(group: FeatureGroup) {
  return new Set(group.items.flatMap((item) => getVarsForSlug(item.slug).map((v) => v.key))).size
}

function getGroupFilledCount(group: FeatureGroup) {
  return group.items.filter((item) => getSavedPrompt(item.slug).trim().length > 0).length
}

function getVariableTokenButtonClass(tone?: FeaturePromptVariableTone) {
  return [variableTokenButtonClass, variableTokenButtonToneClass[tone || 'context']]
}

function getVarsForSlug(slug: FeatureSlug): readonly FeaturePromptVariable[] {
  return getPromptVariables(slug)
}

function insertVariable(slug: FeatureSlug, variableKey: string) {
  const currentPrompt = promptMap[slug] || ''
  const textareaElement = resolveTextareaElement(textareaRefs[slug])
  const position =
    textareaElement instanceof HTMLTextAreaElement
      ? (textareaElement.selectionStart ?? currentPrompt.length)
      : currentPrompt.length
  insertVariableIntoDraft(slug, variableKey, position)
}

function resolveTextareaElement(ref: TextareaRefLike | undefined) {
  if (ref instanceof HTMLTextAreaElement) return ref
  const root = ref?.$el
  if (root instanceof HTMLTextAreaElement) return root
  return root?.querySelector?.('textarea') ?? null
}

function bindTextareaRef(slug: FeatureSlug, element: unknown) {
  if (element) {
    textareaRefs[slug] = element as TextareaRefLike
    return
  }
  delete textareaRefs[slug]
}

function resetGroupDraftFromSaved(group: FeatureGroup) {
  resetDrafts(group.items.map((item) => item.slug))
}
</script>
