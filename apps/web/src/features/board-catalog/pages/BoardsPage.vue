<template>
  <div :class="boardCatalogClasses.page">
    <div :class="boardCatalogClasses.pageBackground"></div>

    <header :class="boardCatalogClasses.header">
      <div :class="boardCatalogClasses.brand">
        <StudioName />
      </div>
      <UserWorkspaceNavigation />
      <UserAccountMenu
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      >
        <template #before-balance>
          <UserLocaleSelect />
        </template>
      </UserAccountMenu>
    </header>

    <main :class="boardCatalogClasses.main">
      <h1 :class="boardCatalogClasses.pageTitle">Welcome to IdeaBoard!</h1>
      <p :class="boardCatalogClasses.pageSubtitle">{{ t('workspace.boards.subtitle') }}</p>

      <section :class="boardCatalogClasses.toolbar">
        <Button
          type="button"
          :label="t('workspace.boards.newCanvas')"
          :class="boardCatalogClasses.createButton"
          @click="createAndOpenBoard"
        />
        <label :class="boardCatalogClasses.searchWrap">
          <Search :class="boardCatalogClasses.searchIcon" />
          <InputText
            :model-value="searchKeyword"
            unstyled
            type="text"
            :placeholder="t('workspace.boards.searchPlaceholder')"
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
          :aria-label="t('workspace.boards.sortLabel')"
          @update:model-value="setSortMode"
        />
      </section>

      <div v-if="catalogRecoveryBoardId" :class="boardCatalogClasses.recoveryState" role="alert">
        <span :class="boardCatalogClasses.recoveryText">{{ t('workspace.catalog.reloadRequired') }}</span>
        <Button
          unstyled
          type="button"
          :label="isCatalogReloadPending ? t('workspace.catalog.reloading') : t('workspace.catalog.reload')"
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
            <div :class="boardCatalogClasses.createTitle">{{ t('workspace.boards.createNewCanvas') }}</div>
            <div :class="boardCatalogClasses.createDate">{{ t('workspace.boards.startFromEmpty') }}</div>
          </div>
        </Button>

        <TransitionGroup name="board-card" appear>
          <BoardCard
            v-for="board in visibleBoards"
            :key="board.id"
            :board="board"
            :preview-snapshot-data-url="boardPreviewMap[board.id] || ''"
            :is-renaming="renamingBoardId === board.id"
            :rename-draft="renameDraft"
            :is-menu-open="activeMenuBoardId === board.id"
            :display-date="formatBoardCatalogDate(board.updatedAt)"
            @click-board="openBoardFromCard(board.id)"
            @toggle-menu="toggleBoardMenu(board.id)"
            @rename-action="startRename(board)"
            @duplicate-action="duplicateBoard(board.id)"
            @delete-action="requestDelete(board)"
            @rename-input="updateRenameDraft"
            @commit-rename="commitRename"
            @cancel-rename="cancelRename"
            @menu-hidden="handleBoardMenuHidden(board.id)"
          />
        </TransitionGroup>
      </section>

      <p
        v-if="!isBoardsLoading && visibleBoards.length === 0 && searchKeyword"
        :class="boardCatalogClasses.emptyTip"
      >
        {{ t('workspace.boards.noMatchingCanvas') }}
      </p>
    </main>

    <Dialog
      ref="deleteDialogRef"
      :visible="deleteDialogVisible"
      modal
      :dismissable-mask="canDismissDeleteDialog('mask')"
      :closable="canDismissDeleteDialog('close-control')"
      :close-on-escape="canDismissDeleteDialog('escape')"
      :draggable="false"
      :show-header="false"
      :style="boardCatalogDeleteDialogStyle"
      :pt="boardCatalogDeleteDialogPt"
      :aria-busy="deleteDialogAriaBusy"
      aria-labelledby="board-delete-dialog-title"
      @update:visible="handleDeleteDialogVisibilityChange"
      @hide="disableDeleteDialogVendorFocusRestore"
      @after-hide="handleDeleteDialogAfterHide"
    >
      <div :class="boardCatalogClasses.deleteDialogShell">
        <h2 id="board-delete-dialog-title" :class="boardCatalogClasses.deleteDialogTitle">
          {{ t('workspace.deleteDialog.title') }}
        </h2>
        <p :class="boardCatalogClasses.deleteDialogDescription">
          {{
            t('workspace.deleteDialog.description', {
              name: pendingDeleteBoard?.name || t('workspace.boards.untitledCanvas'),
            })
          }}
        </p>

        <div :class="boardCatalogClasses.deleteDialogFooter">
          <Button
            unstyled
            type="button"
            :label="t('common.actions.cancel')"
            :disabled="isDeletePending"
            :class="boardCatalogClasses.deleteCancelButton"
            @click="requestDeleteDialogClose('close-control')"
          />
          <Button
            unstyled
            type="button"
            :label="isDeletePending ? t('workspace.deleteDialog.deleting') : t('common.actions.delete')"
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
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BoardCard from '../components/BoardCard.vue'
import {
  boardCatalogClasses,
  boardCatalogDeleteDialogPt,
  boardCatalogDeleteDialogStyle,
  boardCatalogSortSelectPt,
} from '../lib/boardCatalogSkin'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { usePrimeDialogOverlayLifecycle } from '../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { HOME_PATH, USER_CANVAS_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import UserLocaleSelect from '../../../shared/components/UserLocaleSelect.vue'
import StudioName from '../../../shared/components/StudioName.vue'
import UserWorkspaceNavigation from '../../user-shell/components/UserWorkspaceNavigation.vue'
import { Search } from '@iconoir/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import { formatBoardCatalogDate, useBoardsCatalog } from '../boardsCatalog'

const router = useRouter()
const { t } = useI18n()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const boardCatalogController = useBoardsCatalog({
  navigation: {
    openBoard(boardId) {
      void router.push({ path: USER_CANVAS_PATH, query: { boardId } })
    },
  },
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
  handleDeleteDialogHidden,
  confirmDelete,
} = boardCatalogController.commands

const {
  ariaBusy: deleteDialogAriaBusy,
  canDismiss: canDismissDeleteDialog,
  completeClose: completeDeleteDialogClose,
  dialogRef: deleteDialogRef,
  disableVendorFocusRestore: disableDeleteDialogVendorFocusRestore,
  handleVisibilityChange: handleDeleteDialogVisibilityChange,
  requestClose: requestDeleteDialogClose,
} = usePrimeDialogOverlayLifecycle({
  visible: deleteDialogVisible,
  busy: isDeletePending,
  surface: {
    dismissibleBy: ['close-control', 'escape', 'mask'],
    restoreFocus: true,
  },
  onVisibilityChange: (visible) => {
    if (!visible) closeDeleteDialog()
  },
})

function handleDeleteDialogAfterHide() {
  completeDeleteDialogClose()
  handleDeleteDialogHidden()
}

const sortOptions = computed(() => [
  { label: t('workspace.boards.sortRecent'), value: 'recent' },
  { label: t('workspace.boards.sortName'), value: 'name' },
])

const accountControls = useUserAccountControls({
  navigation: {
    navigate: (path) => router.push(path),
    logout: () => logoutAndRedirect(HOME_PATH),
  },
})
</script>
