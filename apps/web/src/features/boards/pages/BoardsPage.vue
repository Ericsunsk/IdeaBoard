<template>
  <div
    :class="[boardCatalogClasses.page, { 'is-scrolling': isBoardsPageScrolling }]"
    @scroll.passive="handleBoardsPageScroll"
  >
    <div :class="boardCatalogClasses.pageBackground"></div>

    <header :class="boardCatalogClasses.header">
      <div :class="boardCatalogClasses.brand">
        <span :class="boardCatalogClasses.brandName">{{ projectName }}</span>
      </div>
      <div :class="boardCatalogClasses.userMenuWrap" @pointerdown.stop @mousedown.stop @click.stop>
        <Button
          unstyled
          type="button"
          title="账户菜单"
          aria-label="账户菜单"
          aria-haspopup="menu"
          :aria-expanded="isUserMenuOpen"
          :class="boardCatalogClasses.userAvatarButton"
          @click="toggleUserMenu"
        >
          <img
            v-if="userStore.user?.avatar"
            :src="userStore.user.avatar"
            :class="boardCatalogClasses.userAvatar"
            alt="用户头像"
          />
          <div v-else :class="boardCatalogClasses.userAvatarPlaceholder">
            <User :class="boardCatalogClasses.avatarIcon" />
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

    <main :class="boardCatalogClasses.main">
      <h1 :class="boardCatalogClasses.pageTitle">Welcome to {{ projectName }}!</h1>
      <p :class="boardCatalogClasses.pageSubtitle">Explore, expand, and refine your ideas</p>

      <section :class="boardCatalogClasses.toolbar">
        <Button
          type="button"
          label="New Canvas"
          :class="boardCatalogClasses.createButton"
          @click="createAndOpenBoard"
        />
        <label :class="boardCatalogClasses.searchWrap">
          <Search :class="boardCatalogClasses.searchIcon" />
          <InputText
            :model-value="searchKeyword"
            unstyled
            type="text"
            placeholder="Search canvases..."
            :class="boardCatalogClasses.searchInput"
            @update:model-value="onSearchInput"
          />
        </label>
        <Select
          v-model="sortMode"
          :options="sortOptions"
          option-label="label"
          option-value="value"
          :class="boardCatalogClasses.sortSelect"
          :pt="boardCatalogSortSelectPt"
          aria-label="Sort canvases"
        />
      </section>

      <section v-if="isBoardsLoading" :class="boardCatalogClasses.loadingState" aria-live="polite">
        <ProgressSpinner :class="boardCatalogClasses.loadingSpinner" stroke-width="4" />
      </section>

      <section v-else :class="boardCatalogClasses.boardsGrid">
        <Button
          unstyled
          type="button"
          :class="boardCatalogClasses.createCard"
          @click="createAndOpenBoard"
        >
          <div :class="boardCatalogClasses.createPreview">
            <div :class="boardCatalogClasses.createPreviewIcon" aria-hidden="true"></div>
          </div>
          <div :class="boardCatalogClasses.createMeta">
            <div :class="boardCatalogClasses.createTitle">Create New Canvas</div>
            <div :class="boardCatalogClasses.createDate">Start from an empty board</div>
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
        :class="boardCatalogClasses.emptyTip"
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
      :style="boardCatalogDeleteDialogStyle"
      :pt="boardCatalogDeleteDialogPt"
      aria-labelledby="board-delete-dialog-title"
      @hide="handleDeleteDialogHide"
    >
      <div :class="boardCatalogClasses.deleteDialogShell">
        <h2 id="board-delete-dialog-title" :class="boardCatalogClasses.deleteDialogTitle">
          删除画布
        </h2>
        <p :class="boardCatalogClasses.deleteDialogDescription">
          确认删除「{{ pendingDeleteBoard?.name || 'Untitled' }}」？此操作不可撤销。
        </p>

        <div :class="boardCatalogClasses.deleteDialogFooter">
          <Button
            unstyled
            type="button"
            label="取消"
            :disabled="isDeleteBoardPending"
            :class="boardCatalogClasses.deleteCancelButton"
            @click="closeDeleteDialog"
          />
          <Button
            unstyled
            type="button"
            :label="isDeleteBoardPending ? '删除中...' : '删除'"
            :disabled="isDeleteBoardPending"
            :class="boardCatalogClasses.deleteConfirmButton"
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
import { hydrateCanvasBoardsFromServer } from '../../canvas-board/catalog/boardSync'
import { createCanvasBoardCommands } from '../lib/canvasBoardCommands'
import { createBoardPreviewRefreshQueue } from '../lib/boardPreviewRefreshQueue'
import {
  boardCatalogClasses,
  boardCatalogDeleteDialogPt,
  boardCatalogDeleteDialogStyle,
  boardCatalogSortSelectPt,
} from '../lib/boardCatalogSkin'
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
