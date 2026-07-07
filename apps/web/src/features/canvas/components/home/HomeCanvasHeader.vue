<template>
  <header
    class="pointer-events-none absolute top-[var(--canvas-chrome-inset)] right-[var(--canvas-chrome-inset)] left-[var(--canvas-chrome-inset)] z-[100] flex items-start justify-between gap-4"
    @wheel.prevent.stop
  >
    <div
      class="pointer-events-auto relative box-border flex h-[39px] w-[min(200px,calc(100vw-32px))] items-center gap-2 rounded-full bg-[var(--canvas-surface-bg-soft)] p-1 [border:var(--canvas-control-border-width)_solid_var(--canvas-surface-border)]"
      @mousedown.stop
    >
      <button
        type="button"
        class="canvas-icon-button !m-0 !flex !size-[29px] !cursor-pointer !items-center !justify-center !rounded-full !border-0 !bg-transparent !p-0 !text-[var(--ib-control-icon)] hover:!bg-[var(--ib-control-bg-hover)] hover:!text-[var(--ib-control-icon-hover)]"
        aria-label="返回画布列表"
        @click.stop="emit('navigate-back')"
      >
        <ArrowLeft class="size-4" />
      </button>

      <div class="relative flex min-w-0 flex-1 items-center gap-2">
        <span
          :ref="bindBoardNameInputRef"
          class="m-0 w-full max-w-none cursor-text overflow-hidden truncate p-0 text-[15px] leading-[1.2] font-normal text-[var(--canvas-text-strong)] outline-none"
          :class="{ 'text-clip': isRenamingBoard }"
          :contenteditable="isRenamingBoard ? 'plaintext-only' : 'false'"
          spellcheck="false"
          title="点击重命名画布"
          @click.stop="emit('start-board-rename')"
          @mousedown.stop
          @input="emit('update:boardNameDraft', ($event.target as HTMLElement).textContent ?? '')"
          @keydown.enter.prevent="emit('confirm-board-rename')"
          @keydown.esc.prevent="emit('cancel-board-rename')"
          @blur="emit('confirm-board-rename')"
        >
          {{ currentBoardName }}
        </span>
      </div>

      <div class="h-[29px] w-[29px] flex-none">
        <button
          type="button"
          class="canvas-icon-button !m-0 !flex !size-[29px] !cursor-pointer !items-center !justify-center !rounded-full !border-0 !bg-transparent !p-0 !text-[var(--ib-control-icon)] hover:!bg-[var(--ib-control-bg-hover)] hover:!text-[var(--ib-control-icon-hover)]"
          aria-label="选择画布"
          @click.stop="emit('toggle-board-menu')"
        >
          <SquareCursor class="size-4" />
        </button>
        <Transition name="home-canvas-menu">
          <div
            v-if="isBoardMenuOpen"
            class="home-canvas-board-menu absolute top-[calc(100%+10px)] left-0 z-[140] max-h-[280px] min-w-[220px] max-w-[280px] overflow-y-auto rounded-[var(--canvas-surface-radius)] bg-[var(--canvas-surface-bg)] p-1.5 backdrop-blur-[var(--canvas-surface-blur)] [border:var(--canvas-control-border-width)_solid_var(--canvas-surface-border)]"
            @mousedown.stop
            @click.stop
          >
            <button
              v-for="board in boardOptions"
              :key="board.id"
              type="button"
              class="h-[34px] w-full cursor-pointer overflow-hidden rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-ellipsis whitespace-nowrap text-[var(--canvas-text-body)] hover:bg-[var(--canvas-accent-soft)] hover:text-[var(--canvas-accent-strong)]"
              :class="{
                '!bg-[var(--canvas-accent-soft-strong)] !text-[var(--canvas-accent-strong)]':
                  board.id === currentBoardId,
              }"
              @click="emit('select-board', board.id)"
            >
              {{ board.name }}
            </button>
            <div
              v-if="boardOptions.length === 0"
              class="flex h-[34px] items-center px-2.5 text-xs text-[var(--canvas-text-subtle)]"
            >
              No canvas
            </div>
          </div>
        </Transition>
      </div>
    </div>
    <div class="canvas-control-group pointer-events-auto">
      <button
        type="button"
        class="ib-round-control ib-page-chrome-control canvas-icon-button share-btn"
        aria-label="分享功能稍后接入"
        title="分享功能稍后接入"
        disabled
        @mousedown.stop
      >
        <span class="canvas-icon-swap" aria-hidden="true">
          <ShareAndroid class="canvas-control-icon canvas-icon-swap-default" />
          <ShareAndroidSolid class="canvas-control-icon canvas-icon-swap-hover" />
        </span>
      </button>
      <button
        class="ib-round-control ib-page-chrome-control canvas-icon-button enabled:!border-[var(--canvas-accent)] enabled:!bg-[var(--canvas-accent)] enabled:!text-white enabled:hover:!border-[var(--canvas-accent-hover)] enabled:hover:!bg-[var(--canvas-accent-hover)] enabled:hover:!text-white"
        type="button"
        aria-label="导出"
        :disabled="exportDisabled"
        @mousedown.stop
        @click="emit('export')"
      >
        <Download class="canvas-control-icon" />
      </button>
      <UserAccountMenu
        :avatar-url="avatarUrl"
        :items="userMenuItems"
        avatar-button-class="ib-round-control ib-page-chrome-control ib-user-avatar-button canvas-user-avatar-button"
      />
    </div>
  </header>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import {
  ArrowLeft,
  Download,
  ShareAndroid,
  ShareAndroidSolid,
  SquareCursor,
} from '@iconoir/vue'
import UserAccountMenu from '../../../../shared/components/UserAccountMenu.vue'
import type { UserAccountMenuItem } from '../../../../shared/components/userAccountMenuActions'

const props = defineProps<{
  currentBoardName: string
  boardOptions: Array<{ id: string; name: string }>
  currentBoardId: string | null
  isBoardMenuOpen: boolean
  isRenamingBoard: boolean
  setBoardNameInputRef: (el: HTMLElement | null) => void
  exportDisabled: boolean
  avatarUrl?: string
  userMenuItems: UserAccountMenuItem[]
}>()

const emit = defineEmits<{
  (e: 'update:boardNameDraft', value: string): void
  (e: 'toggle-board-menu'): void
  (e: 'start-board-rename'): void
  (e: 'cancel-board-rename'): void
  (e: 'confirm-board-rename'): void
  (e: 'select-board', boardId: string): void
  (e: 'export'): void
  (e: 'navigate-back'): void
}>()

function bindBoardNameInputRef(element: Element | ComponentPublicInstance | null) {
  props.setBoardNameInputRef(element as HTMLElement | null)
}
</script>

<style scoped>
.home-canvas-menu-enter-active,
.home-canvas-menu-leave-active {
  transition:
    opacity var(--canvas-motion-fast) ease-out,
    transform var(--canvas-motion-fast) ease-out;
}

.home-canvas-board-menu {
  transform-origin: top left;
}

.home-canvas-menu-enter-from,
.home-canvas-menu-leave-to {
  opacity: 0;
  transform: translateY(6px) scale(0.98);
}

.home-canvas-menu-enter-to,
.home-canvas-menu-leave-from {
  opacity: 1;
  transform: translateY(0) scale(1);
}

@media (prefers-reduced-motion: reduce) {
  .home-canvas-menu-enter-active,
  .home-canvas-menu-leave-active {
    transition: none;
  }
}
</style>
