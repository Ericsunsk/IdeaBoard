import type {
  UserAccountMenuItem,
  UserIdeaBalanceControl,
} from '../../../../account/ui/chrome/userAccountMenu.types'

type CanvasChromeBoardOption = {
  id: string
  name: string
}

type CanvasChromeCatalogRecoveryState =
  { status: 'idle' } | { status: 'reload-required' | 'reloading'; boardId: string }

type CanvasChromeArchivedState = {
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
  userEmail?: string | undefined
  balance: UserIdeaBalanceControl
  menuItems: UserAccountMenuItem[]
}

export type CanvasChromeToolState = {
  activeTool: 'select' | 'hand'
  isLibraryMounted: boolean
  isLibraryOpen: boolean
  isMinimapEnabled: boolean
}

export type CanvasChromeStatusState = {
  isExportDisabled: boolean
  zoomLevel: number
}
