<template>
  <div class="image-editor-adjustment-actions node-tool-surface">
    <NodeToolIconButton
      v-for="item in actionItems"
      :key="item.kind"
      class="image-editor-adjustment-actions__button"
      :active="selectedKind === item.kind"
      :aria-label="item.label"
      :tooltip="item.label"
      @click="emit('select-adjustment', item.kind)"
    >
      <component :is="item.icon" />
    </NodeToolIconButton>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { Brightness, ColorFilter, PrecisionTool, ReportsSolid, SineWave } from '@iconoir/vue'
import type { ImageAdjustmentKind } from '../types'
import NodeToolIconButton from '../../components/nodes/NodeToolIconButton.vue'

defineProps<{
  selectedKind: ImageAdjustmentKind | null
}>()

const emit = defineEmits<{
  (e: 'select-adjustment', kind: ImageAdjustmentKind): void
}>()

const actionItems: Array<{
  kind: ImageAdjustmentKind
  label: string
  icon: Component
}> = [
  { kind: 'threshold', label: '阈值', icon: Brightness },
  { kind: 'hsl', label: '色相/饱和度', icon: ColorFilter },
  { kind: 'sharpen', label: '锐化', icon: PrecisionTool },
  { kind: 'curves', label: '曲线', icon: SineWave },
  { kind: 'levels', label: '色阶', icon: ReportsSolid },
]
</script>

<style scoped>
.image-editor-adjustment-actions {
  display: grid;
  grid-template-columns: repeat(5, var(--node-tool-button-size));
  justify-content: space-between;
  gap: var(--node-tool-button-gap);
}

.image-editor-adjustment-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-primary-hover-border: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-hover-bg: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}
</style>
