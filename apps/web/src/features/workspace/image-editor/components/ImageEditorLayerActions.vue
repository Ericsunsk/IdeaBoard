<template>
  <div class="image-editor-layer-actions node-tool-surface">
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      aria-label-value="添加图层"
      tooltip="添加图层"
      @click="emit('add-layer')"
    >
      <EmptyPage />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      :disabled="!canCopyLayer"
      aria-label-value="复制图层"
      tooltip="复制图层"
      @click="emit('copy-layer')"
    >
      <Copy />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      :disabled="!canMergeLayer"
      aria-label-value="合并图层"
      tooltip="合并图层"
      @click="emit('merge-layer')"
    >
      <VerticalMerge />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button image-editor-layer-actions__button--apply"
      primary
      :disabled="applyDisabled"
      :aria-label-value="applying ? '应用中' : '应用为新节点'"
      :tooltip="applying ? '应用中' : '应用为新节点'"
      @click="emit('apply-adjustments')"
    >
      <SendDiagonalSolid />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      :disabled="!canRemoveLayer"
      aria-label-value="删除图层"
      tooltip="删除图层"
      @click="emit('remove-layer')"
    >
      <Trash />
    </NodeToolIconButton>
  </div>
</template>

<script setup lang="ts">
import { Copy, EmptyPage, SendDiagonalSolid, Trash, VerticalMerge } from '@iconoir/vue'
import NodeToolIconButton from '../../components/nodes/NodeToolIconButton.vue'

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
</script>

<style scoped src="../../components/nodes/styles/node-tool-buttons.css"></style>
<style scoped>
.image-editor-layer-actions {
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
  --node-tool-icon-button-primary-border: var(--workspace-accent-hover);
  --node-tool-icon-button-primary-bg: var(--workspace-accent-hover);
  --node-tool-icon-button-primary-color: #ffffff;
  --node-tool-icon-button-primary-hover-border: var(--workspace-accent-strong);
  --node-tool-icon-button-primary-hover-bg: var(--workspace-accent-strong);
  --node-tool-icon-button-primary-hover-color: #ffffff;
}
</style>
