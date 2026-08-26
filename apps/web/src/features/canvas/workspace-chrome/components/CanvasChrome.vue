<template>
  <header
    class="canvas-chrome-region canvas-chrome-region--top"
    :class="{ 'is-hidden': isHidden }"
    :inert="inert"
    @wheel.prevent.stop
  >
    <CanvasChromeBoardHeader
      :board="board"
      @update:board-name-draft="emit('update:board-name-draft', $event)"
      @toggle-board-menu="emit('toggle-board-menu')"
      @start-board-rename="emit('start-board-rename')"
      @cancel-board-rename="emit('cancel-board-rename')"
      @confirm-board-rename="emit('confirm-board-rename')"
      @reload-board-catalog="emit('reload-board-catalog')"
      @select-board="emit('select-board', $event)"
      @navigate-back="emit('navigate-back')"
    />

    <UserAccountMenu
      class="canvas-chrome-action-group"
      :avatar-url="account.avatarUrl"
      :balance="account.balance"
      :items="account.menuItems"
      :avatar-button-class="canvasChromeAvatarButtonClass"
    >
      <template #before-balance>
        <UserLocaleSelect />
      </template>
      <template #actions>
        <button
          type="button"
          :class="canvasChromeActionControlClass"
          :aria-label="t('canvas.chrome.shareComingSoon')"
          :title="t('canvas.chrome.shareComingSoon')"
          disabled
          @mousedown.stop
        >
          <ShareAndroid class="canvas-chrome-icon" />
        </button>
        <button
          :class="[canvasChromeExportControlClass, { 'is-active': !status.exportDisabled }]"
          type="button"
          :aria-label="t('canvas.chrome.export')"
          :disabled="status.exportDisabled"
          @mousedown.stop
          @click="emit('export')"
        >
          <Download class="canvas-chrome-icon" />
        </button>
      </template>
    </UserAccountMenu>
  </header>

  <aside
    class="canvas-chrome-region canvas-chrome-region--left"
    :class="{ 'is-hidden': isHidden }"
    :inert="inert"
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
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.libraryOpen }]"
      :aria-label="t('common.navigation.library')"
      :aria-pressed="tools.libraryOpen"
      :title="t('common.navigation.library')"
      @click="emit('toggle-library')"
    >
      <BookmarkSolid v-if="tools.libraryOpen" class="canvas-chrome-icon" />
      <Bookmark v-else class="canvas-chrome-icon" />
    </button>
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.minimapEnabled }]"
      :aria-label="t('canvas.minimap.label')"
      :aria-pressed="tools.minimapEnabled"
      :title="t('canvas.minimap.label')"
      @click="emit('toggle-minimap')"
    >
      <Map class="canvas-chrome-icon" />
    </button>
  </aside>

  <section
    v-if="tools.libraryMounted"
    class="canvas-chrome-region canvas-chrome-region--library"
    :class="{ 'is-hidden': isHidden, 'is-open': tools.libraryOpen }"
    :inert="inert"
    @focusin="emit('library-focus-change', true)"
    @focusout="handleLibraryFocusOut"
  >
    <slot name="library" />
  </section>

  <section
    class="canvas-chrome-region canvas-chrome-region--status"
    :class="{ 'is-hidden': isHidden }"
    :inert="inert"
    @wheel.prevent.stop
  >
    <div class="canvas-chrome-status-control">
      <button
        type="button"
        class="canvas-chrome-status-button"
        :aria-label="t('canvas.viewport.zoomOut')"
        @click="emit('zoom-out')"
      >
        <Minus class="size-4" />
      </button>
      <span
        class="min-w-[33px] text-center text-[13px] leading-[1.2] font-normal text-[var(--canvas-text-strong)]"
      >
        {{ status.zoomLevel }}%
      </span>
      <button
        type="button"
        class="canvas-chrome-status-button"
        :aria-label="t('canvas.viewport.zoomIn')"
        @click="emit('zoom-in')"
      >
        <Plus class="size-4" />
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
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
import UserAccountMenu from '../../../../shared/components/UserAccountMenu.vue'
import UserLocaleSelect from '../../../../shared/components/UserLocaleSelect.vue'
import {
  createPageChromeAccountRoundControlClass,
  createPageChromeRoundControlClass,
} from '../../../../shared/lib/pageChrome'
import type {
  CanvasChromeAccountState,
  CanvasChromeBoardState,
  CanvasChromeStatusState,
  CanvasChromeToolState,
} from './types'

defineProps<{
  inert: boolean
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
  'reload-board-catalog': []
  'select-board': [boardId: string]
  export: []
  'navigate-back': []
  'select-tool': [tool: 'select' | 'hand']
  'toggle-library': []
  'library-focus-change': [focused: boolean]
  'toggle-minimap': []
  'zoom-out': []
  'zoom-in': []
}>()

const { t } = useI18n()

const canvasChromeRoundControlClass = createPageChromeRoundControlClass('canvas-chrome-icon-button')
const canvasChromeActionControlClass = createPageChromeRoundControlClass(
  'canvas-chrome-icon-button',
)
const canvasChromeExportControlClass = createPageChromeRoundControlClass(
  'canvas-chrome-icon-button canvas-chrome-export-button',
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
