<template>
  <div
    ref="rootRef"
    class="prompt-text-composer node-tool-surface"
    :class="`variant-${variant}`"
    @mousedown.stop
    @click.stop
    @contextmenu.stop.prevent
  >
    <div
      v-if="shouldShowImageInputRow"
      class="prompt-text-composer-image-row node-tool-surface node-input-thumbnail-contract variant-standard"
    >
      <NodeInputThumbnail
        v-for="item in visibleImageInputs"
        :key="item.id"
        class="prompt-text-composer-image-thumb"
        :src="item.url"
        :removable="!disabled"
        :data-image-id="item.id"
        @mousedown.stop
        @contextmenu.stop.prevent="emit('image-input-preview', item)"
        @remove="emit('image-input-remove', item.id)"
      />
      <button
        v-if="canAddImageInput"
        class="prompt-text-composer-image-add node-tool-output-count node-tool-button node-tool-tooltip-trigger"
        type="button"
        :disabled="disabled"
        aria-label="添加图片输入"
        @click="emit('image-input-open')"
      >
        <Plus class="prompt-text-composer-image-add-icon" />
        <span class="node-tool-tooltip" aria-hidden="true">添加图片输入</span>
      </button>
    </div>
    <textarea
      ref="inputRef"
      class="prompt-text-composer-input"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      @input="onInput"
      @focus="emit('focus')"
      @keydown.enter.exact="onEnter"
    />
    <div ref="actionsRef" class="prompt-text-composer-actions">
      <NodeAspectRatioPicker
        v-if="shouldShowAspectRatioPicker"
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
        class="prompt-text-composer-send node-tool-send node-tool-button node-tool-tooltip-trigger"
        type="button"
        :disabled="!canSubmit"
        aria-label="发送"
        @click="submit"
      >
        <SendDiagonalSolid class="prompt-text-composer-send-icon" />
        <span class="node-tool-tooltip" aria-hidden="true">发送</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { Plus, SendDiagonalSolid } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../config/promptAspectRatio'
import { normalizePromptAspectRatio } from '../../config/promptAspectRatio'
import { clampImageNodeOutputCount } from '../../types/canvasNodes'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { useAutoResizeTextarea } from './composables/useAutoResizeTextarea'
import { useNodeToolPopoverGroup } from './composables/useNodeToolPopoverGroup'

