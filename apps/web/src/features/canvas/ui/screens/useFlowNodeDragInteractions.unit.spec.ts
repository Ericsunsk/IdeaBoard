import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../api/mediaWorkspaceScope'
import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { LibraryDropHit, LibrarySaveModule, LibrarySaveSubmitResult } from '../../../library'
import {
  useFlowNodeDragInteractions,
  type FlowNodeDragLibrarySave,
} from './useFlowNodeDragInteractions'
import { i18n } from '../../../../core/i18n'

function imageNode(id: string, x: number, y: number): Node {
  return {
    id,
    position: { x, y },
    data: {
      snapshotNode: {
        type: 'image-node',
        sourceImageUrl: `https://example.com/${id}.png`,
        sourceImageRatio: 1,
        outputImages: [],
      },
    },
  } as unknown as Node
}

function createHarness(
  options: {
    isDuplicateModifierPressed?: boolean
    isNodeDragLocked?: boolean
    isIslandAvailable?: boolean
  } = {},
) {
  const nodes = ref([imageNode('image-1', 10, 20), imageNode('image-2', 30, 40)]) as unknown as Ref<
    Node[]
  >
  const islandHover: unknown[] = []
  const notifyError = vi.fn((_message: string): void => {})
  const beginNodeDragTransaction = vi.fn()
  const commitNodeDragTransaction = vi.fn()
  const finishNodeDragAsLibrarySave = vi.fn(() => [] as string[])
  const cancelNodeDragTransaction = vi.fn()
  const beginFlowNodeDragDuplicateTracking = vi.fn()
  const finishFlowNodeDragDuplicateTracking = vi.fn()
  const dismissTransientCanvasUi = vi.fn()
  const onDragBegin = vi.fn()
  const onDragEnd = vi.fn()
  const isNodeDragLocked = vi.fn(() => options.isNodeDragLocked ?? false)
  const saveModule: LibrarySaveModule = {
    operations: ref([]),
    submit: vi.fn(async (): Promise<LibrarySaveSubmitResult> => ({
      operationId: 'op-1',
      status: 'succeeded',
      items: [],
    })),
    retryOperation: vi.fn(async (): Promise<LibrarySaveSubmitResult> => ({
      operationId: 'op-1',
      status: 'succeeded',
      items: [],
    })),
    clear: vi.fn(),
    dispose: vi.fn(),
  }
  const librarySave: FlowNodeDragLibrarySave = {
    nodes,
    isIslandAvailable: vi.fn(() => options.isIslandAvailable ?? true),
    setIslandHover: vi.fn((hit: LibraryDropHit | null) => {
      islandHover.push(hit)
    }),
    getSelectedFolderKey: (): string => 'all',
    isFolderKnown: (key: string): boolean => key === 'lfolder_folder0007',
    saveModule,
    notifyError,
  }

  const interactions = useFlowNodeDragInteractions({
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    finishNodeDragAsLibrarySave,
    cancelNodeDragTransaction,
    isDuplicateModifierPressed: ref(options.isDuplicateModifierPressed ?? false),
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    isNodeDragLocked,
    onDragBegin,
    onDragEnd,
    librarySave,
  })

  return {
    interactions,
    nodes,
    islandHover,
    saveModule,
    notifyError,
    beginNodeDragTransaction,
    commitNodeDragTransaction,
    finishNodeDragAsLibrarySave,
    cancelNodeDragTransaction,
    beginFlowNodeDragDuplicateTracking,
    finishFlowNodeDragDuplicateTracking,
    dismissTransientCanvasUi,
    onDragBegin,
    onDragEnd,
    isNodeDragLocked,
    librarySave,
  }
}

function dropTargetAt(folderKey: string): HTMLElement {
  const pill = document.createElement('div')
  pill.setAttribute('data-library-drop-folder', folderKey)
  document.body.appendChild(pill)
  return pill
}

async function awaitSaveSubmit(harness: ReturnType<typeof createHarness>) {
  await vi.waitFor(() => {
    expect(harness.saveModule.submit).toHaveBeenCalled()
  })
}

async function awaitSaveNotified(harness: ReturnType<typeof createHarness>) {
  await vi.waitFor(() => {
    expect(harness.notifyError).toHaveBeenCalled()
  })
}

