/**
 * Read-only Library drop hit results.
 *
 * Both the Pointer drop targets and the Canvas node-drag save path resolve
 * targets through this DOM query. It never mutates drag, selection, or
 * Canvas state — callers apply the result to their own state.
 */

export const LIBRARY_DROP_FOLDER_ATTRIBUTE = 'data-library-drop-folder'
export const LIBRARY_DROP_PANEL_ATTRIBUTE = 'data-library-panel'

export type LibraryDropHit = { kind: 'folder'; folderKey: string } | { kind: 'panel' }

export function queryLibraryDropHit(x: number, y: number): LibraryDropHit | null {
  if (typeof document === 'undefined') return null
  const hit = document.elementFromPoint(x, y)
  if (!(hit instanceof Element)) return null
  const pill = hit.closest(`[${LIBRARY_DROP_FOLDER_ATTRIBUTE}]`)
  if (pill) {
    const folderKey = pill.getAttribute(LIBRARY_DROP_FOLDER_ATTRIBUTE)?.trim()
    if (folderKey) return { kind: 'folder', folderKey }
  }
  if (hit.closest(`[${LIBRARY_DROP_PANEL_ATTRIBUTE}]`)) return { kind: 'panel' }
  return null
}
