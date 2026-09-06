<template>
  <section class="retouch-panel retouch-layers-panel">
    <div
      class="retouch-layers-panel__list app-scrollbar"
      role="listbox"
      :aria-label="t('canvas.retouch.layersListLabel')"
    >
      <div
        v-for="layer in layers"
        :key="layer.id"
        class="retouch-layer-row"
        :class="{ active: selectedLayerId === layer.id }"
        role="option"
        :aria-selected="selectedLayerId === layer.id"
        tabindex="0"
        @click="selectLayer(layer.id)"
        @keydown.enter.prevent="selectLayer(layer.id)"
        @keydown.space.prevent="selectLayer(layer.id)"
      >
        <span class="retouch-layer-row__thumb" :class="`kind-${resolveLayerKind(layer)}`">
          <component :is="resolveLayerIcon(layer)" class="retouch-layer-row__thumb-icon" />
        </span>
        <span class="retouch-layer-row__name">{{ layer.name }}</span>
        <span v-if="layer.type === 'source'" class="retouch-layer-row__lock">
          <Lock class="retouch-layer-row__icon" />
        </span>
        <button
          v-else
          type="button"
          class="retouch-layer-row__visibility canvas-tool-tooltip-trigger"
          :aria-label="
            layer.visible ? t('canvas.retouch.hideLayer') : t('canvas.retouch.showLayer')
          "
          @click.stop="onToggleVisibility(layer)"
        >
          <Eye v-if="layer.visible" class="retouch-layer-row__icon" />
          <EyeClosed v-else class="retouch-layer-row__icon" />
          <span class="canvas-tool-tooltip" aria-hidden="true">
            {{ layer.visible ? t('canvas.retouch.hideLayer') : t('canvas.retouch.showLayer') }}
          </span>
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import './styles/retouch-panel.css'
import type { Component } from 'vue'
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
import type { RetouchLayer } from '../types'

defineProps<{
  layers: RetouchLayer[]
  selectedLayerId: string
}>()

const emit = defineEmits<{
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
}>()

const { t } = useI18n()

function onToggleVisibility(layer: RetouchLayer) {
  if (layer.type === 'source') return
  emit('set-layer-visible', layer.id, !layer.visible)
}

function selectLayer(layerId: string) {
  emit('select-layer', layerId)
}

function resolveLayerKind(layer: RetouchLayer) {
  if (layer.type === 'source') return 'source'
  if (layer.type === 'adjustment') return layer.kind
  return layer.type
}

function resolveLayerIcon(layer: RetouchLayer): Component {
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
  throw new Error(`Unhandled retouch layer kind: ${String(value)}`)
}
</script>

<style scoped>
.retouch-layers-panel {
  height: calc(var(--retouch-panel-inset) * 2 + var(--retouch-panel-row-height) * 5 + 5px * 4);
  min-height: 0;
  margin-top: auto;
  display: flex;
  flex-direction: column;
}

.retouch-layers-panel__list {
  padding: var(--retouch-panel-inset);
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 5px;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.retouch-layer-row {
  width: 100%;
  height: var(--retouch-panel-row-height);
  border: var(--retouch-panel-border-width) solid transparent;
  border-radius: var(--retouch-panel-control-radius);
  padding: 0 6px;
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) 18px;
  align-items: center;
  gap: var(--retouch-panel-inset);
  background: transparent;
  color: var(--canvas-text-body);
  cursor: pointer;
  text-align: left;
  transition:
    background-color var(--motion-control-duration) var(--motion-control-easing),
    border-color var(--motion-control-duration) var(--motion-control-easing),
    color var(--motion-control-duration) var(--motion-control-easing);
}

.retouch-layer-row:hover,
.retouch-layer-row.active {
  border-color: var(--canvas-accent-outline);
  background: var(--canvas-accent-wash);
  color: var(--canvas-text-strong);
}

.retouch-layer-row__visibility,
.retouch-layer-row__lock {
  width: 18px;
  height: 18px;
  border: 0;
  padding: 0;
  display: grid;
  place-items: center;
  background: transparent;
  color: var(--canvas-text-muted);
}

.retouch-layer-row__visibility {
  cursor: pointer;
}

.retouch-layer-row:focus-visible,
.retouch-layer-row__visibility:focus-visible {
  outline: var(--ib-focus-outline-width) solid var(--ib-focus-outline-color);
  outline-offset: var(--ib-focus-outline-offset);
}

.retouch-layer-row__visibility .canvas-tool-tooltip {
  top: 50%;
  bottom: auto;
  right: calc(100% + 6px);
  left: auto;
  transform: translate(-2px, -50%);
}

.retouch-layer-row__visibility.canvas-tool-tooltip-trigger:hover .canvas-tool-tooltip {
  transform: translate(0, -50%);
}

.retouch-layer-row__icon {
  width: 14px;
  height: 14px;
}

.retouch-layer-row__thumb {
  width: 24px;
  height: 24px;
  border: var(--retouch-panel-border-width) solid var(--canvas-surface-border-soft);
  border-radius: 5px;
  display: grid;
  place-items: center;
  background: var(--canvas-surface-bg);
  color: var(--canvas-text-body);
}

.retouch-layer-row__thumb.kind-hsl {
  color: var(--canvas-accent);
  background: var(--canvas-accent-soft);
}

.retouch-layer-row__thumb.kind-threshold {
  color: var(--canvas-accent-strong);
  background: var(--canvas-accent-soft);
}

.retouch-layer-row__thumb.kind-curves,
.retouch-layer-row__thumb.kind-levels,
.retouch-layer-row__thumb.kind-sharpen {
  color: var(--canvas-text-body);
  background: var(--canvas-surface-muted);
}

.retouch-layer-row__thumb.kind-paint {
  color: var(--canvas-text-body);
  background: var(--canvas-surface-bg);
}

.retouch-layer-row__thumb.kind-merged {
  color: var(--canvas-accent-strong);
  background: var(--canvas-accent-soft);
}

.retouch-layer-row__thumb-icon {
  width: 15px;
  height: 15px;
}

.retouch-layer-row__name {
  min-width: 0;
  overflow: hidden;
  font-size: var(--retouch-panel-text-size);
  line-height: 1.1;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