type PromptTextComposerImageInput = {
  id: string
  url: string
  ratio?: number
}

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    placeholder?: string
    disabled?: boolean
    allowEmpty?: boolean
    submitDisabled?: boolean
    aspectRatio?: PromptAspectRatio
    imageInputs?: PromptTextComposerImageInput[]
    maxImageInputs?: number
    showAspectRatio?: boolean
    showImageInput?: boolean
    autofocus?: boolean
    variant?: 'node' | 'floating'
  }>(),
  {
    allowEmpty: false,
    aspectRatio: undefined,
    autofocus: false,
    disabled: false,
    imageInputs: () => [],
    maxImageInputs: 4,
    placeholder: 'type your idea...',
    showAspectRatio: true,
    showImageInput: false,
    submitDisabled: false,
    variant: 'node',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'image-input-open'): void
  (e: 'image-input-remove', imageId: string): void
  (e: 'image-input-preview', image: PromptTextComposerImageInput): void
  (e: 'focus'): void
  (e: 'submit', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const rootRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLTextAreaElement | null>(null)
const actionsRef = ref<HTMLElement | null>(null)
const outputCountValue = computed(() => clampImageNodeOutputCount(props.outputCount))
const aspectRatioValue = computed(() => normalizePromptAspectRatio(props.aspectRatio))
const maxVisibleImageInputs = computed(() =>
  Math.max(0, Math.min(4, Math.floor(Number(props.maxImageInputs) || 0))),
)
const visibleImageInputs = computed(() =>
  props.imageInputs.filter((item) => item.id && item.url).slice(0, maxVisibleImageInputs.value),
)
const shouldShowImageInputRow = computed(
  () => props.showImageInput && maxVisibleImageInputs.value > 0,
)
const canAddImageInput = computed(
  () => visibleImageInputs.value.length < maxVisibleImageInputs.value,
)
const canSubmit = computed(
  () =>
    !props.disabled &&
    !props.submitDisabled &&
    (props.allowEmpty || props.modelValue.trim().length > 0),
)
const shouldRenderAspectRatio = computed(
  () => props.showAspectRatio && props.aspectRatio !== undefined,
)
const toolPopovers = useNodeToolPopoverGroup({
  keys: ['aspect-ratio', 'output-count'] as const,
  rootRefs: [actionsRef],
})
const aspectRatioPickerVisible = toolPopovers.isOpen('aspect-ratio')
const outputCountPickerVisible = toolPopovers.isOpen('output-count')
const shouldShowAspectRatioPicker = computed(() => shouldRenderAspectRatio.value)

const textarea = useAutoResizeTextarea(inputRef, {
  watchSource: () => props.modelValue,
})

onMounted(async () => {
  await nextTick()
  textarea.resize()
  if (props.autofocus) inputRef.value?.focus()
})

function onInput(event: Event) {
  const input = event.target as HTMLTextAreaElement | null
  emit('update:modelValue', input?.value ?? '')
}

function onEnter(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  submit()
}

function submit() {
  if (!canSubmit.value) return
  emit('submit', props.modelValue.trim(), outputCountValue.value, aspectRatioValue.value)
  toolPopovers.close('aspect-ratio')
  toolPopovers.close('output-count')
}

function toggleAspectRatioPicker() {
  if (props.disabled || !shouldRenderAspectRatio.value) return
  toolPopovers.toggle('aspect-ratio')
}

function selectAspectRatio(value: PromptAspectRatio) {
  emit('update:aspect-ratio', value)
  toolPopovers.close('aspect-ratio')
}

function toggleOutputCountPicker() {
  if (props.disabled) return
  toolPopovers.toggle('output-count')
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
  toolPopovers.close('output-count')
}
</script>

<style scoped src="./styles/node-input-thumbnails.css"></style>
<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.prompt-text-composer {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
}

.prompt-text-composer-input {
  flex: 1 1 auto;
  width: 100%;
  min-height: 0;
  cursor: text;
  border: 0;
  resize: none;
  outline: none;
  padding: 0;
  box-sizing: border-box;
  background: transparent;
  color: #334155;
  font-family: inherit;
  font-size: 14px;
  line-height: 1.6;
  overflow: hidden;
}

.prompt-text-composer-input::placeholder {
  color: #94a3b8;
}

.prompt-text-composer-input:disabled {
  cursor: wait;
}

.prompt-text-composer-actions {
  align-self: flex-end;
  display: flex;
  gap: var(--node-tool-button-gap);
}

.prompt-text-composer-actions.node-tool-surface,
.prompt-text-composer.node-tool-surface {
  --node-tool-button-border: #e2e8f0;
}

.prompt-text-composer-send-icon {
  width: 16px;
  height: 16px;
  stroke-width: var(--node-tool-icon-stroke);
  transform: translateX(0.5px);
}

.prompt-text-composer-image-row {
  --node-tool-button-size: var(--node-input-thumbnail-size);
  --node-tool-button-radius: 8px;
  position: relative;
  display: grid;
  grid-template-columns: repeat(4, var(--node-input-thumbnail-size));
  gap: 6px;
  justify-content: space-between;
  overflow: visible;
  z-index: 3;
}

.prompt-text-composer-image-add {
  width: var(--node-input-thumbnail-size);
  height: var(--node-input-thumbnail-size);
  color: #64748b;
  border-style: dashed;
  cursor: copy;
}

.prompt-text-composer-image-add:hover,
.prompt-text-composer-image-add:focus,
.prompt-text-composer-image-add:active {
  color: var(--node-tool-button-accent);
  border-color: var(--node-tool-button-accent);
  background: #f8f7ff;
}

.prompt-text-composer-image-thumb {
  width: var(--node-input-thumbnail-size);
  height: var(--node-input-thumbnail-size);
  aspect-ratio: 1 / 1;
}

.prompt-text-composer-image-add-icon {
  width: 16px;
  height: 16px;
  stroke-width: var(--node-tool-icon-stroke);
}

.prompt-text-composer.variant-floating {
  gap: var(--node-tool-button-gap);
  padding: 9px;
  border: var(--node-tool-border-width) solid rgba(122, 107, 213, 0.28);
  border-radius: 13px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(10px);
}

.prompt-text-composer.variant-floating:focus-within,
.prompt-text-composer.variant-floating:has(.prompt-text-composer-input:not(:placeholder-shown)) {
  border-color: #9278e6;
}

.prompt-text-composer.variant-floating .prompt-text-composer-input {
  font-size: 12px;
  line-height: 18px;
  padding: 6px 4px;
}

.prompt-text-composer.variant-floating .prompt-text-composer-image-row {
  --node-input-thumbnail-size: calc((100% - 18px) / 4);
  --node-tool-button-radius: 9px;
  gap: 6px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  justify-content: stretch;
}

.prompt-text-composer.variant-floating .prompt-text-composer-image-add,
.prompt-text-composer.variant-floating .prompt-text-composer-image-thumb {
  width: 100%;
  height: auto;
  aspect-ratio: 1 / 1;
}

.prompt-text-composer.variant-floating .prompt-text-composer-image-add-icon {
  width: 14px;
  height: 14px;
}
</style>
