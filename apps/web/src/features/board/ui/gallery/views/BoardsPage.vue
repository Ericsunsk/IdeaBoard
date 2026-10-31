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
        <button
          type="button"
          :class="[
            rangeToggleControlClass,
            range === 'team' && catalogClasses.rangeToggleButtonActive,
          ]"
          :aria-pressed="range === 'team'"
          :data-state="range === 'team' ? 'active' : 'inactive'"
          :title="range === 'team' ? t('board.scopeTeam') : t('board.scopePersonal')"
          :aria-label="range === 'team' ? t('board.scopeTeam') : t('board.scopePersonal')"
          @click="onRangeCommand(range === 'team' ? 'personal' : 'team')"
        >
          <User
            v-if="range === 'personal'"
            :class="catalogClasses.rangeToggleIcon"
            aria-hidden="true"
          />
          <Group v-else :class="catalogClasses.rangeToggleIcon" aria-hidden="true" />
        </button>
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
            <SelectValue>{{ selectedSortLabel }}</SelectValue>
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

      <LoadErrorPanel
        v-if="catalogRecoveryBoardId || catalogHydrationFailed"
        class="mb-4"
        :message="
          catalogHydrationFailed
            ? t('workspace.catalog.loadFailed')
            : t('workspace.catalog.reloadRequired')
        "
        :retry-label="
          isCatalogReloadPending
            ? t('workspace.catalog.reloading')
            : catalogHydrationFailed
              ? t('common.actions.retry')
              : t('workspace.catalog.reload')
        "
        :is-retrying="isCatalogReloadPending"
        @retry="reloadCatalog"
      />

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
            :scene-entry="sceneMap[board.id] ?? null"
            :workspace-id="workspaceId"
            :is-renaming="renamingBoardId === board.id"
            :rename-draft="renameDraft"
            :is-menu-open="activeMenuBoardId === board.id"
            :display-date="formatCatalogDate(board.updatedAt)"
            :current-user="userStore.user"
            @click-board="openBoardFromCard(board.id)"
            @toggle-menu="toggleBoardMenu(board.id)"
            @rename-action="startRename(board)"
            @duplicate-action="duplicateBoard(board.id)"
            @share-action="onShareBoardIntent(board.id)"
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

    <!-- 删除二次确认：统一走 ConfirmDialog surface="user" intent=danger（参考 LibraryFloatingIsland）。Busy 提交时组件内部锁住 mask/Escape/close-control。 -->
    <ConfirmDialog
      surface="user"
      :is-visible="deleteDialogVisible"
      :title="t('workspace.deleteDialog.title')"
      :message="
        t('workspace.deleteDialog.description', {
          name: pendingDeleteBoard?.name || t('board.untitledCanvas'),
        })
      "
      :confirm-label="t('common.actions.delete')"
      intent="danger"
      :is-loading="isDeletePending"
      @update:is-visible="handleDeleteConfirmVisibleUpdate"
      @confirm="confirmDelete"
    />

    <ShareDialog :panel="sharePanel" :current-user="userStore.user" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BoardCard from '../cards/BoardCard.vue'
import LoadErrorPanel from '../../../../../core/components/LoadErrorPanel.vue'
import ConfirmDialog from '../../../../../core/components/ConfirmDialog.vue'
import { catalogClasses } from '../styling/skin'
import { USER_CANVAS_PATH } from '../../../../workspace/routePaths'
import { WorkspaceHeader, useWorkspaceSelection } from '../../../../workspace'
import { Group, NavArrowDown, Search, User, Xmark } from '@iconoir/vue'
import { Button } from '@/components/ui/button'
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
import { appOverlayMenuSideOffset } from '../../../../../theme/appOverlayMenu'
import { createPageChromeRoundControlClass } from '../../../../../core/lib/pageChrome'
import { Spinner } from '@/components/ui/spinner'
import { formatCatalogDate, useCatalog, type CatalogSortMode } from '../controllers/useCatalog'
import ShareDialog from '../../share/ShareDialog.vue'
import { useSharePanel } from '../../../session/share/useSharePanel'
import { useUserStore } from '../../../../auth'

const router = useRouter()
const { t } = useI18n()
const rangeToggleControlClass = createPageChromeRoundControlClass()
const sharePanel = useSharePanel()
// Page seam owns auth + workspace reads; gallery leaves receive them as props.
const userStore = useUserStore()
// Fresh profiles need Workspace selection before catalog hydration.
useWorkspaceSelection()
const controller = useCatalog({
  navigation: {
    openBoard(boardId) {
      void router.push({ path: USER_CANVAS_PATH, query: { boardId } })
    },
    shareBoard(boardId) {
      onShareBoardIntent(boardId)
    },
  },
})
const {
  visibleBoards,
  sceneMap,
  workspaceId,
  isBoardsLoading,
  catalogHydrationFailed,
  searchKeyword,
  sortMode,
  range,
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
  setRange: onRangeCommand,
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

function onShareBoardIntent(boardId: string) {
  const board = controller.viewModel.visibleBoards.value.find((entry) => entry.id === boardId)
  if (!board) return
  sharePanel.open(board.id, board.name)
}

// ConfirmDialog 在 Busy 提交时已锁住全部关闭路径；页面只处理 idle
// 关闭后的清理（等价于旧 handleDeleteDialogAfterHide 的后半段，focus
// 恢复由组件内部 lifecycle 负责）。
function handleDeleteConfirmVisibleUpdate(isVisible: boolean) {
  if (isVisible || isDeletePending.value) return
  closeDeleteDialog()
  handleDeleteDialogHidden()
}

const sortOptions = computed<Array<{ label: string; value: CatalogSortMode }>>(() => [
  { label: t('board.sortRecent'), value: 'recent' },
  { label: t('board.sortName'), value: 'name' },
])

const selectedSortLabel = computed(
  () => sortOptions.value.find((option) => option.value === sortMode.value)?.label,
)

// Select emits a loose value; only accept one the option list actually offers so
// a stray emission cannot put the catalog into a mode it has no label for.
function onSortModeChange(value: unknown) {
  const match = sortOptions.value.find((option) => option.value === value)
  if (match) onSortModeCommand(match.value)
}

// The sort trigger is a real form control, so it needs a programmatic label.
const sortSelectId = 'board-catalog-sort-select'
</script>
