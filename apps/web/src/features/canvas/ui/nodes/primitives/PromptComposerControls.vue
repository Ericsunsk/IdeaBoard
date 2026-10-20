<template>
  <textarea
    ref="inputRef"
    v-model="draft"
    name="canvas-prompt"
    class="focus-visible:ring-2 focus-visible:ring-ring/20 focus-visible:outline-none"
    :class="textareaClass"
    :rows="rows"
    :placeholder="placeholder"
    :aria-label="placeholder || t('canvas.controls.prompt')"
    :disabled="isDisabled"
    @focus="emit('focus')"
  />
  <div ref="actionsRef" :class="actionsClass">
    <NodeAspectRatioPicker
      v-if="shouldRenderAspectRatio"
      :model-value="aspectRatioValue"
      :is-visible="isAspectRatioPickerVisible"
      :is-disabled="isDisabled"
      @toggle="toggleAspectRatioPicker"
      @select="selectAspectRatio"
    />
    <!-- @vue-generic {number} -->
    <NodeTextOptionPicker
      v-if="shouldShowOutputCount"
      :model-value="outputCountValue"
      :options="IMAGE_OUTPUT_COUNT_PRESET_OPTIONS"
      :is-visible="isOutputCountPickerVisible"
      :is-disabled="isDisabled"
      :tooltip="t('canvas.controls.selectImageCountTooltip')"
      :label="t('canvas.controls.imageCountLabel')"
      @toggle="toggleOutputCountPicker"
      @select="selectOutputCount"
    />
    <!-- @vue-generic {OutputResolution} -->
    <NodeTextOptionPicker
      v-if="shouldShowOutputResolution"
      :model-value="outputResolutionValue"
      :options="OUTPUT_RESOLUTION_OPTIONS"
      :is-visible="isOutputResolutionPickerVisible"
      :is-disabled="isDisabled"
      :format="formatOutputResolution"
      option-role="menuitemradio"
      :tooltip="t('canvas.controls.selectResolutionTooltip')"
      :label="t('canvas.controls.resolutionLabel')"
      @toggle="toggleOutputResolutionPicker"
      @select="selectOutputResolution"
    />
    <div v-if="shouldShowSubmit" class="canvas-credit-submit-control">
      <button
        :class="sendButtonClass"
        type="button"
        :disabled="!canSubmit"
        :aria-label="t('canvas.controls.send')"
        :title="billingTooltip"
        @click="submit"
      >
        <SendDiagonalSolid class="size-4 translate-x-[0.5px]" />
        <span class="canvas-tool-tooltip" aria-hidden="true">{{ billingTooltip }}</span>
      </button>
      <span
        v-if="touchQuoteLabel"
        class="canvas-credit-submit-control__touch-quote"
        data-touch-quote
      >
        {{ touchQuoteLabel }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { SendDiagonalSolid } from '@iconoir/vue'
import {
  IMAGE_OUTPUT_COUNT_PRESET_OPTIONS,
  type PromptAspectRatio,
} from '../../../../board/ports/canvas'
import {
  OUTPUT_RESOLUTION_OPTIONS,
  type OutputResolution,
} from '../../../../../core/contracts/generationContract'
import { formatIdeaAmount } from '../../../../../core/i18n/userIdeaCurrency'
import type { CanvasBillingQuoteState } from '../../../session/execution'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeTextOptionPicker from './NodeTextOptionPicker.vue'
import {
  usePromptComposerControls,
  type PromptComposerResizePayload,
} from './usePromptComposerControls'

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    outputResolution?: OutputResolution | undefined
    placeholder: string
    textareaClass: string
    actionsClass: string
    sendButtonClass: string
    rows?: number | string
    isDisabled?: boolean
    shouldAllowEmpty?: boolean
    isSubmitDisabled?: boolean
    shouldShowSubmit?: boolean
    shouldShowOutputCount?: boolean
    shouldShowOutputResolution?: boolean
    aspectRatio?: PromptAspectRatio | undefined
    shouldShowAspectRatio?: boolean
    isAutofocus?: boolean
    billingState?: CanvasBillingQuoteState | null
  }>(),
  {
    shouldAllowEmpty: false,
    aspectRatio: undefined,
    isAutofocus: false,
    billingState: null,
    isDisabled: false,
    outputResolution: undefined,
    rows: 1,
    shouldShowAspectRatio: true,
    shouldShowOutputCount: true,
    shouldShowOutputResolution: true,
    shouldShowSubmit: true,
    isSubmitDisabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:output-resolution', value: OutputResolution): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'focus'): void
  (e: 'textarea-resize', payload: PromptComposerResizePayload): void
  (e: 'submit', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const { t } = useI18n()

const quoteLabel = computed(() => {
  const state = props.billingState
  if (!state || state.status !== 'ready') return ''
  const unitTotal = state.unitCredits ?? 0
  if (unitTotal === 0) return t('canvas.controls.free')
  const total = unitTotal * outputCountValue.value
  if (!Number.isSafeInteger(total)) return ''
  return formatIdeaAmount(total)
})
const billingTooltip = computed(() =>
  quoteLabel.value
    ? `${t('canvas.controls.send')} · ${quoteLabel.value}`
    : t('canvas.controls.send'),
)
const touchQuoteLabel = computed(() => quoteLabel.value)
const formatOutputResolution = (value: OutputResolution) => value.toUpperCase()

const {
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
} = usePromptComposerControls({
  modelValue: () => props.modelValue,
  outputCount: () => props.outputCount,
  outputResolution: () => props.outputResolution ?? '1k',
  aspectRatio: () => props.aspectRatio,
  disabled: () => props.isDisabled,
  submitDisabled: () => props.isSubmitDisabled,
  allowEmpty: () => props.shouldAllowEmpty,
  autofocus: () => props.isAutofocus,
  showAspectRatio: () => props.shouldShowAspectRatio,
  onTextUpdate: (value) => emit('update:modelValue', value),
  onOutputCountUpdate: (value) => emit('update:output-count', value),
  onOutputResolutionUpdate: (value) => emit('update:output-resolution', value),
  onAspectRatioUpdate: (value) => emit('update:aspect-ratio', value),
  onTextareaResize: (payload) => emit('textarea-resize', payload),
  onSubmit: (prompt, outputCount, aspectRatio) => emit('submit', prompt, outputCount, aspectRatio),
})
</script>

<style scoped>
.canvas-credit-submit-control {
  position: relative;
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
}

.canvas-credit-submit-control__touch-quote {
  position: absolute;
  right: 0;
  bottom: calc(100% + 3px);
  display: none;
  color: var(--canvas-text-muted);
  font-size: var(--canvas-font-size-micro);
  font-weight: 600;
  line-height: var(--leading-caption);
  white-space: nowrap;
}

@media (hover: none), (pointer: coarse) {
  .canvas-credit-submit-control__touch-quote {
    display: inline-flex;
  }
}
</style>
