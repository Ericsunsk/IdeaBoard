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
        <button class="create-btn" type="button" @click="createAndOpenBoard">
          + Create New Project
        </button>
        <label class="search-wrap">
          <svg
            class="search-icon"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
          >
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

        <BoardCard
          v-for="board in visibleBoards"
          :key="board.id"
          :board="board"
          :preview-snapshot-data-url="boardPreviewMap[board.id] || ''"
          :is-renaming="renamingBoardId === board.id"
          :rename-draft="renameDraft"
          :is-menu-open="activeMenuBoardId === board.id"
          :display-date="formatDate(board.updatedAt)"
          @click-board="onBoardCardClick(board)"
          @toggle-menu="toggleBoardMenu(board.id)"
          @rename-action="handleRenameAction(board)"
          @duplicate-action="handleDuplicateAction(board)"
          @delete-action="handleDeleteAction(board)"
          @rename-input="onRenameInput"
          @commit-rename="commitRename"
          @cancel-rename="cancelRename"
        />
      </section>

      <p v-if="visibleBoards.length === 0 && searchKeyword" class="empty-tip">No matching canvas</p>
    </main>
  </div>
</template>

<script setup lang="ts">
import {
  createCanvasBoard,
  deleteCanvasBoard,
  duplicateCanvasBoard,
  isCanvasBoardNameTaken,
  listCanvasBoards,
  renameCanvasBoard,
  touchCanvasBoard,
  type CanvasBoardMeta,
} from '../lib/canvasBoards'
import {
  deleteCanvasBoardState,
  duplicateCanvasBoardState,
  getCanvasBoardState,
  setCanvasBoardPreviewSnapshot,
} from '../lib/canvasBoardState'
import { renderBoardPreviewDataUrl } from '../lib/boardPreviewSerializer'
import {
  hydrateCanvasBoardsFromServer,
  isBoardNameConflictError,
  syncCanvasBoardToServer,
  syncDeleteCanvasBoardToServer,
} from '../lib/boardSync'

const router = useRouter()
const message = useMessage()

const boards = ref<CanvasBoardMeta[]>([])
const searchKeyword = ref('')
const sortMode = ref<'recent' | 'name'>('recent')
const renamingBoardId = ref<string | null>(null)
const renameDraft = ref('')
const suppressNextBoardOpen = ref(false)
const activeMenuBoardId = ref<string | null>(null)

const boardPreviewMap = ref<Record<string, string>>({})
const previewRefreshQueue: string[] = []
const queuedPreviewBoardIds = new Set<string>()
let previewRefreshInFlight = false

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
  scheduleBoardPreviewRefresh()
}

function formatDate(ts: number): string {
  if (!Number.isFinite(ts)) return '-'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(new Date(ts))
}

function refreshBoardPreviews() {
  const nextMap: Record<string, string> = {}
  boards.value.forEach((board) => {
    nextMap[board.id] = getCanvasBoardState(board.id)?.previewSnapshotDataUrl || ''
  })
  boardPreviewMap.value = nextMap
}

function isBoardPreviewStale(boardId: string): boolean {
  const state = getCanvasBoardState(boardId)
  if (!state) return false
  if (!state.previewSnapshotDataUrl) return true
  if (!Number.isFinite(state.previewSnapshotUpdatedAt)) return true
  return (state.previewSnapshotUpdatedAt || 0) < state.updatedAt
}

function enqueueBoardPreviewRefresh(boardId: string) {
  if (!boardId || queuedPreviewBoardIds.has(boardId)) return
  queuedPreviewBoardIds.add(boardId)
  previewRefreshQueue.push(boardId)
}

async function refreshBoardPreviewInBackground(boardId: string) {
  const startState = getCanvasBoardState(boardId)
  if (!startState || !isBoardPreviewStale(boardId)) return
  const baseUpdatedAt = startState.updatedAt
  let snapshotDataUrl: string | undefined
  let snapshotUpdatedAt: number | undefined
  try {
    const renderedDataUrl = await renderBoardPreviewDataUrl(startState)
    snapshotDataUrl = renderedDataUrl || undefined
    snapshotUpdatedAt = renderedDataUrl ? Date.now() : undefined
  } catch (error) {
    console.warn('[boards] failed to render board preview', boardId, error)
    return
  }
  const latestState = getCanvasBoardState(boardId)
  if (!latestState) return
  if (latestState.updatedAt !== baseUpdatedAt) {
    enqueueBoardPreviewRefresh(boardId)
    return
  }
  const updatedState = setCanvasBoardPreviewSnapshot(boardId, snapshotDataUrl, snapshotUpdatedAt)
  if (!updatedState) return
  boardPreviewMap.value = {
    ...boardPreviewMap.value,
    [boardId]: updatedState.previewSnapshotDataUrl || '',
  }
  void syncCanvasBoardToServer(boardId).catch((error) => {
    if (isBoardNameConflictError(error)) return
    console.warn('[boards] failed to sync refreshed preview', boardId, error)
  })
}

