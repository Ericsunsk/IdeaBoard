import type { ComputedRef, Ref } from 'vue'
import type { BoardListSceneEntry } from '../../../session/sync/boardQueries'
import type { BoardGalleryRange } from '../range/range'

/**
 * Server-computed capabilities.
 * @externalContract Public API BoardCapabilities: consumed verbatim from
 * catalog payloads, never recomputed locally.
 */
type GalleryBoardCapabilities = {
  read: boolean
  write: boolean
  manage: boolean
  generate: boolean
}

/** Derived sharing posture, mirrored from the Public API schema. */
type GalleryBoardAccessState = 'private' | 'workspace'

/** Common sharing gate and role for selected members. */
type GalleryBoardWorkspaceAccess = 'none' | 'viewer' | 'editor'

export type CatalogEntry = {
  id: string
  name: string
  createdAt: number
  updatedAt: number
  revision?: number | undefined
  capabilities?: GalleryBoardCapabilities | undefined
  accessState?: GalleryBoardAccessState | undefined
  workspaceAccess?: GalleryBoardWorkspaceAccess | undefined
  isCreator?: boolean | undefined
  hasDirectGrant?: boolean | undefined
  /**
   * Creator User public identity (usr_ opaque), display-only owner fact
   * carried verbatim from the board payload; absent on local drafts.
   */
  creatorUserId?: string | undefined
  /**
   * Workspace-visible owner avatar public URL (display-only); absent when
   * the server omits it or on local drafts.
   */
  ownerAvatarUrl?: string | undefined
}

export type CatalogSortMode = 'recent' | 'name'

export type CatalogNavigation = {
  openBoard(boardId: string): void
  shareBoard?(boardId: string): void
}

type CatalogRangeCounts = {
  personal: number
  team: number
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
    range: Readonly<Ref<BoardGalleryRange>>
    rangeCounts: ComputedRef<CatalogRangeCounts>
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
    setRange(range: BoardGalleryRange): void
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
