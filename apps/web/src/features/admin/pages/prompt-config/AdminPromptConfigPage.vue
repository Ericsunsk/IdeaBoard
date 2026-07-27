<template>
  <AdminManagementFrame :content-class="workflowPanelContentClass">
    <div
      v-if="loading && workflowFeatures.length === 0"
      class="flex min-h-[260px] items-center justify-center rounded-lg border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <span class="text-admin-title font-semibold text-admin-title-color">正在加载工作流配置</span>
    </div>
    <div
      v-else-if="loadFailed && workflowFeatures.length === 0"
      class="flex min-h-[260px] flex-col items-center justify-center gap-3 rounded-lg border border-admin-border-soft bg-admin-card text-admin-text-2"
    >
      <span class="text-admin-title font-semibold text-admin-title-color">工作流配置加载失败</span>
      <Button size="small" label="重新加载" @click="loadWorkflowFeatures()" />
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
              <Tag
                :value="`${getGroupEnabledCount(group)}/${group.items.length} 已启用`"
                severity="success"
                rounded
              />
              <Tag
                :value="`${getGroupConfiguredCount(group)}/${group.items.length} 已配置模型`"
                severity="secondary"
                rounded
              />
            </div>

            <div class="flex flex-wrap content-start gap-2">
              <Tag
                v-for="item in group.items"
                :key="item.slug"
                :value="item.title"
                :severity="getSavedWorkflowFeature(item.slug)?.enabled ? 'success' : 'secondary'"
                rounded
              />
            </div>

            <div class="mt-auto flex justify-start">
              <Button
                size="small"
                label="编辑"
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
      <span class="text-admin-title font-semibold text-admin-title-color">暂无工作流配置</span>
    </div>
  </AdminManagementFrame>

  <AdminDialogShell
    :visible="Boolean(activeGroup)"
    aria-label="编辑工作流配置"
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
            {{ activeGroup ? `编辑 ${activeGroup.title} 工作流配置` : '' }}
          </h2>
        </div>

        <div v-if="activeGroup" class="flex flex-wrap gap-2">
          <Tag :value="`${activeGroup.items.length} 项配置`" severity="secondary" rounded />
          <Tag :value="`${getGroupEnabledCount(activeGroup)} 项启用`" severity="success" rounded />
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
            <Tag
              :value="getDraft(item.slug).enabled ? '启用' : '停用'"
              :severity="getDraft(item.slug).enabled ? 'success' : 'secondary'"
              rounded
            />
          </div>
        </template>

        <div class="flex flex-col gap-4">
          <div class="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <label
              class="flex min-w-0 flex-col gap-2 text-admin-caption font-bold text-admin-text-2"
            >
              <span>Canonical Model</span>
              <InputText
                :model-value="getDraft(item.slug).canonicalModel"
                class="w-full font-mono"
                placeholder="例如：image-alpha"
                :disabled="saving[item.slug]"
                @update:model-value="updateCanonicalModelDraft(item.slug, String($event ?? ''))"
              />
            </label>

            <div
              class="flex min-h-10 items-center justify-between gap-4 rounded-md border border-admin-border-soft bg-admin-surface-muted px-3 md:min-w-40"
            >
              <span class="text-admin-caption font-bold text-admin-text-2">启用工作流</span>
              <ToggleSwitch
                :model-value="getDraft(item.slug).enabled"
                :disabled="saving[item.slug]"
                :aria-label="`${item.title}启用状态`"
                @update:model-value="updateEnabledDraft(item.slug, Boolean($event))"
              />
            </div>
          </div>

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
              :model-value="getDraft(item.slug).systemPrompt"
              auto-resize
              class="w-full"
              rows="5"
              :placeholder="`为「${item.title}」设置 System Prompt`"
              :disabled="saving[item.slug]"
              @update:model-value="updateSystemPromptDraft(item.slug, String($event ?? ''))"
            />
          </div>

          <div class="flex flex-wrap items-center justify-between gap-2.5">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <Tag
                :value="`${getDraft(item.slug).systemPrompt.length} 字`"
                severity="secondary"
                rounded
              />
              <span
                class="text-admin-caption font-medium"
                :class="getValidationMessageClass(item.slug)"
              >
                {{ getDraftValidation(item.slug).message }}
              </span>
            </div>
            <Button
              size="small"
              label="保存配置"
              :loading="saving[item.slug]"
              :disabled="
                !getDraftValidation(item.slug).canSave || !getDraftValidation(item.slug).isDirty
              "
              @click="saveWorkflowFeature(item.slug)"
            />
          </div>
        </div>
      </Panel>
    </div>

    <template #footer>
      <Button
        label="关闭"
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
import InputText from 'primevue/inputtext'
import Panel from 'primevue/panel'
import Tag from 'primevue/tag'
import type { TagPassThroughOptions } from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useAdminWorkflowFeatureFeedback } from '../../prompt-config/useAdminPromptConfigFeedback'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminManagementFrame from '../../shared/components/AdminManagementFrame.vue'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminPanelPt } from '../../shared/lib/adminPanelPt'
import {
  adminDialogHeaderContentClass,
  adminDialogHeaderSplitClass,
  adminDialogTitleClass,
} from '../../shared/lib/adminUiClasses'
import { useAdminWorkflowFeatureWorkspace } from '../../prompt-config/promptConfigWorkspace'
import type {
  WorkflowFeatureVariable,
  WorkflowFeatureVariableTone,
} from '../../prompt-config/promptConfigWorkspace'
import {
  featureMap,
  groupedFeatureGroups,
  type FeatureGroup,
  type FeatureSlug,
} from './featurePromptMeta'

