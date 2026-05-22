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
      <template v-if="!showStrengthControl">
        <button
          v-for="item in IMAGE_ACTION_MENU_ITEMS"
          :key="item.title"
          type="button"
          class="image-action-icon-btn node-tool-button"
          :disabled="directGenerating"
          :title="item.title"
          @mousedown.stop="onActionMouseDown(item, $event)"
          @click="onActionClick(item)"
        >
          <ImageActionMenuIcon :icon="item.icon" />
          <span class="action-tooltip">{{ item.label }}</span>
        </button>
      </template>
      <div
        v-else
        class="style-strength-pop"
        @mousedown.stop
      >
        <div
          :ref="setStrengthTrackRef"
          class="style-strength-track"
          @mousedown.stop.prevent="emit('strength-track-mousedown', $event)"
        >
          <div class="style-strength-fill" :style="{ width: `${strengthPct}%` }"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import { IMAGE_ACTION_MENU_ITEMS, type ImageActionMenuItem } from '../../config/imageActionMenu'
import { type ImageNodeAction, type ImageStrengthAction } from '../../types/imageActions'
import ImageActionMenuIcon from './ImageActionMenuIcon.vue'

defineProps<{
  showStrengthControl: boolean
  strengthMenuStyle?: string | Record<string, string>
  directGenerating: boolean
  strengthPct: number
  setStrengthTrackRef: (el: Element | ComponentPublicInstance | null) => void
}>()

const emit = defineEmits<{
  (event: 'action', action: ImageNodeAction): void
  (event: 'strength-action-mousedown', action: ImageStrengthAction, value: MouseEvent): void
  (event: 'strength-track-mousedown', value: MouseEvent): void
}>()

function onActionMouseDown(item: ImageActionMenuItem, event: MouseEvent) {
  if (!('strengthAction' in item) || !item.strengthAction) return
  emit('strength-action-mousedown', item.strengthAction, event)
}

function onActionClick(item: ImageActionMenuItem) {
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

.image-action-icon-btn {
  flex: 0 0 var(--node-tool-button-size);
  border: 1px solid var(--node-tool-button-border);
  background: #ffffff;
  position: relative;
  overflow: visible;
  transition:
    transform 0.18s ease,
    color 0.18s ease,
    opacity 0.18s ease,
    border-color 0.18s ease,
    background 0.18s ease;
  color: #6f61c6;
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

.action-tooltip {
  position: absolute;
  left: 50%;
  bottom: calc(100% + 8px);
  transform: translate(-50%, 2px);
  white-space: nowrap;
  background: rgba(24, 24, 27, 0.94);
  color: #f4f4f5;
  border: 1px solid rgba(122, 107, 213, 0.26);
  border-radius: 6px;
  padding: 4px 8px;
  font-size: 11px;
  line-height: 1.2;
  letter-spacing: 0.01em;
  opacity: 0;
  pointer-events: none;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.image-action-icon-btn:hover:not(:disabled) .action-tooltip {
  opacity: 1;
  transform: translate(-50%, 0);
}

.style-strength-pop {
  position: relative;
  width: 100%;
  height: var(--node-tool-button-size);
  display: flex;
  align-items: center;
  padding: 0;
}

.style-strength-track {
  position: relative;
  width: 100%;
  height: 6px;
  border-radius: 999px;
  overflow: hidden;
  border: none;
  background: rgba(122, 107, 213, 0.2);
  cursor: ew-resize;
}

.style-strength-fill {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: #7a6bd5;
  border-radius: inherit;
  transition: width 0.06s linear;
}
</style>
