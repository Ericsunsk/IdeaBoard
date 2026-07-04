import { computed, nextTick, ref, type Ref } from 'vue'
import type { Router } from 'vue-router'
import { listCanvasBoards, normalizeCanvasBoardName } from '../../canvas-board/catalog/canvasBoards'
import {
  createCanvasBoardCommands,
  type CanvasBoardCommands,
} from '../../canvas-board/catalog/canvasBoardCommands'
import { USER_CANVAS_PATH } from '../../../shared/constants/routePaths'

type BoardMeta = { id: string; name: string }

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
  boardCommands?: Pick<CanvasBoardCommands, 'rename'>
}

export function useFlowBoardHeaderActions(options: UseFlowBoardHeaderActionsOptions) {
  const boardCommands = options.boardCommands ?? createCanvasBoardCommands()
  const isBoardMenuOpen = ref(false)
  const isRenamingBoard = ref(false)
  const boardNameDraft = ref('')
  const boardNameInputRef = ref<HTMLElement | null>(null)
  const boardMetas = ref<BoardMeta[]>(listCanvasBoards())

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

  function toggleBoardMenu() {
    if (!isBoardMenuOpen.value) refreshBoardMetas()
    isBoardMenuOpen.value = !isBoardMenuOpen.value
  }

  function startBoardRename() {
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
    const result = await boardCommands.rename({ boardId, nextName: normalized })
    if (!result.ok) {
      switch (result.reason) {
        case 'local-name-conflict':
          options.message.warning('名称已存在')
          return
        case 'invalid-name':
          options.message.warning('重命名失败')
          return
        case 'remote-name-conflict':
        case 'sync-failed':
          refreshBoardMetas()
          options.message.error?.(
            result.reason === 'remote-name-conflict'
              ? '名称已存在，请更换后再试'
              : '重命名同步失败，请稍后重试',
          )
          console.warn('[canvas] failed to sync renamed board', result.error)
          return
      }
    }
    refreshBoardMetas()
    options.message.success('已重命名画布')
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
    currentBoardName,
    boardOptions,
    setBoardNameInputRef,
    refreshBoardMetas,
    toggleBoardMenu,
    startBoardRename,
    cancelBoardRename,
    closeBoardHeaderEditing,
    onConfirmBoardRename,
    onSelectBoard,
    navigateToBoard,
  }
}
