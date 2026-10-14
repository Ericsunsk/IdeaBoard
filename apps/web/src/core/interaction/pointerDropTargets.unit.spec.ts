import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPointerDropTargets } from './pointerDropTargets'

const reference = { id: 'image-1' }

describe('Pointer drop targets', () => {
  let targets: ReturnType<typeof createPointerDropTargets<typeof reference>>

  function region(parent = document.body) {
    const element = document.createElement('div')
    parent.appendChild(element)
    vi.spyOn(element, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 100, 100))
    const onHoverChange = vi.fn()
    const onDrop = vi.fn()
    const canReceive = vi.fn(() => true)
    const unregister = targets.register({ element, canReceive, onHoverChange, onDrop })
    return { element, canReceive, onHoverChange, onDrop, unregister }
  }

  beforeEach(() => {
    targets = createPointerDropTargets<typeof reference>()
  })
  afterEach(() => {
    targets.clear()
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('prioritizes the registered folder over its panel regardless of registration order', () => {
    const panel = region()
    const folder = region(panel.element)
    panel.unregister()
    targets.register(panel)
    const label = document.createElement('span')
    folder.element.appendChild(label)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(label)

    targets.update(10, 10, reference)
    targets.drop(reference, 10, 10)

    expect(panel.onDrop).not.toHaveBeenCalled()
    expect(folder.onDrop).toHaveBeenCalledExactlyOnceWith(reference, { x: 10, y: 10 })
    expect(folder.onDrop.mock.calls[0]?.[0]).toBe(reference)
    expect(folder.onHoverChange.mock.calls).toEqual([[true], [false]])
  })

  it('clears the former hover when moving from a folder to the panel', () => {
    const panel = region()
    const folder = region(panel.element)
    const hit = vi.spyOn(document, 'elementFromPoint').mockReturnValue(folder.element)
    targets.update(10, 10, reference)
    hit.mockReturnValue(panel.element)
    targets.update(10, 10, reference)
    targets.drop(reference, 10, 10)

    expect(folder.onHoverChange.mock.calls).toEqual([[true], [false]])
    expect(panel.onHoverChange.mock.calls).toEqual([[true], [false]])
    expect(panel.onDrop).toHaveBeenCalledOnce()
  })

  it('rechecks eligibility and geometry at release', () => {
    const target = region()
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(target.element)
    targets.update(10, 10, reference)
    target.canReceive.mockReturnValue(false)
    targets.drop(reference, 10, 10)
    expect(target.onDrop).not.toHaveBeenCalled()
    expect(target.onHoverChange.mock.calls).toEqual([[true], [false]])

    target.canReceive.mockReturnValue(true)
    targets.drop(reference, 110, 10)
    expect(target.onDrop).not.toHaveBeenCalled()
  })

  it('ignores covered, hidden and detached regions', () => {
    const target = region()
    const overlay = document.createElement('div')
    document.body.appendChild(overlay)
    const hit = vi.spyOn(document, 'elementFromPoint').mockReturnValue(overlay)
    targets.drop(reference, 10, 10)

    hit.mockReturnValue(target.element)
    vi.mocked(target.element.getBoundingClientRect).mockReturnValue(new DOMRect())
    targets.drop(reference, 10, 10)
    target.element.remove()
    targets.drop(reference, 10, 10)

    expect(target.onDrop).not.toHaveBeenCalled()
    expect(target.onHoverChange).not.toHaveBeenCalled()
  })

  it('unregisters an active region and leaves other regions available', () => {
    const panel = region()
    const folder = region(panel.element)
    vi.spyOn(document, 'elementFromPoint').mockReturnValue(folder.element)
    targets.update(10, 10, reference)
    folder.unregister()
    targets.drop(reference, 10, 10)

    expect(folder.onHoverChange.mock.calls).toEqual([[true], [false]])
    expect(folder.onDrop).not.toHaveBeenCalled()
    expect(panel.onDrop).toHaveBeenCalledOnce()
  })
})
