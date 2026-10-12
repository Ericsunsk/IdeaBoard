<template>
  <ChromeFrame mode="adjust" :anchor-rect="anchorRect">
    <template #dock>
      <RetouchEffectActions
        :selected-kind="chromeSelection.selectedKind"
        @select-retouch="emit('select-retouch', $event)"
      />
      <RetouchPropertiesPanel
        :effect-layer="selectedEffectLayer"
        :histogram-source="histogramSource"
        @update-param="forwardUpdateParam"
        @update:params="emit('update:params', $event)"
      />
      <RetouchLayersPanel
        :layers="layers"
        :selected-layer-id="chromeSelection.selectedLayerId"
        @select-layer="emit('select-layer', $event)"
        @set-layer-visible="(layerId, visible) => emit('set-layer-visible', layerId, visible)"
      />
      <RetouchLayerActions
        :can-copy-layer="canCopySelectedLayer"
        :can-merge-layer="canMergeSelectedLayer"
        :can-remove-layer="canRemoveSelectedLayer"
        :applying="applyingRetouch"
        :apply-disabled="!canApply || applyingRetouch"
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
  RetouchNonHslWholeParams,
  RetouchEffectParamKey,
  RetouchEffectParamMap,
  RetouchLayer,
} from '../../core/model.ts'
import RetouchEffectActions from './EffectActions.vue'
import { ImageToolChromeFrame as ChromeFrame } from '../../../image-tools/ui/chrome'
import RetouchLayerActions from '../layers/LayerActions.vue'
import RetouchLayersPanel from '../layers/LayersPanel.vue'
import RetouchPropertiesPanel from './PropertiesPanel.vue'
import {
  canCopySelectedLayer as canCopySelectedLayerInStack,
  canMergeSelectedLayer as canMergeSelectedLayerInStack,
  canRemoveSelectedLayer as canRemoveSelectedLayerInStack,
} from '../../core/layers/layerStack.ts'
import { resolveRetouchChromeSelection } from '../../session/retouchViewModel.ts'

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
  /** Parent-injected apply gate (same hasRenderable check as commands.readApplySource). */
  canApply: boolean
}>()

const emit = defineEmits<{
  (e: 'select-retouch', kind: ImageRetouchKind): void
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
  (e: 'add-layer'): void
  (e: 'copy-layer'): void
  (e: 'merge-layer'): void
  (e: 'remove-layer', layerId: string): void
  (
    e: 'update-param',
    key: RetouchEffectParamKey,
    value: RetouchEffectParamMap[RetouchEffectParamKey],
  ): void
  (e: 'update:params', params: RetouchNonHslWholeParams): void
  (e: 'apply-retouch'): void
}>()

function forwardUpdateParam<TKey extends RetouchEffectParamKey>(
  key: TKey,
  value: RetouchEffectParamMap[TKey],
) {
  emit('update-param', key, value)
}

/*
 * Capability predicates live in layerStack as pure stack reads; Chrome only
 * binds them to disabled= via LayerActions props.
 *
 * Rail highlight and list cursor resolve through one view-model selection so
 * the two panels stay consistent: the kind follows the single editing
 * identity (pending preview included), the id follows the tree cursor.
 */
const chromeSelection = computed(() =>
  resolveRetouchChromeSelection({
    selectedEffectLayer: props.selectedEffectLayer,
    selectedLayerId: props.selectedLayerId,
  }),
)
const stackForCapabilities = computed(() => ({
  layers: props.layers,
  selectedLayerId: props.selectedLayerId,
}))
const canCopySelectedLayer = computed(() => canCopySelectedLayerInStack(stackForCapabilities.value))
const canMergeSelectedLayer = computed(() =>
  canMergeSelectedLayerInStack(stackForCapabilities.value),
)
const canRemoveSelectedLayer = computed(() =>
  canRemoveSelectedLayerInStack(stackForCapabilities.value),
)
</script>
