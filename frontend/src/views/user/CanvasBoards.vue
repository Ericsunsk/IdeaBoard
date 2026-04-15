<template>
  <div class="boards-page">
    <div class="boards-bg"></div>

    <header class="boards-header">
      <div class="brand">
        <span class="brand-name">IdeaBoard</span>
      </div>
      <button class="header-home-btn" type="button" @click="router.push('/user/home')">Home</button>
    </header>

    <main class="boards-main">
      <h1 class="page-title">Welcome to IdeaBoard!</h1>
      <p class="page-subtitle">Explore, expand, and refine your ideas</p>

      <section class="boards-toolbar">
        <button class="create-btn" type="button" @click="createAndOpenBoard">+ Create New Project</button>
        <label class="search-wrap">
          <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="7"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
          </svg>
          <input v-model.trim="searchKeyword" type="text" placeholder="Search projects..." />
        </label>
        <select v-model="sortMode" class="sort-select">
          <option value="recent">Recent</option>
          <option value="name">Name</option>
        </select>
      </section>

      <section class="boards-grid">
        <button class="board-card board-create-card" type="button" @click="createAndOpenBoard">
          <div class="create-preview">
            <div class="create-preview-icon">+</div>
          </div>
          <div class="board-meta board-create-meta">
            <div class="board-title">Create New Canvas</div>
            <div class="board-date">Start from an empty board</div>
          </div>
        </button>

        <article
          v-for="board in visibleBoards"
          :key="board.id"
          class="board-card board-item"
          @click="onBoardCardClick(board)"
        >
          <div class="board-preview">
            <div v-if="boardPreviewMap[board.id]?.length" class="board-preview-canvas">
              <template v-for="item in boardPreviewMap[board.id]" :key="item.key">
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
                v-if="renamingBoardId === board.id"
                :value="renameDraft"
                class="rename-input"
                maxlength="60"
                @click.stop
                @input="onRenameInput"
                @blur="commitRename"
                @keyup.enter.stop.prevent="commitRename"
                @keyup.esc.stop.prevent="cancelRename"
              />
              <div v-else class="board-title">{{ board.name }}</div>
              <button
                class="board-menu-btn"
                type="button"
                title="More actions"
                @click.stop="toggleBoardMenu(board.id)"
              >
                ⋮
              </button>
              <div
                v-if="activeMenuBoardId === board.id"
                class="board-action-menu"
                @click.stop
              >
                <button type="button" class="board-action-item" @click.stop="handleRenameAction(board)">
                  Rename
                </button>
                <button type="button" class="board-action-item" @click.stop="handleDuplicateAction(board)">
                  Duplicate
                </button>
                <button type="button" class="board-action-item danger" @click.stop="handleDeleteAction(board)">
                  Delete
                </button>
              </div>
            </div>
            <div class="board-date">Edited {{ formatDate(board.updatedAt) }}</div>
          </div>
        </article>
      </section>

      <p v-if="visibleBoards.length === 0 && searchKeyword" class="empty-tip">
        No matching canvas
      </p>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import {
  createCanvasBoard,
  deleteCanvasBoard,
  duplicateCanvasBoard,
  listCanvasBoards,
  renameCanvasBoard,
  touchCanvasBoard,
  type CanvasBoardMeta,
} from '../../utils/canvasBoards'
import {
  deleteCanvasBoardState,
  duplicateCanvasBoardState,
  getCanvasBoardState,
} from '../../utils/canvasBoardState'

const router = useRouter()
const message = useMessage()

const boards = ref<CanvasBoardMeta[]>([])
const searchKeyword = ref('')
const sortMode = ref<'recent' | 'name'>('recent')
const renamingBoardId = ref<string | null>(null)
const renameDraft = ref('')
const suppressNextBoardOpen = ref(false)
const activeMenuBoardId = ref<string | null>(null)

type BoardPreviewItem =
  | {
      key: string
      kind: 'image'
      url: string
      left: number
      top: number
      width: number
      height: number
      zIndex: number
    }
  | {
      key: string
      kind: 'node'
      label: string
      text: string
      thumbUrl: string
      left: number
      top: number
      width: number
      height: number
      zIndex: number
    }

interface BoardPreviewFrame {
  key: string
  kind: 'image' | 'node'
  x: number
  y: number
  width: number
  height: number
  url?: string
  label?: string
  text?: string
  thumbUrl?: string
}

