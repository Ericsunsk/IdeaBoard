export const MASK_EDITOR_CHROME_LAYOUT = {
  imageGap: 12,
  stackGap: 8,
  rightPanelWidth: 203,
  toolButtonSize: 35,
  toolButtonGap: 7,
  viewportPadding: 40,
} as const

export function getMaskEditorChromeOutset() {
  return {
    left: MASK_EDITOR_CHROME_LAYOUT.toolButtonSize + MASK_EDITOR_CHROME_LAYOUT.imageGap,
    right: MASK_EDITOR_CHROME_LAYOUT.rightPanelWidth + MASK_EDITOR_CHROME_LAYOUT.imageGap,
  }
}
