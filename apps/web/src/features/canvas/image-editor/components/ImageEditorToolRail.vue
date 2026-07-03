<template>
  <div ref="railRootRef" class="image-editor-tool-rail" data-image-editor-tool-rail>
    <div
      v-for="item in toolItems"
      :key="item.tool"
      class="image-editor-tool-rail__item node-tool-tooltip-trigger"
    >
      <NodeToolIconButton
        class="image-editor-tool-rail__button"
        :class="{ 'is-disabled': getToolState(item.tool).disabled }"
        :active="activeTool === item.tool"
        :disabled="getToolState(item.tool).disabled"
        :aria-label-value="item.label"
        @click="onToolClick(item.tool)"
      >
        <component :is="item.icon" />
      </NodeToolIconButton>
      <span class="node-tool-tooltip" aria-hidden="true">
        {{ getToolState(item.tool).tooltip }}
      </span>
      <div
        v-if="openSettingsTool === item.tool && isRasterTool(item.tool)"
        class="image-editor-tool-settings"
        role="dialog"
        :aria-label="`${item.label}参数`"
        data-image-editor-popover
        @pointerdown.stop
      >
        <div class="image-editor-tool-settings__topline">
          <span class="image-editor-tool-settings__title">{{ item.label }}</span>
          <div class="image-editor-tool-settings__history">
            <NodeToolIconButton
              class="image-editor-tool-settings__icon-button"
              :disabled="!canUndoRasterStroke"
              aria-label-value="撤销"
              tooltip="撤销"
              @click="emit('undo-raster-stroke')"
            >
              <UndoAction />
            </NodeToolIconButton>
            <NodeToolIconButton
              class="image-editor-tool-settings__icon-button"
              :disabled="!canRedoRasterStroke"
              aria-label-value="重做"
              tooltip="重做"
              @click="emit('redo-raster-stroke')"
            >
              <RedoAction />
            </NodeToolIconButton>
          </div>
        </div>
        <label class="image-editor-tool-settings__control">
          <span>Size</span>
          <input
            type="range"
            min="1"
            max="512"
            :value="rasterToolSettings.size"
            @input="updateNumberSetting('size', $event)"
          />
          <input
            class="image-editor-tool-settings__number"
            type="number"
            min="1"
            max="512"
            :value="rasterToolSettings.size"
            @input="updateNumberSetting('size', $event)"
          />
        </label>
        <label class="image-editor-tool-settings__control">
          <span>Hardness</span>
          <input
            type="range"
            min="0"
            max="100"
            :value="rasterToolSettings.hardness"
            @input="updateNumberSetting('hardness', $event)"
          />
          <input
            class="image-editor-tool-settings__number"
            type="number"
            min="0"
            max="100"
            :value="rasterToolSettings.hardness"
            @input="updateNumberSetting('hardness', $event)"
          />
        </label>
        <label class="image-editor-tool-settings__control">
          <span>Opacity</span>
          <input
            type="range"
            min="1"
            max="100"
            :value="activeOpacity"
            @input="updateOpacitySetting($event)"
          />
          <input
            class="image-editor-tool-settings__number"
            type="number"
            min="1"
            max="100"
            :value="activeOpacity"
            @input="updateOpacitySetting($event)"
          />
        </label>
        <label
          v-if="openSettingsTool === 'brush'"
          class="image-editor-tool-settings__control image-editor-tool-settings__control--color"
        >
          <span>Color</span>
          <input
            class="image-editor-tool-settings__color"
            type="color"
            :value="rasterToolSettings.brushColor"
            @input="updateColorSetting"
          />
        </label>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useEventListener } from '@vueuse/core'
import { computed, onBeforeUnmount, ref, watch, type Component } from 'vue'
import {
  ColorPicker,
  Crop,
  DragHandGesture,
  Erase,
  EditPencil,
  RedoAction,
  SquareCursor,
  UndoAction,
  ZoomIn,
} from '@iconoir/vue'
import type { ImageEditorTool } from '../types'
import type { ImageEditorRasterToolSettings } from '../utils/imageEditorRasterTools'
import NodeToolIconButton from '../../components/nodes/NodeToolIconButton.vue'

const props = defineProps<{
  activeTool: ImageEditorTool
  canUseRasterTool: boolean
  applyingAdjustments: boolean
  rasterToolSettings: ImageEditorRasterToolSettings
  canUndoRasterStroke: boolean
  canRedoRasterStroke: boolean
}>()

const emit = defineEmits<{
  (e: 'select-tool', tool: ImageEditorTool): void
  (e: 'update-raster-settings', patch: Partial<ImageEditorRasterToolSettings>): void
  (e: 'undo-raster-stroke'): void
  (e: 'redo-raster-stroke'): void
}>()

const railRootRef = ref<HTMLElement | null>(null)
const openSettingsTool = ref<ImageEditorTool | null>(null)

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

const activeOpacity = computed(() =>
  openSettingsTool.value === 'eraser'
    ? props.rasterToolSettings.eraserOpacity
    : props.rasterToolSettings.brushOpacity,
)

function isRasterTool(tool: ImageEditorTool): tool is 'brush' | 'eraser' {
  return tool === 'brush' || tool === 'eraser'
}

function getToolState(tool: ImageEditorTool) {
  if (tool === 'move') return { disabled: false, tooltip: '移动' }
  if (isRasterTool(tool)) {
    if (props.applyingAdjustments) return { disabled: true, tooltip: '应用中' }
    if (!props.canUseRasterTool) {
      return { disabled: true, tooltip: '选择可见的原图或绘制图层' }
    }
    return { disabled: false, tooltip: tool === 'brush' ? '画笔' : '橡皮擦' }
  }
  return { disabled: true, tooltip: '暂未开放' }
}

