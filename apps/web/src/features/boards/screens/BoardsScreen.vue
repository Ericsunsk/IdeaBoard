<template>
  <div :class="pageClass">
    <div :class="pageBackgroundClass"></div>

    <header :class="headerClass">
      <div :class="brandClass">
        <span :class="brandNameClass">IdeaBoard</span>
      </div>
      <div :class="userMenuWrapClass" @pointerdown.stop @mousedown.stop @click.stop>
        <Button
          unstyled
          type="button"
          title="账户菜单"
          aria-haspopup="menu"
          :aria-expanded="isUserMenuOpen"
          :class="userAvatarButtonClass"
          @click="toggleUserMenu"
        >
          <img
            v-if="userStore.user?.avatar"
            :src="userStore.user.avatar"
            :class="userAvatarClass"
          />
          <div v-else :class="userAvatarPlaceholderClass">
            <User :class="avatarIconClass" />
          </div>
        </Button>
        <Menu
          ref="userMenuRef"
          :model="userMenuItems"
          popup
          append-to="body"
          :pt="userMenuPt"
          @show="handleUserMenuShow"
          @hide="handleUserMenuHide"
        >
          <template #item="{ item, props }">
            <a
              v-bind="props.action"
              :class="[userMenuItemClass, item.key === 'logout' ? userMenuDangerItemClass : '']"
            >
              <span>{{ item.label }}</span>
            </a>
          </template>
        </Menu>
      </div>
    </header>

    <main :class="mainClass">
      <h1 :class="pageTitleClass">Welcome to IdeaBoard!</h1>
      <p :class="pageSubtitleClass">Explore, expand, and refine your ideas</p>

      <section :class="toolbarClass">
        <Button
          type="button"
          label="New Canvas"
          :class="createButtonClass"
          @click="createAndOpenBoard"
        />
        <label :class="searchWrapClass">
          <Search :class="searchIconClass" />
          <InputText
            :model-value="searchKeyword"
            unstyled
            type="text"
            placeholder="Search canvases..."
            :class="searchInputClass"
            @update:model-value="onSearchInput"
          />
        </label>
        <Select
          v-model="sortMode"
          :options="sortOptions"
          option-label="label"
          option-value="value"
          :class="sortSelectClass"
          :pt="sortSelectPt"
          aria-label="Sort canvases"
        />
      </section>

      <section v-if="isBoardsLoading" :class="loadingStateClass" aria-live="polite">
        <ProgressSpinner class="!h-9 !w-9 text-[#7a6bd5]" stroke-width="4" />
      </section>

      <section v-else :class="boardsGridClass">
        <Button unstyled type="button" :class="createCardClass" @click="createAndOpenBoard">
          <div :class="createPreviewClass">
            <div :class="createPreviewIconClass">+</div>
          </div>
          <div :class="createMetaClass">
            <div :class="createTitleClass">Create New Canvas</div>
            <div :class="createDateClass">Start from an empty board</div>
          </div>
        </Button>

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
          @menu-hidden="handleBoardMenuHidden(board.id)"
        />
      </section>

      <p
        v-if="!isBoardsLoading && visibleBoards.length === 0 && searchKeyword"
        :class="emptyTipClass"
      >
        No matching canvas
      </p>
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
} from '../../canvas/lib/canvasBoardState'
import { blankBoardPreviewSurfaceClass } from '../../canvas/lib/boardPreviewStyle'
import { renderBoardPreviewDataUrl } from '../../canvas/lib/boardPreviewSerializer'
import { BOARD_PREVIEW_RENDERER_VERSION } from '../../canvas/lib/boardPreviewVersion'
import {
  hydrateCanvasBoardsFromServer,
  isBoardNameConflictError,
  syncCanvasBoardToServer,
  syncDeleteCanvasBoardToServer,
} from '../lib/boardSync'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useUserStore } from '../../../app/stores/user'
import { Search, User } from '@iconoir/vue'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import type { MenuItem } from 'primevue/menuitem'
import { userMenuPt, type PrimePopupMenuRef } from '../lib/menuStyles'

const router = useRouter()
const appToast = useAppToast()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect()

const boards = ref<CanvasBoardMeta[]>([])
const isBoardsLoading = ref(true)
const searchKeyword = ref('')
const sortMode = ref<'recent' | 'name'>('recent')
const renamingBoardId = ref<string | null>(null)
const renameDraft = ref('')
const suppressNextBoardOpen = ref(false)
const activeMenuBoardId = ref<string | null>(null)
const isUserMenuOpen = ref(false)
const userMenuRef = ref<PrimePopupMenuRef | null>(null)

const boardPreviewMap = ref<Record<string, string>>({})
const previewRefreshQueue: string[] = []
const queuedPreviewBoardIds = new Set<string>()
let previewRefreshInFlight = false

