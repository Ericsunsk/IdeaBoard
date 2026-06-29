<template>
  <NodeToolIconButton
    v-for="item in toolItems"
    :key="item.tool"
    class="image-editor-tool-rail__button"
    :active="activeTool === item.tool"
    :aria-label-value="item.label"
    :tooltip="item.label"
    @click="emit('select-tool', item.tool)"
  >
    <component :is="item.icon" />
  </NodeToolIconButton>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import {
  ColorPicker,
  Crop,
  DragHandGesture,
  Erase,
  EditPencil,
  SquareCursor,
  ZoomIn,
} from '@iconoir/vue'
import type { ImageEditorTool } from '../types'
import NodeToolIconButton from '../../components/nodes/NodeToolIconButton.vue'

defineProps<{
  activeTool: ImageEditorTool
}>()

const emit = defineEmits<{
  (e: 'select-tool', tool: ImageEditorTool): void
}>()

const toolItems: Array<{
  tool: ImageEditorTool
  label: string
  icon: Component
}> = [
  { tool: 'move', label: '移动', icon: DragHandGesture },
  { tool: 'selection', label: '选区', icon: SquareCursor },
  { tool: 'brush', label: '画笔', icon: EditPencil },
  { tool: 'eraser', label: '橡皮擦', icon: Erase },
  { tool: 'eyedropper', label: '吸管', icon: ColorPicker },
  { tool: 'crop', label: '裁剪', icon: Crop },
  { tool: 'zoom', label: '缩放', icon: ZoomIn },
]
</script>

<style scoped src="../../components/nodes/styles/node-tool-buttons.css"></style>
<style scoped>
.image-editor-tool-rail__button {
  width: 100%;
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
}

.image-editor-tool-rail__button:nth-child(2),
.image-editor-tool-rail__button:nth-child(5),
.image-editor-tool-rail__button:nth-child(7) {
  margin-top: 4px;
}
</style>
