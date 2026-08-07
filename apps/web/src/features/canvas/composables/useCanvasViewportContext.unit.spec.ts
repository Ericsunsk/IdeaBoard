import { describe, expect, it } from 'vitest'
import { createApp, defineComponent, h, ref, type FunctionalComponent } from 'vue'
import {
  DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL,
  type CanvasViewportContext,
  provideCanvasViewportContext,
  resolveCanvasNodeUiScale,
  useCanvasViewportContext,
} from './useCanvasViewportContext'

describe('useCanvasViewportContext', () => {
  it('resolves inverse scaling for screen-sized node controls', () => {
    expect(resolveCanvasNodeUiScale(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)).toBe(1)
    expect(resolveCanvasNodeUiScale(50)).toBe(2)
    expect(resolveCanvasNodeUiScale(200)).toBe(0.5)
    expect(resolveCanvasNodeUiScale(0)).toBe(1)
  })

  it('uses the default viewport context outside a provider', () => {
    const context = useCanvasViewportContext()

    expect(context.zoomLevel.value).toBe(DEFAULT_CANVAS_VIEWPORT_ZOOM_LEVEL)
  })

  it('provides a readonly reactive viewport context', () => {
    const zoomLevel = ref(125)
    const observedContext: { value: CanvasViewportContext | null } = { value: null }
    const Child: FunctionalComponent = () => {
      observedContext.value = useCanvasViewportContext()
      return null
    }
    const Parent = defineComponent({
      setup() {
        provideCanvasViewportContext({ zoomLevel })
        return () => h(Child)
      },
    })
    const root = document.createElement('div')
    const app = createApp(Parent)

    app.mount(root)
    expect(observedContext.value?.zoomLevel.value).toBe(125)

    zoomLevel.value = 80
    expect(observedContext.value?.zoomLevel.value).toBe(80)
    app.unmount()
  })
})
