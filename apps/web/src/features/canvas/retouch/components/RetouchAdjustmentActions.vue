<template>
  <div class="retouch-adjustment-actions node-tool-surface">
    <NodeToolIconButton
      v-for="item in actionItems"
      :key="item.kind"
      class="retouch-adjustment-actions__button"
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
import { useI18n } from 'vue-i18n'
import type { Component } from 'vue'
import { Brightness, ColorFilter, PrecisionTool, ReportsSolid, SineWave } from '@iconoir/vue'
import type { ImageAdjustmentKind } from '../types'
import { NodeToolIconButton } from '../../components/nodes/primitives'

defineProps<{
  selectedKind: ImageAdjustmentKind | null
}>()

const emit = defineEmits<{
  (e: 'select-adjustment', kind: ImageAdjustmentKind): void
}>()

const { t } = useI18n()

const actionItems: Array<{
  kind: ImageAdjustmentKind
  label: string
  icon: Component
}> = [
  { kind: 'threshold', label: t('canvas.retouch.adjustments.threshold'), icon: Brightness },
  { kind: 'hsl', label: t('canvas.retouch.adjustments.hsl'), icon: ColorFilter },
  { kind: 'sharpen', label: t('canvas.retouch.adjustments.sharpen'), icon: PrecisionTool },
  { kind: 'curves', label: t('canvas.retouch.adjustments.curves'), icon: SineWave },
  { kind: 'levels', label: t('canvas.retouch.adjustments.levels'), icon: ReportsSolid },
]
</script>

<style scoped>
.retouch-adjustment-actions {
  display: grid;
  grid-template-columns: repeat(5, var(--node-tool-button-size));
  justify-content: space-between;
  gap: var(--node-tool-button-gap);
}

.retouch-adjustment-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-primary-hover-border: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-hover-bg: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}
</style>
