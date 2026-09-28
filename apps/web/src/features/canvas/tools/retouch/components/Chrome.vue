<template>
  <ChromeFrame mode="adjust" :anchor-rect="anchorRect">
    <template #dock>
      <RetouchEffectActions
        :selected-kind="selectedEffectLayer?.kind ?? null"
        @select-retouch="emit('select-retouch', $event)"
      />
      <RetouchPropertiesPanel
        :effect-layer="selectedEffectLayer"
        :histogram-source="histogramSource"
        @update-param="(key, value) => emit('update-param', key, value)"
        @update:params="emit('update:params', $event)"
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
        :applying="applyingRetouch"
        :apply-disabled="!canApplyRetouch || applyingRetouch"
        @add-layer="emit('add-layer')"
        @copy-layer="emit('copy-layer')"
        @merge-layer="emit('merge-layer')"
        @remove-layer="emit('remove-layer', selectedLayerId)"
        @apply-retouch="emit('apply-retouch')"
      />
    </template>
  </ChromeFrame>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchEffectWholeParams,
  RetouchLayer,
  RetouchParamKey,
  RetouchParamValue,
} from '../model.ts'
import RetouchEffectActions from './EffectActions.vue'
import ChromeFrame from '../../image-tools/components/ChromeFrame.vue'
import RetouchLayerActions from './LayerActions.vue'
import RetouchLayersPanel from './LayersPanel.vue'
import RetouchPropertiesPanel from './PropertiesPanel.vue'
import { hasRenderableRetouchLayerStack } from '../rendering/renderer'

const props = defineProps<{
  anchorRect: {
    top: number
    left: number
    width: number
    height: number
  }
  layers: RetouchLayer[]
  selectedLayerId: string
  selectedEffectLayer: RetouchEffectLayer | null
  histogramSource: ImageData | null
  applyingRetouch: boolean
}>()

const emit = defineEmits<{
  (e: 'select-retouch', kind: ImageRetouchKind): void
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
  (e: 'add-layer'): void
  (e: 'copy-layer'): void
  (e: 'merge-layer'): void
  (e: 'remove-layer', layerId: string): void
  (e: 'update-param', key: RetouchParamKey, value: RetouchParamValue): void
  (e: 'update:params', params: RetouchEffectWholeParams): void
  (e: 'apply-retouch'): void
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
const canApplyRetouch = computed(() => hasRenderableRetouchLayerStack(props.layers))
</script>
