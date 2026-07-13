<template>
  <textarea
    ref="inputRef"
    v-model="draft"
    :class="textareaClass"
    :rows="rows"
    :placeholder="placeholder"
    :disabled="disabled"
    @focus="emit('focus')"
    @keydown.enter.exact="onEnter"
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
      :model-value="outputCountValue"
      :visible="outputCountPickerVisible"
      :disabled="disabled"
      tooltip="选择张数"
      aria-label="输出张数"
      @toggle="toggleOutputCountPicker"
      @select="selectOutputCount"
    />
    <div class="canvas-credit-submit-control">
      <CanvasCreditQuoteBadge
        class="canvas-credit-submit-control__quote"
        :state="billingState ?? null"
      />
      <button
        :class="sendButtonClass"
        type="button"
        :disabled="!canSubmit"
        aria-label="发送"
        :title="billingBlockedReason"
        @click="submit"
      >
        <SendDiagonalSolid class="size-4 translate-x-[0.5px]" />
        <span class="node-tool-tooltip" aria-hidden="true">发送</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { SendDiagonalSolid } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import { isCanvasBillingSubmitBlocked, type CanvasBillingQuoteState } from '../../execution'
import CanvasCreditQuoteBadge from './CanvasCreditQuoteBadge.vue'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import {
  usePromptComposerControls,
  type PromptComposerResizePayload,
} from './composables/usePromptComposerControls'

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    placeholder: string
    textareaClass: string
    actionsClass: string
    sendButtonClass: string
    rows?: number | string
    disabled?: boolean
    allowEmpty?: boolean
    submitDisabled?: boolean
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
    rows: 1,
    showAspectRatio: true,
    submitDisabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'focus'): void
  (e: 'textarea-resize', payload: PromptComposerResizePayload): void
  (e: 'submit', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const billingSubmitBlocked = computed(() =>
  isCanvasBillingSubmitBlocked(props.billingState ?? null),
)
const billingBlockedReason = computed(() =>
  billingSubmitBlocked.value ? props.billingState?.reason || '正在获取Idea价格' : undefined,
)

const {
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
} = usePromptComposerControls({
  modelValue: () => props.modelValue,
  outputCount: () => props.outputCount,
  aspectRatio: () => props.aspectRatio,
  disabled: () => props.disabled,
  submitDisabled: () => props.submitDisabled || billingSubmitBlocked.value,
  allowEmpty: () => props.allowEmpty,
  autofocus: () => props.autofocus,
  showAspectRatio: () => props.showAspectRatio,
  onTextUpdate: (value) => emit('update:modelValue', value),
  onOutputCountUpdate: (value) => emit('update:output-count', value),
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

.canvas-credit-submit-control__quote {
  position: absolute;
  right: 0;
  bottom: calc(100% + 3px);
  max-width: 86px;
}
</style>
