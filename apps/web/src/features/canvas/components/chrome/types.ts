import type {
  UserAccountMenuItem,
  UserIdeaBalanceControl,
} from '../../../../shared/components/userAccountMenu.types'

export type CanvasChromeBoardOption = {
  id: string
  name: string
}

export type CanvasChromeBoardRenameRecoveryState =
  | { status: 'idle' }
  | {
      status: 'pending' | 'failed' | 'conflict'
      boardId: string
      previousName: string
      nextName: string
    }

export type CanvasChromeBoardState = {
  currentName: string
  options: CanvasChromeBoardOption[]
  currentId: string | null
  isMenuOpen: boolean
  isRenaming: boolean
  renameRecovery: CanvasChromeBoardRenameRecoveryState
  setNameInputRef: (el: HTMLElement | null) => void
}

export type CanvasChromeAccountState = {
  avatarUrl?: string
  balance: UserIdeaBalanceControl
  menuItems: UserAccountMenuItem[]
}

export type CanvasChromeToolState = {
  activeTool: 'select' | 'hand'
  minimapEnabled: boolean
}

export type CanvasChromeStatusState = {
  exportDisabled: boolean
  zoomLevel: number
}