describe('useFlowNodeDragInteractions', () => {
  let pills: HTMLElement[] = []
  beforeEach(() => {
    setMediaWorkspaceIdResolver(() => 'ws_test123')
    for (const pill of pills) pill.remove()
    pills = []
    vi.restoreAllMocks()
  })
  afterEach(() => resetMediaWorkspaceIdResolver())

  it('dismisses transient UI before starting a node drag transaction', () => {
    const harness = createHarness({ isDuplicateModifierPressed: true })

    harness.interactions.onNodeDragStart(['node-a'])

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({ shouldCloseOutpaint: true })
    expect(harness.beginNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.beginNodeDragTransaction).toHaveBeenCalledWith(['node-a'])
    expect(harness.onDragBegin).toHaveBeenCalledTimes(1)
    expect(harness.beginFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(true)
    expect(harness.dismissTransientCanvasUi.mock.invocationCallOrder[0]).toBeLessThan(
      harness.beginNodeDragTransaction.mock.invocationCallOrder[0] ?? 0,
    )
    expect(harness.beginNodeDragTransaction.mock.invocationCallOrder[0]).toBeLessThan(
      harness.onDragBegin.mock.invocationCallOrder[0] ?? Number.POSITIVE_INFINITY,
    )
    expect(harness.onDragBegin.mock.invocationCallOrder[0]).toBeLessThan(
      harness.beginFlowNodeDragDuplicateTracking.mock.invocationCallOrder[0] ??
        Number.POSITIVE_INFINITY,
    )
  })

  it('only closes outpaint mode when a node drag stops', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['node-a'])

    harness.interactions.onNodeDragStop()

    expect(harness.dismissTransientCanvasUi).toHaveBeenCalledWith({
      shouldCloseOutpaint: true,
      shouldClearNodePanel: false,
      shouldHideStrength: false,
    })
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
    expect(harness.finishFlowNodeDragDuplicateTracking).not.toHaveBeenCalled()
    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.finishFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(false)
    expect(harness.commitNodeDragTransaction.mock.invocationCallOrder[0]).toBeLessThan(
      harness.finishFlowNodeDragDuplicateTracking.mock.invocationCallOrder[0] ??
        Number.POSITIVE_INFINITY,
    )
    // Telemetry ends exactly once per began drag.
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
  })

  it('only commits the latest queued drag stop', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['node-a'])

    harness.interactions.onNodeDragStop()
    harness.interactions.onNodeDragStop()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
  })

  it('ignores node drag callbacks while node drag is locked', async () => {
    const harness = createHarness({ isNodeDragLocked: true })

    harness.interactions.onNodeDragStart(['node-a'])
    harness.interactions.onNodeDragStop()

    expect(harness.dismissTransientCanvasUi).not.toHaveBeenCalled()
    expect(harness.beginNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.onDragBegin).not.toHaveBeenCalled()
    expect(harness.beginFlowNodeDragDuplicateTracking).not.toHaveBeenCalled()
    expect(harness.finishFlowNodeDragDuplicateTracking).not.toHaveBeenCalled()
    expect(harness.onDragEnd).not.toHaveBeenCalled()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
  })

  it('ignores an empty drag cohort without opening telemetry', () => {
    const harness = createHarness()

    harness.interactions.onNodeDragStart([])

    expect(harness.beginNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.onDragBegin).not.toHaveBeenCalled()
  })

  it('ignores a stray drag stop without ending telemetry', async () => {
    const harness = createHarness()

    harness.interactions.onNodeDragStop()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.finishNodeDragAsLibrarySave).not.toHaveBeenCalled()
    expect(harness.onDragEnd).not.toHaveBeenCalled()
  })

  it('finalizes the stop even when the drag locks before release', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['node-a'])
    harness.isNodeDragLocked.mockReturnValue(true)

    harness.interactions.onNodeDragStop()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
  })

  it('finishes a library save without committing the move', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))

    harness.interactions.onNodeDragStop()

    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.finishNodeDragAsLibrarySave).not.toHaveBeenCalled()

    await Promise.resolve()

    expect(harness.finishNodeDragAsLibrarySave).toHaveBeenCalledTimes(1)
    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
    await awaitSaveSubmit(harness)
    expect(harness.saveModule.submit).toHaveBeenCalledTimes(1)
  })

  it('never duplicates the cohort on a library save, even with Alt held', async () => {
    const harness = createHarness({ isDuplicateModifierPressed: true })
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))

    harness.interactions.onNodeDragStop()
    await Promise.resolve()

    expect(harness.finishFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(false)
    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
  })

  it('routes an image-node release over a folder pill to save and reverts positions', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))

    // The drag moved the rendered node before the stop decision.
    const dragged = harness.nodes.value[0]
    if (dragged) dragged.position = { x: 500, y: 500 }
    harness.interactions.onNodeDragStop()
    await Promise.resolve()
    await awaitSaveSubmit(harness)

    // Rendered positions return to origin; the module receives media bytes
    // for the hovered folder while the position transaction stays untouched.
    expect(harness.nodes.value[0]?.position).toEqual({ x: 10, y: 20 })
    expect(harness.saveModule.submit).toHaveBeenCalledExactlyOnceWith(
      [
        {
          source: 'media',
          file: expect.any(Function),
        },
      ],
      {
        folderId: 'lfolder_folder0007',
        galleryEffect: 'reload-current',
        successMessage: i18n.global.t('library.toasts.publishedToWorkspace'),
      },
    )
    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
  })

  it('saves managed image identity instead of downloading its display rendition', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    const snapshot = harness.nodes.value[0]?.data?.snapshotNode
    if (!snapshot) throw new Error('Expected image node')
    snapshot.sourceMediaRef = { assetId: 'asset_managed001', derivativeTier: 'thumb' }
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    harness.interactions.onNodeDragStop()
    await Promise.resolve()
    await awaitSaveSubmit(harness)

    expect(harness.saveModule.submit).toHaveBeenCalledExactlyOnceWith(
      [{ source: 'media', mediaAssetId: 'asset_managed001' }],
      {
        folderId: 'lfolder_folder0007',
        galleryEffect: 'reload-current',
        successMessage: i18n.global.t('library.toasts.publishedToWorkspace'),
      },
    )
    expect(harness.notifyError).not.toHaveBeenCalled()
  })

  it('notifies when a library save fails without reopening the position transaction', async () => {
    const harness = createHarness()
    vi.mocked(harness.saveModule.submit).mockRejectedValueOnce(new Error('boom'))
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    harness.interactions.onNodeDragStop()
    await Promise.resolve()
    await awaitSaveNotified(harness)

    expect(harness.notifyError).toHaveBeenCalledTimes(1)
    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
    // The cohort still rests at its origin after a failed save.
    expect(harness.nodes.value[0]?.position).toEqual({ x: 10, y: 20 })
  })

  it('keeps the normal move commit away from library targets', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(null)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 400, clientY: 400 }))
    harness.interactions.onNodeDragStop()
    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.saveModule.submit).not.toHaveBeenCalled()
  })

  it('keeps the normal move commit when the island closes before release', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    vi.mocked(harness.librarySave.isIslandAvailable).mockReturnValue(false)

    harness.interactions.onNodeDragStop()
    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.saveModule.submit).not.toHaveBeenCalled()
  })

  it('mirrors library hover into the island display state without touching canvas', () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    expect(harness.islandHover.at(-1)).toEqual({ kind: 'folder', folderKey: 'lfolder_folder0007' })

    harness.interactions.cancelActiveDrag()
    expect(harness.islandHover.at(-1)).toBeNull()
  })

  it('cancels an in-flight drag without committing or saving', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))

    harness.interactions.cancelActiveDrag()
    harness.interactions.cancelActiveDrag()

    expect(harness.cancelNodeDragTransaction).toHaveBeenCalledTimes(1)
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
    expect(harness.finishFlowNodeDragDuplicateTracking).toHaveBeenCalledWith(false)

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.finishNodeDragAsLibrarySave).not.toHaveBeenCalled()
    expect(harness.saveModule.submit).not.toHaveBeenCalled()
    expect(harness.islandHover.at(-1)).toBeNull()
    expect(harness.interactions.isActive()).toBe(false)
  })

  it('aborts a queued stop when a cancel lands first', async () => {
    const harness = createHarness()
    harness.interactions.onNodeDragStart(['image-1'])

    harness.interactions.onNodeDragStop()
    harness.interactions.cancelActiveDrag()

    await Promise.resolve()

    expect(harness.commitNodeDragTransaction).not.toHaveBeenCalled()
    expect(harness.cancelNodeDragTransaction).toHaveBeenCalledTimes(1)
    // Telemetry still ends exactly once.
    expect(harness.onDragEnd).toHaveBeenCalledTimes(1)
  })
})
