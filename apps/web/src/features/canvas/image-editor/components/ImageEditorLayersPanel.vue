<template>
  <section class="image-editor-panel image-editor-layers-panel">
    <div
      class="image-editor-layers-panel__list app-scrollbar"
      :class="{ 'is-scrolling': isLayerListScrolling }"
      role="listbox"
      aria-label="图层"
      @scroll="onLayerListScroll"
    >
      <div
        v-for="layer in layers"
        :key="layer.id"
        class="image-editor-layer-row"
        :class="{ active: selectedLayerId === layer.id }"
        role="option"
        :aria-selected="selectedLayerId === layer.id"
        tabindex="0"
        @click="selectLayer(layer.id)"
        @keydown.enter.prevent="selectLayer(layer.id)"
        @keydown.space.prevent="selectLayer(layer.id)"
      >
        <span class="image-editor-layer-row__thumb" :class="`kind-${resolveLayerKind(layer)}`">
          <component :is="resolveLayerIcon(layer)" class="image-editor-layer-row__thumb-icon" />
        </span>
        <span class="image-editor-layer-row__name">{{ layer.name }}</span>
        <span v-if="layer.type === 'source'" class="image-editor-layer-row__lock">
          <Lock class="image-editor-layer-row__icon" />
        </span>
        <button
          v-else
          type="button"
          class="image-editor-layer-row__visibility node-tool-tooltip-trigger"
          :aria-label="layer.visible ? '隐藏图层' : '显示图层'"
          @click.stop="onToggleVisibility(layer)"
        >
          <Eye v-if="layer.visible" class="image-editor-layer-row__icon" />
          <EyeClosed v-else class="image-editor-layer-row__icon" />
          <span class="node-tool-tooltip" aria-hidden="true">
            {{ layer.visible ? '隐藏图层' : '显示图层' }}
          </span>
        </button>
      </div>
    </div>

  </section>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { onBeforeUnmount, ref } from 'vue'
import {
  Brightness,
  ColorFilter,
  Eye,
  EyeClosed,
  Lock,
  MediaImage,
  EditPencil,
  PrecisionTool,
  ReportsSolid,
  SineWave,
  VerticalMerge,
} from '@iconoir/vue'
import type { ImageEditorLayer } from '../types'

defineProps<{
  layers: ImageEditorLayer[]
  selectedLayerId: string
}>()

const emit = defineEmits<{
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
}>()

const isLayerListScrolling = ref(false)
let layerListScrollTimer: number | null = null

function clearLayerListScrollTimer() {
  if (layerListScrollTimer == null) return
  window.clearTimeout(layerListScrollTimer)
  layerListScrollTimer = null
}

function onLayerListScroll() {
  isLayerListScrolling.value = true
  clearLayerListScrollTimer()
  layerListScrollTimer = window.setTimeout(() => {
    isLayerListScrolling.value = false
    layerListScrollTimer = null
  }, 600)
}

function onToggleVisibility(layer: ImageEditorLayer) {
  if (layer.type === 'source') return
  emit('set-layer-visible', layer.id, !layer.visible)
}

function selectLayer(layerId: string) {
  emit('select-layer', layerId)
}

function resolveLayerKind(layer: ImageEditorLayer) {
  if (layer.type === 'source') return 'source'
  if (layer.type === 'adjustment') return layer.kind
  return layer.type
}

function resolveLayerIcon(layer: ImageEditorLayer): Component {
  if (layer.type === 'source') return MediaImage
  if (layer.type === 'paint') return EditPencil
  if (layer.type === 'merged') return VerticalMerge
  if (layer.kind === 'hsl') return ColorFilter
  if (layer.kind === 'threshold') return Brightness
  if (layer.kind === 'curves') return SineWave
  if (layer.kind === 'levels') return ReportsSolid
  if (layer.kind === 'sharpen') return PrecisionTool
  return assertNever(layer)
}

