<template>
  <div
    class="workflow-node fusion-node"
    data-node-kind="fusion"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode, running: uiState?.isRunning }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'FUSION'"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M7 7h4l2 2h4a2 2 0 0 1 2 2v6H7a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2z" />
          <path d="M9 15c1.4-1.4 4.6-1.4 6 0" />
        </svg>
      </template>
    </NodeHeader>

    <div class="fusion-node-body" @contextmenu.stop>
      <FusionStage
        :active="active"
        :blend-point="node.blendPoint"
        :input-images="node.inputImages"
        :width="node.width"
        @update:blend-point="emit('update:blend-point', $event)"
        @update:input-images="emit('update:input-images', $event)"
        @input-image-drop="emit('input-image-drop', $event)"
        @input-image-remove="emit('input-image-remove', $event)"
        @contextmenu="emit('contextmenu', $event)"
      />
      <NodePromptComposer
        v-if="showPromptComposer"
        v-model="promptDraft"
        allow-empty
        offset="8px"
        placeholder="输入融合提示词"
        :disabled="isGenerating"
        :output-count="outputCountValue"
        :submit-disabled="!canGenerate"
        @update:output-count="selectOutputCount"
        @submit="submitPrompt"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  clampImageNodeOutputCount,
  type FusionNodeItem,
} from '../../types/canvasNodes'
import type { NodeUiState } from '../../types/canvasNodeUi'
import type { FusionBlendPoint } from '../../utils/fusionBlend'
import FusionStage from './FusionStage.vue'
import NodePromptComposer from './NodePromptComposer.vue'
import { useCanvasNodeStyle } from './composables/useCanvasNodeStyle'

const props = defineProps<{
  node: FusionNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  promptComposerVisible?: boolean
  uiState?: NodeUiState
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'update:title', value: string): void
  (e: 'delete'): void
  (e: 'update:output-count', value: number): void
  (e: 'update:prompt', value: string): void
  (e: 'update:blend-point', value: FusionBlendPoint): void
  (e: 'update:input-images', value: FusionNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-remove', value: FusionNodeItem['inputImages'][number]): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const promptDraft = ref('')
const visibleInputImages = computed(() => props.node.inputImages)
const canGenerate = computed(() => visibleInputImages.value.length >= 2)
const showPromptComposer = computed(() => !!props.promptComposerVisible)
const isGenerating = computed(() => !!props.uiState?.isRunning)
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))

watch(
  () => props.node.prompt,
  (value) => {
    promptDraft.value = typeof value === 'string' ? value : ''
  },
  { immediate: true },
)

function submitPrompt(prompt: string, outputCount: number) {
  if (isGenerating.value || !canGenerate.value) return
  emit('update:prompt', prompt)
  emit('prompt-generate', prompt, outputCount)
  promptDraft.value = ''
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
}
</script>

<style scoped src="./styles/node-shared.css"></style>
<style scoped>
.workflow-node.fusion-node {
  overflow: visible;
}

.fusion-node-body {
  position: relative;
  padding: 0;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-radius: 11px;
  background: #ffffff;
  overflow: visible;
}
</style>
