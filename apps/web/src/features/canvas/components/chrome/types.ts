import type { UserAccountMenuItem } from '../../../../shared/components/userAccountMenuActions'

export type CanvasChromeBoardOption = {
  id: string
  name: string
}

export type CanvasChromeBoardState = {
  currentName: string
  options: CanvasChromeBoardOption[]
  currentId: string | null
  isMenuOpen: boolean
  isRenaming: boolean
  setNameInputRef: (el: HTMLElement | null) => void
}

export type CanvasChromeAccountState = {
  avatarUrl?: string
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
