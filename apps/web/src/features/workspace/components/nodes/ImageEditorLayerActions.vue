<template>
  <div class="image-editor-layer-actions node-tool-surface">
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      disabled
      aria-label-value="添加图层（待支持）"
      tooltip="添加图层（待支持）"
    >
      <EmptyPage />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      aria-label-value="添加调整"
      tooltip="添加调整"
      @click="emit('add-adjustment', 'hsl')"
    >
      <PlusSquare />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      disabled
      aria-label-value="复制图层（待支持）"
      tooltip="复制图层（待支持）"
    >
      <Copy />
    </NodeToolIconButton>
    <NodeToolIconButton
      class="image-editor-layer-actions__button"
      disabled
      aria-label-value="添加蒙版（待支持）"
      tooltip="添加蒙版（待支持）"
    >
      <MaskSquare />
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
import { Copy, EmptyPage, MaskSquare, PlusSquare, Trash } from '@iconoir/vue'
import type { ImageAdjustmentKind } from '../../types/imageEditor'
import NodeToolIconButton from './NodeToolIconButton.vue'

defineProps<{
  canRemoveLayer: boolean
}>()

const emit = defineEmits<{
  (e: 'add-adjustment', kind: ImageAdjustmentKind): void
  (e: 'remove-layer'): void
}>()
</script>

<style scoped src="./styles/node-tool-buttons.css"></style>
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
</style>
