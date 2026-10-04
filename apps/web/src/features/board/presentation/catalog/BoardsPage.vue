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
      <h1 :class="catalogClasses.pageTitle">{{ t('workspace.boards.title') }}</h1>
      <p :class="catalogClasses.pageSubtitle">{{ t('workspace.boards.subtitle') }}</p>

      <section :class="catalogClasses.toolbar">
        <Button type="button" :class="catalogClasses.createButton" @click="createAndOpenBoard">{{
          t('workspace.boards.newCanvas')
        }}</Button>
        <label :class="catalogClasses.searchWrap">
          <Search :class="catalogClasses.searchIcon" />
          <span class="sr-only">{{ t('workspace.boards.searchLabel') }}</span>
          <input
            :value="searchKeyword"
            type="text"
            :aria-label="t('workspace.boards.searchLabel')"
            :placeholder="t('workspace.boards.searchPlaceholder')"
            :class="catalogClasses.searchInput"
            @input="onSearchInput(($event.target as HTMLInputElement).value)"
          />
        </label>
        <label :for="sortSelectId" class="sr-only">
          {{ t('workspace.boards.sortLabel') }}
        </label>
        <!--
          sortMode comes from the controller's readonly viewModel, so v-model
          cannot write it; selection goes through the setSortMode command, the
          same shape searchKeyword uses above. The trigger's accessible name
          comes from the sr-only <label :for> above, which this rule cannot
          follow across the Select primitive.
        -->
        <!-- eslint-disable-next-line vuejs-accessibility/form-control-has-label -->
        <Select :model-value="sortMode" @update:model-value="onSortModeChange">
          <SelectTrigger :id="sortSelectId" :class="catalogClasses.sortSelectTrigger">
            <SelectValue />
            <SelectIcon>
              <NavArrowDown
                class="size-3.5 shrink-0 text-muted-foreground transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
                aria-hidden="true"
              />
            </SelectIcon>
          </SelectTrigger>
          <SelectContent
            position="popper"
            :side-offset="appOverlayMenuSideOffset"
            class="w-[140px]"
          >
            <SelectViewport>
              <SelectItem v-for="item in sortOptions" :key="item.value" :value="item.value">
                <SelectItemText>{{ item.label }}</SelectItemText>
              </SelectItem>
            </SelectViewport>
          </SelectContent>
        </Select>
      </section>

      <div
        v-if="catalogRecoveryBoardId || catalogHydrationFailed"
        :class="catalogClasses.recoveryState"
        role="alert"
      >
        <span :class="catalogClasses.recoveryText">{{
          catalogHydrationFailed
            ? t('workspace.catalog.loadFailed')
            : t('workspace.catalog.reloadRequired')
        }}</span>
        <Button
          type="button"
          :disabled="isCatalogReloadPending"
          :class="catalogClasses.recoveryButton"
          @click="reloadCatalog"
          >{{
            isCatalogReloadPending
              ? t('workspace.catalog.reloading')
              : catalogHydrationFailed
                ? t('common.actions.retry')
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
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../../shared/composables/useOverlayInteractionLifecycle'
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
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectItemText,
  SelectTrigger,
  SelectValue,
  SelectViewport,
} from '@/components/ui/select'
import { appOverlayMenuSideOffset } from '../../../../shared/theme/appOverlayMenu'
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
  catalogHydrationFailed,
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
  setSortMode: onSortModeCommand,
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

// Select emits a loose value; only accept one the option list actually offers so
// a stray emission cannot put the catalog into a mode it has no label for.
function onSortModeChange(value: unknown) {
  const match = sortOptions.value.find((option) => option.value === value)
  if (match) onSortModeCommand(match.value)
}

// The sort trigger is a real form control, so it needs a programmatic label.
const sortSelectId = 'board-catalog-sort-select'

const accountControls = useWorkspaceAccountControls({
  navigation: {
    navigate: (path) => router.push(path),
    logout: () => logoutAndRedirect(HOME_PATH),
  },
})
</script>
