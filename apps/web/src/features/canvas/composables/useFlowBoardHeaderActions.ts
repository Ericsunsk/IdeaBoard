import { computed, nextTick, ref, type Ref } from 'vue'
import type { Router } from 'vue-router'
import { listCanvasBoards, normalizeCanvasBoardName } from '../../canvas-board/catalog/canvasBoards'
import {
  createCanvasBoardCommands,
  type CanvasBoardCommands,
  type CanvasBoardRenameRemoteResult,
} from '../../canvas-board/catalog/canvasBoardCommands'
import { USER_CANVAS_PATH } from '../../../shared/constants/routePaths'

type BoardMeta = { id: string; name: string }

export type BoardRenameRecoveryState =
  | { status: 'idle' }
  | {
      status: 'pending' | 'failed' | 'conflict'
      boardId: string
      previousName: string
      nextName: string
    }

type ActiveBoardRenameOperation = {
  version: number
  boardId: string
  previousName: string
  nextName: string
  confirm: () => Promise<CanvasBoardRenameRemoteResult>
}

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
  boardCommands?: Pick<CanvasBoardCommands, 'renameOptimistic' | 'rollbackOptimisticRename'>
}

export function useFlowBoardHeaderActions(options: UseFlowBoardHeaderActionsOptions) {
  const boardCommands = options.boardCommands ?? createCanvasBoardCommands()
  const isBoardMenuOpen = ref(false)
  const isRenamingBoard = ref(false)
  const boardNameDraft = ref('')
  const boardNameInputRef = ref<HTMLElement | null>(null)
  const boardMetas = ref<BoardMeta[]>(listCanvasBoards())
  const boardRenameRecovery = ref<BoardRenameRecoveryState>({ status: 'idle' })
  let renameOperationVersion = 0
  let activeRenameOperation: ActiveBoardRenameOperation | null = null

  const currentBoardName = computed(() => {
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) return 'Untitled Canvas'
    return boardMetas.value.find((board) => board.id === boardId)?.name ?? boardId
  })

  const boardOptions = computed(() =>
    boardMetas.value.map((board) => ({ id: board.id, name: board.name })),
  )

  function setBoardNameInputRef(el: HTMLElement | null) {
    boardNameInputRef.value = el
  }

  function refreshBoardMetas() {
    boardMetas.value = listCanvasBoards()
  }

  function setIdleRenameRecovery() {
    boardRenameRecovery.value = { status: 'idle' }
  }

  function toRecoveryState(
    status: Exclude<BoardRenameRecoveryState['status'], 'idle'>,
    operation: ActiveBoardRenameOperation,
  ): BoardRenameRecoveryState {
    return {
      status,
      boardId: operation.boardId,
      previousName: operation.previousName,
      nextName: operation.nextName,
    }
  }

  function matchesActiveOperation(operation: ActiveBoardRenameOperation): boolean {
    return activeRenameOperation?.version === operation.version
  }

  async function confirmRemoteBoardRename(operation: ActiveBoardRenameOperation) {
    const result = await operation.confirm()
    if (!matchesActiveOperation(operation)) return
    if (result.ok) {
      activeRenameOperation = null
      setIdleRenameRecovery()
      options.message.success('已重命名画布')
      return
    }

    const isConflict = result.reason === 'remote-name-conflict'
    boardRenameRecovery.value = toRecoveryState(isConflict ? 'conflict' : 'failed', operation)
    options.message.error?.(
      isConflict ? '名称冲突，请更换名称或撤回' : '重命名未同步，可重试或撤回',
    )
    console.warn('[canvas] failed to sync renamed board', result.error)
  }

  function toggleBoardMenu() {
    if (!isBoardMenuOpen.value) refreshBoardMetas()
    isBoardMenuOpen.value = !isBoardMenuOpen.value
  }

  function startBoardRename() {
    if (boardRenameRecovery.value.status === 'pending') {
      options.message.warning('画布名称正在同步，请稍后再改名')
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
    const boardId = `${options.currentBoardId.value ?? ''}`.trim()
    if (!boardId) return
    const normalized = normalizeCanvasBoardName(boardNameDraft.value)
    const currentName = normalizeCanvasBoardName(currentBoardName.value)
    isRenamingBoard.value = false
    boardNameDraft.value = ''
    if (!normalized) {
      options.message.warning('请输入画布名称')
      return
    }
    if (normalized === currentName) {
      return
    }
    const result = boardCommands.renameOptimistic({ boardId, nextName: normalized })
    if (!result.ok) {
      switch (result.reason) {
        case 'local-name-conflict':
          options.message.warning('名称已存在')
          return
        case 'invalid-name':
          options.message.warning('重命名失败')
          return
      }
    }

    const operation: ActiveBoardRenameOperation = {
      version: (renameOperationVersion += 1),
      boardId,
      previousName: result.previousBoard.name,
      nextName: result.board.name,
      confirm: result.confirm,
    }
    activeRenameOperation = operation
    boardRenameRecovery.value = toRecoveryState('pending', operation)
    refreshBoardMetas()
    await confirmRemoteBoardRename(operation)
  }

  async function retryBoardRename() {
    const recovery = boardRenameRecovery.value
    if (recovery.status !== 'failed') return
    const operation = activeRenameOperation
    if (
      !operation ||
      operation.boardId !== recovery.boardId ||
      operation.nextName !== recovery.nextName
    ) {
      options.message.warning('没有可重试的重命名')
      return
    }
    boardRenameRecovery.value = toRecoveryState('pending', operation)
    await confirmRemoteBoardRename(operation)
  }

  function rollbackBoardRename() {
    const recovery = boardRenameRecovery.value
    if (recovery.status === 'idle') return
    if (recovery.status === 'pending') {
      options.message.warning('画布名称正在同步，请稍后再撤回')
      return
    }
    const result = boardCommands.rollbackOptimisticRename({
      boardId: recovery.boardId,
      previousName: recovery.previousName,
    })
    if (!result.ok) {
      options.message.error?.(
        result.reason === 'local-name-conflict' ? '原名称已被占用，无法撤回' : '撤回重命名失败',
      )
      return
    }
    if (activeRenameOperation?.boardId === recovery.boardId) {
      activeRenameOperation = null
    }
    setIdleRenameRecovery()
    refreshBoardMetas()
    options.message.success('已撤回重命名')
  }

  function onSelectBoard(boardId: string) {
    if (!boardId) return
    refreshBoardMetas()
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
    boardMetas,
    boardRenameRecovery,
    currentBoardName,
    boardOptions,
    setBoardNameInputRef,
    refreshBoardMetas,
    toggleBoardMenu,
    startBoardRename,
    cancelBoardRename,
    closeBoardHeaderEditing,
    onConfirmBoardRename,
    retryBoardRename,
    rollbackBoardRename,
    onSelectBoard,
    navigateToBoard,
  }
}
