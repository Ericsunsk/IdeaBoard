<template>
  <header class="floating-header" @wheel.prevent.stop>
    <div class="header-shell" @mousedown.stop>
      <button
        type="button"
        class="header-back-btn"
        title="返回画布列表"
        @click.stop="emit('navigate-back')"
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M19 12H7" />
          <path d="M12 17l-5-5 5-5" />
        </svg>
      </button>

      <div class="header-title-slot">
        <span
          :ref="bindBoardNameInputRef"
          class="file-name-btn"
          :class="{ 'is-editing': isRenamingBoard }"
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

      <div class="header-actions">
        <button
          type="button"
          class="board-switch-btn"
          title="选择画布"
          @click.stop="emit('toggle-board-menu')"
        >
          <svg
            class="chevron-down"
            :class="{ open: isBoardMenuOpen }"
            width="12"
            height="12"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="3"
          >
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </button>
        <div v-if="isBoardMenuOpen" class="board-switch-menu" @mousedown.stop @click.stop>
          <button
            v-for="board in boardOptions"
            :key="board.id"
            type="button"
            class="board-switch-item"
            :class="{ active: board.id === currentBoardId }"
            @click="emit('select-board', board.id)"
          >
            {{ board.name }}
          </button>
          <div v-if="boardOptions.length === 0" class="board-switch-empty">No canvas</div>
        </div>
      </div>
    </div>
    <div class="header-right">
      <button class="header-icon-btn share-btn" title="分享" @mousedown.stop>
        <svg
          width="19"
          height="19"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M10 13a5 5 0 0 0 7.07 0l2.12-2.12a5 5 0 0 0-7.07-7.07L10.9 5.03" />
          <path d="M14 11a5 5 0 0 0-7.07 0L4.81 13.12a5 5 0 0 0 7.07 7.07l1.22-1.22" />
        </svg>
      </button>
      <button
        class="header-icon-btn export-btn"
        title="导出"
        :disabled="exportDisabled"
        @mousedown.stop
        @click="emit('export')"
      >
        <svg
          width="19"
          height="19"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <path d="M7 10l5 5 5-5" />
          <path d="M12 15V3" />
        </svg>
      </button>
      <div class="user-avatar-wrap" title="个人中心" @click="emit('open-center')">
        <img v-if="avatarUrl" :src="avatarUrl" class="user-avatar" />
        <div v-else class="user-avatar-placeholder">
          <svg
            width="20"
            height="20"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2" />
            <circle cx="12" cy="7" r="4" />
          </svg>
        </div>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'

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
  (e: 'navigate-back'): void
}>()

function bindBoardNameInputRef(element: Element | ComponentPublicInstance | null) {
  props.setBoardNameInputRef(element as HTMLElement | null)
}
</script>

<style scoped>
.floating-header {
  position: absolute;
  top: 16px;
  left: 16px;
  right: 16px;
  z-index: 100;
  pointer-events: none;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}
.header-shell {
  position: relative;
  width: min(200px, calc(100vw - 32px));
  box-sizing: border-box;
  height: 39px;
  padding: 4px;
  border-radius: 999px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.94);
  display: flex;
  align-items: center;
  gap: 8px;
  pointer-events: auto;
}
.header-right {
  display: flex;
  gap: 12px;
  pointer-events: auto;
}
.header-title-slot {
  position: relative;
  flex: 1;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.header-back-btn {
  width: 29px;
  height: 29px;
  flex: 0 0 auto;
  margin: 0;
  padding: 0;
  border: none;
  border-radius: 999px;
  background: transparent;
  color: #202124;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    background-color 0.18s ease,
    color 0.18s ease;
}
.header-back-btn:hover {
  background: #e8eaed;
  color: #6757c8;
}
.file-name-btn {
  padding: 0;
  margin: 0;
  color: #202124;
  font-size: 15px;
  font-weight: 400;
  cursor: text;
  outline: none;
  line-height: 1.2;
  width: 100%;
  max-width: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-name-btn.is-editing {
  text-overflow: clip;
}
.header-actions {
  flex: 0 0 auto;
  width: 29px;
  height: 29px;
}
.board-switch-btn {
  width: 29px;
  height: 29px;
  margin: 0;
  padding: 0;
  border: none;
  border-radius: 999px;
  background: transparent;
  color: #5f6368;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    background-color 0.2s ease,
    color 0.2s ease;
}
.board-switch-btn:hover {
  background: #e8eaed;
  color: #6757c8;
}
.chevron-down {
  transition: transform 0.2s ease;
}
.chevron-down.open {
  transform: rotate(180deg);
}
.board-switch-menu {
  position: absolute;
  top: calc(100% + 10px);
  left: 0;
  min-width: 220px;
  max-width: 280px;
  max-height: 280px;
  overflow-y: auto;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.98);
  backdrop-filter: blur(10px);
  z-index: 140;
}
.board-switch-item {
  width: 100%;
  height: 34px;
  border: none;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: #4b5565;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.board-switch-item:hover {
  background: rgba(146, 120, 230, 0.12);
  color: #6757c8;
}
.board-switch-item.active {
  background: rgba(146, 120, 230, 0.16);
  color: #6757c8;
}
.board-switch-empty {
  height: 34px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  font-size: 12px;
  color: #8a91a3;
}
.header-icon-btn {
  width: 40px;
  height: 40px;
  padding: 0;
  border: 1px solid #e2e4e8;
  border-radius: 50%;
  background: #fff;
  color: #5f6368;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease,
    color 0.2s ease;
}
.header-icon-btn:hover:not(:disabled) {
  border-color: #d8ddea;
  background: #e8eaed;
  color: #6757c8;
}
.export-btn:not(:disabled) {
  border-color: #9278e6;
  background: #9278e6;
  color: #fff;
}
.export-btn:hover:not(:disabled) {
  border-color: #8067d8;
  background: #8067d8;
  color: #fff;
}
.header-icon-btn:disabled {
  background: #e2e4e8;
  color: #a8adb7;
  cursor: not-allowed;
}
.user-avatar-wrap {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  overflow: hidden;
  cursor: pointer;
  border: 1px solid #e2e4e8;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f1f3f9;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease,
    color 0.2s ease;
  flex-shrink: 0;
  color: #5f6368;
}
.user-avatar-wrap:hover {
  border-color: #d8ddea;
  background: #e8eaed;
  color: #6757c8;
}
.user-avatar {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.user-avatar-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
