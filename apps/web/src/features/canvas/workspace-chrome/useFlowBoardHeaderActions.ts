import { computed, nextTick, ref, type Ref } from 'vue'
import type { Router } from 'vue-router'
import { normalizeCanvasBoardName } from '../../canvas-board/catalog/canvasBoards'
import {
  useCanvasBoardCatalog,
  type CanvasBoardCatalog,
} from '../../canvas-board/catalog/canvasBoardCatalog'
import { USER_CANVAS_PATH } from '../../../shared/constants/routePaths'
import { i18n } from '../../../shared/i18n'

type UseFlowBoardHeaderActionsOptions = {
  currentBoardId: Ref<string | null | undefined>
  message: {
    warning: (msg: string) => void
    success: (msg: string) => void
    error?: (msg: string) => void
  }
  router: Router
  route: { query: Record<string, unknown> }
  flushAutosave: () => Promise<void>
  catalog?: Pick<CanvasBoardCatalog, 'viewModel' | 'commands'>
}

export type CanvasBoardCatalogRecoveryState =
  { status: 'idle' } | { status: 'reload-required' | 'reloading'; boardId: string }

export function useFlowBoardHeaderActions(options: UseFlowBoardHeaderActionsOptions) {
  const { t } = i18n.global
  const boardCatalog = options.catalog ?? useCanvasBoardCatalog()
  const isBoardMenuOpen = ref(false)
  const isRenamingBoard = ref(false)
  const boardNameDraft = ref('')
  const boardNameInputRef = ref<HTMLElement | null>(null)
  const isRenamePending = ref(false)
  const catalogRecovery = ref<CanvasBoardCatalogRecoveryState>({ status: 'idle' })

  const currentBoardName = computed(() => {
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) return 'Untitled Canvas'
    return boardCatalog.viewModel.byId.value.get(boardId)?.name ?? boardId
  })

  const boardOptions = computed(() =>
    boardCatalog.viewModel.boards.value.map((board) => ({ id: board.id, name: board.name })),
  )

  function setBoardNameInputRef(el: HTMLElement | null) {
    boardNameInputRef.value = el
  }

  function toggleBoardMenu() {
    isBoardMenuOpen.value = !isBoardMenuOpen.value
  }

  function startBoardRename() {
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

  async function onConfirmBoardRename() {
    if (isRenamePending.value) return
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) return
    const normalized = normalizeCanvasBoardName(boardNameDraft.value)
    const currentName = normalizeCanvasBoardName(currentBoardName.value)
    isRenamingBoard.value = false
    boardNameDraft.value = ''
    if (!normalized) {
      options.message.warning(t('canvas.chrome.emptyNameWarning'))
      return
    }
    if (normalized === currentName) return

    isRenamePending.value = true
    const result = await boardCatalog.commands
      .rename({ boardId, nextName: normalized })
      .finally(() => {
        isRenamePending.value = false
      })
    if (result.ok) {
      options.message.success(t('canvas.chrome.renameSuccess'))
      return
    }
    if (result.reason === 'local-name-conflict') {
      options.message.warning(t('canvas.chrome.duplicateNameWarning'))
      return
    }
    if (result.reason === 'invalid-name') {
      options.message.warning(t('canvas.chrome.renameFailedWarning'))
      return
    }
    if (result.reason === 'scope-changed') return
    if (!('rollback' in result)) return
    const hydrateSucceeded = result.hydrate?.ok === true
    const rollbackSucceeded = result.rollback.ok
    if (result.reason === 'remote-name-conflict' && hydrateSucceeded) {
      catalogRecovery.value = { status: 'idle' }
      options.message.error?.(t('canvas.chrome.conflictReloadedError'))
    } else if (rollbackSucceeded && result.reason === 'sync-failed') {
      catalogRecovery.value = { status: 'idle' }
      options.message.error?.(t('canvas.chrome.syncFailedRestoredError'))
    } else {
      catalogRecovery.value = { status: 'reload-required', boardId }
      options.message.error?.(
        result.reason === 'remote-name-conflict' && rollbackSucceeded
          ? t('canvas.chrome.conflictRestoredRefreshFailedError')
          : result.reason === 'remote-name-conflict'
            ? t('canvas.chrome.conflictRestoreFailedError')
            : t('canvas.chrome.syncFailedRestoreFailedError'),
      )
    }
    console.warn('[canvas] failed to sync renamed board', result.error)
  }

  async function reloadCatalog() {
    const recovery = catalogRecovery.value
    if (recovery.status !== 'reload-required') return
    catalogRecovery.value = { status: 'reloading', boardId: recovery.boardId }
    const result = await boardCatalog.commands.hydrate({
      preserveLocalNewer: false,
      rollbackBoardIds: [recovery.boardId],
    })
    if (result.ok) {
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
