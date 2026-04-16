<template>
  <div
    class="workflow-node fusion-node"
    data-node-kind="fusion"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Fusion"
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <path d="M8 6.5h4l4 4v7a1.5 1.5 0 0 1-1.5 1.5h-6A1.5 1.5 0 0 1 7 17.5v-9A2 2 0 0 1 8 6.5z" />
          <path d="M12 6.5v4h4" />
          <path d="M9 14c1.5-1.4 4.5-1.4 6 0" />
        </svg>
      </template>
    </NodeHeader>

    <div class="fusion-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="fusion-mode-card node-mode-card">
        <div class="fusion-mode-row node-option-row">
          <NodeActionButton
            v-for="option in fusionModeFamilyOptions"
            :key="option.value"
            variant="chip"
            :active="modeFamily === option.value"
            @click="emit('update:mode', resolveFusionFamilyMode(option.value, modeVariant))"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
        <div v-if="modeFamily !== 'style'" class="fusion-variant-row node-option-row">
          <NodeActionButton
            v-for="option in fusionModeVariantOptions"
            :key="option.value"
            variant="chip"
            :active="modeVariant === option.value"
            @click="emit('update:mode', resolveFusionFamilyMode(modeFamily, option.value))"
          >
            {{ option.label }}
          </NodeActionButton>
        </div>
      </div>

      <NodeConnectionChips :items="connectionChips" />

      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating" full @click="emit('generate')">
          生成
        </NodeActionButton>
      </div>
    </div>

    <NodePorts
      node-kind="fusion"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { FusionNodeItem } from '../../../types/canvasNodes'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeHeader from './NodeHeader.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, FUSION_NODE_IO_SLOTS } from './nodeIoConfig'
import {
  FUSION_MODE_FAMILY_OPTIONS,
  FUSION_MODE_VARIANT_OPTIONS,
  resolveFusionFamilyMode,
} from './nodeOptionConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: FusionNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  imageAInputConnected?: boolean
  imageBInputConnected?: boolean
  promptInputConnected?: boolean
  outputConnected?: boolean
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'update:mode', value: FusionNodeItem['mode']): void
  (e: 'generate'): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const fusionModeFamilyOptions = FUSION_MODE_FAMILY_OPTIONS
const fusionModeVariantOptions = FUSION_MODE_VARIANT_OPTIONS

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(FUSION_NODE_IO_SLOTS, null, {
    connected: {
      imageA: props.imageAInputConnected,
      imageB: props.imageBInputConnected,
      prompt: props.promptInputConnected,
      output: props.outputConnected,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

const modeFamily = computed<'style' | 'pattern' | 'craft'>(() => {
  if (props.node.mode.startsWith('pattern-')) return 'pattern'
  if (props.node.mode.startsWith('craft-')) return 'craft'
  return 'style'
})

const modeVariant = computed<'creative' | 'style'>(() => {
  if (props.node.mode.endsWith('creative')) return 'creative'
  return 'style'
})

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.fusion-node-body {
  gap: 8px;
}

</style>
