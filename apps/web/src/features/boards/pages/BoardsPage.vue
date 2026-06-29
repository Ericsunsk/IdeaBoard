<template>
  <div
    :class="[pageClass, { 'is-scrolling': isBoardsPageScrolling }]"
    @scroll.passive="handleBoardsPageScroll"
  >
    <div :class="pageBackgroundClass"></div>

    <header :class="headerClass">
      <div :class="brandClass">
        <span :class="brandNameClass">{{ projectName }}</span>
      </div>
      <div :class="userMenuWrapClass" @pointerdown.stop @mousedown.stop @click.stop>
        <Button
          unstyled
          type="button"
          title="账户菜单"
          aria-label="账户菜单"
          aria-haspopup="menu"
          :aria-expanded="isUserMenuOpen"
          :class="userAvatarButtonClass"
          @click="toggleUserMenu"
        >
          <img
            v-if="userStore.user?.avatar"
            :src="userStore.user.avatar"
            :class="userAvatarClass"
            alt="用户头像"
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
              :class="[popupMenuItemClass, item.key === 'logout' ? popupMenuDangerItemClass : '']"
            >
              <span>{{ item.label }}</span>
            </a>
          </template>
        </Menu>
      </div>
    </header>

    <main :class="mainClass">
      <h1 :class="pageTitleClass">Welcome to {{ projectName }}!</h1>
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
            <div :class="createPreviewIconClass" aria-hidden="true"></div>
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

    <Dialog
      v-model:visible="deleteDialogVisible"
      modal
      :dismissable-mask="!isDeleteBoardPending"
      :closable="!isDeleteBoardPending"
      :draggable="false"
      :show-header="false"
      :style="{ width: '360px', maxWidth: 'calc(100vw - 28px)' }"
      :pt="deleteDialogPt"
      aria-labelledby="board-delete-dialog-title"
      @hide="handleDeleteDialogHide"
    >
      <div :class="deleteDialogShellClass">
        <h2 id="board-delete-dialog-title" :class="deleteDialogTitleClass">删除画布</h2>
        <p :class="deleteDialogDescriptionClass">
          确认删除「{{ pendingDeleteBoard?.name || 'Untitled' }}」？此操作不可撤销。
        </p>

        <div :class="deleteDialogFooterClass">
          <Button
            unstyled
            type="button"
            label="取消"
            :disabled="isDeleteBoardPending"
            :class="deleteCancelButtonClass"
            @click="closeDeleteDialog"
          />
          <Button
            unstyled
            type="button"
            :label="isDeleteBoardPending ? '删除中...' : '删除'"
            :disabled="isDeleteBoardPending"
            :class="deleteConfirmButtonClass"
            @click="confirmDeleteBoard"
          />
        </div>
      </div>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import {
  listCanvasBoards,
  touchCanvasBoard,
  type CanvasBoardMeta,
} from '../../canvas-board/catalog/canvasBoards'
import { blankBoardPreviewSurfaceClass } from '../../canvas-board/preview'
import { hydrateCanvasBoardsFromServer } from '../../canvas-board/catalog/boardSync'
import { createCanvasBoardCommands } from '../lib/canvasBoardCommands'
import { createBoardPreviewRefreshQueue } from '../lib/boardPreviewRefreshQueue'
import { useAppToast } from '../../../shared/composables/useAppToast'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import {
  USER_CENTER_PATH,
  USER_HOME_PATH,
  USER_WORKSPACE_PATH,
} from '../../../shared/constants/routePaths'
import { useUserStore } from '../../../app/stores/user'
import { Search, User } from '@iconoir/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import type { MenuItem } from 'primevue/menuitem'
import {
  popupMenuDangerItemClass,
  popupMenuItemClass,
  userMenuPt,
  type PrimePopupMenuRef,
} from '../lib/menuStyles'

const router = useRouter()
const appToast = useAppToast()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect()
const { projectName } = useSiteBranding()
const boardCommands = createCanvasBoardCommands()
const boardPreviewRefresh = createBoardPreviewRefreshQueue()

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
const deleteDialogVisible = ref(false)
const pendingDeleteBoard = ref<CanvasBoardMeta | null>(null)
const isDeleteBoardPending = ref(false)
const isBoardsPageScrolling = ref(false)
const boardPreviewMap = boardPreviewRefresh.previewMap
let boardsPageScrollTimer: number | undefined

const sortOptions = [
  { label: 'Recent', value: 'recent' },
  { label: 'Name', value: 'name' },
]

const pageClass =
  'app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden overscroll-contain bg-[#f8f9fd] px-7 pt-4 pb-16 text-[#1f2430] [box-sizing:border-box] [-webkit-overflow-scrolling:touch] max-[860px]:px-3.5 max-[860px]:pt-3.5 max-[860px]:pb-10'
const pageBackgroundClass =
  'pointer-events-none fixed inset-0 bg-[radial-gradient(55%_35%_at_45%_45%,rgba(150,120,230,0.24)_0%,rgba(150,120,230,0)_100%),radial-gradient(circle,#e2e4e8_1px,transparent_1px)] [background-position:center_center,center_center] [background-size:100%_100%,12px_12px]'
