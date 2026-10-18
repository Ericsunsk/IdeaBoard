import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resetMediaWorkspaceIdResolver,
  setMediaWorkspaceIdResolver,
} from '../../../../../api/mediaWorkspaceScope'
import { ref, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type {
  LibraryDropHit,
  LibrarySaveModule,
  LibrarySaveSubmitResult,
} from '../../../../library'
import {
  useCanvasNodeLibrarySaveRouter,
  type CanvasNodeLibrarySaveRouterOptions,
} from './useCanvasNodeLibrarySaveRouter'
import { i18n } from '../../../../../core/i18n'

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

function harness() {
  const nodes = ref([imageNode('image-1', 10, 20)]) as unknown as Ref<Node[]>
  const islandHover: unknown[] = []
  const notifyError = vi.fn((_message: string): void => {})
  const island = {
    setIslandHover: vi.fn((hit: LibraryDropHit | null) => {
      islandHover.push(hit)
    }),
    getSelectedFolderKey: (): string => 'all',
    isFolderKnown: (key: string): boolean => key === 'lfolder_folder0007',
  }
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
  const routerOptions: CanvasNodeLibrarySaveRouterOptions = {
    nodes,
    isNodeDragActive: () => true,
    isNodeDragLocked: () => false,
    isIslandAvailable: () => true,
    setIslandHover: island.setIslandHover,
    getSelectedFolderKey: island.getSelectedFolderKey,
    isFolderKnown: island.isFolderKnown,
    saveModule,
    notifyError,
  }
  const router = useCanvasNodeLibrarySaveRouter(routerOptions)
  return { nodes, island, islandHover, saveModule, notifyError, router }
}

function dropTargetAt(folderKey: string): HTMLElement {
  const pill = document.createElement('div')
  pill.setAttribute('data-library-drop-folder', folderKey)
  document.body.appendChild(pill)
  return pill
}

describe('useCanvasNodeLibrarySaveRouter', () => {
  let pills: HTMLElement[] = []
  beforeEach(() => {
    setMediaWorkspaceIdResolver(() => 'ws_test123')
    for (const pill of pills) pill.remove()
    pills = []
    vi.restoreAllMocks()
  })
  afterEach(() => resetMediaWorkspaceIdResolver())

  it('routes an image-node release over a folder pill to save and reverts positions', async () => {
    const { nodes, router, saveModule } = harness()
    router.start(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    const decision = router.evaluateStop()
    expect(decision.kind).toBe('save')
    if (decision.kind !== 'save') throw new Error('unreachable')

    // The drag moved the rendered node before the stop decision.
    const dragged = nodes.value[0]
    if (dragged) dragged.position = { x: 500, y: 500 }
    await router.finishSave(decision)

    // Rendered positions return to origin; the module receives media bytes
    // for the hovered folder while the position transaction stays untouched.
    expect(nodes.value[0]?.position).toEqual({ x: 10, y: 20 })
    expect(saveModule.submit).toHaveBeenCalledExactlyOnceWith(
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
  })

  it('saves managed image identity instead of downloading its display rendition', async () => {
    const { nodes, router, saveModule, notifyError } = harness()
    const snapshot = nodes.value[0]?.data?.snapshotNode
    if (!snapshot) throw new Error('Expected image node')
    snapshot.sourceMediaRef = { assetId: 'asset_managed001', derivativeTier: 'thumb' }
    router.start(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    const decision = router.evaluateStop()
    if (decision.kind !== 'save') throw new Error('unreachable')
    await router.finishSave(decision)

    expect(saveModule.submit).toHaveBeenCalledExactlyOnceWith(
      [{ source: 'media', mediaAssetId: 'asset_managed001' }],
      {
        folderId: 'lfolder_folder0007',
        galleryEffect: 'reload-current',
        successMessage: i18n.global.t('library.toasts.publishedToWorkspace'),
      },
    )
    expect(notifyError).not.toHaveBeenCalled()
  })

  it('keeps the normal move commit away from library targets', () => {
    const { router } = harness()
    router.start(['image-1'])
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(null)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 400, clientY: 400 }))
    expect(router.evaluateStop()).toEqual({ kind: 'move' })
  })

  it('mirrors library hover into the island display state without touching canvas', () => {
    const { router, islandHover } = harness()
    router.start(['image-1'])
    const pill = dropTargetAt('lfolder_folder0007')
    pills.push(pill)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(pill)

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 10, clientY: 10 }))
    expect(islandHover.at(-1)).toEqual({ kind: 'folder', folderKey: 'lfolder_folder0007' })

    router.cancel()
    expect(islandHover.at(-1)).toBeNull()
  })
})
