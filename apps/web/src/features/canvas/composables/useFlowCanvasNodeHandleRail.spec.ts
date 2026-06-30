import { createApp, defineComponent, nextTick, ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useFlowCanvasNodeHandleRail } from './useFlowCanvasNodeHandleRail'

type HandleRailHarness = {
  rail: ReturnType<typeof useFlowCanvasNodeHandleRail>
  flowNodeRef: ReturnType<typeof ref<HTMLElement | null>>
  unmount: () => void
}

function mountHandleRail(initialRoot: HTMLElement | null): HandleRailHarness {
  const flowNodeRef = ref<HTMLElement | null>(initialRoot)
  let rail!: ReturnType<typeof useFlowCanvasNodeHandleRail>
  const host = document.createElement('div')
  document.body.appendChild(host)
  const app = createApp(
    defineComponent({
      setup() {
        rail = useFlowCanvasNodeHandleRail(flowNodeRef)
        return () => null
      },
    }),
  )
  app.mount(host)

  return {
    rail,
    flowNodeRef,
    unmount: () => {
      app.unmount()
      host.remove()
    },
  }
}

function createWorkflowNodeRoot(options: {
  frameLeft: number
  frameTop: number
  frameWidth: number
  frameHeight: number
  rootWidth?: number
  rootHeight?: number
}) {
  const root = document.createElement('div')
  const workflowNode = document.createElement('div')
  const header = document.createElement('div')
  const frame = document.createElement('div')

  workflowNode.className = 'workflow-node'
  header.className = 'workflow-node-header'
  frame.dataset.flowHandleFrame = ''

  workflowNode.append(header, frame)
  root.append(workflowNode)

  setElementLayout(root, {
    width: options.rootWidth ?? 1,
    height: options.rootHeight ?? 1,
    offsetParent: null,
  })
  setElementLayout(workflowNode, {
    left: 0,
    top: 0,
    width: options.rootWidth ?? 1,
    height: options.rootHeight ?? 1,
    offsetParent: root,
  })
  setElementLayout(frame, {
    left: options.frameLeft,
    top: options.frameTop,
    width: options.frameWidth,
    height: options.frameHeight,
    offsetParent: root,
  })

  return { root, frame }
}

function createWorkflowNodeRootWithoutFrame(options: { rootWidth: number; rootHeight: number }) {
  const root = document.createElement('div')
  const workflowNode = document.createElement('div')
  const header = document.createElement('div')

  workflowNode.className = 'workflow-node'
  header.className = 'workflow-node-header'
  workflowNode.append(header)
  root.append(workflowNode)

  setElementLayout(root, {
    width: options.rootWidth,
    height: options.rootHeight,
    offsetParent: null,
  })
  setElementLayout(workflowNode, {
    width: options.rootWidth,
    height: options.rootHeight,
    offsetParent: root,
  })

  return { root, workflowNode }
}

function setElementLayout(
  element: HTMLElement,
  options: {
    left?: number
    top?: number
    width: number
    height: number
    offsetParent: HTMLElement | null
  },
) {
  Object.defineProperties(element, {
    offsetLeft: { configurable: true, value: options.left ?? 0 },
    offsetTop: { configurable: true, value: options.top ?? 0 },
    offsetWidth: { configurable: true, value: options.width },
    offsetHeight: { configurable: true, value: options.height },
    offsetParent: { configurable: true, get: () => options.offsetParent },
  })
}

function installAnimationFrameMock() {
  const callbacks = new Map<number, FrameRequestCallback>()
  let frameId = 0

  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
    frameId += 1
    callbacks.set(frameId, callback)
    return frameId
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
    callbacks.delete(id)
  })

  return {
    flush() {
      const pending = Array.from(callbacks.values())
      callbacks.clear()
      for (const callback of pending) {
        callback(performance.now())
      }
    },
  }
}

class TestResizeObserver {
  static instances: TestResizeObserver[] = []

  observe = vi.fn()
  unobserve = vi.fn()
  disconnect = vi.fn()

  constructor(_callback: ResizeObserverCallback) {
    TestResizeObserver.instances.push(this)
  }
}

function installResizeObserverMock() {
  TestResizeObserver.instances = []
  vi.stubGlobal('ResizeObserver', TestResizeObserver)
}

