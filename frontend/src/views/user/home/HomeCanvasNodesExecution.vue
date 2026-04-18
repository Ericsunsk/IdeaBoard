<template>
  <template v-for="node in colorNodes" :key="node.id">
    <ColorNode
      :node="node"
      :active="isNodeSelected('color', node.id)"
      :dragging="isNodeDragging('color', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :is-popover-open="activeColorPopoverNodeId === node.id"
      :popover-hex="colorPopoverHex"
      :popover-hue="colorPopoverHue"
      :popover-saturation="colorPopoverSaturation"
      :popover-lightness="colorPopoverLightness"
      :spectrum-style="colorSpectrumStyle"
      :cursor-style="colorSpectrumCursorStyle"
      :quick-swatches="colorQuickSwatches"
      :ui-state="getColorNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('color', node.id)"
      @node-mousedown="emit('color-node-mousedown', $event, node)"
      @delete="emit('color-delete', node.id)"
      @toggle-popover="emit('color-toggle-popover', node)"
      @target-color-change="emit('color-target-color-change', node, $event)"
      @strength-input="emit('color-strength-input', node, $event)"
      @popover-hex-input="emit('color-popover-hex-input', $event)"
      @popover-hex-commit="emit('color-popover-hex-commit')"
      @popover-hue-input="emit('color-popover-hue-input', $event)"
      @popover-saturation-input="emit('color-popover-saturation-input', $event)"
      @popover-lightness-input="emit('color-popover-lightness-input', $event)"
      @spectrum-pointerdown="emit('color-spectrum-pointerdown', $event)"
      @select-swatch="emit('color-select-swatch', $event)"
      @eyedropper="emit('color-eyedropper', node)"
      @generate="emit('color-generate', node)"
      @set-spectrum-ref="emit('color-set-spectrum-ref', $event)"
      @port-mousedown="(event, port) => emit('color-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in editNodes" :key="node.id">
    <EditNode
      :node="node"
      :active="isNodeSelected('edit', node.id)"
      :dragging="isNodeDragging('edit', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getEditNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('edit', node.id)"
      @node-mousedown="emit('edit-node-mousedown', $event, node)"
      @delete="emit('edit-delete', node.id)"
      @update:mode="emit('edit-update-mode', node, $event)"
      @generate="emit('edit-generate', node)"
      @port-mousedown="(event, port) => emit('edit-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in fusionNodes" :key="node.id">
    <FusionNode
      :node="node"
      :active="isNodeSelected('fusion', node.id)"
      :dragging="isNodeDragging('fusion', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getFusionNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('fusion', node.id)"
      @node-mousedown="emit('fusion-node-mousedown', $event, node)"
      @delete="emit('fusion-delete', node.id)"
      @update:mode="emit('fusion-update-mode', node, $event)"
      @generate="emit('fusion-generate', node)"
      @port-mousedown="(event, port) => emit('fusion-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in enhanceNodes" :key="node.id">
    <EnhanceNode
      :node="node"
      :active="isNodeSelected('enhance', node.id)"
      :dragging="isNodeDragging('enhance', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getEnhanceNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('enhance', node.id)"
      @node-mousedown="emit('enhance-node-mousedown', $event, node)"
      @delete="emit('enhance-delete', node.id)"
      @update:mode="emit('enhance-update-mode', node, $event)"
      @update:upscale-mode="emit('enhance-update-upscale-mode', node, $event)"
      @update:upscale-factor="emit('enhance-update-upscale-factor', node, $event)"
      @update:output-format="emit('enhance-update-output-format', node, $event)"
      @update:output-quality="emit('enhance-update-output-quality', node, $event)"
      @generate="emit('enhance-generate', node)"
      @port-mousedown="(event, port) => emit('enhance-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in tryOnNodes" :key="node.id">
    <TryOnNode
      :node="node"
      :active="isNodeSelected('tryon', node.id)"
      :dragging="isNodeDragging('tryon', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getTryOnNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('tryon', node.id)"
      @node-mousedown="emit('tryon-node-mousedown', $event, node)"
      @delete="emit('tryon-delete', node.id)"
      @update:mode="emit('tryon-update-mode', node, $event)"
      @update:tile-mode="emit('tryon-update-tile-mode', node, $event)"
      @update:rotation="emit('tryon-update-rotation', node, $event)"
      @update:scale="emit('tryon-update-scale', node, $event)"
      @update:opacity="emit('tryon-update-opacity', node, $event)"
      @generate="emit('tryon-generate', node)"
      @port-mousedown="(event, port) => emit('tryon-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in faceSwapNodes" :key="node.id">
    <FaceSwapNode
      :node="node"
      :active="isNodeSelected('faceswap', node.id)"
      :dragging="isNodeDragging('faceswap', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getFaceSwapNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('faceswap', node.id)"
      @node-mousedown="emit('faceswap-node-mousedown', $event, node)"
      @delete="emit('faceswap-delete', node.id)"
      @generate="emit('faceswap-generate', node)"
      @port-mousedown="(event, port) => emit('faceswap-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in paletteNodes" :key="node.id">
    <PaletteNode
      :node="node"
      :active="isNodeSelected('palette', node.id)"
      :dragging="isNodeDragging('palette', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getPaletteNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('palette', node.id)"
      @node-mousedown="emit('palette-node-mousedown', $event, node)"
      @delete="emit('palette-delete', node.id)"
      @update:mode="emit('palette-update-mode', node, $event)"
      @apply-preset="emit('palette-apply-preset', node, $event)"
      @remove-color="emit('palette-remove-color', node, $event)"
      @add-color="emit('palette-add-color', node, $event)"
      @generate="emit('palette-generate', node)"
      @port-mousedown="(event, port) => emit('palette-port-mousedown', event, node.id, port)"
    />
  </template>

  <template v-for="node in generateNodes" :key="node.id">
    <GenerateNode
      :node="node"
      :active="isNodeSelected('generate', node.id)"
      :dragging="isNodeDragging('generate', node.id)"
      :copy-mode="copyMode"
      :generating="generating"
      :ui-state="getGenerateNodeUiState(node)"
      :error-port-keys="getNodeConnectionErrorPorts('generate', node.id)"
      :style-options="getGenerateStyleOptions(node.mode)"
      :styles-loading="isGenerateStyleLoading(node.mode)"
      @node-mousedown="emit('generate-node-mousedown', $event, node)"
      @delete="emit('generate-delete', node.id)"
      @update:mode="emit('generate-update-mode', node, $event)"
      @update:style-model="emit('generate-update-style-model', node, $event)"
      @update:ratio="emit('generate-update-ratio', node, $event)"
      @update:count="emit('generate-update-count', node, $event)"
      @update:quality="emit('generate-update-quality', node, $event)"
      @generate="emit('generate-generate', node)"
      @port-mousedown="(event, port) => emit('generate-port-mousedown', event, node.id, port)"
    />
  </template>
