<template>
  <div
    class="image-action-menu node-tool-surface"
    :class="{ 'strength-mode': showStrengthControl }"
    :style="showStrengthControl ? strengthMenuStyle : undefined"
    @mousedown.stop
    @click.stop
  >
    <div
      v-motion="{
        initial: { opacity: 0, y: 6, scale: 0.985 },
        enter: {
          opacity: 1,
          y: 0,
          scale: 1,
          transition: { duration: 160, easing: 'ease-out' },
        },
      }"
      class="image-action-menu-inner"
    >
      <div
        v-for="item in IMAGE_ACTION_MENU_ITEMS"
        :key="item.title"
        class="image-action-control"
        :class="{
          'has-strength-range': !!item.strengthAction,
          'is-strength-active': isStrengthActive(item),
          'is-strength-hidden': isStrengthHidden(item),
        }"
      >
        <NodeToolIconButton
          class="image-action-icon-btn"
          :aria-label-value="item.title"
          :disabled="directGenerating"
          :tooltip="item.label"
          @click="onActionClick(item)"
        >
          <ImageActionMenuIcon :icon="item.icon" />
        </NodeToolIconButton>
        <input
          v-if="item.strengthAction"
          :ref="(el) => setItemStrengthRangeRef(item, el)"
          class="image-action-strength-range canvas-range-input"
          type="range"
          min="0"
          max="100"
          step="1"
          :value="getItemStrengthValue(item)"
          :disabled="directGenerating"
          :tabindex="isStrengthActive(item) ? 0 : -1"
          :aria-label="`${item.label}强度`"
          @pointerdown.stop="onItemStrengthRangePointerDown(item, $event)"
          @input="onItemStrengthInput(item, $event)"
          @change="onItemStrengthChange(item, $event)"
          @pointerup="onItemStrengthPointerUp(item, $event)"
          @click="onStrengthRangeClick(item, $event)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import {
  IMAGE_ACTION_MENU_ITEMS,
  type ImageActionMenuItem,
  type ImageNodeAction,
} from '../../config/imageActionRegistry'
import type { ImageStrengthAction } from '../../execution'
import ImageActionMenuIcon from './ImageActionMenuIcon.vue'
import NodeToolIconButton from '../nodes/NodeToolIconButton.vue'

const props = defineProps<{
  showStrengthControl: boolean
  strengthMenuStyle?: string | Record<string, string>
  directGenerating: boolean
  activeStrengthAction: ImageStrengthAction | null
  getStrengthValue: (action: ImageStrengthAction) => number
  setStrengthRangeRef: (
    action: ImageStrengthAction,
    el: Element | ComponentPublicInstance | null,
  ) => void
  onStrengthRangePointerDown: (action: ImageStrengthAction, event: PointerEvent) => void
  onStrengthInput: (action: ImageStrengthAction, event: Event) => void
  onStrengthChange: (action: ImageStrengthAction, event: Event) => void
  onStrengthPointerUp: (action: ImageStrengthAction, event: PointerEvent) => void
  onStrengthClick: (action: ImageStrengthAction, event: MouseEvent) => void
}>()

const emit = defineEmits<{
  (event: 'action', action: ImageNodeAction): void
}>()

function onActionClick(item: ImageActionMenuItem) {
  emit('action', item.action)
}

function getStrengthAction(item: ImageActionMenuItem): ImageStrengthAction | null {
  return item.strengthAction ?? null
}

function isStrengthActive(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return !!action && props.showStrengthControl && props.activeStrengthAction === action
}

function isStrengthHidden(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return props.showStrengthControl && props.activeStrengthAction !== action
}

function getItemStrengthValue(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return action ? props.getStrengthValue(action) : 0
}

function setItemStrengthRangeRef(
  item: ImageActionMenuItem,
  el: Element | ComponentPublicInstance | null,
) {
  const action = getStrengthAction(item)
  if (!action) return
  props.setStrengthRangeRef(action, el)
}

function onItemStrengthRangePointerDown(item: ImageActionMenuItem, event: PointerEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.onStrengthRangePointerDown(action, event)
}

function onItemStrengthInput(item: ImageActionMenuItem, event: Event) {
  const action = getStrengthAction(item)
  if (!action) return
  props.onStrengthInput(action, event)
}

function onItemStrengthChange(item: ImageActionMenuItem, event: Event) {
  const action = getStrengthAction(item)
  if (!action) return
  props.onStrengthChange(action, event)
}

function onItemStrengthPointerUp(item: ImageActionMenuItem, event: PointerEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.onStrengthPointerUp(action, event)
}

function onStrengthRangeClick(item: ImageActionMenuItem, event: MouseEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.onStrengthClick(action, event)
  emit('action', item.action)
}
</script>

<style scoped src="../nodes/styles/node-tool-buttons.css"></style>
<style scoped>
.image-action-menu {
  position: absolute;
  top: calc(100% + 10px);
  left: 50%;
  width: max-content;
  max-width: min(100vw - 32px, 320px);
  transform: translateX(-50%);
  padding: 0;
  border-radius: 0;
  background: transparent;
  border: none;
  backdrop-filter: none;
  box-shadow: none;
  z-index: 14;
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--node-tool-button-gap);
  box-sizing: border-box;
}

.image-action-menu.strength-mode {
  top: calc(100% + 10px);
  left: 0;
  transform: none;
  width: 100%;
  max-width: none;
  gap: 0;
}

.image-action-menu-inner {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: inherit;
  gap: inherit;
  width: 100%;
}

.image-action-control {
  position: relative;
  flex: 0 0 var(--node-tool-button-size);
  width: var(--node-tool-button-size);
  height: var(--node-tool-button-size);
}

.image-action-control.is-strength-active {
  flex: 1 1 100%;
  width: 100%;
}

.image-action-control.is-strength-hidden {
  display: none;
}

.image-action-icon-btn {
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-border: var(--node-tool-button-border);
  --node-tool-icon-button-color: #6f61c6;
  --node-tool-icon-button-active-border: #7a6bd5;
  --node-tool-icon-button-active-color: #7a6bd5;
  --node-tool-icon-button-active-bg: #ffffff;
  position: relative;
  overflow: visible;
}

.image-action-icon-btn:hover:not(:disabled) {
  border-color: #7a6bd5;
  background: #ffffff;
  color: #7a6bd5;
  transform: scale(1.06);
}

.image-action-icon-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.image-action-strength-range {
  --canvas-range-track-height: 6px;
  --canvas-range-thumb-size: 14px;
  --canvas-range-thumb-offset: -4px;
  --canvas-range-fill-color: #7a6bd5;
  --canvas-range-track-background: rgba(122, 107, 213, 0.2);
  position: absolute;
  inset: 0;
  z-index: 2;
  opacity: 0;
}

.image-action-control.has-strength-range .image-action-icon-btn {
  pointer-events: none;
}

.image-action-control.is-strength-active .image-action-icon-btn {
  display: none;
}

.image-action-control.is-strength-active .image-action-strength-range {
  position: relative;
  opacity: 1;
}
</style>
