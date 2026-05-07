import { computed, nextTick, ref } from 'vue'
import { listCanvasBoards } from '../../boards/lib/canvasBoards'

type BoardMeta = ReturnType<typeof listCanvasBoards>[number]

export function useFlowBoardHeaderState(options: {
  currentBoardId: { value: string | null }
}) {
  const { currentBoardId } = options
  const isBoardMenuOpen = ref(false)
  const isRenamingBoard = ref(false)
  const boardNameDraft = ref('')
  const boardNameInputRef = ref<HTMLInputElement | null>(null)
  const boardMetas = ref<BoardMeta[]>(listCanvasBoards())

  const currentBoardName = computed(() => {
    const boardId = `${currentBoardId.value ?? ''}`.trim()
    if (!boardId) return 'Untitled Canvas'
    return boardMetas.value.find(board => board.id === boardId)?.name ?? boardId
  })

  const boardOptions = computed(() =>
    boardMetas.value.map(board => ({ id: board.id, name: board.name })),
  )

  function setBoardNameInputRef(el: HTMLInputElement | null) {
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
    if (!currentBoardId.value) return
    isRenamingBoard.value = true
    isBoardMenuOpen.value = false
    boardNameDraft.value = currentBoardName.value
    void nextTick(() => {
      boardNameInputRef.value?.focus()
      boardNameInputRef.value?.select()
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

  return {
    boardMetas,
    boardNameDraft,
    isBoardMenuOpen,
    isRenamingBoard,
    currentBoardName,
    boardOptions,
    boardNameInputRef,
    setBoardNameInputRef,
    refreshBoardMetas,
    toggleBoardMenu,
    startBoardRename,
    cancelBoardRename,
    closeBoardHeaderEditing,
  }
}