const feedback = useAdminWorkflowFeatureFeedback()
const workflowFeatureWorkspace = useAdminWorkflowFeatureWorkspace({
  feedback,
  getFeatureTitle: (slug) => featureMap[slug as FeatureSlug]?.title ?? slug,
})
const {
  getDraft,
  getDraftValidation,
  getSavedWorkflowFeature,
  getWorkflowFeatureVariables,
  insertVariableIntoDraft,
  loadFailed,
  loadWorkflowFeatures,
  loading,
  saveWorkflowFeature,
  saving,
  updateCanonicalModelDraft,
  updateEnabledDraft,
  updateSystemPromptDraft,
  workflowFeatures,
} = workflowFeatureWorkspace

type TextareaRefLike = HTMLTextAreaElement | { $el?: Element }
const textareaRefs = reactive<Record<string, TextareaRefLike>>({})
const activeGroupKey = ref<string | null>(null)
const featureGroups = computed<FeatureGroup[]>(() =>
  groupedFeatureGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => Boolean(getSavedWorkflowFeature(item.slug))),
    }))
    .filter((group) => group.items.length > 0),
)
const workflowPanelContentClass = computed(() =>
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
const variableTokenButtonToneClass: Record<WorkflowFeatureVariableTone, string> = {
  prompt: '!bg-[rgba(71,117,168,0.16)] !text-admin-info hover:!bg-[rgba(71,117,168,0.22)]',
  control: '!bg-[rgba(212,136,48,0.16)] !text-admin-warning hover:!bg-[rgba(212,136,48,0.22)]',
  context: '!bg-admin-surface-muted !text-admin-text-2-strong hover:!bg-admin-surface-hover',
}

function openEditor(groupKey: string) {
  activeGroupKey.value = groupKey
}

function closeEditor() {
  activeGroupKey.value = null
}

function handleEditorDialogVisibleUpdate(visible: boolean) {
  if (!visible) closeEditor()
}

function getGroupEnabledCount(group: FeatureGroup) {
  return group.items.filter((item) => getSavedWorkflowFeature(item.slug)?.enabled).length
}

function getGroupConfiguredCount(group: FeatureGroup) {
  return group.items.filter(
    (item) => getSavedWorkflowFeature(item.slug)?.canonical_model.trim().length,
  ).length
}

function getVariableTokenButtonClass(tone?: WorkflowFeatureVariableTone) {
  return [variableTokenButtonClass, variableTokenButtonToneClass[tone || 'context']]
}

function getVarsForSlug(slug: FeatureSlug): WorkflowFeatureVariable[] {
  return getWorkflowFeatureVariables(slug)
}

function getValidationMessageClass(slug: FeatureSlug) {
  const severity = getDraftValidation(slug).severity
  if (severity === 'error') return 'text-admin-danger'
  if (severity === 'warning') return 'text-admin-warning'
  return 'text-admin-text-3'
}

function insertVariable(slug: FeatureSlug, variableKey: string) {
  const currentPrompt = getDraft(slug).systemPrompt
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
</script>
