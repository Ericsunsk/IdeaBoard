<template>
  <div class="canvas-chrome-board-stack" @mousedown.stop>
    <div class="canvas-chrome-board-control">
      <button
        type="button"
        class="canvas-chrome-board-icon-button"
:aria-label="t('canvas.chrome.backToBoards')"
        @click.stop="emit('navigate-back')"
      >
        <ArrowLeft class="size-4" />
      </button>

      <div class="relative flex min-w-0 flex-1 items-center gap-2">
        <span
          :ref="bindBoardNameInputRef"
          class="m-0 w-full max-w-none cursor-text overflow-hidden truncate p-0 text-[15px] leading-[1.2] font-normal text-[var(--canvas-text-strong)] outline-none"
          :class="{ 'text-clip': board.isRenaming }"
          :contenteditable="board.isRenaming && !board.isRenamePending ? 'plaintext-only' : 'false'"
          role="textbox"
          tabindex="0"
:aria-label="t('canvas.chrome.boardName')"
          :aria-readonly="!board.isRenaming || board.isRenamePending"
          :aria-busy="board.isRenamePending"
          spellcheck="false"
:title="t('canvas.chrome.clickToRenameBoard')"
          @click.stop="emit('start-board-rename')"
          @mousedown.stop
          @input="emit('update:board-name-draft', ($event.target as HTMLElement).textContent ?? '')"
          @keydown.enter.prevent="onBoardNameEnter"
          @keydown.esc.prevent="onBoardNameEscape"
          @blur="emit('confirm-board-rename')"
        >
          {{ board.currentName }}
        </span>
      </div>

      <div class="canvas-chrome-board-menu-slot">
        <button
          type="button"
          class="canvas-chrome-board-icon-button"
:aria-label="t('canvas.chrome.selectBoard')"
          @click.stop="emit('toggle-board-menu')"
        >
          <SquareCursor class="size-4" />
        </button>
        <Transition v-bind="boardMenuMotion">
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
    <div v-if="board.isRenamePending" class="canvas-chrome-catalog-recovery" aria-live="polite">
      <span class="canvas-chrome-catalog-recovery-label">{{ t('canvas.chrome.syncingName') }}</span>
    </div>
    <div
      v-if="board.catalogRecovery.status !== 'idle'"
      class="canvas-chrome-catalog-recovery"
      aria-live="polite"
    >
      <span class="canvas-chrome-catalog-recovery-label">
        {{
          board.catalogRecovery.status === 'reloading'
            ? t('canvas.chrome.reloadingCatalog')
            : t('canvas.chrome.catalogReloadRequired')
        }}
      </span>
      <button
        type="button"
        class="canvas-chrome-catalog-recovery-action"
:aria-label="t('canvas.chrome.reloadCatalog')"
        :disabled="board.catalogRecovery.status === 'reloading'"
        @click="emit('reload-board-catalog')"
      >
        {{ t('canvas.chrome.reload') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ComponentPublicInstance } from 'vue'
import { ArrowLeft, SquareCursor } from '@iconoir/vue'
import { createPopoverMotionTransition } from '../../../../shared/motion/popoverMotion'
import type { CanvasChromeBoardState } from './types'

const props = defineProps<{
  board: CanvasChromeBoardState
}>()

const emit = defineEmits<{
  'update:board-name-draft': [value: string]
  'toggle-board-menu': []
  'start-board-rename': []
  'cancel-board-rename': []
  'confirm-board-rename': []
  'reload-board-catalog': []
  'select-board': [boardId: string]
  'navigate-back': []
}>()

const { t } = useI18n()

const boardMenuMotion = createPopoverMotionTransition()

function bindBoardNameInputRef(element: Element | ComponentPublicInstance | null) {
  props.board.setNameInputRef(element as HTMLElement | null)
}

function onBoardNameEnter() {
  if (props.board.isRenaming) {
    emit('confirm-board-rename')
    return
  }
  if (!props.board.isRenamePending) emit('start-board-rename')
}

function onBoardNameEscape() {
  if (props.board.isRenaming) emit('cancel-board-rename')
}
</script>