const headerClass = 'relative z-[1] flex items-center justify-between'
const brandClass = 'inline-flex min-w-0 max-w-[calc(100%-64px)] items-center'
const brandNameClass = 'min-w-0 truncate text-[32px] font-bold tracking-normal'
const userMenuWrapClass = 'relative'
const userAvatarButtonClass = 'ib-user-avatar-button'
const userAvatarClass = 'ib-user-avatar-button__image'
const userAvatarPlaceholderClass = 'ib-user-avatar-button__placeholder'
const avatarIconClass = 'size-5'
const mainClass = 'relative z-[1] mx-auto mt-[46px] max-w-[1080px]'
const pageTitleClass =
  'm-0 break-words text-center text-[62px] leading-[1.05] font-bold tracking-normal max-[860px]:text-[40px]'
const pageSubtitleClass =
  'mt-4 mb-[34px] text-center text-[19px] tracking-normal text-[#6e7487] max-[860px]:text-[15px]'
const toolbarClass =
  'mb-6 grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3.5 max-[860px]:grid-cols-1'
const createButtonClass =
  '!h-[42px] !rounded-full !border-0 !bg-[#7a6bd5] !px-5 !text-sm !font-bold !tracking-normal !text-white !shadow-none hover:!bg-[#6d5fd0] focus-visible:!shadow-[0_0_0_3px_rgba(122,107,213,0.18)] max-[860px]:!w-full'
const searchWrapClass =
  'flex h-[42px] min-w-0 items-center gap-2 rounded-full border border-[#d9dfec] bg-white/90 px-3'
const searchIconClass = 'size-3.5 shrink-0 text-[#8a90a3]'
const searchInputClass = 'min-w-0 flex-1 bg-transparent text-[13px] text-[#2f3446] outline-none'
const sortSelectClass =
  '!inline-flex !h-[42px] !w-[108px] !min-w-[108px] !items-center !justify-center !gap-2 !rounded-full !border-[#d9dfec] !bg-white/90 !text-[13px] !font-semibold !text-[#3e4457] !shadow-none max-[860px]:!w-full'
const boardsGridClass = 'grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3.5'
const loadingStateClass =
  'flex min-h-[244px] items-center justify-center rounded-xl border border-dashed border-[#d9dfec] bg-white/60'
const createCardClass =
  'relative h-[244px] cursor-pointer overflow-hidden rounded-xl border border-[#d9dfec] bg-white/90 p-0 text-left transition-colors hover:border-[#cdd4e6] focus-visible:outline-none focus-visible:shadow-[0_0_0_3px_rgba(122,107,213,0.16)]'
const createPreviewClass = `flex h-[186px] items-center justify-center border-b border-[#e7ebf3] ${blankBoardPreviewSurfaceClass}`
const createPreviewIconClass =
  "relative flex size-[54px] items-center justify-center rounded-[14px] border border-[#cfd7ea] bg-white before:absolute before:top-1/2 before:left-1/2 before:h-[25px] before:w-[2.5px] before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-full before:bg-[#cfd7ea] before:content-[''] after:absolute after:top-1/2 after:left-1/2 after:h-[2.5px] after:w-[25px] after:-translate-x-1/2 after:-translate-y-1/2 after:rounded-full after:bg-[#cfd7ea] after:content-['']"
const createMetaClass = 'px-3 pt-2.5 pb-3'
const createTitleClass = 'text-sm font-bold text-[#394054]'
const createDateClass = 'mt-[3px] text-xs text-[#7b8297]'
const emptyTipClass = 'mt-[18px] text-[13px] text-[#7b8297]'
const sortSelectPt = {
  label: { class: '!w-auto !flex-none !py-0 !pr-0 !pl-0 !leading-[40px]' },
  dropdown: { class: '!w-auto !flex-none' },
  overlay: {
    class:
      '!rounded-xl !border !border-[#d8ddea] !bg-white !shadow-[0_18px_42px_rgba(45,64,89,0.14)]',
  },
  list: { class: '!p-1.5' },
  option: { class: '!rounded-lg !px-3 !py-2 !text-[13px] !font-semibold' },
} as const
const deleteDialogPt = {
  mask: { class: '!bg-[rgba(31,36,48,0.34)]' },
  root: {
    class:
      '!overflow-hidden !rounded-[20px] !border !border-[#d8ddea] !bg-white !shadow-[0_22px_58px_rgba(30,39,60,0.18)]',
  },
  content: { class: '!bg-transparent !p-0' },
} as const
const deleteDialogShellClass = 'p-5'
const deleteDialogTitleClass =
  'm-0 text-[18px] leading-[1.2] font-bold tracking-normal text-[#2f3446]'
const deleteDialogDescriptionClass = 'mt-3 mb-0 text-[13px] leading-6 text-[#6e7487]'
const deleteDialogFooterClass = 'mt-6 flex justify-end gap-2'
const deleteCancelButtonClass =
  'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-transparent px-3.5 text-sm font-bold text-[#6e7487] transition-colors hover:bg-[#f1f4fb] disabled:cursor-not-allowed disabled:opacity-60'
