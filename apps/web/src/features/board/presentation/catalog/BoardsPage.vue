<template>
  <div :class="catalogClasses.page">
    <div :class="catalogClasses.pageBackground"></div>

    <header :class="catalogClasses.header">
      <div :class="catalogClasses.brand">
        <StudioName />
      </div>
      <WorkspaceNavigation />
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
        <Button type="button" :class="catalogClasses.createButton" @click="createAndOpenBoard">{{
          t('workspace.boards.newCanvas')
        }}</Button>
        <label :class="catalogClasses.searchWrap">
          <Search :class="catalogClasses.searchIcon" />
          <input
            :value="searchKeyword"
            type="text"
            :placeholder="t('workspace.boards.searchPlaceholder')"
            :class="catalogClasses.searchInput"
            @input="onSearchInput(($event.target as HTMLInputElement).value)"
          />
        </label>
        <DropdownMenu v-slot="{ open }">
          <DropdownMenuTrigger as-child>
            <button
              type="button"
              :class="catalogClasses.sortMenuTrigger"
              :aria-label="t('workspace.boards.sortLabel')"
              aria-haspopup="menu"
              :aria-expanded="open ? 'true' : 'false'"
            >
              <span class="truncate">{{ activeSortLabel }}</span>
              <NavArrowDown
                class="size-3.5 shrink-0 transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
                :class="open ? 'rotate-180' : ''"
                aria-hidden="true"
              />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" class="w-[140px]">
            <DropdownMenuItem
              v-for="item in sortMenuItems"
              :key="item.value"
              :class="item.value === sortMode ? 'font-medium bg-accent' : ''"
              @select="item.command?.()"
            >
              {{ item.label }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </section>

      <div v-if="catalogRecoveryBoardId" :class="catalogClasses.recoveryState" role="alert">
        <span :class="catalogClasses.recoveryText">{{
          t('workspace.catalog.reloadRequired')
        }}</span>
        <Button
          type="button"
          :disabled="isCatalogReloadPending"
          :class="catalogClasses.recoveryButton"
          @click="reloadCatalog"
          >{{
            isCatalogReloadPending
              ? t('workspace.catalog.reloading')
              : t('workspace.catalog.reload')
          }}</Button
        >
      </div>

      <section v-if="isBoardsLoading" :class="catalogClasses.loadingState" aria-live="polite">
        <Spinner :class="catalogClasses.loadingSpinner" />
      </section>

      <section v-else :class="catalogClasses.boardsGrid">
        <button type="button" :class="catalogClasses.createCard" @click="createAndOpenBoard">
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
        </button>

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

    <DialogRoot :open="deleteDialogVisible" @update:open="handleDeleteDialogOpenUpdate">
      <DialogPortal>
        <DialogOverlay :class="ibDialogMaskClass" @click="handleDeleteDialogMaskClick" />
        <DialogContent
          :class="cn(dialogContentBaseClass, catalogDeleteDialogContentClass)"
          :style="catalogDeleteDialogStyle"
          aria-labelledby="board-delete-dialog-title"
          :aria-busy="deleteDialogAriaBusy"
          @escape-key-down="handleDeleteDialogEscape"
          @pointer-down-outside="blockDeleteDialogOutsideDismiss"
        >
          <div class="min-h-0 overflow-y-auto">
            <DialogDescription class="sr-only">
              {{
                t('workspace.deleteDialog.description', {
                  name: pendingDeleteBoard?.name || t('workspace.boards.untitledCanvas'),
                })
              }}
            </DialogDescription>
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
                  type="button"
                  :disabled="isDeletePending"
                  :class="catalogClasses.deleteCancelButton"
                  @click="closeDeleteDialogView"
                  >{{ t('common.actions.cancel') }}</Button
                >
                <Button
                  type="button"
                  :disabled="isDeletePending"
                  :class="catalogClasses.deleteConfirmButton"
                  @click="confirmDelete"
                  >{{
                    isDeletePending
                      ? t('workspace.deleteDialog.deleting')
                      : t('common.actions.delete')
                  }}</Button
                >
              </div>
            </div>
          </div>
        </DialogContent>
      </DialogPortal>
    </DialogRoot>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BoardCard from './BoardCard.vue'
import { catalogClasses, catalogDeleteDialogContentClass, catalogDeleteDialogStyle } from './skin'
import { useLogoutRedirect } from '../../../../shared/composables/useLogoutRedirect'
import { defineDialogSurfaceIntent, useOverlayInteractionLifecycle } from '../../../../shared/composables/useOverlayInteractionLifecycle'
import { HOME_PATH, USER_CANVAS_PATH } from '../../../../shared/constants/routePaths'
import { useUserStore } from '../../../auth'
import UserAccountMenu from '../../../../shared/components/UserAccountMenu.vue'
import UserLocaleSelect from '../../../../shared/components/UserLocaleSelect.vue'
import StudioName from '../../../../shared/components/StudioName.vue'
import { WorkspaceNavigation, useWorkspaceAccountControls } from '../../../workspace'
import { NavArrowDown, Search } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
import {
  DialogContent,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogRoot,
} from '@/components/ui/dialog'
import { dialogContentBaseClass } from '@/components/ui/dialog/dialogClasses'
import { cn } from '@/lib/utils'
import { ibDialogMaskClass } from '../../../../shared/theme/dialogMask'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import { formatCatalogDate, useCatalog, type CatalogSortMode } from './useCatalog'

const router = useRouter()
const { t } = useI18n()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const controller = useCatalog({
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
  handleVisibilityChange: handleDeleteDialogVisibilityChange,
  requestClose: requestDeleteDialogClose,
} = useOverlayInteractionLifecycle({
  visible: deleteDialogVisible,
  busy: isDeletePending,
  surface: defineDialogSurfaceIntent(),
  onVisibilityChange: (visible) => {
    if (!visible) closeDeleteDialog()
  },
})

function closeDeleteDialogView() {
  if (handleDeleteDialogVisibilityChange(false)) handleDeleteDialogAfterHide()
}

function handleDeleteDialogOpenUpdate(open: boolean) {
  if (!open) closeDeleteDialogView()
}

function handleDeleteDialogMaskClick() {
  if (requestDeleteDialogClose('mask')) handleDeleteDialogAfterHide()
}

function handleDeleteDialogEscape(event: KeyboardEvent) {
  if (!canDismissDeleteDialog('escape')) event.preventDefault()
}

function blockDeleteDialogOutsideDismiss(event: Event) {
  event.preventDefault()
}

function handleDeleteDialogAfterHide() {
  completeDeleteDialogClose()
  handleDeleteDialogHidden()
}

const sortOptions = computed<Array<{ label: string; value: CatalogSortMode }>>(() => [
  { label: t('workspace.boards.sortRecent'), value: 'recent' },
  { label: t('workspace.boards.sortName'), value: 'name' },
])
const activeSortLabel = computed(
  () => sortOptions.value.find((option) => option.value === sortMode.value)?.label ?? '',
)
const sortMenuItems = computed(() =>
  sortOptions.value.map((option) => ({
    ...option,
    command: () => setSortMode(option.value),
  })),
)

const accountControls = useWorkspaceAccountControls({
  navigation: {
    navigate: (path) => router.push(path),
    logout: () => logoutAndRedirect(HOME_PATH),
  },
})
</script>
