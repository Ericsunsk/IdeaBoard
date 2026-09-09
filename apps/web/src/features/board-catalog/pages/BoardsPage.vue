<template>
  <div :class="catalogClasses.page">
    <div :class="catalogClasses.pageBackground"></div>

    <header :class="catalogClasses.header">
      <div :class="catalogClasses.brand">
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

    <main :class="catalogClasses.main">
      <h1 :class="catalogClasses.pageTitle">Welcome to IdeaBoard!</h1>
      <p :class="catalogClasses.pageSubtitle">{{ t('workspace.boards.subtitle') }}</p>

      <section :class="catalogClasses.toolbar">
        <Button
          type="button"
          :label="t('workspace.boards.newCanvas')"
          :class="catalogClasses.createButton"
          @click="createAndOpenBoard"
        />
        <label :class="catalogClasses.searchWrap">
          <Search :class="catalogClasses.searchIcon" />
          <InputText
            :model-value="searchKeyword"
            unstyled
            type="text"
            :placeholder="t('workspace.boards.searchPlaceholder')"
            :class="catalogClasses.searchInput"
            @update:model-value="onSearchInput"
          />
        </label>
        <Button
          ref="sortMenuTriggerRef"
          unstyled
          type="button"
          :class="catalogClasses.sortMenuTrigger"
          :aria-label="t('workspace.boards.sortLabel')"
          aria-haspopup="menu"
          :aria-expanded="isSortMenuOpen"
          @click="toggleSortMenu"
        >
          <span class="truncate">{{ activeSortLabel }}</span>
          <NavArrowDown
            class="size-3.5 shrink-0 transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
            :class="isSortMenuOpen ? 'rotate-180' : ''"
            aria-hidden="true"
          />
        </Button>
        <Menu
          ref="sortMenuRef"
          :model="sortMenuItems"
          popup
          append-to="body"
          :pt="catalogSortMenuPt"
          @show="handleSortMenuShow"
          @hide="handleSortMenuHide"
        >
          <template #item="{ item, props: itemProps }">
            <a
              v-bind="itemProps.action"
              :class="[
                appOverlayMenuItemClass,
                item.value === sortMode
                  ? 'bg-[var(--app-overlay-menu-item-hover-bg)] text-[var(--app-overlay-menu-item-hover-color)]'
                  : '',
              ]"
              :aria-current="item.value === sortMode ? 'true' : undefined"
            >
              {{ item.label }}
            </a>
          </template>
        </Menu>
      </section>

      <div v-if="catalogRecoveryBoardId" :class="catalogClasses.recoveryState" role="alert">
        <span :class="catalogClasses.recoveryText">{{
          t('workspace.catalog.reloadRequired')
        }}</span>
        <Button
          unstyled
          type="button"
          :label="
            isCatalogReloadPending
              ? t('workspace.catalog.reloading')
              : t('workspace.catalog.reload')
          "
          :disabled="isCatalogReloadPending"
          :class="catalogClasses.recoveryButton"
          @click="reloadCatalog"
        />
      </div>

      <section v-if="isBoardsLoading" :class="catalogClasses.loadingState" aria-live="polite">
        <ProgressSpinner :class="catalogClasses.loadingSpinner" stroke-width="4" />
      </section>

      <section v-else :class="catalogClasses.boardsGrid">
        <Button
          unstyled
          type="button"
          :class="catalogClasses.createCard"
          @click="createAndOpenBoard"
        >
          <div :class="catalogClasses.createPreview">
            <div :class="catalogClasses.createPreviewIcon" aria-hidden="true"></div>
          </div>
          <div :class="catalogClasses.createMeta">
            <div :class="catalogClasses.createTitle">
              {{ t('workspace.boards.createNewCanvas') }}
            </div>
            <div :class="catalogClasses.createDate">
              {{ t('workspace.boards.startFromEmpty') }}
            </div>
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
            :display-date="formatCatalogDate(board.updatedAt)"
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
        :class="catalogClasses.emptyTip"
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
      :style="catalogDeleteDialogStyle"
      :pt="catalogDeleteDialogPt"
      :aria-busy="deleteDialogAriaBusy"
      aria-labelledby="board-delete-dialog-title"
      @update:visible="handleDeleteDialogVisibilityChange"
      @hide="disableDeleteDialogVendorFocusRestore"
      @after-hide="handleDeleteDialogAfterHide"
    >
      <div :class="catalogClasses.deleteDialogShell">
        <h2 id="board-delete-dialog-title" :class="catalogClasses.deleteDialogTitle">
          {{ t('workspace.deleteDialog.title') }}
        </h2>
        <p :class="catalogClasses.deleteDialogDescription">
          {{
            t('workspace.deleteDialog.description', {
              name: pendingDeleteBoard?.name || t('workspace.boards.untitledCanvas'),
            })
          }}
        </p>

        <div :class="catalogClasses.deleteDialogFooter">
          <Button
            unstyled
            type="button"
            :label="t('common.actions.cancel')"
            :disabled="isDeletePending"
            :class="catalogClasses.deleteCancelButton"
            @click="requestDeleteDialogClose('close-control')"
          />
          <Button
            unstyled
            type="button"
            :label="
              isDeletePending ? t('workspace.deleteDialog.deleting') : t('common.actions.delete')
            "
            :disabled="isDeletePending"
            :class="catalogClasses.deleteConfirmButton"
            @click="confirmDelete"
          />
        </div>
      </div>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BoardCard from '../components/BoardCard.vue'
