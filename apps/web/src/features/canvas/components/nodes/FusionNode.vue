<template>
  <NodeShell
    kind="fusion"
    :node-id="node.id"
    :width="node.width"
    :title="node.title || 'Fusion'"
    :active="active"
    :dragging="dragging"
    :copy-mode="copyMode"
    rounded-header
    @node-mousedown="emit('node-mousedown', $event)"
    @update:title="emit('update:title', $event)"
    @delete="emit('delete')"
  >
    <template #icon>
      <Keyframes />
    </template>
    <div
      class="relative box-border flex flex-col gap-2 overflow-visible rounded-[11px] bg-[var(--canvas-node-bg)] p-0"
      @contextmenu.stop
    >
      <FusionStage
        :active="active"
        :connection-drop-target="connectionDropTarget"
        :blend-point="node.blendPoint"
        :input-images="node.inputImages"
        :width="node.width"
        @update:blend-point="emit('update:blend-point', $event)"
        @update:input-images="emit('update:input-images', $event)"
        @input-image-drop="emit('input-image-drop', $event)"
        @input-image-remove="emit('input-image-remove', $event)"
        @contextmenu="emit('contextmenu', $event)"
      />
      <NodeFloatingControls v-if="showPromptComposer">
        <NodePromptComposer
          v-model="promptDraft"
          allow-empty
          offset="8px"
          placeholder="输入融合提示词"
          :aspect-ratio="aspectRatio"
          :output-count="outputCountValue"
          :submit-disabled="!canGenerate || props.generationAvailable === false"
          :billing-state="billingQuote"
          @update:aspect-ratio="emit('update:aspect-ratio', $event)"
          @update:output-count="selectOutputCount"
          @submit="submitPrompt"
        />
      </NodeFloatingControls>
    </div>
  </NodeShell>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Keyframes } from '@iconoir/vue'
import { requireImageNodeOutputCount, type FusionNodeItem } from '../../../canvas-board/domain'
import { normalizePromptAspectRatio, type PromptAspectRatio } from '../../../canvas-board/domain'
import type { NodeUiState } from '../../types/canvasNodeUi'
import type { FusionBlendPoint } from '../../../canvas-board/domain'
import FusionStage from './FusionStage.vue'
import NodeShell from './NodeShell.vue'
import NodePromptComposer from './NodePromptComposer.vue'
import NodeFloatingControls from './NodeFloatingControls.vue'
import { WORKFLOW_FEATURE } from '../../../../shared/domain/workflowFeatures'
import { buildFusionBillingReadinessRequest, useCanvasBillingQuote } from '../../execution'

const props = defineProps<{
  node: FusionNodeItem
  active: boolean
  connectionDropTarget?: boolean
  dragging: boolean
  copyMode: boolean
  promptComposerVisible?: boolean
  generationAvailable?: boolean
  uiState?: NodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:output-count', value: number): void
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'update:blend-point', value: FusionBlendPoint): void
  (e: 'update:input-images', value: FusionNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-remove', value: FusionNodeItem['inputImages'][number]): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const promptDraft = ref('')
const visibleInputImages = computed(() => props.node.inputImages)
const canGenerate = computed(() => visibleInputImages.value.length >= 2)
const showPromptComposer = computed(() => !!props.promptComposerVisible)
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const outputCountValue = computed(() => requireImageNodeOutputCount(props.node.outputCount))
const billingQuote = useCanvasBillingQuote(
  () => WORKFLOW_FEATURE.styleFusion,
  () => showPromptComposer.value && canGenerate.value,
  () =>
    buildFusionBillingReadinessRequest({
      imageUrls: visibleInputImages.value.map((image) => image.url),
      aspectRatio: aspectRatio.value,
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
  if (!canGenerate.value || props.generationAvailable === false) return
  emit('prompt-generate', prompt, outputCount, aspectRatio)
  promptDraft.value = ''
}

function selectOutputCount(value: number) {
  emit('update:output-count', requireImageNodeOutputCount(value))
}
</script>
