<template>
  <HomeCanvasHeader
    class="canvas-page-chrome canvas-page-chrome--header"
    :class="{ 'is-hidden': chromeHidden }"
    :inert="inert"
    :current-board-name="currentBoardName"
    :board-options="boardOptions"
    :current-board-id="currentBoardId"
    :is-board-menu-open="isBoardMenuOpen"
    :is-renaming-board="isRenamingBoard"
    :set-board-name-input-ref="setBoardNameInputRef"
    :export-disabled="exportDisabled"
    :avatar-url="avatarUrl"
    :user-menu-items="userMenuItems"
    @update:board-name-draft="emit('update:board-name-draft', $event)"
    @toggle-board-menu="emit('toggle-board-menu')"
    @start-board-rename="emit('start-board-rename')"
    @cancel-board-rename="emit('cancel-board-rename')"
    @confirm-board-rename="emit('confirm-board-rename')"
    @select-board="emit('select-board', $event)"
    @export="emit('export')"
    @navigate-back="emit('navigate-back')"
  />

  <CanvasToolPalette
    class="canvas-page-chrome canvas-page-chrome--left"
    :class="{ 'is-hidden': chromeHidden }"
    :active-tool="activeTool"
    :minimap-enabled="minimapEnabled"
    :inert="inert"
    @select-tool="emit('select-tool', $event)"
    @toggle-minimap="emit('toggle-minimap')"
  />

  <CanvasZoomControls
    class="canvas-page-chrome canvas-page-chrome--status"
    :class="{ 'is-hidden': chromeHidden }"
    :zoom-level="zoomLevel"
    :inert="inert"
    @zoom-out="emit('zoom-out')"
    @zoom-in="emit('zoom-in')"
  />
</template>

<script setup lang="ts">
import HomeCanvasHeader from './home/HomeCanvasHeader.vue'
import CanvasToolPalette from './CanvasToolPalette.vue'
import CanvasZoomControls from './CanvasZoomControls.vue'
import type { UserAccountMenuItem } from '../../../shared/components/userAccountMenuActions'

defineProps<{
  inert: boolean
  chromeHidden: boolean
  currentBoardName: string
  boardOptions: Array<{ id: string; name: string }>
  currentBoardId: string | null
  isBoardMenuOpen: boolean
  isRenamingBoard: boolean
  setBoardNameInputRef: (el: HTMLElement | null) => void
  exportDisabled: boolean
  avatarUrl?: string
  userMenuItems: UserAccountMenuItem[]
  activeTool: 'select' | 'hand'
  minimapEnabled: boolean
  zoomLevel: number
}>()

const emit = defineEmits<{
  'update:board-name-draft': [value: string]
  'toggle-board-menu': []
  'start-board-rename': []
  'cancel-board-rename': []
  'confirm-board-rename': []
  'select-board': [boardId: string]
  export: []
  'navigate-back': []
  'select-tool': [tool: 'select' | 'hand']
  'toggle-minimap': []
  'zoom-out': []
  'zoom-in': []
}>()
</script>

<style scoped>
.canvas-page-chrome {
  opacity: 1;
  filter: blur(0);
  transform-origin: center;
  transition:
    opacity var(--canvas-page-chrome-fade-duration) ease,
    filter var(--canvas-page-chrome-slide-duration) var(--canvas-page-chrome-slide-ease),
    transform var(--canvas-page-chrome-slide-duration) var(--canvas-page-chrome-slide-ease);
  will-change: transform, opacity, filter;
}

.canvas-page-chrome.is-hidden {
  opacity: 0;
  filter: blur(5px);
  pointer-events: none;
}

.canvas-page-chrome--header.is-hidden {
  transform: translateY(-82px) scale(0.985);
}

.canvas-page-chrome--left.is-hidden {
  transform: translate(-76px, -50%) scale(0.985);
}

.canvas-page-chrome--status.is-hidden {
  transform: translateY(76px) scale(0.985);
}

@media (prefers-reduced-motion: reduce) {
  .canvas-page-chrome {
    transition-duration: 1ms;
  }
}
</style>
