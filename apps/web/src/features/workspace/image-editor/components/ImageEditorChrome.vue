<template>
  <ImageEditorChromeFrame mode="adjust" :anchor-rect="anchorRect">
    <template #tools>
      <ImageEditorToolRail :active-tool="activeTool" @select-tool="emit('select-tool', $event)" />
    </template>
    <template #dock>
      <ImageEditorAdjustmentActions
        :selected-kind="selectedAdjustmentLayer?.kind ?? null"
        @select-adjustment="emit('select-adjustment', $event)"
      />
      <ImageEditorPropertiesPanel
        :adjustment-layer="selectedAdjustmentLayer"
        @update-param="(key, value) => emit('update-param', key, value)"
      />
      <ImageEditorLayersPanel
        :layers="layers"
        :selected-layer-id="selectedLayerId"
        @select-layer="emit('select-layer', $event)"
        @set-layer-visible="(layerId, visible) => emit('set-layer-visible', layerId, visible)"
      />
      <ImageEditorLayerActions
        :can-copy-layer="canCopySelectedLayer"
        :can-merge-layer="canMergeSelectedLayer"
        :can-remove-layer="canRemoveSelectedLayer"
        :applying="applyingAdjustments"
        :apply-disabled="!canApplyAdjustments || applyingAdjustments"
        @add-layer="emit('add-layer')"
        @copy-layer="emit('copy-layer')"
        @merge-layer="emit('merge-layer')"
        @remove-layer="emit('remove-layer', selectedLayerId)"
        @apply-adjustments="emit('apply-adjustments')"
      />
    </template>
  </ImageEditorChromeFrame>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorTool,
} from '../types'
import ImageEditorAdjustmentActions from './ImageEditorAdjustmentActions.vue'
import ImageEditorChromeFrame from '../../components/nodes/ImageEditorChromeFrame.vue'
import ImageEditorLayerActions from './ImageEditorLayerActions.vue'
import ImageEditorLayersPanel from './ImageEditorLayersPanel.vue'
import ImageEditorPropertiesPanel from './ImageEditorPropertiesPanel.vue'
import ImageEditorToolRail from './ImageEditorToolRail.vue'
import { flattenVisibleImageEditorAdjustmentLayers } from '../utils/imageEditorLayerAdjustments'

const props = defineProps<{
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
  activeTool: ImageEditorTool
  layers: ImageEditorLayer[]
  selectedLayerId: string
  selectedAdjustmentLayer: ImageEditorAdjustmentLayer | null
  applyingAdjustments: boolean
}>()

const emit = defineEmits<{
  (e: 'select-tool', tool: ImageEditorTool): void
  (e: 'select-adjustment', kind: ImageAdjustmentKind): void
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
  (e: 'add-layer'): void
  (e: 'copy-layer'): void
  (e: 'merge-layer'): void
  (e: 'remove-layer', layerId: string): void
  (e: 'update-param', key: ImageEditorParamKey, value: ImageEditorParamValue): void
  (e: 'apply-adjustments'): void
}>()

const selectedLayerIndex = computed(() =>
  props.layers.findIndex((layer) => layer.id === props.selectedLayerId),
)
const selectedLayer = computed(() => props.layers[selectedLayerIndex.value] ?? null)
const canCopySelectedLayer = computed(
  () => selectedLayer.value != null && selectedLayer.value.type !== 'source',
)
const canMergeSelectedLayer = computed(() => {
  const index = selectedLayerIndex.value
  const layer = props.layers[index]
  const target = props.layers[index + 1]
  return !!layer && !!target && layer.type !== 'source' && target.type !== 'source'
})
const canRemoveSelectedLayer = computed(
  () => selectedLayer.value != null && selectedLayer.value.type !== 'source',
)
const canApplyAdjustments = computed(
  () => flattenVisibleImageEditorAdjustmentLayers(props.layers).length > 0,
)
</script>
