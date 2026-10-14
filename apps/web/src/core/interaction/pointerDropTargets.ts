type PointerDropTarget<T> = {
  element: HTMLElement
  canReceive: (payload: T) => boolean
  onHoverChange: (isHovered: boolean) => void
  onDrop: (payload: T, point: { x: number; y: number }) => void
}

export type PointerDropTargets<T> = ReturnType<typeof createPointerDropTargets<T>>

export function createPointerDropTargets<T>() {
  let cancelDrag: (() => void) | null = null
  const targets = new Set<PointerDropTarget<T>>()
  let active: PointerDropTarget<T> | null = null

  function setActive(next: PointerDropTarget<T> | null) {
    if (active === next) return
    active?.onHoverChange(false)
    active = next
    active?.onHoverChange(true)
  }

  function update(x: number, y: number, payload: T) {
    const hit = document.elementFromPoint(x, y)
    let next: PointerDropTarget<T> | null = null
    for (const target of targets) {
      if (!target.element.isConnected || !target.canReceive(payload) || !hit) continue
      const rect = target.element.getBoundingClientRect()
      if (x < rect.left || x >= rect.right || y < rect.top || y >= rect.bottom) continue
      if (!target.element.contains(hit)) continue
      if (!next || next.element.contains(target.element)) next = target
    }
    setActive(next)
  }

  return {
    begin(cancel: () => void) {
      cancelDrag?.()
      cancelDrag = cancel
      return () => {
        if (cancelDrag === cancel) cancelDrag = null
      }
    },
    cancel() {
      cancelDrag?.()
      setActive(null)
    },
    register(target: PointerDropTarget<T>) {
      targets.add(target)
      return () => {
        targets.delete(target)
        if (active === target) setActive(null)
      }
    },
    update,
    drop(reference: T, x: number, y: number) {
      update(x, y, reference)
      const target = active
      setActive(null)
      target?.onDrop(reference, { x, y })
    },
    clear() {
      setActive(null)
    },
  }
}
