import type { ComputedRef, Ref } from 'vue'

export type BoardCatalogEntry = {
  id: string
  name: string
  createdAt: number
  updatedAt: number
}

export type BoardCatalogSortMode = 'recent' | 'name'

export type BoardsCatalogNavigation = {
  openBoard(boardId: string): void
}

export type BoardsCatalog = {
  viewModel: {
    visibleBoards: ComputedRef<readonly BoardCatalogEntry[]>
    previewMap: Readonly<Ref<Record<string, string>>>
    isBoardsLoading: Readonly<Ref<boolean>>
    searchKeyword: Readonly<Ref<string>>
    sortMode: Readonly<Ref<BoardCatalogSortMode>>
    activeMenuBoardId: Readonly<Ref<string | null>>
    renamingBoardId: Readonly<Ref<string | null>>
    renameDraft: Readonly<Ref<string>>
    catalogRecoveryBoardId: Readonly<Ref<string | null>>
    isCatalogReloadPending: Readonly<Ref<boolean>>
    deleteDialogVisible: Readonly<Ref<boolean>>
    pendingDeleteBoard: Readonly<Ref<BoardCatalogEntry | null>>
    isDeletePending: Readonly<Ref<boolean>>
  }
  commands: {
    initialize(): Promise<void>
    setSearchKeyword(value: string | undefined): void
    setSortMode(mode: BoardCatalogSortMode): void
    toggleBoardMenu(boardId: string): void
    handleBoardMenuHidden(boardId: string): void
    createAndOpenBoard(): Promise<void>
    duplicateBoard(boardId: string): Promise<void>
    startRename(board: BoardCatalogEntry): void
    updateRenameDraft(nextName: string): void
    commitRename(): Promise<void>
    cancelRename(): void
    reloadCatalog(): Promise<void>
    openBoardFromCard(boardId: string): void
    requestDelete(board: BoardCatalogEntry): void
    closeDeleteDialog(): void
    handleDeleteDialogHidden(): void
    confirmDelete(): Promise<void>
  }
  dispose(): void
}
