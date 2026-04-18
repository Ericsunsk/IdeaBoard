<template>
  <div
    class="workflow-node color-node"
    data-node-kind="color"
    :data-node-id="node.id"
    :class="{ active, dragging, 'copy-mode': copyMode }"
    :style="nodeStyle"
    @mousedown.stop
  >
    <NodeHeader
      title="Color"
      rounded
      @node-mousedown="emit('node-mousedown', $event)"
      @delete="emit('delete')"
    >
      <template #icon>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1">
          <circle cx="12" cy="12" r="4.8" />
          <path d="M12 2.8v3M12 18.2v3M2.8 12h3M18.2 12h3M5.2 5.2l2.1 2.1M16.7 16.7l2.1 2.1M5.2 18.8l2.1-2.1M16.7 7.3l2.1-2.1" />
        </svg>
      </template>
    </NodeHeader>
    <div class="color-node-body node-execution-body" @mousedown.stop @contextmenu.stop>
      <div class="color-node-row">
        <div class="color-node-color-wrap">
          <NodeColorField
            class="color-node-color-field"
            :value="node.targetColor"
            swatch-aria-label="Open color editor"
            @swatch-click="emit('toggle-popover')"
            @change:value="emit('target-color-change', $event)"
          />
          <NodeColorPopover
            v-if="isPopoverOpen"
            :hex="popoverHex"
            :hue="popoverHue"
            :saturation="popoverSaturation"
            :lightness="popoverLightness"
            :spectrum-style="spectrumStyle"
            :cursor-style="cursorStyle"
            :quick-swatches="quickSwatches"
            @hex-input="emit('popover-hex-input', $event)"
            @hex-commit="emit('popover-hex-commit')"
            @hue-input="emit('popover-hue-input', $event)"
            @saturation-input="emit('popover-saturation-input', $event)"
            @lightness-input="emit('popover-lightness-input', $event)"
            @spectrum-pointerdown="emit('spectrum-pointerdown', $event)"
            @select-swatch="emit('select-swatch', $event)"
            @set-spectrum-ref="emit('set-spectrum-ref', $event)"
          />
        </div>
      </div>
      <div class="color-node-tools">
        <NodeIconButton
          tone="accent"
          :size="30"
          :disabled="generating"
          title="吸管取色"
          @click="emit('eyedropper')"
        >
          <svg class="color-node-tool-icon" viewBox="0 0 242.63 240.1" fill="none" aria-hidden="true">
            <rect x="94.33" y="57.68" width="124.37" height="55.2" transform="translate(205.65 256.62) rotate(-134.59)" fill="currentColor" />
            <rect x="67.51" y="38.09" width="55.26" height="215.36" rx="27.63" ry="27.63" transform="translate(58.11 315.87) rotate(-134.59)" stroke="currentColor" stroke-width="21" />
            <rect x="162.01" y="-10.5" width="55.53" height="125.99" rx="27.77" ry="27.77" transform="translate(285.61 224.51) rotate(-134.59)" fill="currentColor" />
          </svg>
        </NodeIconButton>
        <input
          :value="node.strength"
          class="color-node-strength-range node-inline-range"
          :style="{ '--slider-pct': `${node.strength}%` }"
          type="range"
          min="0"
          max="100"
          @input="emit('strength-input', toNumber(($event.target as HTMLInputElement).value, node.strength))"
        />
      </div>
      <NodeConnectionChips :items="connectionChips" />
      <div class="node-primary-action-slot">
        <NodeActionButton variant="primary" :disabled="generating || uiState?.primaryAction?.disabled" full @click="emit('generate')">
          {{ uiState?.primaryAction?.label || '应用换色' }}
        </NodeActionButton>
      </div>
    </div>
    <NodePorts
      node-kind="color"
      :node-id="node.id"
      :ports="ports"
      @port-mousedown="onPortMouseDown"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ColorNodeItem } from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'
import NodeActionButton from './NodeActionButton.vue'
import NodeConnectionChips from './NodeConnectionChips.vue'
import NodeColorField from './NodeColorField.vue'
import NodeHeader from './NodeHeader.vue'
import NodeIconButton from './NodeIconButton.vue'
import NodeColorPopover from './NodeColorPopover.vue'
import NodePorts from './NodePorts.vue'
import { buildNodeIoFromSlots, COLOR_NODE_IO_SLOTS } from './nodeIoConfig'
import { useCanvasNodeStyle } from './useCanvasNodeStyle'

const props = defineProps<{
  node: ColorNodeItem
  active: boolean
  dragging: boolean
  copyMode: boolean
  generating: boolean
  isPopoverOpen: boolean
  popoverHex: string
  popoverHue: number
  popoverSaturation: number
  popoverLightness: number
  spectrumStyle: Record<string, string>
  cursorStyle: Record<string, string>
  quickSwatches: string[]
  uiState?: NodeUiState
  errorPortKeys: string[]
}>()

const emit = defineEmits<{
  (e: 'node-mousedown', event: MouseEvent): void
  (e: 'delete'): void
  (e: 'toggle-popover'): void
  (e: 'target-color-change', value: string): void
  (e: 'popover-hex-input', event: Event): void
  (e: 'popover-hex-commit'): void
  (e: 'popover-hue-input', value: number): void
  (e: 'popover-saturation-input', value: number): void
  (e: 'popover-lightness-input', value: number): void
  (e: 'spectrum-pointerdown', event: PointerEvent): void
  (e: 'select-swatch', value: string): void
  (e: 'eyedropper'): void
  (e: 'strength-input', value: number): void
  (e: 'generate'): void
  (e: 'set-spectrum-ref', value: HTMLElement | null): void
  (e: 'port-mousedown', event: MouseEvent, port: 'image'): void
}>()

const nodeStyle = useCanvasNodeStyle(() => props.node)
const io = computed(() =>
  buildNodeIoFromSlots(COLOR_NODE_IO_SLOTS, null, {
    connected: {
      image: props.uiState?.connection?.connected?.image,
      mask: props.uiState?.connection?.connected?.mask,
      prompt: props.uiState?.connection?.connected?.prompt,
      output: props.uiState?.connection?.connected?.output,
    },
    pending: {
      image: props.uiState?.connection?.pending?.image,
      mask: props.uiState?.connection?.pending?.mask,
    },
    errorPortKeys: props.errorPortKeys,
  }),
)
const connectionChips = computed(() => io.value.connectionChips)
const ports = computed(() => io.value.ports)

function toNumber(rawValue: string, fallback: number): number {
  const value = Number(rawValue)
  return Number.isFinite(value) ? value : fallback
}

function onPortMouseDown(event: MouseEvent, port: string) {
  emit('port-mousedown', event, port as 'image')
}
</script>

<style scoped src="./node-shared.css"></style>
<style scoped>
.workflow-node.color-node {
  overflow: visible;
}

.color-node-body {
  gap: 8px;
}

.color-node-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.color-node-tools {
  display: flex;
  align-items: center;
  gap: 8px;
}

.color-node-strength-range {
  flex: 1;
  min-width: 0;
}

.color-node-tool-icon {
  width: 14px;
  height: 14px;
  display: block;
  object-fit: contain;
}

.color-node-color-wrap {
  position: relative;
}

.color-node-color-field {
  width: 100%;
}

</style>