const boardPreviewMap = ref<Record<string, BoardPreviewItem[]>>({})

const visibleBoards = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  const filtered = boards.value.filter((board) => {
    if (!keyword) return true
    return board.name.toLowerCase().includes(keyword)
  })
  if (sortMode.value === 'name') {
    return [...filtered].sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))
  }
  return [...filtered].sort((a, b) => b.updatedAt - a.updatedAt)
})

function reloadBoards() {
  boards.value = listCanvasBoards()
  refreshBoardPreviews()
}

function formatDate(ts: number): string {
  if (!Number.isFinite(ts)) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date(ts))
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

function getImageNodePreview(node: Record<string, unknown>): { url: string; ratio: number } | null {
  const rawOutputs = Array.isArray(node.outputImages) ? node.outputImages : []
  for (let i = rawOutputs.length - 1; i >= 0; i -= 1) {
    const output = rawOutputs[i]
    if (!output || typeof output !== 'object') continue
    const record = output as Record<string, unknown>
    if (typeof record.url !== 'string' || !record.url) continue
    const ratio = isFiniteNumber(record.ratio) && record.ratio > 0 ? record.ratio : 1
    return { url: record.url, ratio }
  }
  if (typeof node.sourceImageUrl === 'string' && node.sourceImageUrl) {
    const ratio = isFiniteNumber(node.sourceImageRatio) && node.sourceImageRatio > 0 ? node.sourceImageRatio : 1
    return { url: node.sourceImageUrl, ratio }
  }
  return null
}

function toTitleFromType(rawType: string): string {
  const normalized = rawType
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!normalized) return 'Node'
  return normalized
    .split(' ')
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function buildGenericNodeFrames(state: Record<string, unknown>): BoardPreviewFrame[] {
  const frames: BoardPreviewFrame[] = []
  const ignoredKeys = new Set(['messages', 'viewport', 'updatedAt'])

  Object.entries(state).forEach(([key, value]) => {
    if (ignoredKeys.has(key) || !Array.isArray(value)) return
    value.slice(-10).forEach((nodeLike, index) => {
      if (!nodeLike || typeof nodeLike !== 'object') return
      const node = nodeLike as Record<string, unknown>
      if (!isFiniteNumber(node.x) || !isFiniteNumber(node.y)) return

      const type = typeof node.type === 'string' ? node.type : key
      const label = toTitleFromType(type)
      const width = isFiniteNumber(node.width) && node.width > 0 ? node.width : 280
      const text = typeof node.text === 'string' ? node.text.trim() : ''
      const imagePreview = getImageNodePreview(node)
      const thumbUrl = imagePreview?.url || ''

      let height = 124
      if (thumbUrl) {
        const ratio = imagePreview?.ratio || 1
        const stageWidth = Math.max(120, width - 24)
        const stageHeight = Math.max(110, Math.min(250, stageWidth * ratio))
        height = stageHeight + 92
      } else if (text) {
        height = 138
      }

      const idPart = isFiniteNumber(node.id) ? String(node.id) : `${key}-${index}`
      frames.push({
        key: `node-${key}-${idPart}`,
        kind: 'node',
        x: node.x,
        y: node.y,
        width,
        height,
        label,
        text,
        thumbUrl,
      })
    })
  })

  return frames
}

function buildBoardPreviewImages(boardId: string): BoardPreviewItem[] {
  const state = getCanvasBoardState(boardId)
  if (!state) return []

  const imageFrames: BoardPreviewFrame[] = state.messages
    .filter(item => item.type === 'image' && !!item.url)
    .slice(-12)
    .map((item) => {
      const width = item.width && item.width > 0 ? item.width : 300
      const ratio = item.height_ratio && item.height_ratio > 0 ? item.height_ratio : 1
      return {
        key: `image-${item.id}`,
        kind: 'image' as const,
        x: item.x,
        y: item.y,
        width,
        height: width * ratio,
        url: item.url,
      }
    })

  const nodeFrames = buildGenericNodeFrames(state as unknown as Record<string, unknown>)
  const frames = [...imageFrames, ...nodeFrames]
  if (frames.length === 0) return []

  const minX = Math.min(...frames.map(item => item.x))
  const minY = Math.min(...frames.map(item => item.y))
  const maxX = Math.max(...frames.map(item => item.x + item.width))
  const maxY = Math.max(...frames.map(item => item.y + item.height))
  const boxW = Math.max(1, maxX - minX)
  const boxH = Math.max(1, maxY - minY)

  const padding = 6
  const viewW = 100 - padding * 2
  const viewH = 100 - padding * 2
  const scale = Math.min(viewW / boxW, viewH / boxH)
  const scaledW = boxW * scale
  const scaledH = boxH * scale
  const offsetX = (100 - scaledW) / 2 - minX * scale
  const offsetY = (100 - scaledH) / 2 - minY * scale

  return frames.map((item, index) => {
    const base = {
      key: item.key,
      left: item.x * scale + offsetX,
      top: item.y * scale + offsetY,
      width: item.width * scale,
      height: item.height * scale,
    }
    if (item.kind === 'image') {
      return {
        ...base,
        kind: 'image' as const,
        url: item.url || '',
        zIndex: 10 + index,
      }
    }
    return {
      ...base,
      kind: 'node' as const,
      label: item.label || 'Node',
      text: item.text || '',
      thumbUrl: item.thumbUrl || '',
      zIndex: 310 + index,
    }
  })
}

