export const IMAGE_EDITOR_CHROME_LAYOUT = {
  imageGap: 12,
  stackGap: 8,
  rightPanelWidth: 203,
  toolButtonSize: 35,
  viewportPadding: 40,
} as const

export function getImageEditorChromeOutset() {
  return {
    left: IMAGE_EDITOR_CHROME_LAYOUT.toolButtonSize + IMAGE_EDITOR_CHROME_LAYOUT.imageGap,
    right: IMAGE_EDITOR_CHROME_LAYOUT.rightPanelWidth + IMAGE_EDITOR_CHROME_LAYOUT.imageGap,
  }
}
