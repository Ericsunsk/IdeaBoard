<template>
  <ImageToolChromeFrame mode="adjust" :anchor-rect="anchorRect">
    <template #dock>
      <RetouchAdjustmentActions
        :selected-kind="selectedAdjustmentLayer?.kind ?? null"
        @select-adjustment="emit('select-adjustment', $event)"
      />
      <RetouchPropertiesPanel
        :adjustment-layer="selectedAdjustmentLayer"
        :histogram-source="histogramSource"
        @update-param="(key, value) => emit('update-param', key, value)"
        @update-params="emit('update-params', $event)"
      />
      <RetouchLayersPanel
        :layers="layers"
        :selected-layer-id="selectedLayerId"
        @select-layer="emit('select-layer', $event)"
        @set-layer-visible="(layerId, visible) => emit('set-layer-visible', layerId, visible)"
      />
      <RetouchLayerActions
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
  </ImageToolChromeFrame>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type {
  ImageAdjustmentKind,
  RetouchAdjustmentLayer,
  RetouchAdjustmentWholeParams,
  RetouchLayer,
  RetouchParamKey,
  RetouchParamValue,
} from '../types'
import RetouchAdjustmentActions from './RetouchAdjustmentActions.vue'
import ImageToolChromeFrame from '../../image-tools/components/ImageToolChromeFrame.vue'
import RetouchLayerActions from './RetouchLayerActions.vue'
import RetouchLayersPanel from './RetouchLayersPanel.vue'
import RetouchPropertiesPanel from './RetouchPropertiesPanel.vue'
import { hasRenderableRetouchLayerStack } from '../services/imageAdjustmentRenderer'

const props = defineProps<{
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
  layers: RetouchLayer[]
  selectedLayerId: string
  selectedAdjustmentLayer: RetouchAdjustmentLayer | null
  histogramSource: ImageData | null
  applyingAdjustments: boolean
}>()

const emit = defineEmits<{
  (e: 'select-adjustment', kind: ImageAdjustmentKind): void
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
  (e: 'add-layer'): void
  (e: 'copy-layer'): void
  (e: 'merge-layer'): void
  (e: 'remove-layer', layerId: string): void
  (e: 'update-param', key: RetouchParamKey, value: RetouchParamValue): void
  (e: 'update-params', params: RetouchAdjustmentWholeParams): void
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
const canApplyAdjustments = computed(() => hasRenderableRetouchLayerStack(props.layers))
</script>
