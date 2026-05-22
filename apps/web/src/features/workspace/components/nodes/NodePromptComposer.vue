<template>
  <form
    class="image-node-prompt-composer node-tool-surface"
    :class="{ 'count-stacked': shouldStackOutputCount, 'has-text': draft.trim() }"
    :style="{ '--node-prompt-composer-offset': offset }"
    @submit.prevent="submitPrompt"
    @mousedown.stop
    @click.stop
    @contextmenu.stop.prevent
  >
    <textarea
      ref="promptInputRef"
      v-model="draft"
      class="image-node-prompt-input"
      rows="1"
      :placeholder="placeholder"
      :disabled="disabled"
      @keydown.enter.exact="onPromptEnter"
    />
    <div ref="actionsRef" class="image-node-prompt-actions">
      <NodeAspectRatioPicker
        v-if="showAspectRatio"
        :model-value="aspectRatioValue"
        :visible="aspectRatioPickerVisible"
        :disabled="disabled"
        @toggle="toggleAspectRatioPicker"
        @select="selectAspectRatio"
      />
      <NodeOutputCountPicker
        :model-value="outputCount"
        :visible="outputCountPickerVisible"
        :disabled="disabled"
        title="选择张数"
        aria-label="输出张数"
        @toggle="toggleOutputCountPicker"
        @select="selectOutputCount"
      />
      <button
        class="image-node-prompt-send node-tool-send node-tool-button"
        type="submit"
        :disabled="submitButtonDisabled"
        title="发送"
      >
        <Send class="node-send-icon" />
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { Send } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../config/promptAspectRatio'
import { normalizePromptAspectRatio } from '../../config/promptAspectRatio'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { useAutoResizeTextarea } from './composables/useAutoResizeTextarea'
import { useNodeToolPopoverGroup } from './composables/useNodeToolPopoverGroup'

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

const promptInputRef = ref<HTMLTextAreaElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const shouldStackOutputCount = ref(false)
const toolPopovers = useNodeToolPopoverGroup({
  keys: ['aspect-ratio', 'output-count'] as const,
  rootRefs: [actionsRef],
})
const showAspectRatio = computed(() => props.aspectRatio !== undefined)
const aspectRatioValue = computed(() => normalizePromptAspectRatio(props.aspectRatio))
const aspectRatioPickerVisible = toolPopovers.isOpen('aspect-ratio')
const outputCountPickerVisible = toolPopovers.isOpen('output-count')
const draft = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})
const submitButtonDisabled = computed(
  () => props.disabled || props.submitDisabled || (!props.allowEmpty && !draft.value.trim()),
)
const promptTextarea = useAutoResizeTextarea(promptInputRef, {
  watchSource: () => props.modelValue,
  onResize: ({ naturalHeight, singleLineHeight }) => {
    shouldStackOutputCount.value = naturalHeight > singleLineHeight + 2
  },
})

onMounted(async () => {
  await nextTick()
  promptTextarea.resize()
  promptInputRef.value?.focus()
})

function submitPrompt() {
  if (submitButtonDisabled.value) return
  emit('submit', draft.value.trim(), props.outputCount, aspectRatioValue.value)
  toolPopovers.close('output-count')
  toolPopovers.close('aspect-ratio')
}

function onPromptEnter(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  submitPrompt()
}

function toggleOutputCountPicker() {
  if (props.disabled) return
  toolPopovers.toggle('output-count')
}

function toggleAspectRatioPicker() {
  if (props.disabled || !showAspectRatio.value) return
  toolPopovers.toggle('aspect-ratio')
}

function selectOutputCount(value: number) {
  emit('update:output-count', value)
  toolPopovers.close('output-count')
}

function selectAspectRatio(value: PromptAspectRatio) {
  emit('update:aspect-ratio', value)
  toolPopovers.close('aspect-ratio')
}
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.image-node-prompt-composer {
  position: absolute;
  left: 0;
  right: 0;
  top: calc(100% + var(--node-prompt-composer-offset));
  z-index: 42;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: end;
  gap: var(--node-tool-button-gap);
  padding: 5px 5px 5px 13px;
  border: 1px solid rgba(122, 107, 213, 0.28);
  border-radius: 13px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
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

.image-node-prompt-input {
  align-self: end;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  color: #272932;
  cursor: text;
  font-size: 12px;
  line-height: 18px;
  resize: none;
  overflow: hidden;
  padding: 6px 0;
}

.image-node-prompt-composer.count-stacked .image-node-prompt-input {
  min-height: 73px;
}

.image-node-prompt-input::placeholder {
  color: #9aa4b2;
}

.image-node-prompt-input:disabled {
  cursor: wait;
}

.image-node-prompt-actions {
  align-self: end;
  display: flex;
  align-items: center;
  gap: var(--node-tool-button-gap);
}
.node-send-icon {
  width: 16px;
  height: 16px;
  stroke-width: 2.1;
  transform: translateX(0.5px);
}

.image-node-prompt-composer.count-stacked .image-node-prompt-actions {
  flex-direction: column;
  gap: var(--node-tool-button-gap);
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
