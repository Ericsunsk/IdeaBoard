<template>
  <div class="retouch-layer-actions node-tool-surface">
    <div class="retouch-layer-actions__buttons">
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
        class="retouch-layer-actions__button retouch-layer-actions__button--apply"
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
import { NodeToolIconButton } from '../../components/nodes/primitives'

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
.retouch-layer-actions {
  display: flex;
  flex-direction: column;
  gap: var(--node-tool-button-gap);
}

.retouch-layer-actions__buttons {
  display: grid;
  grid-template-columns: repeat(5, var(--node-tool-button-size));
  justify-content: space-between;
  gap: var(--node-tool-button-gap);
}

.retouch-layer-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-icon-size: 18px;
}

.retouch-layer-actions__button--apply {
  --node-tool-icon-button-primary-border: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-bg: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-color: #ffffff;
  --node-tool-icon-button-primary-hover-border: var(--canvas-accent-strong);
  --node-tool-icon-button-primary-hover-bg: var(--canvas-accent-strong);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}
</style>