function refreshBoardPreviews() {
  const nextMap: Record<string, BoardPreviewItem[]> = {}
  boards.value.forEach((board) => {
    nextMap[board.id] = buildBoardPreviewImages(board.id)
  })
  boardPreviewMap.value = nextMap
}

function createAndOpenBoard() {
  activeMenuBoardId.value = null
  const board = createCanvasBoard('Untitled')
  reloadBoards()
  router.push({ path: '/user/picture', query: { boardId: board.id } })
}

function openBoard(board: CanvasBoardMeta) {
  touchCanvasBoard(board.id)
  router.push({ path: '/user/picture', query: { boardId: board.id } })
}

function onBoardCardClick(board: CanvasBoardMeta) {
  if (activeMenuBoardId.value) {
    activeMenuBoardId.value = null
    return
  }
  if (renamingBoardId.value || suppressNextBoardOpen.value) {
    suppressNextBoardOpen.value = false
    return
  }
  openBoard(board)
}

function startRename(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  renamingBoardId.value = board.id
  renameDraft.value = board.name
  void nextTick(() => {
    const el = document.querySelector('.rename-input') as HTMLInputElement | null
    if (!el) return
    el.focus()
    el.select()
  })
}

function onRenameInput(e: Event) {
  renameDraft.value = (e.target as HTMLInputElement).value
}

function commitRename() {
  const boardId = renamingBoardId.value
  if (!boardId) return
  const renamed = renameCanvasBoard(boardId, renameDraft.value)
  renamingBoardId.value = null
  suppressNextBoardOpen.value = true
  if (!renamed) {
    message.warning('画布名称不能为空')
    return
  }
  reloadBoards()
  message.success('画布已重命名')
}

function cancelRename() {
  renamingBoardId.value = null
  suppressNextBoardOpen.value = true
}

function toggleBoardMenu(boardId: string) {
  if (activeMenuBoardId.value === boardId) {
    activeMenuBoardId.value = null
    return
  }
  activeMenuBoardId.value = boardId
}

function handleRenameAction(board: CanvasBoardMeta) {
  startRename(board)
}

function handleDuplicateAction(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  const copy = duplicateCanvasBoard(board.id)
  if (!copy) {
    message.error('复制失败')
    return
  }
  duplicateCanvasBoardState(board.id, copy.id)
  reloadBoards()
  message.success('已复制画布')
}

function handleDeleteAction(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  if (!window.confirm(`确认删除画布「${board.name}」？`)) return
  const ok = deleteCanvasBoard(board.id)
  if (!ok) {
    message.error('删除失败')
    return
  }
  deleteCanvasBoardState(board.id)
  if (renamingBoardId.value === board.id) {
    cancelRename()
  }
  reloadBoards()
  message.success('已删除画布')
}

function handleWindowMouseDown(e: MouseEvent) {
  const target = e.target as HTMLElement | null
  if (target?.closest('.board-menu-btn') || target?.closest('.board-action-menu')) {
    return
  }
  activeMenuBoardId.value = null
}

onMounted(() => {
  reloadBoards()
  window.addEventListener('mousedown', handleWindowMouseDown)
})

onUnmounted(() => {
  window.removeEventListener('mousedown', handleWindowMouseDown)
})
</script>

