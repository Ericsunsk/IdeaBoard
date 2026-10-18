import { onScopeDispose, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import {
  queryLibraryDropHit,
  resolveLibrarySaveFolderId,
  type LibraryDropHit,
  type LibrarySaveModule,
} from '../../../../library'
import { resolveImageNodeDisplayPayload } from '../../../session/flow-adapter'
import type { ImageNodeItem } from '../../../../board/ports/canvas'
import { createCanvasImageSaveInputs } from '../../../session/library-save/canvasImageSaveAdapter'
import type { CanvasImageSaveSource } from '../../../session/library-save/canvasImageSaveAdapter'
import { i18n } from '../../../../../core/i18n'
import { userFacingErrorCopy } from '../../../../../core/errors/errorCatalog'

/**
 * Canvas node-drag save routing.
 *
 * While a node drag is active this router hit-tests the Library island
 * read-only (never writes Canvas or island drag state) and mirrors the hit
 * into the island's own hover display. At drag stop it decides move vs
 * save-to-library: image nodes released over the island revert to their
 * origin positions and save through the shared Library save module; every
 * other gesture keeps the normal move commit.
 */

export type CanvasNodeLibrarySaveDecision =
  | {
      kind: 'save'
      hit: LibraryDropHit
      sources: CanvasImageSaveSource[]
      origins: Map<string, { x: number; y: number }>
    }
  | { kind: 'move' }

export type CanvasNodeLibrarySaveRouterOptions = {
  nodes: Ref<Node[]>
  isNodeDragActive: () => boolean
  isNodeDragLocked: () => boolean
  isIslandAvailable: () => boolean
  setIslandHover: (hit: LibraryDropHit | null) => void
  getSelectedFolderKey: () => string
  isFolderKnown: (key: string) => boolean
  saveModule: LibrarySaveModule
  notifyError: (message: string) => void
}

function readNodePosition(node: Node): { x: number; y: number } {
  const position = node.position as { x?: number; y?: number } | undefined
  return { x: Number(position?.x ?? 0), y: Number(position?.y ?? 0) }
}

export function useCanvasNodeLibrarySaveRouter(options: CanvasNodeLibrarySaveRouterOptions) {
  let trackedNodeIds: string[] | null = null
  let origins: Map<string, { x: number; y: number }> | null = null
  let lastHit: LibraryDropHit | null = null
  let lastPosition: { x: number; y: number } | null = null
  let stopTracking: (() => void) | null = null

  function cleanup() {
    stopTracking?.()
    stopTracking = null
    trackedNodeIds = null
    origins = null
    lastHit = null
    lastPosition = null
    options.setIslandHover(null)
  }

  function onPointerMove(event: PointerEvent) {
    if (!options.isNodeDragActive()) {
      cleanup()
      return
    }
    lastPosition = { x: event.clientX, y: event.clientY }
    const hit = queryLibraryDropHit(event.clientX, event.clientY)
    lastHit = hit
    options.setIslandHover(hit)
  }

  function start(nodeIds: readonly string[]) {
    cleanup()
    if (options.isNodeDragLocked() || nodeIds.length === 0 || !options.isIslandAvailable()) return
    const snapshot = new Map<string, { x: number; y: number }>()
    for (const node of options.nodes.value) {
      if (nodeIds.includes(node.id)) snapshot.set(node.id, readNodePosition(node))
    }
    trackedNodeIds = [...nodeIds]
    origins = snapshot
    lastHit = null
    lastPosition = null
    window.addEventListener('pointermove', onPointerMove, { capture: true })
    stopTracking = () => window.removeEventListener('pointermove', onPointerMove, true)
  }

  function collectImageSources(nodeIds: readonly string[]): CanvasImageSaveSource[] {
    const sources: CanvasImageSaveSource[] = []
    for (const node of options.nodes.value) {
      if (!nodeIds.includes(node.id)) continue
      const snapshot = (node.data as { snapshotNode?: unknown } | undefined)?.snapshotNode
      if (!snapshot || (snapshot as ImageNodeItem).type !== 'image-node') continue
      const payload = resolveImageNodeDisplayPayload(snapshot as ImageNodeItem)
      if (!payload?.url) continue
      const title = (snapshot as { title?: unknown }).title
      sources.push({
        displayUrl: payload.url,
        ...(payload.assetId ? { mediaAssetId: payload.assetId } : {}),
        ...(typeof title === 'string' && title.trim() ? { title: title.trim() } : {}),
      })
    }
    return sources
  }

  function evaluateStop(): CanvasNodeLibrarySaveDecision {
    const nodeIds = trackedNodeIds
    const cohortOrigins = origins
    if (!nodeIds || !cohortOrigins) return { kind: 'move' }
    const hit =
      lastHit ?? (lastPosition ? queryLibraryDropHit(lastPosition.x, lastPosition.y) : null)
    const sources = collectImageSources(nodeIds)
    if (!hit || sources.length === 0) return { kind: 'move' }
    return { kind: 'save', hit, sources, origins: cohortOrigins }
  }

  function revertPositions(decision: Extract<CanvasNodeLibrarySaveDecision, { kind: 'save' }>) {
    options.nodes.value = options.nodes.value.map((node) => {
      const origin = decision.origins.get(node.id)
      if (!origin) return node
      return { ...node, position: { x: origin.x, y: origin.y } }
    })
  }

  async function finishSave(decision: Extract<CanvasNodeLibrarySaveDecision, { kind: 'save' }>) {
    // The position transaction already ended with no commit: nodes keep
    // their origin, no history entry, no dirty state. Save outcome never
    // reopens it.
    revertPositions(decision)
    cleanup()
    try {
      const resolved = resolveLibrarySaveFolderId(
        decision.hit.kind === 'folder' ? decision.hit.folderKey : undefined,
        options.getSelectedFolderKey(),
        options.isFolderKnown,
      )
      if (resolved.kind === 'invalid') return
      await options.saveModule.submit(createCanvasImageSaveInputs(decision.sources), {
        ...(resolved.folderId == null ? {} : { folderId: resolved.folderId }),
        galleryEffect: 'reload-current',
        // Manual canvas saves publish to the workspace Library: the
        // success copy states the workspace visibility explicitly (#937).
        successMessage: i18n.global.t('library.toasts.publishedToWorkspace'),
      })
    } catch (error) {
      options.notifyError(
        userFacingErrorCopy(error, i18n.global.t('library.errors.saveAssetFailed'), {
          board_forbidden: 'library.errors.saveRestrictedDenied',
        }),
      )
    }
  }

  onScopeDispose(cleanup)

  return { start, evaluateStop, finishSave, cancel: cleanup }
}
