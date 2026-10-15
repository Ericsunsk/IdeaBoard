import type { ComputedRef, Ref } from 'vue'
import type { BoardListSceneEntry } from '../../session/sync/boardQueries'

export type CatalogEntry = {
  id: string
  name: string
  createdAt: number
  updatedAt: number
}

export type CatalogSortMode = 'recent' | 'name'

export type CatalogNavigation = {
  openBoard(boardId: string): void
}

export type Catalog = {
  viewModel: {
    visibleBoards: ComputedRef<readonly CatalogEntry[]>
    sceneMap: Readonly<Ref<Record<string, BoardListSceneEntry>>>
    workspaceId: Readonly<Ref<string | null>>
    isBoardsLoading: Readonly<Ref<boolean>>
    catalogHydrationFailed: Readonly<Ref<boolean>>
    searchKeyword: Readonly<Ref<string>>
    sortMode: Readonly<Ref<CatalogSortMode>>
    activeMenuBoardId: Readonly<Ref<string | null>>
    renamingBoardId: Readonly<Ref<string | null>>
    renameDraft: Readonly<Ref<string>>
    catalogRecoveryBoardId: Readonly<Ref<string | null>>
    isCatalogReloadPending: Readonly<Ref<boolean>>
    deleteDialogVisible: Readonly<Ref<boolean>>
    pendingDeleteBoard: Readonly<Ref<CatalogEntry | null>>
    isDeletePending: Readonly<Ref<boolean>>
  }
  commands: {
    initialize(): Promise<void>
    setSearchKeyword(value: string | undefined): void
    setSortMode(mode: CatalogSortMode): void
    toggleBoardMenu(boardId: string): void
    handleBoardMenuHidden(boardId: string): void
    createAndOpenBoard(): Promise<void>
    duplicateBoard(boardId: string): Promise<void>
    startRename(board: CatalogEntry): void
    updateRenameDraft(nextName: string): void
    commitRename(): Promise<void>
    cancelRename(): void
    reloadCatalog(): Promise<void>
    openBoardFromCard(boardId: string): void
    requestDelete(board: CatalogEntry): void
    closeDeleteDialog(): void
    handleDeleteDialogHidden(): void
    confirmDelete(): Promise<void>
  }
  dispose(): void
}
