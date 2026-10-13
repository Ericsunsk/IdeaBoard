<template>
  <!-- 提示词编辑弹窗 (遵循 AdminDialogShell Token，单功能独立弹窗) -->
  <AdminDialogShell
    :visible="Boolean(item)"
    :header="header"
    :description="PROMPTS_MESSAGES.editorDescription"
    size="form"
    content-layout="plain"
    :busy="isSaving"
    @update:visible="onVisibleUpdate"
  >
    <div v-if="item" class="flex flex-col gap-4">
      <!-- 提示词编辑与变量注入 -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-meta font-admin-emphasis text-muted-foreground"
            :for="`prompt-textarea-${item.slug}`"
          >
            {{ PROMPTS_MESSAGES.editorSystemLabel }}
          </label>
          <!-- 变量快捷填充标签 -->
          <div v-if="variables.length > 0" class="flex min-w-0 flex-wrap items-center gap-1.5">
            <span class="text-admin-caption font-medium text-muted-foreground">{{
              PROMPTS_MESSAGES.editorInsertVariables
            }}</span>
            <button
              v-for="variable in variables"
              :key="variable.key"
              type="button"
              :disabled="isSaving"
              class="rounded-control border border-border bg-muted/50 px-1.5 py-0.5 font-mono text-admin-caption font-medium text-muted-foreground transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:border-primary-line-strong hover:bg-primary-faint-soft hover:text-primary cursor-pointer disabled:opacity-50"
              @click="insertVariable(variable.key)"
            >
              {{ variable.key }}
            </button>
          </div>
        </div>

        <AdminTextarea
          :id="`prompt-textarea-${item.slug}`"
          :ref="bindTextareaRef"
          :model-value="draft"
          class="w-full text-xs font-mono min-h-[160px] max-h-96"
          :rows="6"
          auto-grow
          :aria-label="PROMPTS_MESSAGES.editorTemplateAria(item.title)"
          :placeholder="PROMPTS_MESSAGES.editorPlaceholder"
          :disabled="isSaving"
          @update:model-value="emit('update:draft', $event)"
        />
      </div>

      <!-- 字符统计与校验提示信息 -->
      <div
        class="flex items-center justify-between text-admin-meta text-muted-foreground font-mono"
      >
        <span>{{ PROMPTS_MESSAGES.editorCharCount(draft.length) }}</span>
        <span
          v-if="validation?.severity === 'error' && validation?.message"
          class="text-destructive font-sans text-xs"
        >
          {{ validation?.message }}
        </span>
        <span
          v-else-if="validation?.severity === 'warning' && validation?.message"
          class="text-warning font-sans text-xs"
        >
          {{ validation?.message }}
        </span>
      </div>
    </div>

    <template #footer>
      <AdminDialogActions
        :cancel-label="PROMPTS_MESSAGES.cancel"
        :confirm-label="PROMPTS_MESSAGES.save"
        :loading="isSaving"
        :disabled="isSaving || !validation?.canSave || !validation?.isDirty"
        :cancel-disabled="isSaving"
        @cancel="emit('close')"
        @confirm="emit('save')"
      />
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { nextTick, shallowRef, watch } from 'vue'
import { AdminDialogActions } from '../../../shared'
import { AdminDialogShell } from '../../../shared'
import { AdminTextarea } from '../../../shared'
import { PROMPTS_MESSAGES } from '../pages/messages'
import type { FeaturePromptVariable, PromptDraftValidation } from '../../session/workspace'
import type { FeatureItem } from '../../core/featurePromptMeta'

type TextareaRefLike = HTMLTextAreaElement | { $el?: Element }

const props = defineProps<{
  item: FeatureItem | null
  draft: string
  validation: PromptDraftValidation | null
  isSaving: boolean
  variables: readonly FeaturePromptVariable[]
  header: string
}>()
const emit = defineEmits<{
  'update:draft': [value: string]
  save: []
  close: []
  'insert-variable': [payload: { key: string; position: number }]
}>()

const textareaRef = shallowRef<TextareaRefLike | null>(null)

function bindTextareaRef(element: unknown) {
  textareaRef.value = (element as TextareaRefLike | null) ?? null
}

function resolveTextareaElement(ref: TextareaRefLike | null | undefined) {
  if (!ref) return null
  if (ref instanceof HTMLTextAreaElement) return ref
  const root = ref.$el
  if (root instanceof HTMLTextAreaElement) return root
  return root?.querySelector?.('textarea') ?? null
}

function insertVariable(key: string) {
  const textareaElement = resolveTextareaElement(textareaRef.value)
  const position =
    textareaElement instanceof HTMLTextAreaElement
      ? (textareaElement.selectionStart ?? props.draft.length)
      : props.draft.length
  emit('insert-variable', { key, position })
}

function onVisibleUpdate(visible: boolean) {
  if (!visible) emit('close')
}

watch(
  () => props.item,
  (item) => {
    if (!item) return
    void nextTick(() => {
      resolveTextareaElement(textareaRef.value)?.focus()
    })
  },
)
</script>
