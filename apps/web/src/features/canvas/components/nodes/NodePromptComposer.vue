<template>
  <form
    class="image-node-prompt-composer node-tool-surface absolute right-0 left-0 z-[42] grid grid-cols-[minmax(0,1fr)_auto] items-end gap-[var(--node-tool-button-gap)] rounded-[var(--canvas-floating-radius)] bg-white/[0.96] py-[5px] pr-[5px] pl-[13px] backdrop-blur-[var(--canvas-surface-blur)] [border:var(--node-tool-border-width)_solid_var(--node-tool-button-border)] [top:calc(100%_+_var(--node-prompt-composer-offset))]"
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
      :aspect-ratio="aspectRatio"
      :placeholder="placeholder"
      :disabled="disabled"
      :allow-empty="allowEmpty"
      :submit-disabled="submitDisabled"
      :show-aspect-ratio="showAspectRatio"
      :textarea-class="promptInputClass"
      :actions-class="promptActionsClass"
      send-button-class="image-node-prompt-send node-tool-send node-tool-button node-tool-tooltip-trigger"
      @update:model-value="emit('update:modelValue', $event)"
      @update:output-count="emit('update:output-count', $event)"
      @update:aspect-ratio="emit('update:aspect-ratio', $event)"
      @textarea-resize="onTextareaResize"
      @submit="onSubmit"
    />
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import PromptComposerControls from './PromptComposerControls.vue'
import type { PromptComposerResizePayload } from './composables/usePromptComposerControls'

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    placeholder: string
    disabled?: boolean
    allowEmpty?: boolean
    submitDisabled?: boolean
    aspectRatio?: PromptAspectRatio
    offset?: string
  }>(),
  {
    allowEmpty: false,
    aspectRatio: undefined,
    disabled: false,
    offset: '8px',
    submitDisabled: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'submit', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const shouldStackOutputCount = ref(false)
const showAspectRatio = computed(() => props.aspectRatio !== undefined)
const hasText = computed(() => props.modelValue.trim().length > 0)
const promptInputClass = computed(() =>
  [
    'image-node-prompt-input min-w-0 cursor-text self-end resize-none overflow-hidden border-0 bg-transparent px-0 py-1.5 text-xs leading-[18px] text-[#272932] outline-none placeholder:text-[#9aa4b2] disabled:cursor-wait',
    shouldStackOutputCount.value && 'min-h-[73px]',
  ]
    .filter(Boolean)
    .join(' '),
)
const promptActionsClass = computed(() =>
  [
    'image-node-prompt-actions flex self-end items-center gap-[var(--node-tool-button-gap)]',
    shouldStackOutputCount.value && 'flex-col',
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

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.image-node-prompt-composer {
  animation: image-node-prompt-rise var(--canvas-motion-fast) ease-out;
  transition: border-color var(--canvas-motion-fast) ease;
}

.image-node-prompt-composer:focus-within,
.image-node-prompt-composer.has-text {
  border-color: var(--ib-focus-border-color);
}

.image-node-prompt-composer.count-stacked {
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
</style>
