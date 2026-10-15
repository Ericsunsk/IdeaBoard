import { computed, nextTick, ref, watch, type Ref } from 'vue'
import type { Router } from 'vue-router'
import {
  fetchBoardArchivedAt,
  isBoardArchivedError,
  markBoardArchivedLocally,
  normalizeBoardName,
  syncRestoreBoardToServer,
  useBoardRepository,
  type BoardRepository,
} from '../../../board/ports/canvasSession'
import { USER_CANVAS_PATH } from '../../../workspace/routePaths'
import { i18n } from '../../../../core/i18n'

type UseBoardHeaderActionsOptions = {
  currentBoardId: Ref<string | null | undefined>
  message: {
    warning: (msg: string) => void
    success: (msg: string) => void
    error?: (msg: string) => void
  }
  router: Router
  route: { query: Record<string, unknown> }
  flushAutosave: () => Promise<void>
  catalog?: Pick<BoardRepository, 'viewModel' | 'commands'>
}

export type CatalogRecoveryState =
  { status: 'idle' } | { status: 'reload-required' | 'reloading'; boardId: string }

export function useBoardHeaderActions(options: UseBoardHeaderActionsOptions) {
  const { t } = i18n.global
  const boardRepository = options.catalog ?? useBoardRepository()
  const isBoardMenuOpen = ref(false)
  const isRenamingBoard = ref(false)
  const boardNameDraft = ref('')
  const boardNameInputRef = ref<HTMLElement | null>(null)
  const isRenamePending = ref(false)
  const catalogRecovery = ref<CatalogRecoveryState>({ status: 'idle' })
  const isBoardArchived = ref(false)
  const isRestoringArchivedBoard = ref(false)

  // Archived banner source: the board list excludes archived boards, so the
  // flag is resolved per-board via a direct meta read. Fail-closed and
  // silent: when the read fails (no workspace, offline, unknown board) the
  // banner stays hidden and the save-409 path discovers archived state.
  let archivedCheckSequence = 0
  async function refreshArchivedState(boardId?: string | null): Promise<boolean> {
    const normalized = `${boardId ?? options.currentBoardId.value ?? ''}`.trim()
    if (!normalized) {
      isBoardArchived.value = false
      return false
    }
    const checkToken = ++archivedCheckSequence
    try {
      const archivedAt = await fetchBoardArchivedAt(normalized)
      if (checkToken !== archivedCheckSequence) return isBoardArchived.value
      isBoardArchived.value = archivedAt != null
      return isBoardArchived.value
    } catch {
      return isBoardArchived.value
    }
  }

  /** Save-409 fast path: banner the board without the generic toast (dedupe: no-op when shown). */
  function markArchivedFromSave(boardId: string): void {
    const normalized = boardId.trim()
    if (!normalized) return
    markBoardArchivedLocally(normalized)
    isBoardArchived.value = true
  }

  async function restoreArchivedBoard(): Promise<boolean> {
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId || isRestoringArchivedBoard.value) return false
    isRestoringArchivedBoard.value = true
    try {
      await syncRestoreBoardToServer(boardId)
      isBoardArchived.value = false
      options.message.success(t('canvas.chrome.boardRestoredSuccess'))
      return true
    } catch (error) {
      console.warn('[canvas] failed to restore archived board', error)
      options.message.error?.(t('canvas.chrome.boardRestoreFailedError'))
      return false
    } finally {
      isRestoringArchivedBoard.value = false
    }
  }

  watch(
    () => options.currentBoardId.value,
    (boardId) => {
      isBoardArchived.value = false
      void refreshArchivedState(boardId)
    },
    { immediate: true },
  )

  const currentBoardName = computed(() => {
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) return t('canvas.chrome.noCanvas')
    return boardRepository.viewModel.byId.value.get(boardId)?.name ?? boardId
  })

  const boardOptions = computed(() =>
    boardRepository.viewModel.boards.value.map((board) => ({ id: board.id, name: board.name })),
  )

  function setBoardNameInputRef(el: HTMLElement | null) {
    boardNameInputRef.value = el
  }

  function toggleBoardMenu() {
    isBoardMenuOpen.value = !isBoardMenuOpen.value
  }

  function startBoardRename() {
    if (isBoardArchived.value) {
      options.message.warning(t('canvas.chrome.boardArchivedRenameBlocked'))
      return
    }
    if (isRenamePending.value) {
      options.message.warning(t('canvas.chrome.renamePendingWarning'))
      return
    }
    if (!options.currentBoardId.value) return
    isRenamingBoard.value = true
    isBoardMenuOpen.value = false
    boardNameDraft.value = currentBoardName.value
    void nextTick(() => {
      const titleElement = boardNameInputRef.value
      if (!titleElement) return
      titleElement.textContent = boardNameDraft.value
      titleElement.focus()
      const range = document.createRange()
      range.selectNodeContents(titleElement)
      range.collapse(false)
      const selection = window.getSelection()
      selection?.removeAllRanges()
      selection?.addRange(range)
    })
  }

  function cancelBoardRename() {
    isRenamingBoard.value = false
    boardNameDraft.value = ''
  }

  function closeBoardHeaderEditing() {
    isBoardMenuOpen.value = false
    cancelBoardRename()
  }

  type BoardRenameOutcome = Awaited<ReturnType<typeof boardRepository.commands.rename>>

  function isSyncConflictReason(reason: string): boolean {
    return reason === 'revision-conflict' || reason === 'idempotency-conflict'
  }

  function selectRenameRestoreErrorMessage(reason: string, isRollbackSucceeded: boolean): string {
    if (isSyncConflictReason(reason) && isRollbackSucceeded) {
      return t('canvas.chrome.conflictRestoredRefreshFailedError')
    }
    if (isSyncConflictReason(reason)) {
      return t('canvas.chrome.conflictRestoreFailedError')
    }
    return t('canvas.chrome.syncFailedRestoreFailedError')
  }

  function reportBoardRenameFailure(
    result: Extract<BoardRenameOutcome, { isOk: false }>,
    boardId: string,
  ): void {
    if (result.reason === 'local-name-conflict') {
      options.message.warning(t('canvas.chrome.duplicateNameWarning'))
      return
    }
    if (result.reason === 'invalid-name') {
      options.message.warning(t('canvas.chrome.renameFailedWarning'))
      return
    }
    if (result.reason === 'scope-changed') return
    if (isBoardArchivedError(result.error)) {
      markArchivedFromSave(boardId)
      options.message.error?.(t('canvas.chrome.boardArchivedRenameBlocked'))
      console.warn('[canvas] failed to rename archived board', result.error)
      return
    }
    if (!('rollback' in result)) return
    const isHydrateSucceeded = result.hydrate?.isOk === true
    const isRollbackSucceeded = result.rollback.isOk
    if (isSyncConflictReason(result.reason) && isHydrateSucceeded) {
      catalogRecovery.value = { status: 'idle' }
      options.message.error?.(t('canvas.chrome.conflictReloadedError'))
    } else if (isRollbackSucceeded && result.reason === 'sync-failed') {
      catalogRecovery.value = { status: 'idle' }
      options.message.error?.(t('canvas.chrome.syncFailedRestoredError'))
    } else {
      catalogRecovery.value = { status: 'reload-required', boardId }
      options.message.error?.(selectRenameRestoreErrorMessage(result.reason, isRollbackSucceeded))
    }
    console.warn('[canvas] failed to sync renamed board', result.error)
  }

  async function onConfirmBoardRename() {
    if (isRenamePending.value) return
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) return
    if (isBoardArchived.value) {
      isRenamingBoard.value = false
      boardNameDraft.value = ''
      options.message.warning(t('canvas.chrome.boardArchivedRenameBlocked'))
      return
    }
    const normalized = normalizeBoardName(boardNameDraft.value)
    const currentName = normalizeBoardName(currentBoardName.value)
    isRenamingBoard.value = false
    boardNameDraft.value = ''
    if (!normalized) {
      options.message.warning(t('canvas.chrome.emptyNameWarning'))
      return
    }
    if (normalized === currentName) return

    isRenamePending.value = true
    const result = await boardRepository.commands
      .rename({ boardId, nextName: normalized })
      .finally(() => {
        isRenamePending.value = false
      })
    if (result.isOk) {
      options.message.success(t('canvas.chrome.renameSuccess'))
      return
    }
    reportBoardRenameFailure(result, boardId)
  }

  async function reloadCatalog() {
    const recovery = catalogRecovery.value
    if (recovery.status !== 'reload-required') return
    catalogRecovery.value = { status: 'reloading', boardId: recovery.boardId }
    const result = await boardRepository.commands.hydrate({
      shouldPreserveLocalNewer: false,
      rollbackBoardIds: [recovery.boardId],
    })
    if (result.isOk) {
      catalogRecovery.value = { status: 'idle' }
      options.message.success(t('canvas.chrome.catalogReloadedSuccess'))
      return
    }
    catalogRecovery.value = { status: 'reload-required', boardId: recovery.boardId }
    options.message.error?.(t('canvas.chrome.catalogReloadFailedError'))
  }

  function onSelectBoard(boardId: string) {
    if (!boardId) return
    isBoardMenuOpen.value = false
    if (boardId === options.currentBoardId.value) return
    void navigateToBoard(boardId)
  }

  async function navigateToBoard(boardId: string) {
    await options.flushAutosave()
    await options.router.push({
      path: USER_CANVAS_PATH,
      query: { ...options.route.query, boardId },
    })
  }

  return {
    isBoardMenuOpen,
    isRenamingBoard,
    boardNameDraft,
    boardNameInputRef,
    isRenamePending,
    catalogRecovery,
    isBoardArchived,
    isRestoringArchivedBoard,
    refreshArchivedState,
    markArchivedFromSave,
    restoreArchivedBoard,
    currentBoardName,
    boardOptions,
    setBoardNameInputRef,
    toggleBoardMenu,
    startBoardRename,
    cancelBoardRename,
    closeBoardHeaderEditing,
    onConfirmBoardRename,
    reloadCatalog,
    onSelectBoard,
    navigateToBoard,
  }
}
