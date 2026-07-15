<template>
  <div :class="boardCatalogClasses.page">
    <div :class="boardCatalogClasses.pageBackground"></div>

    <header :class="boardCatalogClasses.header">
      <div :class="boardCatalogClasses.brand">
        <span :class="boardCatalogClasses.brandName">{{ projectName }}</span>
      </div>
      <UserAccountMenu
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
        :wrap-class="boardCatalogClasses.userMenuWrap"
        :avatar-button-class="boardCatalogClasses.userAvatarButton"
        :avatar-image-class="boardCatalogClasses.userAvatar"
        :avatar-placeholder-class="boardCatalogClasses.userAvatarPlaceholder"
        :avatar-icon-class="boardCatalogClasses.avatarIcon"
      />
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
          :model-value="sortMode"
          :options="sortOptions"
          option-label="label"
          option-value="value"
          :class="boardCatalogClasses.sortSelect"
          :pt="boardCatalogSortSelectPt"
          aria-label="Sort canvases"
          @update:model-value="setSortMode"
        />
      </section>

      <div v-if="catalogRecoveryBoardId" :class="boardCatalogClasses.recoveryState" role="alert">
        <span :class="boardCatalogClasses.recoveryText">画布列表需要重新加载</span>
        <Button
          unstyled
          type="button"
          :label="isCatalogReloadPending ? '重新加载中...' : '重新加载'"
          :disabled="isCatalogReloadPending"
          :class="boardCatalogClasses.recoveryButton"
          @click="reloadCatalog"
        />
      </div>

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
          :display-date="formatBoardCatalogDate(board.updatedAt)"
          @click-board="openBoardFromCard(board)"
          @toggle-menu="toggleBoardMenu(board.id)"
          @rename-action="startRename(board)"
          @duplicate-action="duplicateBoard(board)"
          @delete-action="requestDelete(board)"
          @rename-input="updateRenameDraft"
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
      :visible="deleteDialogVisible"
      modal
      :dismissable-mask="!isDeletePending"
      :closable="!isDeletePending"
      :draggable="false"
      :show-header="false"
      :style="boardCatalogDeleteDialogStyle"
      :pt="boardCatalogDeleteDialogPt"
      aria-labelledby="board-delete-dialog-title"
      @update:visible="setDeleteDialogVisible"
      @hide="handleDeleteDialogHidden"
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
            :disabled="isDeletePending"
            :class="boardCatalogClasses.deleteCancelButton"
            @click="closeDeleteDialog"
          />
          <Button
            unstyled
            type="button"
            :label="isDeletePending ? '删除中...' : '删除'"
            :disabled="isDeletePending"
            :class="boardCatalogClasses.deleteConfirmButton"
            @click="confirmDelete"
          />
        </div>
      </div>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useCanvasBoardCatalog } from '../../canvas-board/catalog/canvasBoardCatalog'
import BoardCard from '../components/BoardCard.vue'
import { useBoardFeedback } from '../feedback/useBoardFeedback'
import { createBoardPreviewRefreshQueue } from '../../canvas-board/preview'
import {
  boardCatalogClasses,
  boardCatalogDeleteDialogPt,
  boardCatalogDeleteDialogStyle,
  boardCatalogSortSelectPt,
} from '../lib/boardCatalogSkin'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { useSiteBranding } from '../../../shared/composables/useSiteBranding'
import { USER_CANVAS_PATH, USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../../app/stores/user'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import { Search } from '@iconoir/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import {
  formatBoardCatalogDate,
  useBoardCatalogController,
} from '../controller/boardCatalogController'

const router = useRouter()
const boardFeedback = useBoardFeedback()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const { projectName } = useSiteBranding()
const boardCatalog = useCanvasBoardCatalog()
const boardPreviewRefresh = createBoardPreviewRefreshQueue()
const boardCatalogController = useBoardCatalogController({
  catalog: boardCatalog,
  preview: boardPreviewRefresh,
  navigation: {
    openBoard(boardId) {
      void router.push({ path: USER_CANVAS_PATH, query: { boardId } })
    },
  },
  feedback: boardFeedback,
  warn: console.warn,
})
const {
  visibleBoards,
  previewMap: boardPreviewMap,
  isBoardsLoading,
  searchKeyword,
  sortMode,
  activeMenuBoardId,
  renamingBoardId,
  renameDraft,
  catalogRecoveryBoardId,
  isCatalogReloadPending,
  deleteDialogVisible,
  pendingDeleteBoard,
  isDeletePending,
} = boardCatalogController.viewModel
const {
  setSearchKeyword: onSearchInput,
  setSortMode,
  toggleBoardMenu,
  handleBoardMenuHidden,
  createAndOpenBoard,
  duplicateBoard,
  startRename,
  updateRenameDraft,
  commitRename,
  cancelRename,
  reloadCatalog,
  openBoardFromCard,
  requestDelete,
  closeDeleteDialog,
  setDeleteDialogVisible,
  handleDeleteDialogHidden,
  confirmDelete,
} = boardCatalogController.commands

const sortOptions = [
  { label: 'Recent', value: 'recent' },
  { label: 'Name', value: 'name' },
]

const accountControls = useUserAccountControls({
  navigation: {
    navigate: (path) => router.push(path),
    logout: () => logoutAndRedirect(USER_HOME_PATH),
  },
})
</script>