<style scoped>
.boards-page {
  position: relative;
  width: 100%;
  height: 100%;
  padding: 16px 28px 64px;
  box-sizing: border-box;
  background: #f8f9fd;
  color: #1f2430;
  overflow-y: auto;
  overflow-x: hidden;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: thin;
  scrollbar-color: #c8d0e4 transparent;
}

.boards-page::-webkit-scrollbar {
  width: 10px;
}

.boards-page::-webkit-scrollbar-track {
  background: transparent;
}

.boards-page::-webkit-scrollbar-thumb {
  border-radius: 999px;
  border: 2px solid transparent;
  background: rgba(116, 126, 154, 0.38);
  background-clip: padding-box;
}

.boards-page::-webkit-scrollbar-thumb:hover {
  background: rgba(116, 126, 154, 0.58);
  background-clip: padding-box;
}

.boards-page::-webkit-scrollbar-corner {
  background: transparent;
}

.boards-bg {
  position: fixed;
  inset: 0;
  pointer-events: none;
  background:
    radial-gradient(55% 35% at 45% 45%, rgba(150, 120, 230, 0.24) 0%, rgba(150, 120, 230, 0) 100%),
    radial-gradient(circle, #e2e4e8 1.5px, transparent 1.5px);
  background-size: 100% 100%, 24px 24px;
  background-position: center center, center center;
}

.boards-header {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.brand-name {
  font-size: 32px;
  font-weight: 700;
  letter-spacing: -0.03em;
}

.header-home-btn {
  border: 1px solid #d9dfec;
  background: rgba(255, 255, 255, 0.9);
  color: #3d4253;
  height: 34px;
  min-width: 78px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.boards-main {
  position: relative;
  z-index: 1;
  max-width: 1080px;
  margin: 46px auto 0;
}

.page-title {
  margin: 0;
  text-align: center;
  font-size: 62px;
  font-weight: 700;
  letter-spacing: -0.03em;
  line-height: 1.05;
}

.page-subtitle {
  margin: 16px 0 34px;
  text-align: center;
  color: #6e7487;
  font-size: 19px;
  letter-spacing: 0.01em;
}

.boards-toolbar {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 14px;
  align-items: center;
  margin-bottom: 24px;
}

.create-btn {
  height: 42px;
  border: none;
  border-radius: 999px;
  padding: 0 20px;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0.01em;
  background: #7a6bd5;
  color: #fff;
  cursor: pointer;
}

.create-btn:hover {
  background: #6d5fd0;
}

.search-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  height: 42px;
  border-radius: 999px;
  border: 1px solid #d9dfec;
  background: rgba(255, 255, 255, 0.9);
  padding: 0 12px;
}

.search-icon {
  width: 14px;
  height: 14px;
  color: #8a90a3;
}

.search-wrap input {
  width: 100%;
  border: none;
  outline: none;
  background: transparent;
  font-size: 13px;
  color: #2f3446;
}

.sort-select {
  height: 42px;
  border-radius: 999px;
  border: 1px solid #d9dfec;
  background: rgba(255, 255, 255, 0.9);
  padding: 0 14px;
  font-size: 13px;
  font-weight: 600;
  color: #3e4457;
}

.boards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 14px;
}

.board-card {
  border: 1px solid #d9dfec;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.92);
  overflow: hidden;
  text-align: left;
}

.board-create-card {
  padding: 0;
  cursor: pointer;
}

.create-preview {
  height: 186px;
  background:
    radial-gradient(circle, #dfe4ef 1px, transparent 1px),
    #fbfcff;
  background-size: 10px 10px, cover;
  border-bottom: 1px solid #e7ebf3;
  display: flex;
  align-items: center;
  justify-content: center;
}

.create-preview-icon {
  width: 54px;
  height: 54px;
  border-radius: 14px;
  border: 1px solid #cfd7ea;
  background: #ffffff;
  color: #69708a;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  font-weight: 500;
}

.board-create-meta .board-title {
  color: #394054;
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

.empty-tip {
  margin-top: 18px;
  font-size: 13px;
  color: #7b8297;
}

@media (max-width: 860px) {
  .boards-page {
    padding: 14px 14px 40px;
  }

  .page-title {
    font-size: 40px;
  }

  .page-subtitle {
    font-size: 15px;
  }

  .boards-toolbar {
    grid-template-columns: 1fr;
  }

  .create-btn,
  .sort-select {
    width: 100%;
  }
}
</style>
