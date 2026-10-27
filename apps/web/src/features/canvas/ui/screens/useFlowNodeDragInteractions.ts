import { getCurrentScope, onScopeDispose, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { DismissTransientCanvasUi } from './useFlowTransientUiDismissal'
import {
  queryLibraryDropHit,
  resolveLibrarySaveFolderId,
  type LibraryDropHit,
  type LibrarySaveModule,
} from '../../../library'
import { resolveImageNodeDisplayPayload } from '../../session/flow-adapter'
import type { ImageNodeItem } from '../../../board/ports/canvas'
import { createCanvasImageSaveInputs } from '../../session/library-save/canvasImageSaveAdapter'
import type { CanvasImageSaveSource } from '../../session/library-save/canvasImageSaveAdapter'
import { i18n } from '../../../../core/i18n'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'

/**
 * Single owner of the Canvas node-drag lifecycle.
 *
 * Vue Flow adapter events, the Canvas graph transaction and the Library drop
 * target all plug into this module; no parallel module tracks positions,
 * selection or end-of-drag state. A drag has exactly three terminal outcomes:
 * move (commit once), save-to-library (end the position transaction with no
 * commit, revert the renderer to the drag origins, then save through the
 * shared Library save module) or cancel (abort with no commit and no save).
 * Telemetry (`onDragBegin`/`onDragEnd`) fires exactly once per began drag.
 */
export type FlowNodeDragLibrarySave = {
  nodes: Ref<Node[]>
  isIslandAvailable: () => boolean
  setIslandHover: (hit: LibraryDropHit | null) => void
  getSelectedFolderKey: () => string
  isFolderKnown: (key: string) => boolean
  saveModule: LibrarySaveModule
  notifyError: (message: string) => void
}

type UseFlowNodeDragInteractionsOptions = {
  beginNodeDragTransaction: (nodeIds: readonly string[]) => void
  commitNodeDragTransaction: () => void
  /** Third drag outcome (save-to-library): ends the drag with no position commit. */
  finishNodeDragAsLibrarySave: () => void
  /** Abort path for unmount and Board/Workspace switches: no commit, no save. */
  cancelNodeDragTransaction: () => void
  isDuplicateModifierPressed: Ref<boolean>
  beginFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  finishFlowNodeDragDuplicateTracking: (altPressed: boolean) => void
  dismissTransientCanvasUi: DismissTransientCanvasUi
  isNodeDragLocked?: () => boolean
  /** Viewport interaction tracking + editing activity begin; fires once per began drag. */
  onDragBegin: () => void
  /** Viewport interaction tracking + editing activity end; fires once per began drag. */
  onDragEnd: () => void
  librarySave?: FlowNodeDragLibrarySave | undefined
}

type ActiveNodeDrag = {
  nodeIds: readonly string[]
  origins: Map<string, { x: number; y: number }>
  lastHit: LibraryDropHit | null
}

function readNodePosition(node: Node): { x: number; y: number } {
  const position = node.position as { x?: number; y?: number } | undefined
  return { x: Number(position?.x ?? 0), y: Number(position?.y ?? 0) }
}

export function useFlowNodeDragInteractions(options: UseFlowNodeDragInteractionsOptions) {
  let pendingDragStopCommit: object | null = null
  let activeDrag: ActiveNodeDrag | null = null
  let stopHoverTracking: (() => void) | null = null

  function isActive() {
    return activeDrag !== null
  }

  function cleanupHoverTracking() {
    stopHoverTracking?.()
    stopHoverTracking = null
    if (activeDrag) activeDrag.lastHit = null
    options.librarySave?.setIslandHover(null)
  }

  function onPointerMove(event: PointerEvent) {
    const drag = activeDrag
    const librarySave = options.librarySave
    if (!drag || !librarySave) {
      cleanupHoverTracking()
      return
    }
    const hit = queryLibraryDropHit(event.clientX, event.clientY)
    if (
      (hit === null && drag.lastHit === null) ||
      (hit !== null &&
        drag.lastHit !== null &&
        hit.kind === drag.lastHit.kind &&
        (hit.kind !== 'folder' ||
          (drag.lastHit.kind === 'folder' && hit.folderKey === drag.lastHit.folderKey)))
    ) {
      return
    }
    drag.lastHit = hit
    librarySave.setIslandHover(hit)
  }

  function collectImageSources(nodeIds: readonly string[]): CanvasImageSaveSource[] {
    const librarySave = options.librarySave
    if (!librarySave) return []
    const sources: CanvasImageSaveSource[] = []
    for (const node of librarySave.nodes.value) {
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

  function revertPositions(drag: ActiveNodeDrag) {
    const librarySave = options.librarySave
    if (!librarySave) return
    librarySave.nodes.value = librarySave.nodes.value.map((node) => {
      const origin = drag.origins.get(node.id)
      if (!origin) return node
      return { ...node, position: { x: origin.x, y: origin.y } }
    })
  }

  async function finishLibrarySave(
    drag: ActiveNodeDrag,
    hit: LibraryDropHit,
    sources: CanvasImageSaveSource[],
  ) {
    const librarySave = options.librarySave
    if (!librarySave) return
    // The position transaction already ended with no commit: nodes keep
    // their origin, no history entry, no dirty state. Save outcome never
    // reopens it.
    revertPositions(drag)
    try {
      const resolved = resolveLibrarySaveFolderId(
        hit.kind === 'folder' ? hit.folderKey : undefined,
        librarySave.getSelectedFolderKey(),
        librarySave.isFolderKnown,
      )
      if (resolved.kind === 'invalid') return
      await librarySave.saveModule.submit(createCanvasImageSaveInputs(sources), {
        ...(resolved.folderId == null ? {} : { folderId: resolved.folderId }),
        galleryEffect: 'reload-current',
        // Manual canvas saves publish to the workspace Library: the
        // success copy states the workspace visibility explicitly (#937).
        successMessage: i18n.global.t('library.toasts.publishedToWorkspace'),
      })
    } catch (error) {
      librarySave.notifyError(
        userFacingErrorCopy(error, i18n.global.t('library.errors.saveAssetFailed'), {
          board_forbidden: 'library.errors.saveRestrictedDenied',
        }),
      )
    }
  }

  function onNodeDragStart(nodeIds: readonly string[]) {
    if (options.isNodeDragLocked?.()) return
    if (nodeIds.length === 0) return
    pendingDragStopCommit = null
    cleanupHoverTracking()
    activeDrag = null
    options.dismissTransientCanvasUi({ shouldCloseOutpaint: true })
    options.beginNodeDragTransaction(nodeIds)
    try {
      options.onDragBegin()
      options.beginFlowNodeDragDuplicateTracking(options.isDuplicateModifierPressed.value)
    } catch (error) {
      try {
        options.cancelNodeDragTransaction()
      } finally {
        activeDrag = null
        options.onDragEnd()
      }
      throw error
    }
    const origins = new Map<string, { x: number; y: number }>()
    if (options.librarySave) {
      for (const node of options.librarySave.nodes.value) {
        if (nodeIds.includes(node.id)) origins.set(node.id, readNodePosition(node))
      }
    }
    activeDrag = { nodeIds: [...nodeIds], origins, lastHit: null }
    if (options.librarySave && typeof window !== 'undefined') {
      window.addEventListener('pointermove', onPointerMove, { capture: true })
      stopHoverTracking = () => window.removeEventListener('pointermove', onPointerMove, true)
    }
  }

  function scheduleStop(finalize: () => void, isSave: boolean) {
    // A stop is single-shot: a second stop before finalization runs is
    // ignored so telemetry still ends exactly once.
    if (!activeDrag || pendingDragStopCommit) {
      cleanupHoverTracking()
      return
    }
    // The terminal decision is captured now; hover state is no longer needed
    // and clears exactly once even though finalization runs later. The drag
    // stays active until the transaction resolves so a cancel in between can
    // still abort it instead of leaking the transaction.
    cleanupHoverTracking()
    options.dismissTransientCanvasUi({
      shouldCloseOutpaint: true,
      shouldClearNodePanel: false,
      shouldHideStrength: false,
    })
    const isDuplicateMode = options.isDuplicateModifierPressed.value
    options.onDragEnd()
    const commitToken = {}
    pendingDragStopCommit = commitToken
    queueMicrotask(() => {
      if (pendingDragStopCommit !== commitToken) return
      pendingDragStopCommit = null
      const drag = activeDrag
      activeDrag = null
      if (!drag) return
      try {
        finalize()
      } finally {
        // Always clear duplicate tracking, even when a graph commit fails.
        // Keeping this cleanup after commit preserves the duplicate position
        // calculation while preventing stale Alt-drag state from leaking.
        // A Library save never duplicates: the cohort stays at its origin.
        options.finishFlowNodeDragDuplicateTracking(isSave ? false : isDuplicateMode)
      }
    })
  }

  function evaluateSave(
    drag: ActiveNodeDrag,
  ): { hit: LibraryDropHit; sources: CanvasImageSaveSource[] } | null {
    const librarySave = options.librarySave
    if (!librarySave || !librarySave.isIslandAvailable()) return null
    const hit = drag.lastHit
    if (!hit) return null
    const sources = collectImageSources(drag.nodeIds)
    if (sources.length === 0) return null
    return { hit, sources }
  }

  function onNodeDragStop() {
    const drag = activeDrag
    if (!drag) {
      cleanupHoverTracking()
      return
    }
    const save = evaluateSave(drag)
    if (!save) {
      scheduleStop(options.commitNodeDragTransaction, false)
      return
    }
    scheduleStop(() => {
      options.finishNodeDragAsLibrarySave()
      // Fire-and-forget: the position transaction already ended with no
      // commit, and the save module owns retries plus notifications.
      void finishLibrarySave(drag, save.hit, save.sources)
    }, true)
  }

  function cancelActiveDrag() {
    const wasStopping = pendingDragStopCommit !== null
    const drag = activeDrag
    pendingDragStopCommit = null
    cleanupHoverTracking()
    activeDrag = null
    if (!drag) return
    try {
      options.cancelNodeDragTransaction()
    } finally {
      // A cancelled drag never duplicates, even with Alt held.
      options.finishFlowNodeDragDuplicateTracking(false)
      // A stop already requested ended telemetry at release; only end it
      // here when the drag never reached the stop path.
      if (!wasStopping) options.onDragEnd()
    }
  }

  if (getCurrentScope()) {
    onScopeDispose(cancelActiveDrag)
  }

  return {
    onNodeDragStart,
    onNodeDragStop,
    cancelActiveDrag,
    isActive,
  }
}
