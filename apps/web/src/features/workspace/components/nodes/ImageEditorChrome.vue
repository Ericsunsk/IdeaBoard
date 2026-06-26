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
        @remove-layer="emit('remove-layer', $event)"
      />
      <ImageEditorLayerActions
        :can-remove-layer="canRemoveSelectedLayer"
        @add-adjustment="emit('select-adjustment', $event)"
        @remove-layer="emit('remove-layer', selectedLayerId)"
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
} from '../../types/imageEditor'
import ImageEditorAdjustmentActions from './ImageEditorAdjustmentActions.vue'
import ImageEditorChromeFrame from './ImageEditorChromeFrame.vue'
import ImageEditorLayerActions from './ImageEditorLayerActions.vue'
import ImageEditorLayersPanel from './ImageEditorLayersPanel.vue'
import ImageEditorPropertiesPanel from './ImageEditorPropertiesPanel.vue'
import ImageEditorToolRail from './ImageEditorToolRail.vue'

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
}>()

const emit = defineEmits<{
  (e: 'select-tool', tool: ImageEditorTool): void
  (e: 'select-adjustment', kind: ImageAdjustmentKind): void
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
  (e: 'remove-layer', layerId: string): void
  (e: 'update-param', key: ImageEditorParamKey, value: ImageEditorParamValue): void
}>()

const canRemoveSelectedLayer = computed(
  () => props.layers.find((layer) => layer.id === props.selectedLayerId)?.type === 'adjustment',
)
</script>