const sortOptions = [
  { label: 'Recent', value: 'recent' },
  { label: 'Name', value: 'name' },
]

const pageClass =
  'relative h-full w-full overflow-y-auto overflow-x-hidden bg-[#f8f9fd] px-7 pt-4 pb-16 text-[#1f2430] [box-sizing:border-box] [-webkit-overflow-scrolling:touch] [scrollbar-color:#c8d0e4_transparent] [scrollbar-width:thin] max-[860px]:px-3.5 max-[860px]:pt-3.5 max-[860px]:pb-10'
const pageBackgroundClass =
  'pointer-events-none fixed inset-0 bg-[radial-gradient(55%_35%_at_45%_45%,rgba(150,120,230,0.24)_0%,rgba(150,120,230,0)_100%),radial-gradient(circle,#e2e4e8_1.5px,transparent_1.5px)] [background-position:center_center,center_center] [background-size:100%_100%,24px_24px]'
const headerClass = 'relative z-[1] flex items-center justify-between'
const brandClass = 'inline-flex items-center gap-2'
const brandNameClass = 'text-[32px] font-bold tracking-normal'
const userMenuWrapClass = 'relative'
const userAvatarButtonClass = 'ib-user-avatar-button'
const userAvatarClass = 'ib-user-avatar-button__image'
const userAvatarPlaceholderClass = 'ib-user-avatar-button__placeholder'
const avatarIconClass = 'size-5 stroke-2'
const userMenuItemClass =
  'flex w-full items-center rounded-lg px-2.5 py-2 text-[13px] font-semibold text-[#4b5565] no-underline transition-colors hover:bg-[rgba(146,120,230,0.12)]'
const userMenuDangerItemClass = '!text-[#d14343] hover:!bg-[rgba(209,67,67,0.1)]'
const mainClass = 'relative z-[1] mx-auto mt-[46px] max-w-[1080px]'
const pageTitleClass =
  'm-0 text-center text-[62px] leading-[1.05] font-bold tracking-normal max-[860px]:text-[40px]'
const pageSubtitleClass =
  'mt-4 mb-[34px] text-center text-[19px] tracking-normal text-[#6e7487] max-[860px]:text-[15px]'
const toolbarClass =
  'mb-6 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3.5 max-[860px]:grid-cols-1'
const createButtonClass =
  '!h-[42px] !rounded-full !border-0 !bg-[#7a6bd5] !px-5 !text-sm !font-bold !tracking-normal !text-white !shadow-none hover:!bg-[#6d5fd0] focus-visible:!shadow-[0_0_0_3px_rgba(122,107,213,0.18)] max-[860px]:!w-full'
const searchWrapClass =
  'flex h-[42px] min-w-0 items-center gap-2 rounded-full border border-[#d9dfec] bg-white/90 px-3'
const searchIconClass = 'size-3.5 shrink-0 text-[#8a90a3] stroke-2'
const searchInputClass = 'min-w-0 flex-1 bg-transparent text-[13px] text-[#2f3446] outline-none'
const sortSelectClass =
  '!h-[42px] !min-w-[120px] !rounded-full !border-[#d9dfec] !bg-white/90 !text-[13px] !font-semibold !text-[#3e4457] !shadow-none max-[860px]:!w-full'
const boardsGridClass = 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3.5'
const loadingStateClass =
  'flex min-h-[244px] items-center justify-center rounded-xl border border-dashed border-[#d9dfec] bg-white/60'
const createCardClass =
  'relative h-[244px] cursor-pointer overflow-hidden rounded-xl border border-[#d9dfec] bg-white/90 p-0 text-left transition-colors hover:border-[#cdd4e6] focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_rgba(122,107,213,0.16)]'
const createPreviewClass = `flex h-[186px] items-center justify-center border-b border-[#e7ebf3] ${blankBoardPreviewSurfaceClass}`
const createPreviewIconClass =
  'flex size-[54px] items-center justify-center rounded-[14px] border border-[#cfd7ea] bg-white text-[28px] font-medium text-[#69708a]'
const createMetaClass = 'px-3 pt-2.5 pb-3'
const createTitleClass = 'text-sm font-bold text-[#394054]'
const createDateClass = 'mt-[3px] text-xs text-[#7b8297]'
const emptyTipClass = 'mt-[18px] text-[13px] text-[#7b8297]'
const sortSelectPt = {
  label: { class: '!py-0 !pl-3.5 !pr-1 !leading-[40px]' },
  dropdown: { class: '!w-9' },
  overlay: {
    class:
      '!rounded-xl !border !border-[#d8ddea] !bg-white !shadow-[0_18px_42px_rgba(45,64,89,0.14)]',
  },
  list: { class: '!p-1.5' },
  option: { class: '!rounded-lg !px-3 !py-2 !text-[13px] !font-semibold' },
} as const

