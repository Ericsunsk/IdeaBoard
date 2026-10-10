<template>
  <div class="retouch-layer-actions node-tool-surface">
    <div class="retouch-action-row">
      <NodeToolIconButton
        class="retouch-layer-actions__button"
        :aria-label="t('canvas.retouch.addLayer')"
        :tooltip="t('canvas.retouch.addLayer')"
        @click="emit('add-layer')"
      >
        <EmptyPage />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="retouch-layer-actions__button"
        :disabled="!canCopyLayer"
        :aria-label="t('canvas.retouch.duplicateLayer')"
        :tooltip="t('canvas.retouch.duplicateLayer')"
        @click="emit('copy-layer')"
      >
        <Copy />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="retouch-layer-actions__button"
        :disabled="!canMergeLayer"
        :aria-label="t('canvas.retouch.mergeLayers')"
        :tooltip="t('canvas.retouch.mergeLayers')"
        @click="emit('merge-layer')"
      >
        <VerticalMerge />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="retouch-layer-actions__button"
        :disabled="!canRemoveLayer"
        :aria-label="t('canvas.retouch.deleteLayer')"
        :tooltip="t('canvas.retouch.deleteLayer')"
        @click="emit('remove-layer')"
      >
        <Trash />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="retouch-layer-actions__button"
        primary
        :disabled="applyDisabled"
        :aria-label="applying ? t('canvas.retouch.applying') : t('canvas.retouch.applyAsNewNode')"
        :tooltip="applying ? t('canvas.retouch.applying') : t('canvas.retouch.applyAsNewNode')"
        @click="emit('apply-retouch')"
      >
        <SendDiagonalSolid />
      </NodeToolIconButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Copy, EmptyPage, SendDiagonalSolid, Trash, VerticalMerge } from '@iconoir/vue'
import { NodeToolIconButton } from '../../../../presentation/nodes/primitives'
import '../shared/styles/retouch-panel.css'

defineProps<{
  canCopyLayer: boolean
  canMergeLayer: boolean
  canRemoveLayer: boolean
  applying: boolean
  applyDisabled: boolean
}>()

const emit = defineEmits<{
  (e: 'add-layer'): void
  (e: 'copy-layer'): void
  (e: 'merge-layer'): void
  (e: 'remove-layer'): void
  (e: 'apply-retouch'): void
}>()

const { t } = useI18n()
</script>

<style scoped>
/*
 * Commit affordance stays distinct from preview controls: the apply button is
 * the row's only `primary` NodeToolIconButton, and its treatment resolves
 * through node-tool-surface (primary fill from --canvas-accent-hover, hover
 * from --canvas-accent-strong, ink from --canvas-text-inverse).
 * Do not re-add local primary overrides; they duplicated the surface defaults.
 */
.retouch-layer-actions {
  display: flex;
  flex-direction: column;
  gap: var(--node-tool-button-gap);
}

.retouch-layer-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-icon-size: 18px;
}
</style>
