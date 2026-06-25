<template>
  <form
    class="image-node-prompt-composer node-tool-surface absolute right-0 left-0 z-[42] grid grid-cols-[minmax(0,1fr)_auto] items-end gap-[var(--node-tool-button-gap)] rounded-[13px] bg-white/[0.96] py-[5px] pr-[5px] pl-[13px] backdrop-blur-[10px] [border:var(--node-tool-border-width)_solid_rgba(122,107,213,0.28)] [top:calc(100%_+_var(--node-prompt-composer-offset))]"
    :class="{ 'count-stacked': shouldStackOutputCount, 'has-text': draft.trim() }"
    :style="{ '--node-prompt-composer-offset': offset }"
    @submit.prevent="submit"
    @mousedown.stop
    @click.stop
    @contextmenu.stop.prevent
  >
    <textarea
      ref="promptInputRef"
      v-model="draft"
      class="image-node-prompt-input min-w-0 cursor-text self-end resize-none overflow-hidden border-0 bg-transparent px-0 py-1.5 text-xs leading-[18px] text-[#272932] outline-none placeholder:text-[#9aa4b2] disabled:cursor-wait"
      rows="1"
      :placeholder="placeholder"
      :disabled="disabled"
      @keydown.enter.exact="onPromptEnter"
    />
    <div
      ref="actionsRef"
      class="image-node-prompt-actions flex self-end items-center gap-[var(--node-tool-button-gap)]"
    >
      <NodeAspectRatioPicker
        v-if="showAspectRatio"
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
      <button
        class="image-node-prompt-send node-tool-send node-tool-button node-tool-tooltip-trigger"
        type="submit"
        :disabled="!canSubmit"
        aria-label="发送"
      >
        <SendDiagonalSolid class="size-4 translate-x-[0.5px]" />
        <span class="node-tool-tooltip" aria-hidden="true">发送</span>
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { SendDiagonalSolid } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../config/promptAspectRatio'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { usePromptComposerControls } from './composables/usePromptComposerControls'

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

const {
  actionsRef,
  aspectRatioPickerVisible,
  aspectRatioValue,
  canSubmit,
  draft,
  inputRef: promptInputRef,
  onEnter: onPromptEnter,
  outputCountPickerVisible,
  outputCountValue,
  selectAspectRatio,
  selectOutputCount,
  submit,
  toggleAspectRatioPicker,
  toggleOutputCountPicker,
} = usePromptComposerControls({
  modelValue: () => props.modelValue,
  outputCount: () => props.outputCount,
  aspectRatio: () => props.aspectRatio,
  disabled: () => props.disabled,
  submitDisabled: () => props.submitDisabled,
  allowEmpty: () => props.allowEmpty,
  autofocus: () => true,
  showAspectRatio: () => showAspectRatio.value,
  onTextUpdate: (value) => emit('update:modelValue', value),
  onOutputCountUpdate: (value) => emit('update:output-count', value),
  onAspectRatioUpdate: (value) => emit('update:aspect-ratio', value),
  onSubmit: (prompt, outputCount, aspectRatio) => emit('submit', prompt, outputCount, aspectRatio),
  onTextareaResize: ({ naturalHeight, singleLineHeight }) => {
    shouldStackOutputCount.value = naturalHeight > singleLineHeight + 2
  },
})
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.image-node-prompt-composer {
  animation: image-node-prompt-rise 0.16s ease-out;
}

.image-node-prompt-composer.node-tool-surface {
  --node-tool-button-border: #e2e8f0;
}

.image-node-prompt-composer.has-text {
  border-color: #9278e6;
}

.image-node-prompt-composer.count-stacked {
  min-height: 73px;
}

.image-node-prompt-composer.count-stacked .image-node-prompt-input {
  min-height: 73px;
}

.image-node-prompt-composer.count-stacked .image-node-prompt-actions {
  flex-direction: column;
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