const userMenuItems = computed<MenuItem[]>(() => [
  {
    key: 'center',
    label: '个人中心',
    command: () => {
      void openCenter()
    },
  },
  {
    key: 'home',
    label: '返回首页',
    command: () => {
      void goHome()
    },
  },
  {
    key: 'logout',
    label: '退出登录',
    command: () => {
      void logout()
    },
  },
])

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

function onSearchInput(value: string | undefined) {
  searchKeyword.value = (value || '').trim()
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
  if (state.previewSnapshotVersion !== BOARD_PREVIEW_RENDERER_VERSION) return true
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
  const updatedState = setCanvasBoardPreviewSnapshot(
    boardId,
    snapshotDataUrl,
    snapshotUpdatedAt,
    snapshotDataUrl ? BOARD_PREVIEW_RENDERER_VERSION : undefined,
  )
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
      deleteCanvasBoard(board.id)
      deleteCanvasBoardState(board.id)
      await hydrateCanvasBoardsFromServer()
      reloadBoards()
      appToast.error('画布名称已存在，请重试')
      return
    }
    console.warn('[boards] failed to sync new board', error)
    deleteCanvasBoard(board.id)
    reloadBoards()
    appToast.error('创建同步失败，请稍后重试')
    return
  }
  reloadBoards()
  router.push({ path: '/user/workspace', query: { boardId: board.id } })
}

function openBoard(board: CanvasBoardMeta) {
  touchCanvasBoard(board.id)
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
    appToast.warning('名称已存在')
    return
  }
  const renamed = renameCanvasBoard(boardId, renameDraft.value)
  renamingBoardId.value = null
  suppressNextBoardOpen.value = true
  if (!renamed) {
    appToast.warning('画布名称不能为空')
    return
  }
  try {
    await syncCanvasBoardToServer(boardId)
  } catch (error) {
    await hydrateCanvasBoardsFromServer(
      isBoardNameConflictError(error)
        ? { preserveLocalNewer: false, rollbackBoardIds: [boardId] }
        : undefined,
    )
    reloadBoards()
    appToast.error(
      isBoardNameConflictError(error) ? '名称已存在，请更换后再试' : '重命名同步失败，请稍后重试',
    )
    console.warn('[boards] failed to sync renamed board', error)
    return
  }
  reloadBoards()
  appToast.success('画布已重命名')
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

function handleBoardMenuHidden(boardId: string) {
  if (activeMenuBoardId.value === boardId) {
    activeMenuBoardId.value = null
  }
}

function handleRenameAction(board: CanvasBoardMeta) {
  startRename(board)
}

async function handleDuplicateAction(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  const copy = duplicateCanvasBoard(board.id)
  if (!copy) {
    appToast.error('复制失败')
    return
  }
  duplicateCanvasBoardState(board.id, copy.id)
  try {
    await syncCanvasBoardToServer(copy.id)
  } catch (error) {
    if (isBoardNameConflictError(error)) {
      deleteCanvasBoard(copy.id)
      deleteCanvasBoardState(copy.id)
      await hydrateCanvasBoardsFromServer()
      reloadBoards()
      appToast.error('复制后的画布名称已存在，请重试')
      return
    }
    console.warn('[boards] failed to sync duplicated board', error)
    deleteCanvasBoard(copy.id)
    deleteCanvasBoardState(copy.id)
    reloadBoards()
    appToast.error('复制同步失败，请稍后重试')
    return
  }
  reloadBoards()
  appToast.success('已复制画布')
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
      appToast.error('删除失败，请稍后重试')
      console.warn('[boards] failed to sync deleted board', error)
      return
    }
  }
  const ok = deleteCanvasBoard(board.id)
  if (!ok) {
    appToast.error('删除失败')
    return
  }
  deleteCanvasBoardState(board.id)
  if (renamingBoardId.value === board.id) {
    cancelRename()
  }
  reloadBoards()
  appToast.success('已删除画布')
}

function handleUserMenuShow() {
  isUserMenuOpen.value = true
}

function handleUserMenuHide() {
  isUserMenuOpen.value = false
}

function closeUserMenu() {
  isUserMenuOpen.value = false
  userMenuRef.value?.hide()
}

function toggleUserMenu(event: Event) {
  userMenuRef.value?.toggle(event)
}

function openCenter() {
  closeUserMenu()
  router.push('/user/center')
}

function goHome() {
  closeUserMenu()
  router.push('/user/home')
}

async function logout() {
  closeUserMenu()
  await logoutAndRedirect('/user/home')
}

onMounted(async () => {
  try {
    await hydrateCanvasBoardsFromServer()
    reloadBoards()
  } finally {
    isBoardsLoading.value = false
  }
})
</script>
