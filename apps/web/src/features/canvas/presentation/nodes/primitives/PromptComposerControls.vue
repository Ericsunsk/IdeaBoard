<template>
  <textarea
    ref="inputRef"
    v-model="draft"
    name="canvas-prompt"
    :class="textareaClass"
    class="focus-visible:shadow-ib-focus"
    :rows="rows"
    :placeholder="placeholder"
    :aria-label="placeholder || t('canvas.controls.prompt')"
    :disabled="disabled"
    @focus="emit('focus')"
  />
  <div ref="actionsRef" :class="actionsClass">
    <NodeAspectRatioPicker
      v-if="shouldRenderAspectRatio"
      :model-value="aspectRatioValue"
      :visible="aspectRatioPickerVisible"
      :disabled="disabled"
      @toggle="toggleAspectRatioPicker"
      @select="selectAspectRatio"
    />
    <!-- @vue-generic {number} -->
    <NodeTextOptionPicker
      v-if="showOutputCount"
      :model-value="outputCountValue"
      :options="IMAGE_OUTPUT_COUNT_PRESET_OPTIONS"
      :visible="outputCountPickerVisible"
      :disabled="disabled"
      :tooltip="t('canvas.controls.selectImageCountTooltip')"
      :label="t('canvas.controls.imageCountLabel')"
      @toggle="toggleOutputCountPicker"
      @select="selectOutputCount"
    />
    <!-- @vue-generic {OutputResolution} -->
    <NodeTextOptionPicker
      v-if="showOutputResolution"
      :model-value="outputResolutionValue"
      :options="OUTPUT_RESOLUTION_OPTIONS"
      :visible="outputResolutionPickerVisible"
      :disabled="disabled"
      :format="formatOutputResolution"
      option-role="menuitemradio"
      :tooltip="t('canvas.controls.selectResolutionTooltip')"
      :label="t('canvas.controls.resolutionLabel')"
      @toggle="toggleOutputResolutionPicker"
      @select="selectOutputResolution"
    />
    <div v-if="showSubmit" class="canvas-credit-submit-control">
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
import { IMAGE_OUTPUT_COUNT_PRESET_OPTIONS, type PromptAspectRatio } from '../../../../board/ports/canvas'
import {
  OUTPUT_RESOLUTION_OPTIONS,
  type OutputResolution,
} from '../../../../../shared/domain/generationContract'
import { formatIdeaAmount } from '../../../../../shared/i18n/userIdeaCurrency'
import type { CanvasBillingQuoteState } from '../../../application/execution'
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
    disabled?: boolean
    allowEmpty?: boolean
    submitDisabled?: boolean
    showSubmit?: boolean
    showOutputCount?: boolean
    showOutputResolution?: boolean
    aspectRatio?: PromptAspectRatio | undefined
    showAspectRatio?: boolean
    autofocus?: boolean
    billingState?: CanvasBillingQuoteState | null
  }>(),
  {
    allowEmpty: false,
    aspectRatio: undefined,
    autofocus: false,
    billingState: null,
    disabled: false,
    outputResolution: undefined,
    rows: 1,
    showAspectRatio: true,
    showOutputCount: true,
    showOutputResolution: true,
    showSubmit: true,
    submitDisabled: false,
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
  aspectRatioPickerVisible,
  aspectRatioValue,
  canSubmit,
  draft,
  inputRef,
  outputCountPickerVisible,
  outputCountValue,
  outputResolutionPickerVisible,
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
  disabled: () => props.disabled,
  submitDisabled: () => props.submitDisabled,
  allowEmpty: () => props.allowEmpty,
  autofocus: () => props.autofocus,
  showAspectRatio: () => props.showAspectRatio,
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
  font-size: 10px;
  font-weight: 600;
  line-height: 14px;
  white-space: nowrap;
}

@media (hover: none), (pointer: coarse) {
  .canvas-credit-submit-control__touch-quote {
    display: inline-flex;
  }
}
</style>
