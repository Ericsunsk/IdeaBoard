<template>
  <div
    class="prompt-text-composer node-tool-surface flex min-w-0 flex-col gap-3"
    :class="variantClass"
    @mousedown.stop
    @click.stop
    @contextmenu.stop.prevent
  >
    <div
      v-if="shouldShowImageInputRow"
      class="prompt-text-composer-image-row node-tool-surface node-input-thumbnail-contract variant-standard relative z-[3] grid justify-between gap-1.5 overflow-visible [grid-template-columns:repeat(4,var(--node-input-thumbnail-size))]"
    >
      <NodeInputThumbnail
        v-for="item in visibleImageInputs"
        :key="item.id"
        class="prompt-text-composer-image-thumb aspect-square h-[var(--node-input-thumbnail-size)] w-[var(--node-input-thumbnail-size)]"
        :src="item.url"
        :removable="!disabled"
        :data-image-id="item.id"
        @mousedown.stop
        @contextmenu.stop.prevent="emit('image-input-preview', item)"
        @remove="emit('image-input-remove', item.id)"
      />
      <button
        v-if="canAddImageInput"
        class="prompt-text-composer-image-add node-input-thumbnail-add node-tool-output-count node-tool-button node-tool-tooltip-trigger"
        type="button"
        :disabled="disabled"
        aria-label="添加图片输入"
        @click="emit('image-input-open')"
      >
        <Plus class="prompt-text-composer-image-add-icon size-4" />
        <span class="node-tool-tooltip" aria-hidden="true">添加图片输入</span>
      </button>
    </div>
    <textarea
      ref="inputRef"
      v-model="draft"
      class="prompt-text-composer-input box-border min-h-0 w-full flex-auto cursor-text resize-none overflow-hidden border-0 bg-transparent p-0 font-[inherit] text-[14px] leading-[1.6] text-[#334155] outline-none placeholder:text-[#94a3b8] disabled:cursor-wait"
      :placeholder="placeholder"
      :disabled="disabled"
      @focus="emit('focus')"
      @keydown.enter.exact="onEnter"
    />
    <div
      ref="actionsRef"
      class="prompt-text-composer-actions flex self-end gap-[var(--node-tool-button-gap)]"
    >
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
        <SendDiagonalSolid class="size-4 translate-x-[0.5px]" />
        <span class="node-tool-tooltip" aria-hidden="true">发送</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Plus, SendDiagonalSolid } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import NodeAspectRatioPicker from './NodeAspectRatioPicker.vue'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import NodeOutputCountPicker from './NodeOutputCountPicker.vue'
import { usePromptComposerControls } from './composables/usePromptComposerControls'

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

type PromptTextComposerVariant = 'node' | 'floating'

const promptTextComposerVariantClasses: Record<PromptTextComposerVariant, string> = {
  node: 'variant-node',
  floating: 'variant-floating',
}

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
const variantClass = computed(() => promptTextComposerVariantClasses[props.variant])

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
  shouldRenderAspectRatio: shouldShowAspectRatioPicker,
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
  autofocus: () => props.autofocus,
  showAspectRatio: () => props.showAspectRatio,
  onTextUpdate: (value) => emit('update:modelValue', value),
  onOutputCountUpdate: (value) => emit('update:output-count', value),
  onAspectRatioUpdate: (value) => emit('update:aspect-ratio', value),
  onSubmit: (prompt, outputCount, aspectRatio) => emit('submit', prompt, outputCount, aspectRatio),
})
</script>

<style scoped src="./styles/node-input-thumbnails.css"></style>
<style scoped src="./styles/node-tool-buttons.css"></style>
<style scoped>
.prompt-text-composer.node-tool-surface {
  --node-tool-button-border: #e2e8f0;
}

.prompt-text-composer-image-row {
  --node-tool-button-size: var(--node-input-thumbnail-size);
  --node-tool-button-radius: 8px;
}

.prompt-text-composer.variant-floating {
  gap: var(--node-tool-button-gap);
  padding: 9px;
  border: var(--node-tool-border-width) solid rgba(122, 107, 213, 0.28);
  border-radius: var(--canvas-floating-radius);
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(var(--canvas-surface-blur));
}

.prompt-text-composer.variant-floating:focus-within,
.prompt-text-composer.variant-floating:has(.prompt-text-composer-input:not(:placeholder-shown)) {
  border-color: var(--ib-focus-border-color);
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