function onToolClick(tool: ImageEditorTool) {
  if (getToolState(tool).disabled) return
  if (!isRasterTool(tool)) {
    openSettingsTool.value = null
    emit('select-tool', tool)
    return
  }
  if (props.activeTool === tool && openSettingsTool.value === tool) {
    openSettingsTool.value = null
    return
  }
  openSettingsTool.value = tool
  emit('select-tool', tool)
}

function updateNumberSetting(
  key: Extract<keyof ImageEditorRasterToolSettings, 'size' | 'hardness'>,
  event: Event,
) {
  emit('update-raster-settings', {
    [key]: Number((event.target as HTMLInputElement | null)?.value),
  })
}

function updateOpacitySetting(event: Event) {
  const value = Number((event.target as HTMLInputElement | null)?.value)
  emit('update-raster-settings', {
    [openSettingsTool.value === 'eraser' ? 'eraserOpacity' : 'brushOpacity']: value,
  })
}

function updateColorSetting(event: Event) {
  emit('update-raster-settings', {
    brushColor: (event.target as HTMLInputElement | null)?.value ?? '#000000',
  })
}

function closeSettingsPopover() {
  openSettingsTool.value = null
}

useEventListener<PointerEvent>(
  () => window,
  'pointerdown',
  (event) => {
    if (!openSettingsTool.value) return
    const target = event.target as HTMLElement | null
    if (target && railRootRef.value?.contains(target)) return
    if (target?.closest('[data-image-editor-raster-overlay]')) return
    closeSettingsPopover()
  },
  true,
)

useEventListener<KeyboardEvent>(
  () => window,
  'keydown',
  (event) => {
    if (!openSettingsTool.value || event.key !== 'Escape') return
    event.preventDefault()
    event.stopPropagation()
    closeSettingsPopover()
  },
  true,
)

watch(
  () => [props.canUseRasterTool, props.applyingAdjustments] as const,
  ([canUseRasterTool, applyingAdjustments]) => {
    if ((!canUseRasterTool || applyingAdjustments) && openSettingsTool.value) {
      closeSettingsPopover()
    }
  },
)

watch(
  openSettingsTool,
  (tool) => {
    if (typeof document === 'undefined') return
    if (tool) {
      document.documentElement.dataset.imageEditorPopoverOpen = 'true'
      return
    }
    delete document.documentElement.dataset.imageEditorPopoverOpen
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (typeof document === 'undefined') return
  delete document.documentElement.dataset.imageEditorPopoverOpen
})
</script>

<style scoped src="../../components/nodes/styles/node-tool-buttons.css"></style>
<style scoped>
.image-editor-tool-rail__button {
  width: 100%;
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
}

.image-editor-tool-rail {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: var(--node-tool-button-gap);
}

.image-editor-tool-rail__item {
  position: relative;
  width: 100%;
}

.image-editor-tool-rail__item .node-tool-tooltip {
  top: 50%;
  bottom: auto;
  left: calc(100% + 8px);
  transform: translate(2px, -50%);
}

.image-editor-tool-rail__item.node-tool-tooltip-trigger:hover .node-tool-tooltip {
  transform: translate(0, -50%);
}

.image-editor-tool-rail__item:nth-child(2),
.image-editor-tool-rail__item:nth-child(5),
.image-editor-tool-rail__item:nth-child(7) {
  margin-top: 4px;
}

.image-editor-tool-settings {
  position: absolute;
  top: 0;
  left: calc(100% + 8px);
  width: 214px;
  border: var(--node-tool-border-width) solid var(--canvas-surface-border);
  border-radius: var(--canvas-floating-radius);
  padding: 9px;
  display: grid;
  gap: 9px;
  background: var(--canvas-surface-bg);
  color: var(--canvas-text-body);
  box-shadow: var(--canvas-floating-shadow);
}

.image-editor-tool-settings__topline,
.image-editor-tool-settings__control {
  display: grid;
  grid-template-columns: 58px minmax(0, 1fr) 44px;
  align-items: center;
  gap: 8px;
}

.image-editor-tool-settings__topline {
  grid-template-columns: minmax(0, 1fr) auto;
}

.image-editor-tool-settings__title,
.image-editor-tool-settings__control span {
  min-width: 0;
  color: var(--canvas-text-strong);
  font-size: 11px;
  font-weight: 700;
  line-height: 1.1;
}

.image-editor-tool-settings__history {
  display: flex;
  gap: 5px;
}

.image-editor-tool-settings__icon-button {
  --node-tool-icon-button-size: 25px;
  --node-tool-icon-button-radius: 7px;
  --node-tool-icon-button-icon-size: 15px;
}

.image-editor-tool-settings__control input[type='range'] {
  min-width: 0;
  accent-color: var(--canvas-accent);
}

.image-editor-tool-settings__number {
  width: 44px;
  min-width: 0;
  border: var(--node-tool-border-width) solid var(--canvas-surface-border-soft);
  border-radius: 7px;
  padding: 3px 4px;
  background: var(--canvas-surface-muted);
  color: var(--canvas-text-strong);
  font-size: 11px;
  line-height: 1.1;
}

.image-editor-tool-settings__control--color {
  grid-template-columns: 58px 34px 1fr;
}

.image-editor-tool-settings__color {
  width: 30px;
  height: 25px;
  border: var(--node-tool-border-width) solid var(--canvas-surface-border-soft);
  border-radius: 7px;
  padding: 1px;
  background: var(--canvas-surface-muted);
}
</style>