import {
  catalogClasses,
  catalogDeleteDialogPt,
  catalogDeleteDialogStyle,
  catalogSortMenuPt,
} from '../lib/skin'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import { usePrimeDialogOverlayLifecycle } from '../../../shared/composables/usePrimeDialogOverlayLifecycle'
import { usePrimePopupMenuOverlayLifecycle } from '../../../shared/composables/usePrimePopupMenuOverlayLifecycle'
import { HOME_PATH, USER_CANVAS_PATH } from '../../../shared/constants/routePaths'
import { appOverlayMenuItemClass } from '../../../shared/theme/appOverlayMenu'
import { useUserStore } from '../../auth/session/session'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import UserLocaleSelect from '../../../shared/components/UserLocaleSelect.vue'
import StudioName from '../../../shared/components/StudioName.vue'
import UserWorkspaceNavigation from '../../user-shell/components/UserWorkspaceNavigation.vue'
import { NavArrowDown, Search } from '@iconoir/vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Menu from 'primevue/menu'
import type { MenuItem } from 'primevue/menuitem'
import ProgressSpinner from 'primevue/progressspinner'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import {
  formatCatalogDate,
  useBoardCatalog,
  type BoardCatalogSortMode,
} from '../useBoardCatalog'

const router = useRouter()
const { t } = useI18n()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const controller = useBoardCatalog({
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
} = controller.viewModel
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
} = controller.commands

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

const sortOptions = computed<Array<{ label: string; value: BoardCatalogSortMode }>>(() => [
  { label: t('workspace.boards.sortRecent'), value: 'recent' },
  { label: t('workspace.boards.sortName'), value: 'name' },
])
const activeSortLabel = computed(
  () => sortOptions.value.find((option) => option.value === sortMode.value)?.label ?? '',
)
const sortMenuTriggerRef = ref<HTMLElement | { $el?: HTMLElement } | null>(null)
const {
  popupRef: sortMenuRef,
  isOpen: isSortMenuOpen,
  toggle: toggleSortMenuOverlay,
  handleShow: handleSortMenuShow,
  handleHide: handleSortMenuHide,
  select: selectSortMenuItem,
} = usePrimePopupMenuOverlayLifecycle({})
const sortMenuItems = computed<MenuItem[]>(() =>
  sortOptions.value.map((option) => ({
    ...option,
    command: () => selectSortMenuItem(() => setSortMode(option.value)),
  })),
)

function toggleSortMenu(event: Event) {
  const trigger = sortMenuTriggerRef.value
  const target = trigger instanceof HTMLElement ? trigger : trigger?.$el
  toggleSortMenuOverlay(event, target)
}

const accountControls = useUserAccountControls({
  navigation: {
    navigate: (path) => router.push(path),
    logout: () => logoutAndRedirect(HOME_PATH),
  },
})
</script>