function assertNever(value: never): never {
  throw new Error(`Unhandled image editor layer kind: ${String(value)}`)
}

onBeforeUnmount(clearLayerListScrollTimer)
</script>

<style src="./styles/image-editor-panel.css"></style>
<style scoped src="../../components/nodes/styles/node-tool-buttons.css"></style>
<style scoped>
.image-editor-layers-panel {
  height: calc(
    var(--image-editor-panel-inset) * 2 + var(--image-editor-panel-row-height) * 5 + 5px * 4
  );
  min-height: 0;
  margin-top: auto;
  display: flex;
  flex-direction: column;
}

.image-editor-layers-panel__list {
  padding: var(--image-editor-panel-inset);
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 5px;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.image-editor-layer-row {
  width: 100%;
  height: var(--image-editor-panel-row-height);
  border: var(--image-editor-panel-border-width) solid transparent;
  border-radius: var(--image-editor-panel-control-radius);
  padding: 0 6px;
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) 18px;
  align-items: center;
  gap: var(--image-editor-panel-inset);
  background: transparent;
  color: var(--canvas-text-body);
  cursor: pointer;
  text-align: left;
  transition:
    background-color var(--canvas-motion-fast) ease,
    border-color var(--canvas-motion-fast) ease,
    color var(--canvas-motion-fast) ease;
}

.image-editor-layer-row:hover,
.image-editor-layer-row.active {
  border-color: var(--canvas-accent-outline);
  background: var(--canvas-accent-wash);
  color: var(--canvas-text-strong);
}

.image-editor-layer-row__visibility,
.image-editor-layer-row__lock {
  width: 18px;
  height: 18px;
  border: 0;
  padding: 0;
  display: grid;
  place-items: center;
  background: transparent;
  color: var(--canvas-text-muted);
}

.image-editor-layer-row__visibility {
  cursor: pointer;
}

.image-editor-layer-row:focus-visible,
.image-editor-layer-row__visibility:focus-visible {
  outline: var(--ib-focus-outline-width) solid var(--ib-focus-outline-color);
  outline-offset: var(--ib-focus-outline-offset);
}

.image-editor-layer-row__visibility .node-tool-tooltip {
  top: 50%;
  bottom: auto;
  right: calc(100% + 6px);
  left: auto;
  transform: translate(-2px, -50%);
}

.image-editor-layer-row__visibility.node-tool-tooltip-trigger:hover .node-tool-tooltip {
  transform: translate(0, -50%);
}

.image-editor-layer-row__icon {
  width: 14px;
  height: 14px;
}

.image-editor-layer-row__thumb {
  width: 24px;
  height: 24px;
  border: var(--image-editor-panel-border-width) solid var(--canvas-surface-border-soft);
  border-radius: 5px;
  display: grid;
  place-items: center;
  background: var(--canvas-surface-bg);
  color: var(--canvas-text-body);
}

.image-editor-layer-row__thumb.kind-hsl {
  color: var(--canvas-accent);
  background: var(--canvas-accent-soft);
}

.image-editor-layer-row__thumb.kind-threshold {
  color: var(--canvas-accent-strong);
  background: var(--canvas-accent-soft);
}

.image-editor-layer-row__thumb.kind-curves,
.image-editor-layer-row__thumb.kind-levels,
.image-editor-layer-row__thumb.kind-sharpen {
  color: var(--canvas-text-body);
  background: var(--canvas-surface-muted);
}

.image-editor-layer-row__thumb.kind-paint {
  color: var(--canvas-text-body);
  background: var(--canvas-surface-bg);
}

.image-editor-layer-row__thumb.kind-merged {
  color: var(--canvas-accent-strong);
  background: var(--canvas-accent-soft);
}

.image-editor-layer-row__thumb-icon {
  width: 15px;
  height: 15px;
}

.image-editor-layer-row__name {
  min-width: 0;
  overflow: hidden;
  font-size: var(--image-editor-panel-text-size);
  line-height: 1.1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

</style>