const deleteConfirmButtonClass =
  'inline-flex h-9 min-w-[72px] cursor-pointer items-center justify-center rounded-full border-0 bg-[#d14343] px-3.5 text-sm font-bold text-white transition-colors hover:bg-[#bd333a] disabled:cursor-not-allowed disabled:opacity-70'

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
  boardPreviewRefresh.reloadFromLocal(boards.value)
  boardPreviewRefresh.schedule(boards.value)
}

function formatDate(ts: number): string {
  if (!Number.isFinite(ts)) return '-'
  const date = new Date(ts)
  const year = date.getFullYear()
  const month = `${date.getMonth() + 1}`.padStart(2, '0')
  const day = `${date.getDate()}`.padStart(2, '0')
  return `${year}.${month}.${day}`
}

function onSearchInput(value: string | undefined) {
  searchKeyword.value = (value || '').trim()
}

function handleBoardsPageScroll() {
  isBoardsPageScrolling.value = true
  if (boardsPageScrollTimer) window.clearTimeout(boardsPageScrollTimer)
  boardsPageScrollTimer = window.setTimeout(() => {
    isBoardsPageScrolling.value = false
    boardsPageScrollTimer = undefined
  }, 700)
}

async function createAndOpenBoard() {
  activeMenuBoardId.value = null
  const result = await boardCommands.create()
  if (!result.ok) {
    reloadBoards()
    appToast.error(result.reason === 'name-conflict' ? '画布名称已存在，请重试' : '创建同步失败，请稍后重试')
    console.warn('[boards] failed to sync new board', result.error)
    return
  }
  reloadBoards()
  router.push({ path: USER_WORKSPACE_PATH, query: { boardId: result.board.id } })
}

function openBoard(board: CanvasBoardMeta) {
  touchCanvasBoard(board.id)
  router.push({ path: USER_WORKSPACE_PATH, query: { boardId: board.id } })
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
  const result = await boardCommands.rename({ boardId, nextName: renameDraft.value })
  if (!result.ok) {
    if (result.reason === 'local-name-conflict') {
      appToast.warning('名称已存在')
      return
    }
    renamingBoardId.value = null
    suppressNextBoardOpen.value = true
    if (result.reason === 'invalid-name') {
      appToast.warning('画布名称不能为空')
      return
    }
    reloadBoards()
    appToast.error(
      result.reason === 'remote-name-conflict'
        ? '名称已存在，请更换后再试'
        : '重命名同步失败，请稍后重试',
    )
    console.warn('[boards] failed to sync renamed board', result.error)
    return
  }
  renamingBoardId.value = null
  suppressNextBoardOpen.value = true
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
  const result = await boardCommands.duplicate({ sourceBoardId: board.id })
  if (!result.ok) {
    reloadBoards()
    if (result.reason === 'source-missing') {
      appToast.error('复制失败')
      return
    }
    appToast.error(
      result.reason === 'name-conflict'
        ? '复制后的画布名称已存在，请重试'
        : '复制同步失败，请稍后重试',
    )
    console.warn('[boards] failed to sync duplicated board', result.error)
    return
  }
  reloadBoards()
  appToast.success('已复制画布')
}

function handleDeleteAction(board: CanvasBoardMeta) {
  activeMenuBoardId.value = null
  pendingDeleteBoard.value = board
  deleteDialogVisible.value = true
}

function closeDeleteDialog() {
  if (isDeleteBoardPending.value) return
  deleteDialogVisible.value = false
}

function handleDeleteDialogHide() {
  if (!isDeleteBoardPending.value) {
    pendingDeleteBoard.value = null
  }
}

async function confirmDeleteBoard() {
  const board = pendingDeleteBoard.value
  if (!board || isDeleteBoardPending.value) return
  isDeleteBoardPending.value = true
  const result = await boardCommands.delete({ boardId: board.id })
  if (!result.ok) {
    appToast.error(result.reason === 'sync-failed' ? '删除失败，请稍后重试' : '删除失败')
    if (result.reason === 'sync-failed') {
      console.warn('[boards] failed to sync deleted board', result.error)
    }
    isDeleteBoardPending.value = false
    return
  }
  if (renamingBoardId.value === board.id) {
    cancelRename()
  }
  reloadBoards()
  deleteDialogVisible.value = false
  pendingDeleteBoard.value = null
  isDeleteBoardPending.value = false
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
  router.push(USER_CENTER_PATH)
}

function goHome() {
  closeUserMenu()
  router.push(USER_HOME_PATH)
}

async function logout() {
  closeUserMenu()
  await logoutAndRedirect(USER_HOME_PATH)
}

onMounted(async () => {
  try {
    const hydrateResult = await hydrateCanvasBoardsFromServer()
    if (!hydrateResult.ok) {
      appToast.error('画布同步失败，请稍后重试')
    }
    reloadBoards()
  } finally {
    isBoardsLoading.value = false
  }
})

onBeforeUnmount(() => {
  boardPreviewRefresh.dispose()
  if (boardsPageScrollTimer) {
    window.clearTimeout(boardsPageScrollTimer)
    boardsPageScrollTimer = undefined
  }
})
</script>
