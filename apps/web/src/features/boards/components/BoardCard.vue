<template>
  <article class="board-card board-item" @click="emit('click-board')">
    <div class="board-preview">
      <img v-if="previewSnapshotDataUrl" :src="previewSnapshotDataUrl" class="board-preview-image" alt="" />
      <div v-else class="preview-avatar">{{ board.name.slice(0, 1).toUpperCase() }}</div>
      <div class="board-preview-fade"></div>
    </div>

    <div class="board-meta">
      <input
        v-if="isRenaming"
        ref="renameInputEl"
        :value="renameDraft"
        class="rename-input"
        maxlength="60"
        @click.stop
        @input="onRenameInput"
        @blur="emit('commit-rename')"
        @keyup.enter.stop.prevent="emit('commit-rename')"
        @keyup.esc.stop.prevent="emit('cancel-rename')"
      />
      <div v-else class="board-title">{{ board.name }}</div>
      <div class="board-date">Edited {{ displayDate }}</div>
    </div>

    <div class="board-menu-anchor">
      <button
        class="board-menu-btn"
        type="button"
        title="More actions"
        aria-label="More actions"
        @click.stop="emit('toggle-menu')"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="5" r="1.8" fill="currentColor" />
          <circle cx="12" cy="12" r="1.8" fill="currentColor" />
          <circle cx="12" cy="19" r="1.8" fill="currentColor" />
        </svg>
      </button>
      <div
        v-if="isMenuOpen"
        class="board-action-menu"
        @click.stop
      >
        <button type="button" class="board-action-item" @click.stop="emit('rename-action')">
          Rename
        </button>
        <button type="button" class="board-action-item" @click.stop="emit('duplicate-action')">
          Duplicate
        </button>
        <button type="button" class="board-action-item danger" @click.stop="emit('delete-action')">
          Delete
        </button>
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { CanvasBoardMeta } from '../lib/canvasBoards'

const props = defineProps<{
  board: CanvasBoardMeta
  previewSnapshotDataUrl: string
  isRenaming: boolean
  renameDraft: string
  isMenuOpen: boolean
  displayDate: string
}>()

const emit = defineEmits<{
  (e: 'click-board'): void
  (e: 'toggle-menu'): void
  (e: 'rename-action'): void
  (e: 'duplicate-action'): void
  (e: 'delete-action'): void
  (e: 'rename-input', value: string): void
  (e: 'commit-rename'): void
  (e: 'cancel-rename'): void
}>()

const renameInputEl = ref<HTMLInputElement | null>(null)

watch(
  () => props.isRenaming,
  (isRenaming) => {
    if (!isRenaming) return
    void nextTick(() => {
      if (!renameInputEl.value) return
      renameInputEl.value.focus()
      renameInputEl.value.select()
    })
  },
)

function onRenameInput(event: Event) {
  emit('rename-input', (event.target as HTMLInputElement).value)
}
</script>

<style scoped>
.board-card {
  position: relative;
  border: 1px solid #d9dfec;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.92);
  overflow: hidden;
  height: 244px;
  text-align: left;
}

.board-item {
  cursor: pointer;
  transition:
    transform 0.14s ease,
    box-shadow 0.14s ease,
    border-color 0.14s ease;
}

.board-item:hover {
  border-color: #cdd4e6;
  transform: translateY(-1px);
  box-shadow: 0 8px 24px rgba(20, 26, 45, 0.12);
}

.board-preview {
  position: relative;
  z-index: 0;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f7f8ff 0%, #eff2fb 100%);
  overflow: hidden;
}

.board-preview-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.board-preview-fade {
  position: absolute;
  inset: auto 0 0;
  height: 92px;
  pointer-events: none;
  background: linear-gradient(to top, rgba(245, 248, 255, 0.88) 0%, rgba(245, 248, 255, 0) 100%);
}

.preview-avatar {
  width: 58px;
  height: 58px;
  border-radius: 16px;
  background: linear-gradient(145deg, #7a6bd5, #8b7bdd);
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  font-weight: 700;
}

.board-meta {
  position: absolute;
  left: 12px;
  right: 56px;
  bottom: 12px;
  z-index: 2;
  pointer-events: none;
}

.board-title {
  font-size: 14px;
  font-weight: 650;
  color: #293042;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.rename-input {
  width: 100%;
  border: 1px solid rgba(224, 230, 245, 0.75);
  border-radius: 8px;
  height: 30px;
  padding: 0 8px;
  font-size: 13px;
  color: #2b3142;
  background: rgba(255, 255, 255, 0.82);
  outline: none;
  pointer-events: auto;
}

.board-menu-btn {
  width: 34px;
  height: 34px;
  border: none;
  background: transparent;
  color: #4e556b;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.board-menu-btn:hover {
  color: #2b3142;
}

.board-menu-anchor {
  position: absolute;
  right: 12px;
  bottom: 12px;
  z-index: 3;
}

.board-action-menu {
  position: absolute;
  right: 0;
  bottom: calc(100% + 8px);
  z-index: 30;
  min-width: 118px;
  padding: 6px;
  border-radius: 10px;
  border: 1px solid #d8ddea;
  background: #ffffff;
  box-shadow: 0 10px 24px rgba(24, 34, 56, 0.14);
}

.board-action-item {
  width: 100%;
  height: 32px;
  border: none;
  border-radius: 8px;
  background: transparent;
  text-align: left;
  padding: 0 10px;
  font-size: 13px;
  font-weight: 600;
  color: #3f475d;
  cursor: pointer;
}

.board-action-item:hover {
  background: #eff3fb;
}

.board-action-item.danger {
  color: #c23b42;
}

.board-action-item.danger:hover {
  background: #fdeced;
}

.board-date {
  margin-top: 3px;
  font-size: 12px;
  color: #7b8297;
}
</style>
