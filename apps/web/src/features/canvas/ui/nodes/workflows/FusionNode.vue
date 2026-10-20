<template>
  <NodeShell
    kind="fusion"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || t('canvas.node.fusion')"
    :is-active="isActive"
    :is-dragging="isDragging"
    :is-copy-mode="isCopyMode"
    is-rounded-header
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <component :is="serviceIcon" />
    </template>
    <div
      class="relative box-border flex flex-col gap-2 overflow-visible rounded-panel bg-[var(--canvas-node-bg)] p-0"
      @contextmenu.stop
    >
      <FusionStage
        :is-active="isActive"
        :is-connection-drop-target="isConnectionDropTarget"
        :blend-point="node.blendPoint"
        :input-images="node.inputImages"
        :width="node.width"
        @update:blend-point="emit('update:blend-point', $event)"
        @update:input-images="emit('update:input-images', $event)"
        @input-image-drop="emit('input-image-drop', $event)"
        @input-image-remove="emit('input-image-remove', $event)"
        @contextmenu="emit('contextmenu', $event)"
      />
      <NodeFloatingControls v-if="shouldShowPromptComposer">
        <NodePromptComposer
          v-model="promptDraft"
          should-allow-empty
          offset="8px"
          :placeholder="t('canvas.controls.fusionPlaceholder')"
          :aspect-ratio="aspectRatio"
          :output-resolution="node.outputResolution"
          :output-count="outputCountValue"
          :is-submit-disabled="!canGenerate || props.isGenerationAvailable === false"
          :billing-state="billingQuote"
          @update:aspect-ratio="emit('update:aspect-ratio', $event)"
          @update:output-resolution="emit('update:output-resolution', $event)"
          @update:output-count="selectOutputCount"
          @submit="submitPrompt"
        />
      </NodeFloatingControls>
    </div>
  </NodeShell>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import { getServiceIcon } from '../../../../../core/components/serviceIcons'
import {
  requireImageNodeOutputCount,
  type FusionNodeItem,
  normalizePromptAspectRatio,
  type PromptAspectRatio,
  type FusionBlendPoint,
} from '../../../../board/ports/canvas'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import type { NodeUiState } from '../../../core/nodes/canvasNodeUi.ts'
import FusionStage from './FusionStage.vue'
import { NodeShell, NodePromptComposer, NodeFloatingControls } from '../primitives'
import { SERVICE } from '../../../../../core/contracts/imageServices'
import {
  buildFusionBillingReadinessRequest,
  useCanvasBillingQuote,
} from '../../../session/execution'

const props = defineProps<{
  node: FusionNodeItem
  isActive: boolean
  isConnectionDropTarget?: boolean
  isDragging: boolean
  isCopyMode: boolean
  isPromptComposerVisible?: boolean
  isGenerationAvailable?: boolean
  uiState?: NodeUiState | undefined
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:output-count', value: number): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'update:output-resolution', value: OutputResolution): void
  (e: 'update:blend-point', value: FusionBlendPoint): void
  (e: 'update:input-images', value: FusionNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-remove', value: FusionNodeItem['inputImages'][number]): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const { t } = useI18n()
const serviceIcon = getServiceIcon(SERVICE.fusion)

const promptDraft = ref('')
const visibleInputImages = computed(() => props.node.inputImages)
const canGenerate = computed(() => visibleInputImages.value.length >= 2)
const shouldShowPromptComposer = computed(() => !!props.isPromptComposerVisible)
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const outputCountValue = computed(() => requireImageNodeOutputCount(props.node.outputCount))
const billingQuote = useCanvasBillingQuote(
  () => SERVICE.fusion,
  () => shouldShowPromptComposer.value && canGenerate.value,
  () =>
    buildFusionBillingReadinessRequest({
      imageUrls: visibleInputImages.value.map((image) => image.url),
      aspectRatio: aspectRatio.value,
      outputCount: outputCountValue.value,
      outputResolution: props.node.outputResolution,
    }) ?? undefined,
)

watch(
  () => props.node.prompt,
  (value) => {
    promptDraft.value = typeof value === 'string' ? value : ''
  },
  { immediate: true },
)

function submitPrompt(prompt: string, outputCount: number, aspectRatio: PromptAspectRatio) {
  if (!canGenerate.value || props.isGenerationAvailable === false) return
  emit('prompt-generate', prompt, outputCount, aspectRatio)
  promptDraft.value = ''
}

function selectOutputCount(value: number) {
  emit('update:output-count', requireImageNodeOutputCount(value))
}
</script>
