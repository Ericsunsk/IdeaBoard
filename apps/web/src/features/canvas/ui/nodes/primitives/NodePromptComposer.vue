<template>
  <form
    class="image-node-prompt-composer node-tool-surface absolute right-0 left-0 z-popover grid grid-cols-[minmax(0,1fr)_auto] gap-[var(--node-tool-button-gap)] rounded-panel bg-[var(--canvas-glass-bg)] py-[5px] pr-[5px] pl-[13px] backdrop-blur-[var(--canvas-glass-blur)] [border:var(--node-tool-border-width)_solid_var(--canvas-glass-border)] [top:calc(100%_+_var(--node-prompt-composer-offset))]"
    :class="{ 'count-stacked': shouldStackOutputCount, 'has-text': hasText }"
    :style="{ '--node-prompt-composer-offset': offset }"
    @submit.prevent
    @mousedown.stop
    @click.stop
    @contextmenu.stop.prevent
  >
    <PromptComposerControls
      :model-value="modelValue"
      :output-count="outputCount"
      :output-resolution="outputResolution"
      :aspect-ratio="aspectRatio"
      :placeholder="placeholder"
      :is-disabled="isDisabled"
      :should-allow-empty="shouldAllowEmpty"
      :is-submit-disabled="isSubmitDisabled"
      :should-show-submit="shouldShowSubmit"
      :should-show-aspect-ratio="shouldShowAspectRatio"
      :should-show-output-count="shouldShowOutputCount"
      :should-show-output-resolution="shouldShowOutputResolution"
      :billing-state="billingState"
      :textarea-class="promptInputClass"
      :actions-class="promptActionsClass"
      send-button-class="image-node-prompt-send node-tool-send node-tool-button canvas-tool-tooltip-trigger"
      @update:model-value="emit('update:modelValue', $event)"
      @update:output-count="emit('update:output-count', $event)"
      @update:output-resolution="emit('update:output-resolution', $event)"
      @update:aspect-ratio="emit('update:aspect-ratio', $event)"
      @textarea-resize="onTextareaResize"
      @submit="onSubmit"
    />
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PromptAspectRatio } from '../../../../board/ports/canvas'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import type { CanvasBillingQuoteState } from '../../../session/execution'
import PromptComposerControls from './PromptComposerControls.vue'
import type { PromptComposerResizePayload } from './usePromptComposerControls'

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    outputResolution?: OutputResolution | undefined
    placeholder: string
    isDisabled?: boolean
    shouldAllowEmpty?: boolean
    isSubmitDisabled?: boolean
    shouldShowSubmit?: boolean
    shouldShowOutputCount?: boolean
    shouldShowOutputResolution?: boolean
    aspectRatio?: PromptAspectRatio | undefined
    offset?: string
    billingState?: CanvasBillingQuoteState | null
  }>(),
  {
    shouldAllowEmpty: false,
    aspectRatio: undefined,
    isDisabled: false,
    offset: '8px',
    shouldShowSubmit: true,
    shouldShowOutputCount: true,
    shouldShowOutputResolution: true,
    isSubmitDisabled: false,
    billingState: null,
    outputResolution: '1k',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:output-resolution', value: OutputResolution): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'submit', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const shouldStackOutputCount = ref(false)
const shouldShowAspectRatio = computed(() => props.aspectRatio !== undefined)
const hasText = computed(() => props.modelValue.trim().length > 0)
const promptInputClass = computed(() =>
  [
    'image-node-prompt-input min-w-0 cursor-text resize-none overflow-hidden border-0 bg-transparent px-0 py-1.5 text-admin-meta leading-body text-[var(--canvas-node-text-primary)] outline-none placeholder:text-[var(--canvas-node-text-muted)] disabled:cursor-wait',
    shouldStackOutputCount.value ? 'min-h-[73px] self-end' : 'self-center',
  ]
    .filter(Boolean)
    .join(' '),
)
const promptActionsClass = computed(() =>
  [
    'image-node-prompt-actions flex items-center gap-[var(--node-tool-button-gap)]',
    shouldStackOutputCount.value ? 'self-end flex-col' : 'self-center',
  ]
    .filter(Boolean)
    .join(' '),
)

function onTextareaResize(payload: PromptComposerResizePayload) {
  shouldStackOutputCount.value = payload.naturalHeight > payload.singleLineHeight + 2
}

function onSubmit(prompt: string, outputCount: number, aspectRatio: PromptAspectRatio) {
  emit('submit', prompt, outputCount, aspectRatio)
}
</script>

<style scoped>
.image-node-prompt-composer {
  align-items: center;
  animation: image-node-prompt-rise var(--motion-enter-duration) var(--motion-enter-easing);
  transition: border-color var(--motion-control-duration) var(--motion-control-easing);
}

.image-node-prompt-composer:focus-within,
.image-node-prompt-composer.has-text {
  border-color: var(--focus-border-color);
}

.image-node-prompt-composer.count-stacked {
  align-items: end;
  min-height: 73px;
}

@keyframes image-node-prompt-rise {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .image-node-prompt-composer {
    animation: none;
  }
}
</style>