async function runBoardPreviewRefreshQueue() {
  if (previewRefreshInFlight) return
  previewRefreshInFlight = true
  try {
    while (previewRefreshQueue.length > 0) {
      const boardId = previewRefreshQueue.shift()
      if (!boardId) continue
      queuedPreviewBoardIds.delete(boardId)
      if (!isBoardPreviewStale(boardId)) continue
      await refreshBoardPreviewInBackground(boardId)
    }
  } finally {
    previewRefreshInFlight = false
    if (previewRefreshQueue.length > 0) {
      void runBoardPreviewRefreshQueue()
    }
  }
}

function scheduleBoardPreviewRefresh() {
  boards.value.forEach((board) => {
    if (isBoardPreviewStale(board.id)) {
      enqueueBoardPreviewRefresh(board.id)
    }
  })
  void runBoardPreviewRefreshQueue()
}

async function createAndOpenBoard() {
  activeMenuBoardId.value = null
  const board = createCanvasBoard('Untitled')
  try {
    await syncCanvasBoardToServer(board.id)
  } catch (error) {
    if (isBoardNameConflictError(error)) {
      await hydrateCanvasBoardsFromServer()
      reloadBoards()
      message.error('画布名称已存在，请重试')
      return
    }
    console.warn('[boards] failed to sync new board', error)
  }
  reloadBoards()
  router.push({ path: '/user/workspace', query: { boardId: board.id } })
}

function openBoard(board: CanvasBoardMeta) {
  touchCanvasBoard(board.id)
  void syncCanvasBoardToServer(board.id).catch((error) => {
    if (isBoardNameConflictError(error)) {
      message.error('当前画布名称与其他画布重复，请先重命名')
    }
    console.warn('[boards] failed to sync board open touch', error)
  })
  router.push({ path: '/user/workspace', query: { boardId: board.id } })
}

function isNotFoundError(error: unknown): boolean {
  if (typeof error !== 'object' || !error) return false
  const status = 'status' in error ? (error as { status?: unknown }).status : undefined
  if (status === 404) return true
  const code = 'code' in error ? (error as { code?: unknown }).code : undefined
  if (code === 404) return true
  const msg = 'msg' in error ? (error as { msg?: unknown }).msg : undefined
  if (
    typeof msg === 'string' &&
    (msg.includes('画布不存在') || msg.toLowerCase().includes('not found'))
  ) {
    return true
  }
  return false
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
}

function onRenameInput(nextName: string) {
  renameDraft.value = nextName
}

async function commitRename() {
  const boardId = renamingBoardId.value
  if (!boardId) return
  if (isCanvasBoardNameTaken(renameDraft.value, boardId)) {
    message.warning('名称已存在')
    return
  }
  const renamed = renameCanvasBoard(boardId, renameDraft.value)
  renamingBoardId.value = null
  suppressNextBoardOpen.value = true
  if (!renamed) {
    message.warning('画布名称不能为空')
    return
  }
  try {
    await syncCanvasBoardToServer(boardId)
  } catch (error) {
    if (isBoardNameConflictError(error)) {
      await hydrateCanvasBoardsFromServer()
      reloadBoards()
      message.error('名称已存在，请更换后再试')
      return
    }
    console.warn('[boards] failed to sync renamed board', error)
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

async function handleDuplicateAction(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  const copy = duplicateCanvasBoard(board.id)
  if (!copy) {
    message.error('复制失败')
    return
  }
  duplicateCanvasBoardState(board.id, copy.id)
  try {
    await syncCanvasBoardToServer(copy.id)
  } catch (error) {
    if (isBoardNameConflictError(error)) {
      await hydrateCanvasBoardsFromServer()
      reloadBoards()
      message.error('复制后的画布名称已存在，请重试')
      return
    }
    console.warn('[boards] failed to sync duplicated board', error)
  }
  reloadBoards()
  message.success('已复制画布')
}

async function handleDeleteAction(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  if (!window.confirm(`确认删除画布「${board.name}」？`)) return
  try {
    await syncDeleteCanvasBoardToServer(board.id)
  } catch (error) {
    if (isNotFoundError(error)) {
      // remote already deleted, continue cleaning local state
    } else {
      message.error('删除失败，请稍后重试')
      console.warn('[boards] failed to sync deleted board', error)
      return
    }
  }
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

onMounted(async () => {
  await hydrateCanvasBoardsFromServer()
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
  background-size:
    100% 100%,
    24px 24px;
  background-position:
    center center,
    center center;
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
  height: 244px;
  text-align: left;
}

.board-create-card {
  padding: 0;
  cursor: pointer;
}

.create-preview {
  height: 186px;
  background: radial-gradient(circle, #dfe4ef 1px, transparent 1px), #fbfcff;
  background-size:
    10px 10px,
    cover;
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

.board-meta {
  padding: 10px 12px 12px;
}

.board-title {
  font-size: 14px;
  font-weight: 700;
  color: #2b3142;
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
