<template>
  <header
    class="chrome-presence canvas-chrome-region canvas-chrome-region--top"
    :class="{ 'is-hidden': isHidden }"
    :inert="isInert"
    @wheel.prevent.stop
  >
    <CanvasChromeBoardHeader
      :board="board"
      @update:board-name-draft="emit('update:board-name-draft', $event)"
      @toggle-board-menu="emit('toggle-board-menu')"
      @start-board-rename="emit('start-board-rename')"
      @cancel-board-rename="emit('cancel-board-rename')"
      @confirm-board-rename="emit('confirm-board-rename')"
      @reload-catalog="emit('reload-catalog')"
      @restore-archived-board="emit('restore-archived-board')"
      @select-board="emit('select-board', $event)"
      @navigate-back="emit('navigate-back')"
    />

    <div class="canvas-chrome-presence-slot">
      <slot name="presence" />
    </div>

    <UserAccountMenu
      class="canvas-chrome-action-group"
      :avatar-url="account.avatarUrl"
      :user-email="account.userEmail"
      :balance="account.balance"
      :items="account.menuItems"
      :workspace="account.workspace"
      :support-channels="supportChannels"
      :avatar-button-class="canvasChromeAvatarButtonClass"
    >
      <template #actions>
        <button
          type="button"
          :class="canvasChromeActionControlClass"
          :aria-label="t('canvas.chrome.share')"
          :title="t('canvas.chrome.share')"
          @mousedown.stop
          @click="emit('share')"
        >
          <ShareAndroid class="canvas-chrome-icon" />
        </button>
        <button
          :class="[canvasChromeActionControlClass, { 'is-active': !status.isExportDisabled }]"
          type="button"
          :aria-label="t('canvas.chrome.export')"
          :disabled="status.isExportDisabled"
          @mousedown.stop
          @click="emit('export')"
        >
          <Download class="canvas-chrome-icon" />
        </button>
        <NotificationBell />
      </template>
    </UserAccountMenu>
  </header>

  <aside
    class="chrome-presence canvas-chrome-region canvas-chrome-region--left"
    :class="{ 'is-hidden': isHidden }"
    :inert="isInert"
    @wheel.prevent.stop
  >
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.activeTool === 'select' }]"
      :aria-label="t('canvas.chrome.selectTool')"
      :aria-pressed="tools.activeTool === 'select'"
      @click="emit('select-tool', 'select')"
    >
      <OneFingerSelectHandGesture class="canvas-chrome-icon" />
    </button>
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.activeTool === 'hand' }]"
      :aria-label="t('canvas.chrome.handTool')"
      :aria-pressed="tools.activeTool === 'hand'"
      @click="emit('select-tool', 'hand')"
    >
      <SpockHandGesture class="canvas-chrome-icon" />
    </button>
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.isLibraryOpen }]"
      :aria-label="t('common.navigation.library')"
      :aria-pressed="tools.isLibraryOpen"
      :title="t('common.navigation.library')"
      @click="emit('toggle-library')"
    >
      <BookmarkSolid v-if="tools.isLibraryOpen" class="canvas-chrome-icon" />
      <Bookmark v-else class="canvas-chrome-icon" />
    </button>
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.isMinimapEnabled }]"
      :aria-label="t('canvas.minimap.label')"
      :aria-pressed="tools.isMinimapEnabled"
      :title="t('canvas.minimap.label')"
      @click="emit('toggle-minimap')"
    >
      <Map class="canvas-chrome-icon" />
    </button>
  </aside>

  <section
    v-if="tools.isLibraryMounted"
    class="canvas-chrome-library"
    :class="{ 'is-open': tools.isLibraryOpen && !isHidden }"
    :inert="isInert"
    @focusin="emit('library-focus-change', true)"
    @focusout="handleLibraryFocusOut"
  >
    <slot name="library" />
  </section>

  <section
    class="chrome-presence canvas-chrome-region canvas-chrome-region--status"
    :class="{ 'is-hidden': isHidden }"
    :inert="isInert"
    @wheel.prevent.stop
  >
    <div class="page-chrome-surface canvas-chrome-status-control">
      <button
        type="button"
        class="canvas-chrome-status-button hit-target-32"
        :aria-label="t('canvas.viewport.zoomOut')"
        @click="emit('zoom-out')"
      >
        <Minus class="size-4" />
      </button>
      <span
        class="min-w-[33px] text-center text-admin-body leading-[1.2] font-normal text-[var(--canvas-text-strong)]"
      >
        {{ status.zoomLevel }}%
      </span>
      <button
        type="button"
        class="canvas-chrome-status-button hit-target-32"
        :aria-label="t('canvas.viewport.zoomIn')"
        @click="emit('zoom-in')"
      >
        <Plus class="size-4" />
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { defineAsyncComponent } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Bookmark,
  BookmarkSolid,
  Download,
  Map,
  Minus,
  OneFingerSelectHandGesture,
  Plus,
  ShareAndroid,
  SpockHandGesture,
} from '@iconoir/vue'
import CanvasChromeBoardHeader from './CanvasChromeBoardHeader.vue'
import UserAccountMenu from '../../../../account/ui/chrome/UserAccountMenu.vue'
import { useSupportChannels } from '../../../../../core/composables/useSupportChannels'
import {
  createPageChromeAccountRoundControlClass,
  createPageChromeRoundControlClass,
} from '../../../../../core/lib/pageChrome'
import type {
  CanvasChromeAccountState,
  CanvasChromeBoardState,
  CanvasChromeStatusState,
  CanvasChromeToolState,
} from './types'

defineProps<{
  isInert: boolean
  isHidden: boolean
  board: CanvasChromeBoardState
  account: CanvasChromeAccountState
  tools: CanvasChromeToolState
  status: CanvasChromeStatusState
}>()

const emit = defineEmits<{
  'update:board-name-draft': [value: string]
  'toggle-board-menu': []
  'start-board-rename': []
  'cancel-board-rename': []
  'confirm-board-rename': []
  'reload-catalog': []
  'restore-archived-board': []
  'select-board': [boardId: string]
  share: []
  export: []
  'navigate-back': []
  'select-tool': [tool: 'select' | 'hand']
  'toggle-library': []
  'library-focus-change': [focused: boolean]
  'toggle-minimap': []
  'zoom-out': []
  'zoom-in': []
}>()

const NotificationBell = defineAsyncComponent(
  () => import('../../../../notification/ui/NotificationBell.vue'),
)

const { t } = useI18n()
const { channels: supportChannels } = useSupportChannels()

const canvasChromeRoundControlClass = createPageChromeRoundControlClass('canvas-chrome-icon-button')
const canvasChromeActionControlClass = createPageChromeRoundControlClass(
  'canvas-chrome-icon-button',
)
const canvasChromeAvatarButtonClass = createPageChromeAccountRoundControlClass()

function handleLibraryFocusOut(event: FocusEvent) {
  const nextTarget = event.relatedTarget
  const currentTarget = event.currentTarget
  if (
    nextTarget instanceof Node &&
    currentTarget instanceof HTMLElement &&
    currentTarget.contains(nextTarget)
  ) {
    return
  }
  emit('library-focus-change', false)
}
</script>

<style scoped>
.canvas-chrome-presence-slot {
  pointer-events: none;
  display: flex;
  flex: 1;
  justify-content: center;
  min-width: 0;
}

.canvas-chrome-presence-slot:empty {
  display: none;
}
</style>
