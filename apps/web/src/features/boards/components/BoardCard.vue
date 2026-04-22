<template>
  <article class="board-card board-item" @click="emit('click-board')">
    <div class="board-preview">
      <div v-if="previewItems.length" class="board-preview-canvas">
        <template v-for="item in previewItems" :key="item.key">
          <img
            v-if="item.kind === 'image'"
            :src="item.url"
            class="board-preview-image"
            :style="{
              left: `${item.left}%`,
              top: `${item.top}%`,
              width: `${item.width}%`,
              height: `${item.height}%`,
              zIndex: item.zIndex,
            }"
          />
          <div
            v-else
            class="board-preview-node"
            :style="{
              left: `${item.left}%`,
              top: `${item.top}%`,
              width: `${item.width}%`,
              height: `${item.height}%`,
              zIndex: item.zIndex,
            }"
          >
            <div class="board-preview-node-head">{{ item.label }}</div>
            <img
              v-if="item.thumbUrl"
              :src="item.thumbUrl"
              class="board-preview-node-thumb"
            />
            <div v-else class="board-preview-node-text">{{ item.text || 'Node' }}</div>
          </div>
        </template>
      </div>
      <div v-else class="preview-avatar">{{ board.name.slice(0, 1).toUpperCase() }}</div>
    </div>

    <div class="board-meta">
      <div class="board-title-row">
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
        <button
          class="board-menu-btn"
          type="button"
          title="More actions"
          @click.stop="emit('toggle-menu')"
        >
          ⋮
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
      <div class="board-date">Edited {{ displayDate }}</div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { CanvasBoardMeta } from '../lib/canvasBoards'
import type { BoardPreviewItem } from '../lib/boardPreviewSerializer'

const props = defineProps<{
  board: CanvasBoardMeta
  previewItems: BoardPreviewItem[]
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
  border: 1px solid #d9dfec;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.92);
  overflow: hidden;
  text-align: left;
}

.board-item {
  cursor: pointer;
}

.board-item:hover {
  border-color: #cdd4e6;
  transform: translateY(-1px);
}

.board-preview {
  position: relative;
  height: 186px;
  border-bottom: 1px solid #e7ebf3;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f7f8ff 0%, #eff2fb 100%);
  overflow: hidden;
}

.board-preview-canvas {
  position: absolute;
  inset: 0;
}

.board-preview-image {
  position: absolute;
  object-fit: cover;
  border-radius: 6px;
  border: 1px solid rgba(196, 205, 227, 0.8);
  box-shadow: 0 2px 8px rgba(20, 26, 45, 0.1);
}

.board-preview-node {
  position: absolute;
  border-radius: 6px;
  border: 1px solid rgba(195, 204, 226, 0.9);
  overflow: hidden;
  background: #ffffff;
  box-shadow: 0 2px 8px rgba(20, 26, 45, 0.08);
}

.board-preview-node-head {
  height: 14px;
  background: #f5f7fb;
  color: #6b7488;
  border-bottom: 1px solid #eceff6;
  padding: 0 5px;
  font-size: 9px;
  font-weight: 700;
  line-height: 14px;
}

.board-preview-node-text {
  padding: 5px;
  color: #7b8398;
  font-size: 8px;
  line-height: 1.35;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 5;
  overflow: hidden;
}

.board-preview-node-thumb {
  width: 100%;
  height: calc(100% - 14px);
  object-fit: cover;
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
  padding: 10px 12px 12px;
}

.board-title-row {
  position: relative;
  display: flex;
  align-items: center;
  gap: 8px;
}

.board-title {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  font-weight: 700;
  color: #2b3142;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.rename-input {
  flex: 1;
  min-width: 0;
  border: 1px solid #b5c0e4;
  border-radius: 8px;
  height: 30px;
  padding: 0 8px;
  font-size: 13px;
  color: #2b3142;
  outline: none;
}

.board-menu-btn {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  border: none;
  background: transparent;
  font-size: 18px;
  color: #6f768c;
  cursor: pointer;
}

.board-menu-btn:hover {
  background: #edf1fa;
}

.board-action-menu {
  position: absolute;
  right: 0;
  bottom: 34px;
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
