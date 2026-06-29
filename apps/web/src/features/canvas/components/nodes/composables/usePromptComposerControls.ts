import { computed, nextTick, onMounted, ref } from 'vue'
import type { PromptAspectRatio } from '../../../../canvas-board/domain'
import { normalizePromptAspectRatio } from '../../../../canvas-board/domain'
import { clampImageNodeOutputCount } from '../../../../canvas-board/domain'
import { useAutoResizeTextarea } from './useAutoResizeTextarea'
import { useNodeToolPopoverGroup } from './useNodeToolPopoverGroup'

type PromptComposerResizePayload = {
  naturalHeight: number
  singleLineHeight: number
}

type UsePromptComposerControlsOptions = {
  modelValue: () => string
  outputCount: () => number
  aspectRatio: () => PromptAspectRatio | undefined
  disabled: () => boolean
  submitDisabled: () => boolean
  allowEmpty: () => boolean
  autofocus: () => boolean
  showAspectRatio?: () => boolean
  onTextUpdate: (value: string) => void
  onOutputCountUpdate: (value: number) => void
  onAspectRatioUpdate: (value: PromptAspectRatio) => void
  onSubmit: (prompt: string, outputCount: number, aspectRatio: PromptAspectRatio) => void
  onTextareaResize?: (payload: PromptComposerResizePayload) => void
}

export function usePromptComposerControls(options: UsePromptComposerControlsOptions) {
  const inputRef = ref<HTMLTextAreaElement | null>(null)
  const actionsRef = ref<HTMLElement | null>(null)
  const draft = computed({
    get: () => options.modelValue(),
    set: (value) => options.onTextUpdate(value),
  })
  const outputCountValue = computed(() => clampImageNodeOutputCount(options.outputCount()))
  const shouldRenderAspectRatio = computed(
    () => (options.showAspectRatio?.() ?? true) && options.aspectRatio() !== undefined,
  )
  const aspectRatioValue = computed(() => normalizePromptAspectRatio(options.aspectRatio()))
  const canSubmit = computed(
    () =>
      !options.disabled() &&
      !options.submitDisabled() &&
      (options.allowEmpty() || draft.value.trim().length > 0),
  )
  const toolPopovers = useNodeToolPopoverGroup({
    keys: ['aspect-ratio', 'output-count'] as const,
    rootRefs: [actionsRef],
  })
  const aspectRatioPickerVisible = toolPopovers.isOpen('aspect-ratio')
  const outputCountPickerVisible = toolPopovers.isOpen('output-count')
  const textarea = useAutoResizeTextarea(inputRef, {
    watchSource: options.modelValue,
    onResize: options.onTextareaResize,
  })

  onMounted(async () => {
    await nextTick()
    textarea.resize()
    if (options.autofocus()) inputRef.value?.focus()
  })

  function closePopovers() {
    toolPopovers.close('aspect-ratio')
    toolPopovers.close('output-count')
  }

  function submit() {
    if (!canSubmit.value) return
    options.onSubmit(draft.value.trim(), outputCountValue.value, aspectRatioValue.value)
    closePopovers()
  }

  function onEnter(event: KeyboardEvent) {
    if (event.isComposing) return
    event.preventDefault()
    submit()
  }

  function toggleAspectRatioPicker() {
    if (options.disabled() || !shouldRenderAspectRatio.value) return
    toolPopovers.toggle('aspect-ratio')
  }

  function selectAspectRatio(value: PromptAspectRatio) {
    options.onAspectRatioUpdate(value)
    toolPopovers.close('aspect-ratio')
  }

  function toggleOutputCountPicker() {
    if (options.disabled()) return
    toolPopovers.toggle('output-count')
  }

  function selectOutputCount(value: number) {
    options.onOutputCountUpdate(clampImageNodeOutputCount(value))
    toolPopovers.close('output-count')
  }

  return {
    actionsRef,
    aspectRatioPickerVisible,
    aspectRatioValue,
    canSubmit,
    draft,
    inputRef,
    onEnter,
    outputCountPickerVisible,
    outputCountValue,
    selectAspectRatio,
    selectOutputCount,
    shouldRenderAspectRatio,
    submit,
    toggleAspectRatioPicker,
    toggleOutputCountPicker,
  }
}
