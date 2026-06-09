<template>
  <div
    class="workflow-node fusion-node overflow-visible"
    data-node-kind="fusion"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
  >
    <NodeHeader
      :title="node.title || 'Fusion'"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @update:title="emit('update:title', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <Keyframes />
      </template>
    </NodeHeader>

    <div
      class="relative box-border flex flex-col gap-2 overflow-visible rounded-[11px] bg-white p-0"
      @contextmenu.stop
    >
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
        :aspect-ratio="aspectRatio"
        :output-count="outputCountValue"
        :submit-disabled="!canGenerate"
        @update:aspect-ratio="emit('update:aspect-ratio', $event)"
        @update:output-count="selectOutputCount"
        @submit="submitPrompt"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Keyframes } from '@iconoir/vue'
import { clampImageNodeOutputCount, type FusionNodeItem } from '../../types/canvasNodes'
import { normalizePromptAspectRatio, type PromptAspectRatio } from '../../config/promptAspectRatio'
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
  (e: 'update:aspect-ratio', value: PromptAspectRatio): void
  (e: 'update:blend-point', value: FusionBlendPoint): void
  (e: 'update:input-images', value: FusionNodeItem['inputImages']): void
  (e: 'input-image-drop', event: DragEvent): void
  (e: 'input-image-remove', value: FusionNodeItem['inputImages'][number]): void
  (e: 'contextmenu', event: MouseEvent): void
  (e: 'prompt-generate', prompt: string, outputCount: number, aspectRatio: PromptAspectRatio): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const promptDraft = ref('')
const visibleInputImages = computed(() => props.node.inputImages)
const canGenerate = computed(() => visibleInputImages.value.length >= 2)
const showPromptComposer = computed(() => !!props.promptComposerVisible)
const aspectRatio = computed(() => normalizePromptAspectRatio(props.node.aspectRatio))
const outputCountValue = computed(() => clampImageNodeOutputCount(props.node.outputCount))

watch(
  () => props.node.prompt,
  (value) => {
    promptDraft.value = typeof value === 'string' ? value : ''
  },
  { immediate: true },
)

function submitPrompt(prompt: string, outputCount: number, aspectRatio: PromptAspectRatio) {
  if (!canGenerate.value) return
  emit('prompt-generate', prompt, outputCount, aspectRatio)
  promptDraft.value = ''
}

function selectOutputCount(value: number) {
  emit('update:output-count', clampImageNodeOutputCount(value))
}
</script>

<style scoped src="./styles/node-shared.css"></style>
