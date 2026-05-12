<template>
  <div
    class="boards-page relative box-border h-full w-full [scrollbar-width:thin] [scrollbar-color:#c8d0e4_transparent] overflow-x-hidden overflow-y-auto bg-[#f8f9fd] bg-[radial-gradient(circle,#e2e4e8_1.5px,transparent_1.5px)] bg-[length:24px_24px] [background-attachment:fixed] bg-center px-[28px] pt-[30px] pb-[64px] text-[#1f2430] [-webkit-overflow-scrolling:touch] max-[860px]:px-[14px] max-[860px]:pt-[30px] max-[860px]:pb-[40px]"
  >
    <div
      class="pointer-events-none fixed top-[28%] left-[23%] h-[44vh] w-[54vw] rounded-[9999px] bg-[rgba(150,120,230,0.24)] blur-[120px]"
    ></div>

    <div class="relative z-[1]">
      <header class="flex translate-y-[-8px] items-center justify-between">
        <div class="inline-flex items-center gap-[8px]">
          <span class="text-[32px] [line-height:normal] font-[700] tracking-[-0.03em]"
            >IdeaBoard</span
          >
        </div>
        <button
          class="h-[34px] min-w-[78px] cursor-pointer rounded-[9999px] border border-[#d9dfec] bg-[rgba(255,255,255,0.9)] px-[0px] text-[13px] font-[600] text-[#3d4253]"
          type="button"
          @click="router.push('/user/home')"
        >
          Home
        </button>
      </header>

      <main class="mx-auto mt-[46px] max-w-[1080px]">
        <h1
          class="m-0 text-center text-[62px] leading-[1.05] font-[700] tracking-[-0.03em] max-[860px]:text-[40px]"
        >
          Welcome to IdeaBoard!
        </h1>
        <p
          class="mt-[16px] mb-[34px] translate-y-px text-center text-[19px] tracking-[0.01em] text-[#6e7487] max-[860px]:text-[15px]"
        >
          Explore, expand, and refine your ideas
        </p>

        <section
          class="mb-[24px] grid grid-cols-[auto_1fr_auto] items-center gap-[14px] max-[860px]:grid-cols-1"
        >
          <button
            class="h-[42px] cursor-pointer rounded-[9999px] border-none bg-[#7a6bd5] px-[20px] text-[14px] font-[700] tracking-[0.01em] text-[#ffffff] hover:bg-[#6d5fd0] max-[860px]:w-full"
            type="button"
            @click="createAndOpenBoard"
          >
            + Create New Project
          </button>
          <label
            class="flex h-[42px] items-center gap-[8px] rounded-[9999px] border border-[#d9dfec] bg-[rgba(255,255,255,0.9)] px-[12px]"
          >
            <svg
              class="h-[14px] w-[14px] [flex-shrink:0] text-[#8a90a3]"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            >
              <circle cx="11" cy="11" r="7"></circle>
              <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
            </svg>
            <input
              v-model.trim="searchKeyword"
              class="w-full border-none bg-transparent text-[13px] text-[#2f3446] outline-none"
              type="text"
              placeholder="Search projects..."
            />
          </label>
          <div class="relative max-[860px]:w-full">
            <select
              v-model="sortMode"
              class="h-[42px] min-w-[96px] appearance-none rounded-[9999px] border border-[#d9dfec] bg-[rgba(255,255,255,0.9)] px-[14px] text-center text-[13px] font-[600] text-[#3e4457] max-[860px]:w-full"
            >
              <option value="recent">Recent</option>
              <option value="name">Name</option>
            </select>
          </div>
        </section>

        <section class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-[14px]">
          <button
            class="h-[244px] cursor-pointer overflow-hidden rounded-[12px] border border-[#d9dfec] bg-[rgba(255,255,255,0.92)] p-0 text-left"
            type="button"
            @click="createAndOpenBoard"
          >
            <div
              class="flex h-[186px] items-center justify-center border-b border-[#e7ebf3] bg-[radial-gradient(circle,#dfe4ef_1px,transparent_1px),#fbfcff] bg-[length:10px_10px,cover]"
            >
              <div
                class="flex h-[54px] w-[54px] items-center justify-center rounded-[14px] border border-[#cfd7ea] bg-[#ffffff] text-[28px] font-medium text-[#69708a]"
              >
                +
              </div>
            </div>
            <div class="px-[12px] pt-[10px] pb-[12px]">
              <div class="text-[14px] font-[700] text-[#394054]">Create New Canvas</div>
              <div class="mt-[3px] text-[12px] text-[#7b8297]">Start from an empty board</div>
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

        <p
          v-if="visibleBoards.length === 0 && searchKeyword"
          class="mt-[18px] text-[13px] text-[#7b8297]"
        >
          No matching canvas
        </p>
      </main>
    </div>
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
</style>
