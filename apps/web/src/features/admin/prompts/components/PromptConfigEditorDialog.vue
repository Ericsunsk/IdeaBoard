<template>
  <!-- 提示词编辑弹窗 (遵循 AdminDialogShell Token，单功能独立弹窗) -->
  <AdminDialogShell
    :visible="Boolean(item)"
    :header="header"
    description="编辑该功能的系统提示词模板后保存。"
    size="form"
    content-layout="plain"
    :busy="saving"
    @update:visible="onVisibleUpdate"
  >
    <div v-if="item" class="flex flex-col gap-4">
      <!-- 提示词编辑与变量注入 -->
      <div class="flex flex-col gap-1.5">
        <div class="flex items-center justify-between">
          <label
            class="text-admin-meta font-[680] text-admin-text-2"
            :for="`prompt-textarea-${item.slug}`"
          >
            系统提示词
          </label>
          <!-- 变量快捷填充标签 -->
          <div v-if="variables.length > 0" class="flex min-w-0 flex-wrap items-center gap-1.5">
            <span class="text-[11px] font-medium text-admin-text-3">插入变量:</span>
            <button
              v-for="variable in variables"
              :key="variable.key"
              type="button"
              :disabled="saving"
              class="rounded-md border border-admin-border-soft bg-admin-surface-muted/50 px-1.5 py-0.5 font-mono text-[11px] font-medium text-admin-text-2 transition-colors hover:border-admin-accent/40 hover:bg-admin-accent/5 hover:text-admin-accent cursor-pointer disabled:opacity-50"
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
          :aria-label="item.title + ' 提示词模板'"
          placeholder="输入系统提示词模板，可点击上方变量按钮插入..."
          :disabled="saving"
          @update:model-value="emit('update:draft', $event)"
        />
      </div>

      <!-- 字符统计与校验提示信息 -->
      <div class="flex items-center justify-between text-admin-meta text-admin-text-3 font-mono">
        <span>{{ draft.length }} 字符</span>
        <span
          v-if="validation?.severity === 'error' && validation?.message"
          class="text-admin-danger font-sans text-xs"
        >
          {{ validation?.message }}
        </span>
        <span
          v-else-if="validation?.severity === 'warning' && validation?.message"
          class="text-admin-warning font-sans text-xs"
        >
          {{ validation?.message }}
        </span>
      </div>
    </div>

    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="saving"
        @click="emit('close')"
      >
        取消
      </AdminButton>
      <AdminButton
        v-if="item"
        :class="adminActionButtonClass"
        :loading="saving"
        :disabled="saving || !validation?.canSave || !validation?.isDirty"
        @click="emit('save')"
      >
        保存
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>

<script setup lang="ts">
import { nextTick, shallowRef, watch } from 'vue'
import AdminButton from '../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../shared/components/AdminDialogShell.vue'
import AdminTextarea from '../../shared/components/AdminTextarea.vue'
import { adminActionButtonClass } from '../../shared/lib/uiClasses'
import type { FeaturePromptVariable, PromptDraftValidation } from '../workspace'
import type { FeatureItem } from '../featurePromptMeta'

type TextareaRefLike = HTMLTextAreaElement | { $el?: Element }

const props = defineProps<{
  item: FeatureItem | null
  draft: string
  validation: PromptDraftValidation | null
  saving: boolean
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
