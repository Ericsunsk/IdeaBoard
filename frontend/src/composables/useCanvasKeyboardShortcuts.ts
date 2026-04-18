import { onMounted, onUnmounted, type Ref } from 'vue'

interface UseCanvasKeyboardShortcutsOptions {
  isLeaveConfirmVisible: () => boolean
  resolveLeaveConfirm: (decision: 'save' | 'discard' | 'cancel') => void
  isNodeContextMenuVisible: () => boolean
  closeNodeContextMenu: () => void
  hasSelectedConnection: () => boolean
  clearSelectedConnection: () => void
  activeImageActionId: Ref<number | null>
  showStyleStrengthControl: Ref<boolean>
  clearCanvasQuickActionState: () => void
  hasActiveNodeSelection: () => boolean
  clearActiveNodeSelection: () => void
  selectedIds: Ref<number[]>
  clearSelectedIds: () => void
  deleteSelectedConnection: () => boolean
  deleteActiveNode: () => boolean
  deleteSelectedCanvasImages: () => void
  isSpacePressed: Ref<boolean>
  isDuplicateModifierPressed: Ref<boolean>
  undo: () => Promise<boolean> | boolean
  redo: () => Promise<boolean> | boolean
}

function isInputLike(target: EventTarget | null): target is HTMLInputElement | HTMLTextAreaElement {
  return target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (isInputLike(target)) return true
  if (!(target instanceof HTMLElement)) return false
  return !!target.closest('input, textarea, [contenteditable="true"]')
}

export function useCanvasKeyboardShortcuts(options: UseCanvasKeyboardShortcutsOptions) {
  function handleKey(e: KeyboardEvent) {
    if (options.isLeaveConfirmVisible()) {
      if (e.type === 'keydown' && e.key === 'Escape') {
        options.resolveLeaveConfirm('cancel')
      }
      return
    }

    if (e.type === 'keydown' && e.key === 'Escape') {
      if (isEditableTarget(e.target)) return
      e.preventDefault()

      if (options.isNodeContextMenuVisible()) {
        options.closeNodeContextMenu()
        return
      }
      if (options.hasSelectedConnection()) {
        options.clearSelectedConnection()
        return
      }
      if (options.activeImageActionId.value !== null || options.showStyleStrengthControl.value) {
        options.clearCanvasQuickActionState()
        return
      }
      if (options.hasActiveNodeSelection()) {
        options.clearActiveNodeSelection()
        return
      }
      if (options.selectedIds.value.length > 0) {
        options.clearSelectedIds()
      }
      return
    }

    if (
      e.type === 'keydown' &&
      (e.metaKey || e.ctrlKey) &&
      e.shiftKey &&
      e.key.toLowerCase() === 'z'
    ) {
      if (isEditableTarget(e.target)) return
      e.preventDefault()
      void options.redo()
      return
    }

    if (
      e.type === 'keydown' &&
      (e.metaKey || e.ctrlKey) &&
      !e.shiftKey &&
      e.key.toLowerCase() === 'z'
    ) {
      if (isEditableTarget(e.target)) return
      e.preventDefault()
      void options.undo()
      return
    }

    if (e.code === 'Space') {
      options.isSpacePressed.value = e.type === 'keydown'
    }

    if (e.key === 'Alt') {
      options.isDuplicateModifierPressed.value = e.type === 'keydown'
    }

    if (e.key === 'Delete' || e.key === 'Backspace') {
      if (isEditableTarget(e.target)) return
      if (options.deleteSelectedConnection()) {
        options.closeNodeContextMenu()
        options.clearCanvasQuickActionState()
        return
      }
      if (options.deleteActiveNode()) {
        options.closeNodeContextMenu()
        options.clearCanvasQuickActionState()
        return
      }
      if (options.selectedIds.value.length > 0) {
        options.deleteSelectedCanvasImages()
        options.clearCanvasQuickActionState()
      }
    }
  }

  function handleSelectStart(e: Event) {
    const target = e.target as HTMLElement | null
    if (!target?.closest('.canvas-layout')) return
    if (isEditableTarget(target)) return
    e.preventDefault()
  }

  function handleWindowBlur() {
    options.isSpacePressed.value = false
    options.isDuplicateModifierPressed.value = false
  }

  onMounted(() => {
    window.addEventListener('keydown', handleKey)
    window.addEventListener('keyup', handleKey)
    window.addEventListener('selectstart', handleSelectStart)
    window.addEventListener('blur', handleWindowBlur)
  })

  onUnmounted(() => {
    window.removeEventListener('keydown', handleKey)
    window.removeEventListener('keyup', handleKey)
    window.removeEventListener('selectstart', handleSelectStart)
    window.removeEventListener('blur', handleWindowBlur)
  })

  return {
    handleKey,
    handleSelectStart,
    handleWindowBlur,
  }
}
