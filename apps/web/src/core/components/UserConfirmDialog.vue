<template>
  <DialogRoot :open="props.isVisible" @update:open="handleOpenUpdate">
    <DialogPortal>
      <DialogOverlay @click="handleMaskClick" />
      <DialogContent
        class="bg-popover"
        :style="{ width: 'min(420px, calc(100vw - 32px))' }"
        :aria-label="props.title"
        :aria-busy="props.isLoading || undefined"
        @escape-key-down="handleEscape"
        @pointer-down-outside="blockOutsideDismiss"
      >
        <div class="min-h-0 overflow-y-auto">
          <DialogDescription class="sr-only">
            {{ props.message }}
          </DialogDescription>
          <div class="p-6">
            <DialogTitle as="h3" class="m-0 text-lg font-semibold text-foreground">
              {{ props.title }}
            </DialogTitle>
            <p class="mt-5 mb-0 text-sm leading-6 text-muted-foreground">
              {{ props.message }}
            </p>
            <div v-if="props.expectedConfirmText" class="mt-4 flex flex-col gap-2">
              <p class="m-0 text-sm font-medium text-foreground">
                <template v-for="(part, index) in confirmLabelParts" :key="index">
                  <button
                    v-if="part.isName"
                    type="button"
                    class="inline-flex max-w-full items-center gap-1 rounded-control border border-border bg-muted px-1.5 py-0.5 align-baseline font-mono text-xs text-foreground hover:bg-accent"
                    :title="t('common.actions.copy')"
                    :aria-label="t('common.actions.copy')"
                    @click="copyExpectedText"
                  >
                    <Copy class="size-3 shrink-0" aria-hidden="true" />
                    <span class="truncate">{{
                      isCopyFeedbackVisible ? t('common.actions.copied') : part.text
                    }}</span>
                  </button>
                  <template v-else>{{ part.text }}</template>
                </template>
              </p>
              <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -- The typed-confirm input carries its accessible name via aria-label -->
              <Input
                :id="confirmInputId"
                v-model="confirmInput"
                size="sm"
                class="h-9 text-sm bg-background"
                :disabled="props.isLoading"
                :aria-label="props.confirmInputLabel || props.expectedConfirmText"
                :placeholder="props.confirmInputPlaceholder"
                @keydown.enter="handleConfirmInputEnter"
              />
            </div>
            <div class="mt-6 flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                :is-disabled="props.isLoading"
                class="rounded-pill px-4 py-2 text-sm font-semibold text-muted-foreground hover:bg-muted hover:text-foreground"
                @click="emit('update:isVisible', false)"
              >
                {{ t('common.actions.cancel') }}
              </Button>
              <Button
                type="button"
                :is-disabled="props.isLoading || !isConfirmTextSatisfied"
                :class="[
                  'rounded-pill px-4 py-2 text-sm font-semibold disabled:opacity-50',
                  props.intent === 'danger'
                    ? 'bg-destructive text-destructive-foreground hover:bg-destructive-hover'
                    : 'bg-primary text-primary-foreground hover:bg-primary-hover',
                ]"
                @click="emit('confirm')"
              >
                {{ props.confirmLabel || t('common.actions.confirm') }}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </DialogPortal>
  </DialogRoot>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy } from '@iconoir/vue'
import { useAppClipboard } from '@/core/composables/useAppClipboard'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../core/composables/useOverlayInteractionLifecycle'

const props = withDefaults(
  defineProps<{
    isVisible: boolean
    title: string
    message: string
    confirmLabel?: string
    intent?: 'primary' | 'danger'
    isLoading?: boolean
    expectedConfirmText?: string | undefined
    confirmInputLabel?: string | undefined
    confirmInputPlaceholder?: string | undefined
  }>(),
  {
    confirmLabel: '',
    intent: 'primary',
    isLoading: false,
    expectedConfirmText: undefined,
    confirmInputLabel: '',
    confirmInputPlaceholder: '',
  },
)

const emit = defineEmits<{
  'update:isVisible': [visible: boolean]
  confirm: []
}>()

const { t } = useI18n()

// Typed confirmation for irreversible actions (e.g. workspace permanent
// delete): the caller passes the exact expected text and the confirm control
// stays disabled until the input matches. Callers without
// `expectedConfirmText` keep the direct confirm.
const confirmInputId = 'user-confirm-typed-input'
const confirmInput = ref('')
watch(
  () => props.isVisible,
  () => {
    confirmInput.value = ''
  },
)
const isConfirmTextSatisfied = computed(() => {
  const expected = props.expectedConfirmText
  if (!expected) return true
  return confirmInput.value === expected
})

type ConfirmLabelPart = { text: string; isName: boolean }

// The label sentence embeds the expected name as an inline copy target
// (a button cannot live inside a <label>, so the sentence renders as text
// and the input keeps its aria-label association).
const confirmLabelParts = computed<ConfirmLabelPart[]>(() => {
  const expected = props.expectedConfirmText
  const label = props.confirmInputLabel || ''
  if (!expected || !label.includes(expected)) return [{ text: label, isName: false }]
  const parts: ConfirmLabelPart[] = []
  for (const [index, chunk] of label.split(expected).entries()) {
    if (index > 0) parts.push({ text: expected, isName: true })
    if (chunk) parts.push({ text: chunk, isName: false })
  }
  return parts
})

function handleConfirmInputEnter() {
  if (props.isLoading || !isConfirmTextSatisfied.value) return
  emit('confirm')
}

const { copyText } = useAppClipboard()
const isCopyFeedbackVisible = ref(false)
let copyFeedbackTimer: ReturnType<typeof setTimeout> | undefined
onUnmounted(() => {
  if (copyFeedbackTimer !== undefined) clearTimeout(copyFeedbackTimer)
})

async function copyExpectedText(): Promise<void> {
  const expected = props.expectedConfirmText
  if (!expected) return
  if (!(await copyText(expected))) return
  isCopyFeedbackVisible.value = true
  if (copyFeedbackTimer !== undefined) clearTimeout(copyFeedbackTimer)
  copyFeedbackTimer = setTimeout(() => {
    isCopyFeedbackVisible.value = false
    copyFeedbackTimer = undefined
  }, 1500)
}

// Unified dialog contract: mask/Escape/close-control dismiss while idle; the
// irreversible confirm commit (loading) locks every path. Promise settlement
// on parent unmount stays in useUserConfirmDialog.dispose.
const { canDismiss, requestClose } = useOverlayInteractionLifecycle({
  isVisible: () => props.isVisible,
  busy: () => props.isLoading,
  surface: defineDialogSurfaceIntent(),
  onVisibilityChange: (visible) => {
    if (!visible) emit('update:isVisible', false)
  },
})

function handleOpenUpdate(open: boolean) {
  if (!open) requestClose('close-control')
}

function handleMaskClick() {
  requestClose('mask')
}

function handleEscape(event: KeyboardEvent) {
  if (!canDismiss('escape')) event.preventDefault()
}

function blockOutsideDismiss(event: Event) {
  event.preventDefault()
}
</script>
