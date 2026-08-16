<template>
  <textarea
    ref="inputRef"
    v-model="draft"
    name="canvas-prompt"
    :class="textareaClass"
    :rows="rows"
    :placeholder="placeholder"
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
    <NodeOutputCountPicker
      v-if="showOutputCount"
      :model-value="outputCountValue"
      :visible="outputCountPickerVisible"
      :disabled="disabled"
      tooltip="选择张数"
      aria-label="输出张数"
      @toggle="toggleOutputCountPicker"
      @select="selectOutputCount"
    />
    <NodeOutputResolutionPicker
      :model-value="outputResolutionValue"
      :visible="outputResolutionPickerVisible"
      :disabled="disabled"
      @toggle="toggleOutputResolutionPicker"
      @select="selectOutputResolution"
    />
    <div v-if="showSubmit" class="canvas-credit-submit-control">
      <button
        :class="sendButtonClass"
        type="button"
        :disabled="!canSubmit"
        aria-label="发送"
        :title="billingTooltip"
        @click="submit"
      >
        <SendDiagonalSolid class="size-4 translate-x-[0.5px]" />
        <span class="node-tool-tooltip" aria-hidden="true">{{ billingTooltip }}</span>
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
import { SendDiagonalSolid } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import type { OutputResolution } from '../../../../shared/domain/generationContract'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import type { CanvasBillingQuoteState } from '../../execution'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import NodeOutputResolutionPicker from './NodeOutputResolutionPicker.vue'
import {
  usePromptComposerControls,
  type PromptComposerResizePayload,
} from './composables/usePromptComposerControls'

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    outputResolution?: OutputResolution
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
    aspectRatio?: PromptAspectRatio
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

const quoteLabel = computed(() => {
  const state = props.billingState
  if (!state || state.status !== 'ready') return ''
  const unitTotal = state.unitCredits ?? 0
  if (unitTotal === 0) return '免费'
  const total = unitTotal * outputCountValue.value
  if (!Number.isSafeInteger(total)) return ''
  return formatIdeaAmount(total)
})
const billingTooltip = computed(() => (quoteLabel.value ? `发送 · ${quoteLabel.value}` : '发送'))
const touchQuoteLabel = computed(() => quoteLabel.value)

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
