import { afterEach, describe, expect, it, vi } from 'vitest'
import { effectScope, ref } from 'vue'
import { startPointerDrag } from '../../../../../core/interaction/pointerDrag'
import type { LibraryDragPayload } from '../../../../library'
import { useCanvasLibraryDrop } from './useCanvasLibraryDrop'

const payload: LibraryDragPayload = {
  kind: 'asset',
  asset: {
    assetId: 'lasset_asset0042',
    title: 'Library image',
    previewUrl: 'https://example.com/image.png',
    width: 1200,
    height: 800,
  },
}

function pointer(type: string, x = 20, y = 30) {
  return new PointerEvent(type, { pointerId: 1, isPrimary: true, clientX: x, clientY: y })
}

function harness() {
  const scope = effectScope()
  const element = document.createElement('div')
  document.body.appendChild(element)
  vi.spyOn(element, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 200, 200))
  vi.spyOn(document, 'elementFromPoint').mockReturnValue(element)
  const surface = ref<HTMLElement | null>(element)
  const boardId = ref('board-1')
  const workspaceId = ref('workspace-1')
  const isLocked = ref(false)
  const add = vi.fn()
  const screenToFlowCoordinate = vi.fn(({ x, y }: { x: number; y: number }) => ({
    x: x / 2,
    y: y / 2,
  }))
  const targets = scope.run(() =>
    useCanvasLibraryDrop({
      element: surface,
      context: () => [boardId.value, workspaceId.value, isLocked.value],
      canReceive: () => !isLocked.value,
      screenToFlowCoordinate,
      add,
    }),
  )!
  return {
    scope,
    element,
    surface,
    boardId,
    workspaceId,
    isLocked,
    add,
    screenToFlowCoordinate,
    targets,
  }
}

describe('Canvas Library pointer drop', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('imports the asset at the current flow coordinate through the shared controller', () => {
    const h = harness()
    try {
      startPointerDrag(pointer('pointerdown'), {
        payload,
        previewUrl: payload.asset.previewUrl,
        targets: h.targets,
      })
      window.dispatchEvent(pointer('pointermove', 80, 100))
      window.dispatchEvent(pointer('pointerup', 80, 100))
      expect(h.screenToFlowCoordinate).toHaveBeenCalledExactlyOnceWith({ x: 80, y: 100 })
      expect(h.add).toHaveBeenCalledExactlyOnceWith(payload.asset, { x: 40, y: 50 })
    } finally {
      h.scope.stop()
    }
  })

  it('ignores covered surfaces, disabled editing and targets after unmount', () => {
    const h = harness()
    try {
      const overlay = document.createElement('div')
      document.body.appendChild(overlay)
      vi.mocked(document.elementFromPoint).mockReturnValue(overlay)
      h.targets.drop(payload, 80, 100)
      vi.mocked(document.elementFromPoint).mockReturnValue(h.element)
      h.isLocked.value = true
      h.targets.drop(payload, 80, 100)
      h.isLocked.value = false
      h.scope.stop()
      h.targets.drop(payload, 80, 100)
      expect(h.add).not.toHaveBeenCalled()
    } finally {
      h.scope.stop()
    }
  })

  it.each(['board', 'workspace', 'lock', 'dispose'])(
    'cancels the source when %s changes',
    (change) => {
      const h = harness()
      const onEnd = vi.fn()
      try {
        startPointerDrag(pointer('pointerdown'), {
          payload,
          previewUrl: payload.asset.previewUrl,
          targets: h.targets,
          onEnd,
        })
        window.dispatchEvent(pointer('pointermove', 80, 100))
        if (change === 'board') h.boardId.value = 'board-2'
        else if (change === 'workspace') h.workspaceId.value = 'workspace-2'
        else if (change === 'lock') h.isLocked.value = true
        else h.scope.stop()
        window.dispatchEvent(pointer('pointerup', 80, 100))
        expect(h.add).not.toHaveBeenCalled()
        expect(onEnd).toHaveBeenCalledOnce()
      } finally {
        h.scope.stop()
      }
    },
  )
})