describe('useFlowCanvasNodeHandleRail', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('updates rail bounds after requestMeasure', async () => {
    const raf = installAnimationFrameMock()
    installResizeObserverMock()
    const { root } = createWorkflowNodeRoot({
      frameLeft: 12,
      frameTop: 18,
      frameWidth: 240,
      frameHeight: 160,
    })
    const harness = mountHandleRail(root)

    try {
      await nextTick()
      harness.rail.requestMeasure()
      raf.flush()

      expect(harness.rail.handleRailStyle.value).toEqual({
        left: '12px',
        top: '18px',
        width: '240px',
        height: '160px',
      })
    } finally {
      harness.unmount()
    }
  })

  it('reuses the frame lookup for repeated measurements on the same root', async () => {
    const raf = installAnimationFrameMock()
    installResizeObserverMock()
    const { root } = createWorkflowNodeRoot({
      frameLeft: 12,
      frameTop: 18,
      frameWidth: 240,
      frameHeight: 160,
    })
    const querySelector = vi.spyOn(root, 'querySelector')
    const harness = mountHandleRail(root)

    try {
      await nextTick()
      harness.rail.requestMeasure()
      raf.flush()
      harness.rail.requestMeasure()
      raf.flush()

      expect(querySelector).toHaveBeenCalledTimes(1)
    } finally {
      harness.unmount()
    }
  })

  it('retries frame lookup after an initial missing frame', async () => {
    const raf = installAnimationFrameMock()
    installResizeObserverMock()
    const { root, workflowNode } = createWorkflowNodeRootWithoutFrame({
      rootWidth: 320,
      rootHeight: 180,
    })
    const harness = mountHandleRail(root)

    try {
      await nextTick()
      harness.rail.requestMeasure()
      raf.flush()

      expect(harness.rail.handleRailStyle.value).toEqual({
        left: '0px',
        top: '0px',
        width: '320px',
        height: '180px',
      })

      const frame = document.createElement('div')
      frame.dataset.flowHandleFrame = ''
      workflowNode.append(frame)
      setElementLayout(frame, {
        left: 24,
        top: 32,
        width: 220,
        height: 120,
        offsetParent: root,
      })

      harness.rail.requestMeasure()
      raf.flush()

      expect(harness.rail.handleRailStyle.value).toEqual({
        left: '24px',
        top: '32px',
        width: '220px',
        height: '120px',
      })
    } finally {
      harness.unmount()
    }
  })

  it('re-measures when the root element changes', async () => {
    const raf = installAnimationFrameMock()
    installResizeObserverMock()
    const first = createWorkflowNodeRoot({
      frameLeft: 4,
      frameTop: 8,
      frameWidth: 120,
      frameHeight: 90,
    })
    const second = createWorkflowNodeRoot({
      frameLeft: 30,
      frameTop: 36,
      frameWidth: 260,
      frameHeight: 180,
    })
    const harness = mountHandleRail(first.root)

    try {
      await nextTick()
      harness.rail.requestMeasure()
      raf.flush()

      harness.flowNodeRef.value = second.root
      harness.rail.requestMeasure()
      raf.flush()

      expect(harness.rail.handleRailStyle.value).toEqual({
        left: '30px',
        top: '36px',
        width: '260px',
        height: '180px',
      })

      const observer = TestResizeObserver.instances.at(-1)
      expect(observer?.unobserve).toHaveBeenCalledWith(first.root)
      expect(observer?.unobserve).toHaveBeenCalledWith(first.frame)
      expect(observer?.observe).toHaveBeenCalledWith(second.root)
      expect(observer?.observe).toHaveBeenCalledWith(second.frame)
    } finally {
      harness.unmount()
    }
  })

  it('cleans up observer and pending measurement on unmount', async () => {
    const raf = installAnimationFrameMock()
    installResizeObserverMock()
    const { root } = createWorkflowNodeRoot({
      frameLeft: 0,
      frameTop: 0,
      frameWidth: 100,
      frameHeight: 80,
    })
    const harness = mountHandleRail(root)

    await nextTick()
    harness.rail.requestMeasure()
    raf.flush()
    harness.rail.requestMeasure()
    const observer = TestResizeObserver.instances.at(-1)

    expect(() => harness.unmount()).not.toThrow()
    expect(observer?.disconnect).toHaveBeenCalledTimes(1)
    expect(window.cancelAnimationFrame).toHaveBeenCalled()
  })
})
