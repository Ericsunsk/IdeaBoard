import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { page, userEvent } from 'vitest/browser'
import { usePointerDragSession, type PointerDragSessionContext } from './usePointerDragSession'

type DragPayload = {
  id: string
}

type PointerSnapshot = {
  pointerId: number
  clientX: number
  clientY: number
  targetTestId: string | null
  hasCapture: boolean
  payloadId: string
}

const attachedHosts: HTMLElement[] = []

function snapshotPointer(
  source: HTMLElement,
  context: PointerDragSessionContext<DragPayload>,
): PointerSnapshot {
  const eventTarget = context.event.target
  const targetElement = eventTarget instanceof HTMLElement ? eventTarget : null

  return {
    pointerId: context.event.pointerId,
    clientX: context.event.clientX,
    clientY: context.event.clientY,
    targetTestId: targetElement?.dataset.testid ?? null,
    hasCapture: source.hasPointerCapture(context.event.pointerId),
    payloadId: context.payload.id,
  }
}

function appendDragHarness() {
  const host = document.createElement('div')
  host.style.cssText = [
    'display: flex',
    'align-items: center',
    'gap: 180px',
    'width: 560px',
    'height: 220px',
    'padding: 48px',
  ].join(';')

  const source = document.createElement('div')
  source.dataset.testid = 'drag-source'
  source.textContent = 'Drag source'
  source.style.cssText = [
    'width: 96px',
    'height: 96px',
    'display: grid',
    'place-items: center',
    'border: 1px solid rgb(40 80 140)',
    'background: rgb(220 235 255)',
    'user-select: none',
    'touch-action: none',
  ].join(';')

  const target = document.createElement('div')
  target.dataset.testid = 'drag-target'
  target.textContent = 'Drag target'
  target.style.cssText = [
    'width: 96px',
    'height: 96px',
    'display: grid',
    'place-items: center',
    'border: 1px solid rgb(120 80 20)',
    'background: rgb(255 240 210)',
    'user-select: none',
  ].join(';')

  host.append(source, target)
  document.body.append(host)
  attachedHosts.push(host)

  return { source, target }
}

afterEach(() => {
  for (const host of attachedHosts.splice(0)) host.remove()
  vi.restoreAllMocks()
})

describe('usePointerDragSession browser contract', () => {
  it('keeps a real pointer drag captured while forwarding window move and end events', async () => {
    await page.viewport(900, 500)

    const { source } = appendDragHarness()
    const payload = { id: 'browser-drag' }
    const startSnapshots: PointerSnapshot[] = []
    const moveSnapshots: PointerSnapshot[] = []
    const endSnapshots: PointerSnapshot[] = []
    const onCancel = vi.fn()

    const scope = effectScope()
    const controller = scope.run(() =>
      usePointerDragSession<DragPayload>({
        onStart: (context) => startSnapshots.push(snapshotPointer(source, context)),
        onMove: (context) => moveSnapshots.push(snapshotPointer(source, context)),
        onEnd: (context) => endSnapshots.push(snapshotPointer(source, context)),
        onCancel,
      }),
    )

    if (!controller) throw new Error('expected pointer drag controller')

    source.addEventListener('pointerdown', (event) => {
      controller.beginDrag(event, payload)
    })

    try {
      await userEvent.dragAndDrop(
        page.getByTestId('drag-source'),
        page.getByTestId('drag-target'),
        {
          sourcePosition: { x: 48, y: 48 },
          targetPosition: { x: 48, y: 48 },
        },
      )

      expect(onCancel).not.toHaveBeenCalled()
      expect(controller.isDragging.value).toBe(false)
      expect(startSnapshots).toHaveLength(1)
      expect(moveSnapshots.length).toBeGreaterThan(0)
      expect(endSnapshots).toHaveLength(1)

      const start = startSnapshots[0]
      const end = endSnapshots[0]
      if (!start || !end) throw new Error('expected drag start and end snapshots')
      expect(start).toMatchObject({
        hasCapture: true,
        payloadId: payload.id,
        targetTestId: 'drag-source',
      })
      expect(end).toMatchObject({
        hasCapture: false,
        payloadId: payload.id,
        pointerId: start.pointerId,
      })
      expect(moveSnapshots.every((move) => move.pointerId === start.pointerId)).toBe(true)
      expect(moveSnapshots.every((move) => move.hasCapture)).toBe(true)
      expect(moveSnapshots.every((move) => move.targetTestId === 'drag-source')).toBe(true)
      expect(
        moveSnapshots.some(
          (move) => move.clientX > start.clientX && move.clientY === start.clientY,
        ),
      ).toBe(true)
    } finally {
      scope.stop()
    }
  })
})
