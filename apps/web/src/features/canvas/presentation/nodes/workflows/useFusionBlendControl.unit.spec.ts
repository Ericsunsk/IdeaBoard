import { computed, effectScope, ref, type EffectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFusionBlendControl } from './useFusionBlendControl'

function createPointerEvent(type: string, init: Record<string, unknown> = {}) {
  const event = new Event(type, { bubbles: true, cancelable: true }) as PointerEvent
  Object.defineProperties(event, {
    clientX: { value: init.clientX ?? 0 },
    clientY: { value: init.clientY ?? 0 },
    pointerId: { value: init.pointerId ?? 1 },
  })
  return event
}

function mountBlendControl() {
  const stage = document.createElement('div')
  document.body.appendChild(stage)
  vi.spyOn(stage, 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: 200,
    height: 100,
    right: 200,
    bottom: 100,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect)
  const handle = document.createElement('button')
  document.body.appendChild(handle)

  const onCommit = vi.fn()
  const scope = effectScope()
  const control = scope.run(() =>
    useFusionBlendControl({
      stageRef: ref(stage),
      enabled: ref(true),
      imageCount: ref(2),
      blendPoint: ref({ x: 0.5, y: 0.5 }),
      nodeWidth: computed(() => 286),
      onCommit,
    }),
  )
  if (!control) throw new Error('expected blend control')
  return { stage, handle, onCommit, control, scope }
}

function pointerDownOnHandle(
  control: ReturnType<typeof useFusionBlendControl>,
  handle: HTMLElement,
  init: Record<string, unknown> = {},
) {
  const event = createPointerEvent('pointerdown', { pointerId: 7, clientX: 50, clientY: 25, ...init })
  Object.defineProperty(event, 'currentTarget', { value: handle })
  control.onBlendHandlePointerDown(event)
}

describe('useFusionBlendControl', () => {
  const scopes: EffectScope[] = []
  afterEach(() => {
    for (const scope of scopes.splice(0)) scope.stop()
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('commits the draft blend point on pointerup', () => {
    const { handle, onCommit, control, scope } = mountBlendControl()
    scopes.push(scope)
    pointerDownOnHandle(control, handle)

    window.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 150, clientY: 75 }))
    expect(control.blendDragging.value).toBe(true)

    window.dispatchEvent(createPointerEvent('pointerup', { pointerId: 7, clientX: 150, clientY: 75 }))
    expect(onCommit).toHaveBeenCalledTimes(1)
    expect(control.blendDragging.value).toBe(false)
  })

  it('reverts the draft blend point on pointercancel without committing', () => {
    const { handle, onCommit, control, scope } = mountBlendControl()
    scopes.push(scope)
    pointerDownOnHandle(control, handle)

    window.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 150, clientY: 75 }))
    expect(control.blendDragging.value).toBe(true)

    window.dispatchEvent(createPointerEvent('pointercancel', { pointerId: 7, clientX: 150, clientY: 75 }))
    expect(onCommit).not.toHaveBeenCalled()
    expect(control.blendDragging.value).toBe(false)
    expect(control.activeBlendPoint.value).toEqual({ x: 0.5, y: 0.5 })
  })

  it('ignores window pointer moves when no drag is active', () => {
    const { onCommit, control, scope } = mountBlendControl()
    scopes.push(scope)
    window.dispatchEvent(createPointerEvent('pointermove', { pointerId: 7, clientX: 150, clientY: 75 }))
    window.dispatchEvent(createPointerEvent('pointerup', { pointerId: 7, clientX: 150, clientY: 75 }))
    expect(onCommit).not.toHaveBeenCalled()
    expect(control.blendDragging.value).toBe(false)
  })
})
