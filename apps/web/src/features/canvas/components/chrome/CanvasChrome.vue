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

    <div class="canvas-chrome-control-group canvas-chrome-action-group">
      <UserCreditBalanceButton :balance="account.balance" />
      <button
        type="button"
        :class="canvasChromeActionControlClass"
        aria-label="分享功能稍后接入"
        title="分享功能稍后接入"
        disabled
        @mousedown.stop
      >
        <span class="canvas-chrome-icon-swap" aria-hidden="true">
          <ShareAndroid class="canvas-chrome-icon canvas-chrome-icon-swap-default" />
          <ShareAndroidSolid class="canvas-chrome-icon canvas-chrome-icon-swap-hover" />
        </span>
      </button>
      <button
        :class="canvasChromeExportControlClass"
        type="button"
        aria-label="导出"
        :disabled="status.exportDisabled"
        @mousedown.stop
        @click="emit('export')"
      >
        <Download class="canvas-chrome-icon" />
      </button>
      <UserAccountMenu
        :avatar-url="account.avatarUrl"
        :balance="account.balance"
        :items="account.menuItems"
        :avatar-button-class="canvasChromeAvatarButtonClass"
        :show-credits="false"
      />
    </div>
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
      aria-label="选择 (V)"
      :aria-pressed="tools.activeTool === 'select'"
      @click="emit('select-tool', 'select')"
    >
      <OneFingerSelectHandGesture class="canvas-chrome-icon" />
    </button>
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.activeTool === 'hand' }]"
      aria-label="抓手 (H)"
      :aria-pressed="tools.activeTool === 'hand'"
      @click="emit('select-tool', 'hand')"
    >
      <SpockHandGesture class="canvas-chrome-icon" />
    </button>
    <button
      type="button"
      :class="[canvasChromeRoundControlClass, { 'is-active': tools.minimapEnabled }]"
      aria-label="小地图"
      :aria-pressed="tools.minimapEnabled"
      title="小地图"
      @click="emit('toggle-minimap')"
    >
      <Map class="canvas-chrome-icon" />
    </button>
  </aside>

  <section
    class="canvas-chrome-region canvas-chrome-region--status"
    :class="{ 'is-hidden': isHidden }"
    :inert="inert"
    @wheel.prevent.stop
  >
    <div class="canvas-chrome-status-control">
      <button
        type="button"
        class="canvas-chrome-icon-button canvas-chrome-status-button"
        aria-label="缩小"
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
        class="canvas-chrome-icon-button canvas-chrome-status-button"
        aria-label="放大"
        @click="emit('zoom-in')"
      >
        <Plus class="size-4" />
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import {
  Download,
  Map,
  Minus,
  OneFingerSelectHandGesture,
  Plus,
  ShareAndroid,
  ShareAndroidSolid,
  SpockHandGesture,
} from '@iconoir/vue'
import CanvasChromeBoardHeader from './CanvasChromeBoardHeader.vue'
import UserAccountMenu from '../../../../shared/components/UserAccountMenu.vue'
import UserCreditBalanceButton from '../../../../shared/components/UserCreditBalanceButton.vue'
import {
  createPageChromeAccountAvatarButtonClass,
  createPageChromeRoundControlClass,
  pageChromeRoundControlVariants,
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
  'toggle-minimap': []
  'zoom-out': []
  'zoom-in': []
}>()

const canvasChromeRoundControlClass = createPageChromeRoundControlClass('canvas-chrome-icon-button')
const canvasChromeActionControlClass = createPageChromeRoundControlClass(
  pageChromeRoundControlVariants.borderless,
  'canvas-chrome-icon-button',
)
const canvasChromeExportControlClass = createPageChromeRoundControlClass(
  pageChromeRoundControlVariants.borderless,
  'canvas-chrome-icon-button enabled:!bg-[var(--canvas-accent)] enabled:!text-white enabled:hover:!bg-[var(--canvas-accent-hover)] enabled:hover:!text-white',
)
const canvasChromeAvatarButtonClass = createPageChromeAccountAvatarButtonClass(
  'canvas-user-avatar-button',
)
</script>
