<template>
  <div class="flow-canvas-node__handle-rail">
    <FlowNodeHandles :presentation="presentation" />
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from 'vue'
import type { FlowNodeData } from '../types/flowTypes'
import { useFlowCanvasNodeHandleRail } from '../composables/useFlowCanvasNodeHandleRail'
import { useFlowNodeHandlesPresentation } from '../composables/useFlowNodeHandlesPresentation'
import FlowNodeHandles from './FlowNodeHandles.vue'

const props = defineProps<{
  kind: FlowNodeData['kind']
  interactionRoot: HTMLElement | null
  visibilityRefreshToken?: number
  selected?: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{
  (event: 'request-internals-update'): void
}>()

const interactionRoot = computed(() => props.interactionRoot)
const { showHandleControls, refreshTransientHandleVisibility } =
  useFlowCanvasNodeHandleRail(interactionRoot)
const { handleLayoutIdentity, presentation } = useFlowNodeHandlesPresentation({
  kind: () => props.kind,
  showHandles: () => props.selected === true || showHandleControls.value,
  disabled: () => props.disabled === true,
})

watch(handleLayoutIdentity, () => emit('request-internals-update'), { flush: 'post' })
watch(
  () => props.visibilityRefreshToken,
  () => refreshTransientHandleVisibility(),
)
</script>

<style scoped>
.flow-canvas-node__handle-rail {
  position: absolute;
  inset: 0;
  z-index: 24;
  pointer-events: none;
  overflow: visible;
}
</style>