</template>

<script setup lang="ts">
import ColorNode from '../../../components/canvas/nodes/ColorNode.vue'
import EditNode from '../../../components/canvas/nodes/EditNode.vue'
import EnhanceNode from '../../../components/canvas/nodes/EnhanceNode.vue'
import FaceSwapNode from '../../../components/canvas/nodes/FaceSwapNode.vue'
import FusionNode from '../../../components/canvas/nodes/FusionNode.vue'
import GenerateNode from '../../../components/canvas/nodes/GenerateNode.vue'
import PaletteNode from '../../../components/canvas/nodes/PaletteNode.vue'
import TryOnNode from '../../../components/canvas/nodes/TryOnNode.vue'
import type {
  ColorNodeItem,
  EditNodeItem,
  EnhanceNodeItem,
  FaceSwapNodeItem,
  FusionNodeItem,
  GenerateNodeItem,
  PaletteNodeItem,
  TryOnNodeItem,
} from '../../../types/canvasNodes'
import type { NodeUiState } from '../../../types/canvasNodeUi'

const props = defineProps<{
  colorNodes: ColorNodeItem[]
  editNodes: EditNodeItem[]
  fusionNodes: FusionNodeItem[]
  enhanceNodes: EnhanceNodeItem[]
  tryOnNodes: TryOnNodeItem[]
  faceSwapNodes: FaceSwapNodeItem[]
  paletteNodes: PaletteNodeItem[]
  generateNodes: GenerateNodeItem[]
  generating: boolean
  copyMode: boolean
  isNodeSelected: (
    kind: 'color' | 'edit' | 'fusion' | 'enhance' | 'tryon' | 'faceswap' | 'palette' | 'generate',
    id: number,
  ) => boolean
  isNodeDragging: (
    kind: 'color' | 'edit' | 'fusion' | 'enhance' | 'tryon' | 'faceswap' | 'palette' | 'generate',
    id: number,
  ) => boolean
  getColorNodeUiState: (node: ColorNodeItem) => NodeUiState | undefined
  getEditNodeUiState: (node: EditNodeItem) => NodeUiState | undefined
  getFusionNodeUiState: (node: FusionNodeItem) => NodeUiState | undefined
  getEnhanceNodeUiState: (node: EnhanceNodeItem) => NodeUiState | undefined
  getTryOnNodeUiState: (node: TryOnNodeItem) => NodeUiState | undefined
  getFaceSwapNodeUiState: (node: FaceSwapNodeItem) => NodeUiState | undefined
  getPaletteNodeUiState: (node: PaletteNodeItem) => NodeUiState | undefined
  getGenerateNodeUiState: (node: GenerateNodeItem) => NodeUiState | undefined
  getNodeConnectionErrorPorts: (
    kind: 'color' | 'edit' | 'fusion' | 'enhance' | 'tryon' | 'faceswap' | 'palette' | 'generate',
    id: number,
  ) => string[]
  activeColorPopoverNodeId: number | null
  colorPopoverHex: string
  colorPopoverHue: number
  colorPopoverSaturation: number
  colorPopoverLightness: number
  colorSpectrumStyle: Record<string, string>
  colorSpectrumCursorStyle: Record<string, string>
  colorQuickSwatches: string[]
  getGenerateStyleOptions: (mode: GenerateNodeItem['mode']) => string[]
  isGenerateStyleLoading: (mode: GenerateNodeItem['mode']) => boolean
}>()

