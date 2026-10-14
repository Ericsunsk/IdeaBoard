import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPointerDropTargets, type PointerDropTargets } from './pointerDropTargets'
import { startPointerDrag } from './pointerDrag'

const result = { id: 'photo-1' }

function pointer(type: string, overrides: PointerEventInit = {}) {
  return new PointerEvent(type, {
    pointerId: 1,
    isPrimary: true,
    pointerType: 'mouse',
    clientX: 10,
    clientY: 10,
    cancelable: true,
    ...overrides,
  })
}

describe('Image pointer drag', () => {
  let target: HTMLElement
  let dispose: (() => void) | undefined
  let targets: PointerDropTargets<typeof result>
  let unregister: () => void
  const onEnd = vi.fn()
  const onHoverChange = vi.fn()
  const onDrop = vi.fn()

  function start(initial = pointer('pointerdown')) {
    dispose = startPointerDrag(initial, {
      payload: result,
      previewUrl: 'https://images.example/thumb.jpg',
      targets,
      onEnd,
    })
  }

  function move() {
    window.dispatchEvent(pointer('pointermove', { clientX: 60, clientY: 70 }))
  }

  beforeEach(() => {
    vi.useFakeTimers()
    onEnd.mockReset()
    onHoverChange.mockReset()
    onDrop.mockReset()
    targets = createPointerDropTargets<typeof result>()
    target = document.createElement('div')
    document.body.appendChild(target)
    vi.spyOn(target, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 100, 100))
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(target)
    unregister = targets.register({
      element: target,
      canReceive: () => true,
      onHoverChange,
      onDrop,
    })
  })

  afterEach(() => {
    dispose?.()
    dispose = undefined
    document.body.innerHTML = ''
    vi.runAllTimers()
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('preserves ordinary clicks below the drag threshold', () => {
    start()
    window.dispatchEvent(pointer('pointermove', { clientX: 12 }))
    window.dispatchEvent(pointer('pointerup'))

    expect(document.body.querySelector('img')).toBeNull()
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
    expect(target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))).toBe(
      true,
    )
  })

  it('moves the ghost, delivers the reference and suppresses the trailing click', () => {
    start()
    move()

    const ghost = document.body.querySelector('img')?.parentElement
    expect(ghost?.style.left).toBe('31px')
    expect(ghost?.style.top).toBe('48px')
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(true)

    window.dispatchEvent(pointer('pointerup', { clientX: 60, clientY: 70 }))

    expect(onDrop).toHaveBeenCalledExactlyOnceWith(result, { x: 60, y: 70 })
    expect(onDrop.mock.calls[0]?.[0]).toBe(result)
    expect(onHoverChange.mock.calls).toEqual([[true], [false]])
    expect(ghost?.isConnected).toBe(false)
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
    expect(target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))).toBe(
      false,
    )
    expect(target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))).toBe(
      true,
    )
    expect(onEnd).toHaveBeenCalledOnce()
  })

  it('does not drop on an unregistered target', () => {
    unregister()
    start()
    move()
    window.dispatchEvent(pointer('pointerup'))

    expect(onDrop).not.toHaveBeenCalled()
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
  })

  it.each(['Escape', 'pointercancel', 'pointerout', 'blur', 'dispose'])(
    'restores the cursor and clears the hovered target on %s',
    (reason) => {
      start()
      move()

      if (reason === 'Escape') window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      else if (reason === 'pointercancel') window.dispatchEvent(pointer('pointercancel'))
      else if (reason === 'pointerout') window.dispatchEvent(pointer('pointerout'))
      else if (reason === 'blur') window.dispatchEvent(new Event('blur'))
      else dispose?.()
      move()
      window.dispatchEvent(pointer('pointerup'))

      expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
      expect(document.body.querySelector('img')).toBeNull()
      expect(onHoverChange.mock.calls).toEqual([[true], [false]])
      expect(onDrop).not.toHaveBeenCalled()
      expect(onEnd).toHaveBeenCalledOnce()
    },
  )

  it('ignores other pointers during an active drag', () => {
    start()
    move()
    window.dispatchEvent(pointer('pointerup', { pointerId: 2 }))
    window.dispatchEvent(pointer('pointercancel', { pointerId: 2 }))

    expect(onEnd).not.toHaveBeenCalled()
    expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(true)
  })

  it('rechecks a newly mounted target without another pointer move', () => {
    unregister()
    start()
    move()
    expect(onHoverChange).not.toHaveBeenCalled()

    targets.register({ element: target, canReceive: () => true, onHoverChange, onDrop })
    vi.advanceTimersToNextFrame()

    expect(onHoverChange).toHaveBeenCalledWith(true)
    window.dispatchEvent(pointer('pointerup', { clientX: 60, clientY: 70 }))
    expect(onDrop).toHaveBeenCalledExactlyOnceWith(result, { x: 60, y: 70 })
  })

  it.each([{ pointerType: 'touch' }, { button: 2 }, { isPrimary: false }])(
    'does not start dragging with %j',
    (options) => {
      start(pointer('pointerdown', options))
      move()

      expect(document.body.querySelector('img')).toBeNull()
      expect(document.documentElement.classList.contains('image-pointer-dragging')).toBe(false)
    },
  )
})
