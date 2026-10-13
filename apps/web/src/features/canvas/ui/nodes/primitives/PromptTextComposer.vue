<template>
  <div
    class="prompt-text-composer node-tool-surface flex min-w-0 flex-col gap-3"
    :class="[variantClass, { 'has-text': hasText }]"
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
        :asset-id="item.mediaRef?.assetId"
        :is-removable="!isDisabled"
        :data-image-id="item.id"
        @mousedown.stop
        @contextmenu.stop.prevent="emit('image-input-preview', item)"
        @remove="emit('image-input-remove', item.id)"
      />
      <button
        v-if="canAddImageInput"
        class="prompt-text-composer-image-add node-input-thumbnail-add node-tool-output-count node-tool-button canvas-tool-tooltip-trigger"
        type="button"
        :disabled="isDisabled"
        :aria-label="t('canvas.controls.addImageInput')"
        @click="emit('image-input-open')"
      >
        <Plus class="prompt-text-composer-image-add-icon size-4" />
        <span class="canvas-tool-tooltip" aria-hidden="true">{{
          t('canvas.controls.addImageInput')
        }}</span>
      </button>
    </div>
    <PromptComposerControls
      :model-value="modelValue"
      :output-count="outputCount"
      :output-resolution="outputResolution"
      :aspect-ratio="aspectRatio"
      :placeholder="placeholder"
      :is-disabled="isDisabled"
      :should-allow-empty="shouldAllowEmpty"
      :is-submit-disabled="isSubmitDisabled"
      :should-show-aspect-ratio="shouldShowAspectRatio"
      :should-show-output-count="shouldShowOutputCount"
      :should-show-output-resolution="shouldShowOutputResolution"
      :is-autofocus="isAutofocus"
      :billing-state="billingState"
      :textarea-class="promptTextInputClass"
      actions-class="prompt-text-composer-actions flex self-end gap-[var(--node-tool-button-gap)]"
      send-button-class="prompt-text-composer-send node-tool-send node-tool-button canvas-tool-tooltip-trigger"
      @update:model-value="emit('update:modelValue', $event)"
      @update:output-count="emit('update:output-count', $event)"
      @update:output-resolution="emit('update:output-resolution', $event)"
      @update:aspect-ratio="emit('update:aspect-ratio', $event)"
      @focus="emit('focus')"
      @submit="onSubmit"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus } from '@iconoir/vue'
import type { PromptAspectRatio } from '../../../../board/ports/canvas'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import type { CanvasBillingQuoteState } from '../../../session/execution'
import NodeInputThumbnail from './NodeInputThumbnail.vue'
import PromptComposerControls from './PromptComposerControls.vue'

const props = withDefaults(
  defineProps<{
    modelValue: string
    outputCount: number
    outputResolution?: OutputResolution | undefined
    placeholder?: string
    isDisabled?: boolean
    shouldAllowEmpty?: boolean
    isSubmitDisabled?: boolean
    aspectRatio?: PromptAspectRatio | undefined
    imageInputs?: PromptTextComposerImageInput[]
    maxImageInputs?: number
    shouldShowAspectRatio?: boolean
    shouldShowImageInput?: boolean
    shouldShowOutputCount?: boolean
    shouldShowOutputResolution?: boolean
    isAutofocus?: boolean
    variant?: 'node' | 'floating'
    billingState?: CanvasBillingQuoteState | null
  }>(),
  {
    shouldAllowEmpty: false,
    aspectRatio: undefined,
    isAutofocus: false,
    isDisabled: false,
    imageInputs: () => [],
    maxImageInputs: 4,
    outputResolution: undefined,
    placeholder: 'type your idea...',
    shouldShowAspectRatio: true,
    shouldShowImageInput: false,
    shouldShowOutputCount: true,
    shouldShowOutputResolution: true,
    isSubmitDisabled: false,
    variant: 'node',
    billingState: null,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'update:output-count', value: number): void
  (e: 'update:output-resolution', value: OutputResolution): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'image-input-open'): void
  (e: 'image-input-remove', imageId: string): void
  (e: 'image-input-preview', image: PromptTextComposerImageInput): void
  (e: 'focus'): void
  (e: 'submit', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const { t } = useI18n()

type PromptTextComposerImageInput = {
  id: string
  url: string
  ratio?: number
  mediaAssetId?: number
  mediaRef?: { assetId: string } | undefined
}

type PromptTextComposerVariant = 'node' | 'floating'

const promptTextComposerVariantClasses: Record<PromptTextComposerVariant, string> = {
  node: 'variant-node',
  floating: 'variant-floating node-tool-glass-surface',
}

const maxVisibleImageInputs = computed(() =>
  Math.max(0, Math.min(4, Math.floor(Number(props.maxImageInputs) || 0))),
)
const visibleImageInputs = computed(() =>
  props.imageInputs.filter((item) => item.id && item.url).slice(0, maxVisibleImageInputs.value),
)
const shouldShowImageInputRow = computed(
  () => props.shouldShowImageInput && maxVisibleImageInputs.value > 0,
)
const canAddImageInput = computed(
  () => visibleImageInputs.value.length < maxVisibleImageInputs.value,
)
const variantClass = computed(() => promptTextComposerVariantClasses[props.variant])
const hasText = computed(() => props.modelValue.trim().length > 0)
const promptTextInputClass = computed(() =>
  [
    'prompt-text-composer-input box-border min-h-0 w-full flex-auto cursor-text resize-none overflow-hidden border-0 bg-transparent font-[inherit] text-[var(--canvas-text-body)] outline-none placeholder:text-[var(--canvas-text-subtle)] disabled:cursor-wait',
    props.variant === 'floating'
      ? 'px-1 py-1.5 text-xs leading-[18px]'
      : 'p-0 text-admin-content leading-[1.6]',
  ].join(' '),
)

function onSubmit(prompt: string, outputCount: number, aspectRatio: PromptAspectRatio) {
  emit('submit', prompt, outputCount, aspectRatio)
}
</script>

<style scoped>
.prompt-text-composer.node-tool-surface {
  --node-tool-button-border: var(--canvas-node-border);
}

.prompt-text-composer-image-row {
  --node-tool-button-size: var(--node-input-thumbnail-size);
  --node-tool-button-radius: var(--radius-control);
}

.prompt-text-composer.variant-floating {
  gap: var(--node-tool-button-gap);
  padding: 9px;
}

.prompt-text-composer.variant-floating:focus-within,
.prompt-text-composer.variant-floating.has-text {
  border-color: var(--focus-border-color);
}

/*
 * The node variant is borderless by design, so it cannot signal focus through
 * a border stroke like the floating variant. A focus-within glow mirrors that
 * intent without touching layout (box-shadow only); the textarea keeps its own
 * ring as the primary indicator.
 */
.prompt-text-composer.variant-node:focus-within {
  border-radius: var(--radius-control);
  box-shadow: 0 0 0 2px var(--focus-outline-color);
}

.prompt-text-composer.variant-floating .prompt-text-composer-image-row {
  --node-input-thumbnail-size: calc((100% - 18px) / 4);
  --node-tool-button-radius: var(--radius-control);
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
