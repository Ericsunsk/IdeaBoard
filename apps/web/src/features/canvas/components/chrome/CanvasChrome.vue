<template>
  <header
    class="canvas-chrome-region canvas-chrome-region--top"
    :class="{ 'is-hidden': isHidden }"
    :inert="inert"
    @wheel.prevent.stop
  >
    <div class="canvas-chrome-board-control" @mousedown.stop>
      <button
        type="button"
        class="canvas-chrome-icon-button canvas-chrome-board-icon-button"
        aria-label="返回画布列表"
        @click.stop="emit('navigate-back')"
      >
        <ArrowLeft class="size-4" />
      </button>

      <div class="relative flex min-w-0 flex-1 items-center gap-2">
        <span
          :ref="bindBoardNameInputRef"
          class="m-0 w-full max-w-none cursor-text overflow-hidden truncate p-0 text-[15px] leading-[1.2] font-normal text-[var(--canvas-text-strong)] outline-none"
          :class="{ 'text-clip': board.isRenaming }"
          :contenteditable="board.isRenaming ? 'plaintext-only' : 'false'"
          role="textbox"
          aria-label="画布名称"
          :aria-readonly="!board.isRenaming"
          spellcheck="false"
          title="点击重命名画布"
          @click.stop="emit('start-board-rename')"
          @mousedown.stop
          @input="emit('update:board-name-draft', ($event.target as HTMLElement).textContent ?? '')"
          @keydown.enter.prevent="emit('confirm-board-rename')"
          @keydown.esc.prevent="emit('cancel-board-rename')"
          @blur="emit('confirm-board-rename')"
        >
          {{ board.currentName }}
        </span>
      </div>

      <div class="canvas-chrome-board-menu-slot">
        <button
          type="button"
          class="canvas-chrome-icon-button canvas-chrome-board-icon-button"
          aria-label="选择画布"
          @click.stop="emit('toggle-board-menu')"
        >
          <SquareCursor class="size-4" />
        </button>
        <Transition name="canvas-chrome-board-menu">
          <div v-if="board.isMenuOpen" class="canvas-chrome-board-menu" @mousedown.stop @click.stop>
            <button
              v-for="option in board.options"
              :key="option.id"
              type="button"
              class="h-[34px] w-full cursor-pointer overflow-hidden rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-ellipsis whitespace-nowrap text-[var(--canvas-text-body)] hover:bg-[var(--canvas-accent-soft)] hover:text-[var(--canvas-accent-strong)]"
              :class="{
                '!bg-[var(--canvas-accent-soft-strong)] !text-[var(--canvas-accent-strong)]':
                  option.id === board.currentId,
              }"
              @click="emit('select-board', option.id)"
            >
              {{ option.name }}
            </button>
            <div
              v-if="board.options.length === 0"
              class="flex h-[34px] items-center px-2.5 text-xs text-[var(--canvas-text-subtle)]"
            >
              No canvas
            </div>
          </div>
        </Transition>
      </div>
    </div>

    <div class="canvas-chrome-control-group canvas-chrome-action-group">
      <button
        type="button"
        :class="canvasChromeRoundControlClass"
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
        :items="account.menuItems"
        :avatar-button-class="canvasChromeAvatarButtonClass"
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
import type { ComponentPublicInstance } from 'vue'
import {
  ArrowLeft,
  Download,
  Map,
  Minus,
  OneFingerSelectHandGesture,
  Plus,
  ShareAndroid,
  ShareAndroidSolid,
  SpockHandGesture,
  SquareCursor,
} from '@iconoir/vue'
import UserAccountMenu from '../../../../shared/components/UserAccountMenu.vue'
import {
  createPageChromeAccountAvatarButtonClass,
  createPageChromeRoundControlClass,
} from '../../../../shared/lib/pageChrome'
import type {
  CanvasChromeAccountState,
  CanvasChromeBoardState,
  CanvasChromeStatusState,
  CanvasChromeToolState,
} from './types'

const props = defineProps<{
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
  'select-board': [boardId: string]
  export: []
  'navigate-back': []
  'select-tool': [tool: 'select' | 'hand']
  'toggle-minimap': []
  'zoom-out': []
  'zoom-in': []
}>()

const canvasChromeRoundControlClass = createPageChromeRoundControlClass('canvas-chrome-icon-button')
const canvasChromeExportControlClass = createPageChromeRoundControlClass(
  'canvas-chrome-icon-button enabled:!border-[var(--canvas-accent)] enabled:!bg-[var(--canvas-accent)] enabled:!text-white enabled:hover:!border-[var(--canvas-accent-hover)] enabled:hover:!bg-[var(--canvas-accent-hover)] enabled:hover:!text-white',
)
const canvasChromeAvatarButtonClass = createPageChromeAccountAvatarButtonClass(
  'canvas-user-avatar-button',
)

function bindBoardNameInputRef(element: Element | ComponentPublicInstance | null) {
  props.board.setNameInputRef(element as HTMLElement | null)
}
</script>
