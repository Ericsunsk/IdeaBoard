<template>
  <div class="retouch-action-row node-tool-surface">
    <NodeToolIconButton
      v-for="item in actionItems"
      :key="item.kind"
      class="retouch-effect-actions__button node-tool-icon-button--accent-hover"
      :active="selectedKind === item.kind"
      :aria-label="item.label"
      :tooltip="item.label"
      @click="emit('select-retouch', item.kind)"
    >
      <component :is="item.icon" />
    </NodeToolIconButton>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Component } from 'vue'
import type { ImageRetouchKind } from '../../core/model.ts'
import { RETOUCH_EFFECT_RAIL } from '../../session/retouchViewModel.ts'
import { NodeToolIconButton } from '../../../../presentation/nodes/primitives'
import '../shared/styles/retouch-panel.css'

defineProps<{
  selectedKind: ImageRetouchKind | null
}>()

const emit = defineEmits<{
  (e: 'select-retouch', kind: ImageRetouchKind): void
}>()

const { t } = useI18n()

const actionItems = computed<
  Array<{
    kind: ImageRetouchKind
    label: string
    icon: Component
  }>
>(() =>
  // Rail order, icons, and labelKeys come from the retouch view model; t()
  // runs here at call time so locale switches re-translate (the same
  // runtime precedent CurvesControl uses for its preset labels).
  RETOUCH_EFFECT_RAIL.map((item) => ({
    kind: item.kind,
    label: t(item.labelKey),
    icon: item.icon,
  })),
)
</script>

<style scoped>
/* Row grid lives in the shared retouch panel contract (`.retouch-action-row`). */
.retouch-effect-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
}
</style>
