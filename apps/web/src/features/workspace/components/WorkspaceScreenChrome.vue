<template>
  <HomeCanvasHeader
    class="canvas-chrome canvas-chrome-header"
    :inert="inert"
    :current-board-name="currentBoardName"
    :board-options="boardOptions"
    :current-board-id="currentBoardId"
    :is-board-menu-open="isBoardMenuOpen"
    :is-renaming-board="isRenamingBoard"
    :set-board-name-input-ref="setBoardNameInputRef"
    :export-disabled="exportDisabled"
    :avatar-url="avatarUrl"
    @update:board-name-draft="emit('update:board-name-draft', $event)"
    @toggle-board-menu="emit('toggle-board-menu')"
    @start-board-rename="emit('start-board-rename')"
    @cancel-board-rename="emit('cancel-board-rename')"
    @confirm-board-rename="emit('confirm-board-rename')"
    @select-board="emit('select-board', $event)"
    @export="emit('export')"
    @open-center="emit('open-center')"
    @logout="emit('logout')"
    @navigate-back="emit('navigate-back')"
  />

  <WorkspaceToolPalette
    :active-tool="activeTool"
    :inert="inert"
    @select-tool="emit('select-tool', $event)"
  />

  <WorkspaceZoomControls
    :zoom-level="zoomLevel"
    :inert="inert"
    @zoom-out="emit('zoom-out')"
    @zoom-in="emit('zoom-in')"
  />
</template>

<script setup lang="ts">
import HomeCanvasHeader from './home/HomeCanvasHeader.vue'
import WorkspaceToolPalette from './WorkspaceToolPalette.vue'
import WorkspaceZoomControls from './WorkspaceZoomControls.vue'

defineProps<{
  inert: boolean
  currentBoardName: string
  boardOptions: Array<{ id: string; name: string }>
  currentBoardId: string | null
  isBoardMenuOpen: boolean
  isRenamingBoard: boolean
  setBoardNameInputRef: (el: HTMLElement | null) => void
  exportDisabled: boolean
  avatarUrl?: string
  activeTool: 'select' | 'hand' | 'text'
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
  'open-center': []
  logout: []
  'navigate-back': []
  'select-tool': [tool: 'select' | 'hand' | 'text']
  'zoom-out': []
  'zoom-in': []
}>()
</script>
