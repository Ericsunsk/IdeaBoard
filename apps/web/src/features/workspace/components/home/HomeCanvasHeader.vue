<template>
  <header class="floating-header">
    <div class="header-left">
      <div class="file-name-chip" @mousedown.stop>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>
        </svg>
        <input
          v-if="isRenamingBoard"
          :ref="bindBoardNameInputRef"
          :value="boardNameDraft"
          class="board-name-input"
          type="text"
          maxlength="60"
          @input="emit('update:boardNameDraft', ($event.target as HTMLInputElement).value)"
          @keydown.enter.prevent="emit('confirm-board-rename')"
          @keydown.esc.prevent="emit('cancel-board-rename')"
          @blur="emit('confirm-board-rename')"
        />
        <button
          v-else
          type="button"
          class="file-name-btn"
          title="点击重命名画布"
          @click.stop="emit('start-board-rename')"
        >
          {{ currentBoardName }}
        </button>
        <button
          type="button"
          class="board-switch-btn"
          title="选择画布"
          @click.stop="emit('toggle-board-menu')"
        >
          <svg class="chevron-down" :class="{ open: isBoardMenuOpen }" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
            <polyline points="6 9 12 15 18 9"/>
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
      <button class="btn-ghost share-btn" @mousedown.stop>Share</button>
      <button class="btn-primary export-btn" :disabled="exportDisabled" @mousedown.stop @click="emit('export')">Export</button>
      <div class="user-avatar-wrap" title="个人中心" @click="emit('open-center')">
        <img v-if="avatarUrl" :src="avatarUrl" class="user-avatar" />
        <div v-else class="user-avatar-placeholder">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
            <circle cx="12" cy="7" r="4"/>
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
  boardNameDraft: string
  setBoardNameInputRef: (el: HTMLInputElement | null) => void
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
}>()

function bindBoardNameInputRef(element: Element | ComponentPublicInstance | null) {
  props.setBoardNameInputRef(element as HTMLInputElement | null)
}
</script>

<style scoped>
.floating-header { position: absolute; top: 16px; left: 16px; right: 16px; display: flex; justify-content: space-between; z-index: 100; pointer-events: none; }
.header-left, .header-right { display: flex; gap: 12px; pointer-events: auto; }
.file-name-chip {
  background: #f1f3f9;
  padding: 8px 16px;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 500;
  color: #202124;
  cursor: pointer;
  display: flex;
  gap: 8px;
  align-items: center;
  transition: background 0.2s;
  position: relative;
}
.file-name-chip:hover { background: #e8eaed; }
.file-name-btn {
  border: none;
  background: transparent;
  padding: 0;
  margin: 0;
  color: #202124;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  line-height: 1.2;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-name-btn:hover { color: #6757c8; }
.board-name-input {
  height: 24px;
  min-width: 140px;
  max-width: 220px;
  border-radius: 6px;
  border: 1px solid #cfd5e3;
  padding: 0 8px;
  font-size: 14px;
  outline: none;
  color: #202124;
  background: #fff;
}
.board-name-input:focus { border-color: #9278e6; }
.board-switch-btn {
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #5f6368;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: background-color 0.2s ease, color 0.2s ease;
}
.board-switch-btn:hover { background: #fff; color: #6757c8; }
.chevron-down { transition: transform 0.2s ease; }
.chevron-down.open { transform: rotate(180deg); }
.board-switch-menu {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  min-width: 220px;
  max-width: 280px;
  max-height: 280px;
  overflow-y: auto;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid #d8ddea;
  background: rgba(255, 255, 255, 0.96);
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
.board-switch-item:hover { background: rgba(146, 120, 230, 0.12); color: #6757c8; }
.board-switch-item.active { background: rgba(146, 120, 230, 0.16); color: #6757c8; }
.board-switch-empty {
  height: 34px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  font-size: 12px;
  color: #8a91a3;
}
.btn-ghost, .btn-primary { height: 40px; padding: 0 16px; border-radius: 10px; font-size: 14px; border: none; cursor: pointer; }
.btn-ghost { background: #f1f3f9; }
.btn-ghost:hover { background: #f1f3f9; }
.share-btn { background: #f1f3f9 !important; }
.share-btn:hover { background: #f1f3f9 !important; transform: none; }
.btn-primary { background: #9278e6; color: white; }
.btn-primary:disabled { background: #e2e4e8; color: #a8adb7; cursor: not-allowed; }
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
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
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
