<template>
  <div :class="catalogClasses.page">
    <div :class="catalogClasses.pageBackground"></div>

    <WorkspaceHeader />

    <main :class="catalogClasses.main">
      <h1 :class="catalogClasses.pageTitle">{{ t('board.title') }}</h1>
      <p :class="catalogClasses.pageSubtitle">{{ t('board.subtitle') }}</p>

      <section :class="catalogClasses.toolbar">
        <Button type="button" :class="catalogClasses.createButton" @click="createAndOpenBoard">{{
          t('board.newCanvas')
        }}</Button>
        <label :class="catalogClasses.searchWrap">
          <Search :class="catalogClasses.searchIcon" />
          <span class="sr-only">{{ t('board.searchLabel') }}</span>
          <input
            :value="searchKeyword"
            type="text"
            :aria-label="t('board.searchLabel')"
            :placeholder="t('board.searchPlaceholder')"
            :class="catalogClasses.searchInput"
            @input="onSearchInput(($event.target as HTMLInputElement).value)"
            @keydown.esc="onSearchInput('')"
          />
          <button
            v-if="searchKeyword"
            type="button"
            :class="catalogClasses.searchClearButton"
            :title="t('search.tooltips.clearSearch')"
            :aria-label="t('search.tooltips.clearSearch')"
            @click.prevent="onSearchInput('')"
          >
            <Xmark class="size-3.5" aria-hidden="true" />
          </button>
        </label>
        <label :for="sortSelectId" class="sr-only">
          {{ t('board.sortLabel') }}
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
              <NavArrowDown :class="catalogClasses.sortSelectIcon" aria-hidden="true" />
            </SelectIcon>
          </SelectTrigger>
          <SelectContent
            position="popper"
            align="end"
            :side-offset="appOverlayMenuSideOffset"
            :class="catalogClasses.sortSelectContent"
          >
            <SelectViewport>
              <SelectItem
                v-for="item in sortOptions"
                :key="item.value"
                :value="item.value"
                :class="catalogClasses.sortSelectItem"
              >
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
              {{ t('board.createNewCanvas') }}
            </div>
            <div :class="catalogClasses.createDate">
              {{ t('board.startFromEmpty') }}
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
        {{ t('board.noMatchingCanvas') }}
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
                  name: pendingDeleteBoard?.name || t('board.untitledCanvas'),
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
                    name: pendingDeleteBoard?.name || t('board.untitledCanvas'),
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
import { USER_CANVAS_PATH } from '../../../workspace/routePaths'
import {
  defineDialogSurfaceIntent,
  useOverlayInteractionLifecycle,
} from '../../../../core/composables/useOverlayInteractionLifecycle'
import { WorkspaceHeader, useWorkspaceSelection } from '../../../workspace'
import { NavArrowDown, Search, Xmark } from '@iconoir/vue'
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
import { ibDialogMaskClass } from '../../../../theme/dialogMask'
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
import { appOverlayMenuSideOffset } from '../../../../theme/appOverlayMenu'
import { Spinner } from '@/components/ui/spinner'
import { formatCatalogDate, useCatalog, type CatalogSortMode } from './useCatalog'

const router = useRouter()
const { t } = useI18n()
// Ensure the workspace list loads on this surface: fresh profiles have no
// persisted selection, so the store stays null until this query resolves and
// auto-selects. The catalog watches scope and auto-hydrates when it becomes
// ready; hydrate itself waits while the list is pending.
useWorkspaceSelection()
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
  { label: t('board.sortRecent'), value: 'recent' },
  { label: t('board.sortName'), value: 'name' },
])

// Select emits a loose value; only accept one the option list actually offers so
// a stray emission cannot put the catalog into a mode it has no label for.
function onSortModeChange(value: unknown) {
  const match = sortOptions.value.find((option) => option.value === value)
  if (match) onSortModeCommand(match.value)
}

// The sort trigger is a real form control, so it needs a programmatic label.
const sortSelectId = 'board-catalog-sort-select'
</script>
