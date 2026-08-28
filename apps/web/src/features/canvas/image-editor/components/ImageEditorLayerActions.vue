<template>
  <div class="image-editor-layer-actions node-tool-surface">
    <div class="image-editor-layer-actions__buttons">
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :aria-label="t('canvas.imageEditor.addLayer')"
        :tooltip="t('canvas.imageEditor.addLayer')"
        @click="emit('add-layer')"
      >
        <EmptyPage />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :disabled="!canCopyLayer"
        :aria-label="t('canvas.imageEditor.duplicateLayer')"
        :tooltip="t('canvas.imageEditor.duplicateLayer')"
        @click="emit('copy-layer')"
      >
        <Copy />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :disabled="!canMergeLayer"
        :aria-label="t('canvas.imageEditor.mergeLayers')"
        :tooltip="t('canvas.imageEditor.mergeLayers')"
        @click="emit('merge-layer')"
      >
        <VerticalMerge />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button"
        :disabled="!canRemoveLayer"
        :aria-label="t('canvas.imageEditor.deleteLayer')"
        :tooltip="t('canvas.imageEditor.deleteLayer')"
        @click="emit('remove-layer')"
      >
        <Trash />
      </NodeToolIconButton>
      <NodeToolIconButton
        class="image-editor-layer-actions__button image-editor-layer-actions__button--apply"
        primary
        :disabled="applyDisabled"
        :aria-label="
          applying ? t('canvas.imageEditor.applying') : t('canvas.imageEditor.applyAsNewNode')
        "
        :tooltip="
          applying ? t('canvas.imageEditor.applying') : t('canvas.imageEditor.applyAsNewNode')
        "
        @click="emit('apply-adjustments')"
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
  (e: 'apply-adjustments'): void
}>()

const { t } = useI18n()
</script>

<style scoped>
.image-editor-layer-actions {
  display: flex;
  flex-direction: column;
  gap: var(--node-tool-button-gap);
}

.image-editor-layer-actions__buttons {
  display: grid;
  grid-template-columns: repeat(5, var(--node-tool-button-size));
  justify-content: space-between;
  gap: var(--node-tool-button-gap);
}

.image-editor-layer-actions__button {
  width: var(--node-tool-button-size);
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-icon-size: 18px;
}

.image-editor-layer-actions__button--apply {
  --node-tool-icon-button-primary-border: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-bg: var(--canvas-accent-hover);
  --node-tool-icon-button-primary-color: #ffffff;
  --node-tool-icon-button-primary-hover-border: var(--canvas-accent-strong);
  --node-tool-icon-button-primary-hover-bg: var(--canvas-accent-strong);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}
</style>