const emit = defineEmits<{
  (e: 'color-node-mousedown', event: MouseEvent, node: ColorNodeItem): void
  (e: 'color-delete', nodeId: number): void
  (e: 'color-toggle-popover', node: ColorNodeItem): void
  (e: 'color-target-color-change', node: ColorNodeItem, value: string): void
  (e: 'color-strength-input', node: ColorNodeItem, value: number): void
  (e: 'color-popover-hex-input', event: Event): void
  (e: 'color-popover-hex-commit'): void
  (e: 'color-popover-hue-input', value: number): void
  (e: 'color-popover-saturation-input', value: number): void
  (e: 'color-popover-lightness-input', value: number): void
  (e: 'color-spectrum-pointerdown', event: PointerEvent): void
  (e: 'color-select-swatch', value: string): void
  (e: 'color-eyedropper', node: ColorNodeItem): void
  (e: 'color-generate', node: ColorNodeItem): void
  (e: 'color-set-spectrum-ref', element: Element | null): void
  (e: 'color-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'edit-node-mousedown', event: MouseEvent, node: EditNodeItem): void
  (e: 'edit-delete', nodeId: number): void
  (e: 'edit-update-mode', node: EditNodeItem, value: EditNodeItem['mode']): void
  (e: 'edit-generate', node: EditNodeItem): void
  (e: 'edit-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'fusion-node-mousedown', event: MouseEvent, node: FusionNodeItem): void
  (e: 'fusion-delete', nodeId: number): void
  (e: 'fusion-update-mode', node: FusionNodeItem, value: FusionNodeItem['mode']): void
  (e: 'fusion-generate', node: FusionNodeItem): void
  (e: 'fusion-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'enhance-node-mousedown', event: MouseEvent, node: EnhanceNodeItem): void
  (e: 'enhance-delete', nodeId: number): void
  (e: 'enhance-update-mode', node: EnhanceNodeItem, value: EnhanceNodeItem['mode']): void
  (e: 'enhance-update-upscale-mode', node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleMode']): void
  (e: 'enhance-update-upscale-factor', node: EnhanceNodeItem, value: EnhanceNodeItem['upscaleFactor']): void
  (e: 'enhance-update-output-format', node: EnhanceNodeItem, value: EnhanceNodeItem['outputFormat']): void
  (e: 'enhance-update-output-quality', node: EnhanceNodeItem, value: number): void
  (e: 'enhance-generate', node: EnhanceNodeItem): void
  (e: 'enhance-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'tryon-node-mousedown', event: MouseEvent, node: TryOnNodeItem): void
  (e: 'tryon-delete', nodeId: number): void
  (e: 'tryon-update-mode', node: TryOnNodeItem, value: TryOnNodeItem['mode']): void
  (e: 'tryon-update-tile-mode', node: TryOnNodeItem, value: TryOnNodeItem['tileMode']): void
  (e: 'tryon-update-rotation', node: TryOnNodeItem, value: number): void
  (e: 'tryon-update-scale', node: TryOnNodeItem, value: number): void
  (e: 'tryon-update-opacity', node: TryOnNodeItem, value: number): void
  (e: 'tryon-generate', node: TryOnNodeItem): void
  (e: 'tryon-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'faceswap-node-mousedown', event: MouseEvent, node: FaceSwapNodeItem): void
  (e: 'faceswap-delete', nodeId: number): void
  (e: 'faceswap-generate', node: FaceSwapNodeItem): void
  (e: 'faceswap-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'palette-node-mousedown', event: MouseEvent, node: PaletteNodeItem): void
  (e: 'palette-delete', nodeId: number): void
  (e: 'palette-update-mode', node: PaletteNodeItem, value: PaletteNodeItem['mode']): void
  (e: 'palette-apply-preset', node: PaletteNodeItem, value: string[]): void
  (e: 'palette-remove-color', node: PaletteNodeItem, index: number): void
  (e: 'palette-add-color', node: PaletteNodeItem, value: string): void
  (e: 'palette-generate', node: PaletteNodeItem): void
  (e: 'palette-port-mousedown', event: MouseEvent, nodeId: number, port: string): void

  (e: 'generate-node-mousedown', event: MouseEvent, node: GenerateNodeItem): void
  (e: 'generate-delete', nodeId: number): void
  (e: 'generate-update-mode', node: GenerateNodeItem, value: GenerateNodeItem['mode']): void
  (e: 'generate-update-style-model', node: GenerateNodeItem, value: string): void
  (e: 'generate-update-ratio', node: GenerateNodeItem, value: GenerateNodeItem['ratio']): void
  (e: 'generate-update-count', node: GenerateNodeItem, value: GenerateNodeItem['count']): void
  (e: 'generate-update-quality', node: GenerateNodeItem, value: GenerateNodeItem['quality']): void
  (e: 'generate-generate', node: GenerateNodeItem): void
  (e: 'generate-port-mousedown', event: MouseEvent, nodeId: number, port: string): void
}>()
</script>
