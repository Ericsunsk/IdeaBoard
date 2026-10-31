import { computed, nextTick, onMounted, ref } from 'vue'
import type { PromptAspectRatio } from '../../../../../board/ports/canvas'
import { normalizePromptAspectRatio } from '../../../../../board/ports/canvas'
import { requireImageNodeOutputCount } from '../../../../../board/ports/canvas'
import { useAutoResizeTextarea } from './useAutoResizeTextarea'
import { useNodeToolPopoverGroup } from '../tools/useNodeToolPopoverGroup'
import type { OutputResolution } from '../../../../../../core/contracts/generationContract'
import { normalizeOutputResolution } from '../../../../../../core/contracts/generationContract'

export type PromptComposerResizePayload = {
  naturalHeight: number
  singleLineHeight: number
}

type UsePromptComposerControlsOptions = {
  modelValue: () => string
  outputCount: () => number
  outputResolution: () => OutputResolution
  aspectRatio: () => PromptAspectRatio | undefined
  disabled: () => boolean
  submitDisabled: () => boolean
  allowEmpty: () => boolean
  autofocus: () => boolean
  showAspectRatio?: () => boolean
  onTextUpdate: (value: string) => void
  onOutputCountUpdate: (value: number) => void
  onOutputResolutionUpdate: (value: OutputResolution) => void
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
  const outputCountValue = computed(() => requireImageNodeOutputCount(options.outputCount()))
  const outputResolutionValue = computed(() =>
    normalizeOutputResolution(options.outputResolution()),
  )
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
    keys: ['aspect-ratio', 'output-count', 'output-resolution'] as const,
    rootRefs: [actionsRef],
  })
  const isAspectRatioPickerVisible = toolPopovers.isOpen('aspect-ratio')
  const isOutputCountPickerVisible = toolPopovers.isOpen('output-count')
  const isOutputResolutionPickerVisible = toolPopovers.isOpen('output-resolution')
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
    toolPopovers.close('output-resolution')
  }

  function submit() {
    if (!canSubmit.value) return
    options.onSubmit(draft.value.trim(), outputCountValue.value, aspectRatioValue.value)
    closePopovers()
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

  function toggleOutputResolutionPicker() {
    if (options.disabled()) return
    toolPopovers.toggle('output-resolution')
  }

  function selectOutputResolution(value: OutputResolution) {
    options.onOutputResolutionUpdate(value)
    toolPopovers.close('output-resolution')
  }

  function selectOutputCount(value: number) {
    options.onOutputCountUpdate(requireImageNodeOutputCount(value))
    toolPopovers.close('output-count')
  }

  return {
    actionsRef,
    isAspectRatioPickerVisible,
    aspectRatioValue,
    canSubmit,
    draft,
    inputRef,
    isOutputCountPickerVisible,
    outputCountValue,
    isOutputResolutionPickerVisible,
    outputResolutionValue,
    selectAspectRatio,
    selectOutputCount,
    selectOutputResolution,
    shouldRenderAspectRatio,
    submit,
    toggleAspectRatioPicker,
    toggleOutputCountPicker,
    toggleOutputResolutionPicker,
  }
}
