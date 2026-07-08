<template>
  <AdminManagementFrame
    :content-class="promptPanelContentClass"
    :is-scrolling="isPromptPanelScrolling"
    @scroll="handlePromptPanelScroll"
  >
    <template v-if="featureGroups.length > 0">
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

    <div
      v-if="activeGroup"
      :class="[
        adminDialogScrollableBodyClass,
        'h-[min(66vh,620px)]',
        { 'is-scrolling': isPromptDialogScrolling },
      ]"
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
                class="!min-w-0 max-w-full font-mono !text-admin-caption"
                :pt="promptSlugTagPt"
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
              <ToggleSwitch
                v-model="enabledMap[item.slug]"
                :disabled="saving[item.slug]"
                :aria-label="`${item.title} 启用状态`"
              />
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
                :pt="variableTokenButtonPt"
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
              <div class="flex flex-wrap justify-end gap-2">
                <Button
                  size="small"
                  label="保存"
                  :loading="saving[item.slug]"
                  @click="savePrompt(item.slug)"
                />
              </div>
            </div>
          </div>
        </Panel>
      </div>
    </div>

    <template #footer>
      <div :class="adminDialogFooterActionsClass">
        <Button label="关闭" severity="secondary" outlined @click="closeEditor" />
      </div>
    </template>
  </Dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import Button from 'primevue/button'
import type { ButtonPassThroughOptions } from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import Panel from 'primevue/panel'
import Tag from 'primevue/tag'
import type { TagPassThroughOptions } from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useAdminFeedback } from '../../feedback/useAdminFeedback'
import AdminManagementFrame from '../../shared/components/AdminManagementFrame.vue'
import { useTransientScrolling } from '../../shared/composables/useTransientScrolling'
import { adminCardPt } from '../../shared/lib/adminCardPt'
import { adminPanelPt } from '../../shared/lib/adminPanelPt'
import {
  adminDialogFooterActionsClass,
  adminDialogHeaderContentClass,
  adminDialogHeaderSplitClass,
  adminDialogScrollableBodyClass,
  adminDialogTitleClass,
} from '../../shared/lib/adminUiClasses'
import { usePromptConfigResource } from '../../prompt-config/promptConfigResource'
import type {
  FeaturePromptVariable,
  FeaturePromptVariableTone,
} from '../../prompt-config/promptConfigResource'
import {
  featureMap,
  groupedFeatureGroups,
  type FeatureGroup,
  type FeatureSlug,
} from './featurePromptMeta'

const adminFeedback = useAdminFeedback()
const { isScrolling: isPromptPanelScrolling, handleScroll: handlePromptPanelScroll } =
  useTransientScrolling()
const { isScrolling: isPromptDialogScrolling, handleScroll: handlePromptDialogScroll } =
  useTransientScrolling()
const promptConfigResource = usePromptConfigResource({
  feedback: adminFeedback.promptConfig,
  getFeatureTitle: (slug) => featureMap[slug as FeatureSlug]?.title ?? slug,
})
const {
  enabledMap,
  getPromptVariables,
  getSavedPrompt,
  promptMap,
  resetDrafts,
  savePrompt,
  saving,
} = promptConfigResource

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
  'max-w-full !h-7 !min-h-0 !rounded-full !border-transparent !px-2.5 !py-0 font-mono !text-admin-caption !font-[760] !leading-4'
const variableTokenButtonPt = {
  label: { class: 'break-all leading-4' },
} satisfies ButtonPassThroughOptions
const promptSlugTagPt = {
  label: { class: 'truncate' },
} satisfies TagPassThroughOptions
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
  return group.items.filter((item) => getSavedPrompt(item.slug).trim().length > 0).length
}

function getVariableTokenButtonClass(tone?: FeaturePromptVariableTone) {
  return [variableTokenButtonClass, variableTokenButtonToneClass[tone || 'context']]
}

function getVarsForSlug(slug: FeatureSlug): FeaturePromptVariable[] {
  return getPromptVariables(slug)
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

function resetGroupDraftFromSaved(group: FeatureGroup) {
  resetDrafts(group.items.map((item) => item.slug))
}

</script>
