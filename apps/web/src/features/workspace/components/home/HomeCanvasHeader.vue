<template>
  <header
    class="pointer-events-none absolute top-4 right-4 left-4 z-[100] flex items-start justify-between gap-4 [--canvas-control-border-width:1.2px] [--canvas-control-icon-stroke:1.5]"
    @wheel.prevent.stop
  >
    <div
      class="pointer-events-auto relative box-border flex h-[39px] w-[min(200px,calc(100vw-32px))] items-center gap-2 rounded-full bg-white/[0.94] p-1 [border:var(--canvas-control-border-width)_solid_#d8ddea]"
      @mousedown.stop
    >
      <button
        type="button"
        class="canvas-icon-button !m-0 !flex !size-[29px] !cursor-pointer !items-center !justify-center !rounded-full !border-0 !bg-transparent !p-0 !text-[var(--ib-control-icon)] hover:!bg-[var(--ib-control-bg-hover)] hover:!text-[var(--ib-control-icon-hover)]"
        aria-label="返回画布列表"
        @click.stop="emit('navigate-back')"
      >
        <ArrowLeft class="size-4 [stroke-width:var(--canvas-control-icon-stroke)]" />
      </button>

      <div class="relative flex min-w-0 flex-1 items-center gap-2">
        <span
          :ref="bindBoardNameInputRef"
          class="m-0 w-full max-w-none cursor-text overflow-hidden truncate p-0 text-[15px] leading-[1.2] font-normal text-[#202124] outline-none"
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
          <SquareCursor class="size-4 [stroke-width:var(--canvas-control-icon-stroke)]" />
        </button>
        <div
          v-if="isBoardMenuOpen"
          v-motion="{
            initial: { opacity: 0, y: 6, scale: 0.98 },
            enter: {
              opacity: 1,
              y: 0,
              scale: 1,
              transition: { duration: 160, easing: 'ease-out' },
            },
          }"
          class="absolute top-[calc(100%+10px)] left-0 z-[140] max-h-[280px] min-w-[220px] max-w-[280px] overflow-y-auto rounded-[10px] bg-white/[0.98] p-1.5 backdrop-blur-[10px] [border:var(--canvas-control-border-width)_solid_#d8ddea]"
          @mousedown.stop
          @click.stop
        >
          <button
            v-for="board in boardOptions"
            :key="board.id"
            type="button"
            class="h-[34px] w-full cursor-pointer overflow-hidden rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-ellipsis whitespace-nowrap text-[#4b5565] hover:bg-[rgba(146,120,230,0.12)] hover:text-[#6757c8]"
            :class="{
              '!bg-[rgba(146,120,230,0.16)] !text-[#6757c8]': board.id === currentBoardId,
            }"
            @click="emit('select-board', board.id)"
          >
            {{ board.name }}
          </button>
          <div
            v-if="boardOptions.length === 0"
            class="flex h-[34px] items-center px-2.5 text-xs text-[#8a91a3]"
          >
            No canvas
          </div>
        </div>
      </div>
    </div>
    <div class="canvas-control-group pointer-events-auto">
      <button
        type="button"
        class="canvas-icon-button share-btn"
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
        class="canvas-icon-button enabled:!border-[#9278e6] enabled:!bg-[#9278e6] enabled:!text-white enabled:hover:!border-[#8067d8] enabled:hover:!bg-[#8067d8] enabled:hover:!text-white"
        type="button"
        aria-label="导出"
        :disabled="exportDisabled"
        @mousedown.stop
        @click="emit('export')"
      >
        <Download class="canvas-control-icon" />
      </button>
      <div class="relative" @pointerdown.stop @mousedown.stop @click.stop>
        <button
          type="button"
          class="ib-user-avatar-button workspace-user-avatar-button"
          aria-label="账户菜单"
          aria-haspopup="menu"
          :aria-expanded="isUserMenuOpen"
          aria-controls="workspace-user-menu"
          @click="toggleUserMenu"
        >
          <img
            v-if="avatarUrl"
            :src="avatarUrl"
            class="ib-user-avatar-button__image"
            alt="用户头像"
          />
          <div
            v-else
            class="ib-user-avatar-button__placeholder"
          >
            <User class="size-5 [stroke-width:var(--canvas-control-icon-stroke)]" />
          </div>
        </button>
        <div
          v-if="isUserMenuOpen"
          id="workspace-user-menu"
          v-motion="{
            initial: { opacity: 0, y: 6, scale: 0.98 },
            enter: {
              opacity: 1,
              y: 0,
              scale: 1,
              transition: { duration: 160, easing: 'ease-out' },
            },
          }"
          role="menu"
          class="absolute top-[calc(100%+8px)] right-0 z-[140] flex w-32 flex-col gap-0.5 rounded-[10px] bg-white/[0.98] p-1.5 backdrop-blur-[10px] [border:var(--canvas-control-border-width)_solid_#d8ddea]"
        >
          <button
            class="h-[34px] w-full cursor-pointer rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-[#4b5565] hover:bg-[rgba(146,120,230,0.12)]"
            role="menuitem"
            @click="onOpenCenter"
          >
            个人中心
          </button>
          <button
            class="h-[34px] w-full cursor-pointer rounded-lg border-0 bg-transparent px-2.5 text-left text-[13px] font-semibold text-[#d14343] hover:bg-[rgba(209,67,67,0.1)]"
            role="menuitem"
            @click="onLogout"
          >
            退出登录
          </button>
        </div>
      </div>
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
  User,
} from '@iconoir/vue'

const props = defineProps<{
  currentBoardName: string
  boardOptions: Array<{ id: string; name: string }>
  currentBoardId: string | null
  isBoardMenuOpen: boolean
  isRenamingBoard: boolean
  setBoardNameInputRef: (el: HTMLElement | null) => void
  exportDisabled: boolean
  avatarUrl?: string
}>()

const emit = defineEmits<{
  (e: 'update:boardNameDraft', value: string): void
  (e: 'toggle-board-menu'): void
  (e: 'start-board-rename'): void
  (e: 'cancel-board-rename'): void
  (e: 'confirm-board-rename'): void
  (e: 'select-board', boardId: string): void
  (e: 'export'): void
  (e: 'open-center'): void
  (e: 'logout'): void
  (e: 'navigate-back'): void
}>()

const isUserMenuOpen = ref(false)

function closeUserMenu() {
  isUserMenuOpen.value = false
}

function toggleUserMenu() {
  isUserMenuOpen.value = !isUserMenuOpen.value
}

function onOpenCenter() {
  closeUserMenu()
  emit('open-center')
}

function onLogout() {
  closeUserMenu()
  emit('logout')
}

function onGlobalPointerDown() {
  closeUserMenu()
}

onMounted(() => {
  window.addEventListener('pointerdown', onGlobalPointerDown)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onGlobalPointerDown)
})

function bindBoardNameInputRef(element: Element | ComponentPublicInstance | null) {
  props.setBoardNameInputRef(element as HTMLElement | null)
}
</script>

<style scoped>
@import '../styles/canvas-controls.css';
</style>
