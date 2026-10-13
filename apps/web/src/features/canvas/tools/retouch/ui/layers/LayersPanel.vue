<template>
  <section class="retouch-panel retouch-layers-panel">
    <div
      class="retouch-layers-panel__list app-scrollbar"
      role="listbox"
      :aria-label="t('canvas.retouch.layersListLabel')"
    >
      <!--
        Roving tabindex: only the active descendant is tabbable, which is what a
        listbox is supposed to do. `interactive-supports-focus` reads a static
        `tabindex` attribute and cannot see through the binding.
      -->
      <!-- eslint-disable-next-line vuejs-accessibility/interactive-supports-focus -->
      <div
        v-for="(layer, index) in layers"
        :key="layer.id"
        :ref="(el) => setRowRef(el, index)"
        class="retouch-layer-row"
        :class="{ active: selectedLayerId === layer.id }"
        role="option"
        :aria-selected="selectedLayerId === layer.id"
        :tabindex="isSelectedLayer(layer.id) ? 0 : -1"
        @click="selectLayer(layer.id)"
        @keydown="onRowKeydown($event, index)"
      >
        <span class="retouch-layer-row__thumb" :class="`kind-${resolveLayerKind(layer)}`">
          <component :is="resolveLayerIcon(layer)" class="retouch-layer-row__thumb-icon" />
        </span>
        <span class="retouch-layer-row__name">{{ resolveLayerName(layer) }}</span>
        <span v-if="layer.type === 'source'" class="retouch-layer-row__lock">
          <Lock class="retouch-layer-row__icon" />
        </span>
        <button
          v-else
          :ref="(el) => setVisibilityButtonRef(el, index)"
          type="button"
          class="retouch-layer-row__visibility hit-target-32-inline canvas-tool-tooltip-trigger"
          :aria-label="
            layer.visible ? t('canvas.retouch.hideLayer') : t('canvas.retouch.showLayer')
          "
          tabindex="-1"
          @click.stop="onToggleVisibility(layer)"
          @keydown.stop="onVisibilityKeydown($event, index)"
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
import { nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import '../shared/styles/retouch-panel.css'
import type { Component } from 'vue'
import { Eye, EyeClosed, Lock } from '@iconoir/vue'
import type { RetouchLayer } from '../../core/model.ts'
import {
  resolveRetouchLayerIcon as resolveRetouchLayerIconInViewModel,
  resolveRetouchLayerKindKey as resolveRetouchLayerKindKeyInViewModel,
} from '../../session/retouchViewModel.ts'

const props = defineProps<{
  layers: RetouchLayer[]
  selectedLayerId: string
}>()

const emit = defineEmits<{
  (e: 'select-layer', layerId: string): void
  (e: 'set-layer-visible', layerId: string, visible: boolean): void
}>()

const { t } = useI18n()

/*
 * The panel declares `role="listbox"`, which promises a composite widget: one Tab
 * stop, arrow keys between options, Home/End to jump. It used to declare the
 * role and then hand every option `tabindex="0"` with only Enter and Space bound,
 * so Tab walked all five layers and no arrow key did anything — the same
 * advertised-but-absent keyboard model that AdminOptionFilter had.
 *
 * Roving tabindex is the APG answer: the active descendant is the single tab stop,
 * arrows move focus *and* selection, and Tab leaves the widget entirely.
 */

type RowElement = HTMLElement | null
const rowRefs = ref<RowElement[]>([])
const visibilityButtonRefs = ref<(HTMLButtonElement | null)[]>([])

function setRowRef(el: unknown, index: number) {
  rowRefs.value[index] = (el as RowElement) ?? null
}

function setVisibilityButtonRef(el: unknown, index: number) {
  visibilityButtonRefs.value[index] = (el as HTMLButtonElement) ?? null
}

function isSelectedLayer(layerId: string): boolean {
  return layerId === props.selectedLayerId
}

function focusRow(index: number) {
  const bounded = Math.min(Math.max(index, 0), props.layers.length - 1)
  const layer = props.layers[bounded]
  if (!layer) return
  selectLayer(layer.id)
  void nextTick(() => rowRefs.value[bounded]?.focus())
}

function onRowKeydown(event: KeyboardEvent, index: number) {
  // The visibility action is an independent control. Its key events must not
  // activate the row's listbox behavior when it is focused programmatically.
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      focusRow(index + 1)
      return
    case 'ArrowUp':
      event.preventDefault()
      focusRow(index - 1)
      return
    case 'Home':
      event.preventDefault()
      focusRow(0)
      return
    case 'End':
      event.preventDefault()
      focusRow(props.layers.length - 1)
      return
    case 'ArrowRight':
      if (visibilityButtonRefs.value[index]) {
        event.preventDefault()
        visibilityButtonRefs.value[index]?.focus()
      }
      return
    case 'Enter':
    case ' ':
      event.preventDefault()
      selectLayer(props.layers[index]?.id ?? '')
      return
    default:
  }
}

function onVisibilityKeydown(event: KeyboardEvent, index: number) {
  // Keep activation explicit because this button is programmatically focused
  // from the composite listbox and intentionally removed from the Tab order.
  const layer = props.layers[index]
  switch (event.key) {
    case 'Enter':
    case ' ':
    case 'Space':
      event.preventDefault()
      if (layer) onToggleVisibility(layer)
      return
    case 'ArrowLeft':
      event.preventDefault()
      rowRefs.value[index]?.focus()
      return
    default:
  }
}

// If the selection moves without keyboard focus (a click elsewhere, an undo),
// keep the tab stop pointing at the active option.
watch(
  () => props.selectedLayerId,
  (selectedId) => {
    const index = props.layers.findIndex((layer) => layer.id === selectedId)
    if (index < 0 || index >= rowRefs.value.length) return
    const active = document.activeElement
    if (active && active !== document.body && !rowRefs.value.includes(active as RowElement)) return
    rowRefs.value[index]?.focus()
  },
)

function onToggleVisibility(layer: RetouchLayer) {
  if (layer.type === 'source') return
  emit('set-layer-visible', layer.id, !layer.visible)
}

function selectLayer(layerId: string) {
  emit('select-layer', layerId)
}

function resolveLayerKind(layer: RetouchLayer) {
  return resolveRetouchLayerKindKeyInViewModel(layer)
}

function resolveLayerName(layer: RetouchLayer): string {
  const parts = layer.nameParts
  if (!parts) return layer.name
  const generatedName = parts
    .map((part) => (typeof part === 'string' ? part : part.labelKey))
    .join(' ')
  if (layer.name !== generatedName) return layer.name
  return parts.map((part) => (typeof part === 'string' ? part : t(part.labelKey))).join(' ')
}

// Table-driven through the retouch view model: unknown effect kinds degrade
// to the generic thumb instead of throwing.
function resolveLayerIcon(layer: RetouchLayer): Component {
  return resolveRetouchLayerIconInViewModel(layer)
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

@media (pointer: coarse) {
  .retouch-layer-row {
    min-height: var(--spacing-touch-target-coarse);
  }
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
  outline: var(--focus-outline-width) solid var(--focus-outline-color);
  outline-offset: var(--focus-outline-offset);
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
  border-radius: calc(var(--retouch-panel-control-radius) - 3px);
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
