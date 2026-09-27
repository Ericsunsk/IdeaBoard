import type {
  UserAccountMenuItem,
  UserIdeaBalanceControl,
} from '../../../../../shared/components/userAccountMenu.types'

export type CanvasChromeBoardOption = {
  id: string
  name: string
}

export type CanvasChromeCatalogRecoveryState =
  { status: 'idle' } | { status: 'reload-required' | 'reloading'; boardId: string }

export type CanvasChromeArchivedState = {
  isArchived: boolean
  isRestoring: boolean
}

export type CanvasChromeBoardState = {
  currentName: string
  options: CanvasChromeBoardOption[]
  currentId: string | null
  isMenuOpen: boolean
  isRenaming: boolean
  isRenamePending: boolean
  catalogRecovery: CanvasChromeCatalogRecoveryState
  archived: CanvasChromeArchivedState
  setNameInputRef: (el: HTMLElement | null) => void
}

export type CanvasChromeAccountState = {
  avatarUrl?: string | undefined
  balance: UserIdeaBalanceControl
  menuItems: UserAccountMenuItem[]
}

export type CanvasChromeToolState = {
  activeTool: 'select' | 'hand'
  libraryMounted: boolean
  libraryOpen: boolean
  minimapEnabled: boolean
}

export type CanvasChromeStatusState = {
  exportDisabled: boolean
  zoomLevel: number
}
