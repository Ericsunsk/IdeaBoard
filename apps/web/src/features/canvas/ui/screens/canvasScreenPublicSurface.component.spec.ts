import { defineComponent, h, ref } from 'vue'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { render } from '../../../../test/render'
import * as screenBarrel from './index'
import CanvasScreenComponent from './CanvasScreen.vue'
import { useCanvasScreenImageToolUi } from './internal/useCanvasScreenImageToolUi'
import { useCanvasScreenLifecycle } from './internal/useCanvasScreenLifecycle'

// Canvas Screen public-surface behavior contract.
//
// Observable replacement for three retired
// `canvasScreenArchitecture.contract.spec.ts` source-string groups (ledger
// `src/features/canvas/presentation/screens/canvasScreenArchitecture.contract.spec.ts`):
// - "keeps CanvasScreen as the sole public workspace-ui seam" (lines 20-21,
//   `behavior-existing-partial`, 2 IDs): the barrel module surface exposes
//   exactly CanvasScreen and resolves to the Screen component —
//   `CanvasPage.vue` imports `{ CanvasScreen } from '../screens'`, so this
//   is the seam production reads. A renamed file, a dropped export, or an
//   internal leak changes the module surface and fails here; file text is
//   never read.
// - "gets Image Workflow locks and selection from runtime seams instead of
//   Screen callbacks" (lines 49-50, 2 IDs): the Screen-provided ui adapter
//   carries only tool/panel refs at runtime — no lock flag, no selection
//   pass-through. Locks come from the graph seam and selection from the
//   flow-pane adapter (covered in `useCanvasImageWorkflow.unit.spec.ts`
//   "wires Image Tool Session without exposing fill execution internals").
// - "assembles cohesive private ownership modules ..." benchmark half
//   (lines 29-30, 2 IDs): the benchmark hook is installed only by the
//   lifecycle module behind the explicit query flag, and removed on
//   unmount — never by the Screen itself.

function createRouterPlugin() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/', component: { template: '<div />' } }],
  })
  return router
}

function mountLifecycle(query: Record<string, string | undefined>) {
  const persistence = { recoverBoardSession: vi.fn(async () => {}) }
  const benchmark = { connectIntent: vi.fn(), onNodesChange: vi.fn() }
  const router = createRouterPlugin()
  const Inner = defineComponent({
    setup() {
      useCanvasScreenLifecycle({
        route: { query } as never,
        persistence: persistence as never,
        history: { undo: vi.fn(), redo: vi.fn() } as never,
        duplicateModifierPressed: ref(false),
        keyboard: {
          onDeleteInlineSelection: () => false,
          isShortcutSuppressed: () => false,
          onEscape: () => false,
          onPanViewport: vi.fn() as never,
          setActiveTool: vi.fn(),
        },
        benchmark,
      })
      return () => h('div')
    },
  })
  // The shell toggles the lifecycle host through v-if so unmount (and its
  // benchmark cleanup) runs without a raw unmount call.
  const Shell = defineComponent({
    props: { visible: { type: Boolean, default: true } },
    setup(props) {
      return () => (props.visible ? h(Inner) : h('div'))
    },
  })
  const view = render(Shell, { props: { visible: true }, global: { plugins: [router] } })
  return { view, persistence, benchmark }
}

describe('CanvasScreen public surface', () => {
  beforeEach(() => {
    delete (window as unknown as Record<string, unknown>).__ideaboardCanvasBenchmarkActions
  })

  it('exposes CanvasScreen as the sole public workspace-ui seam', () => {
    expect(Object.keys(screenBarrel).sort()).toEqual(['CanvasScreen'])
    expect(screenBarrel.CanvasScreen).toBe(CanvasScreenComponent)
  })

  it('provides an Image Workflow ui adapter without locks or selection pass-through', () => {
    const { imageWorkflowUi } = useCanvasScreenImageToolUi({
      activeTool: ref('select'),
      canvasWrapRef: ref(null),
    })

    expect(Object.keys(imageWorkflowUi).sort()).toEqual([
      'activeTool',
      'canvasWrapRef',
      'getImageStageElement',
      'panels',
    ])
    expect('isCanvasInteractionLocked' in imageWorkflowUi).toBe(false)
    expect('selection' in imageWorkflowUi).toBe(false)
    expect(Object.keys(imageWorkflowUi.panels).sort()).toEqual([
      'clear',
      'getActiveNodeId',
      'isActive',
      'open',
    ])

    // The adapter carries live panel refs, not snapshots.
    imageWorkflowUi.panels.open({ kind: 'image-tools', nodeId: 'node-a' })
    expect(imageWorkflowUi.panels.isActive('image-tools', 'node-a')).toBe(true)
    expect(imageWorkflowUi.panels.getActiveNodeId('image-tools')).toBe('node-a')
    imageWorkflowUi.panels.clear()
    expect(imageWorkflowUi.panels.getActiveNodeId('image-tools')).toBeNull()
  })

  it('installs benchmark actions only behind the explicit flag and removes them on unmount', async () => {
    const flagged = mountLifecycle({ canvasBenchmarkActions: '1' })
    const hook = (
      window as unknown as {
        __ideaboardCanvasBenchmarkActions?: {
          connectDelete: (target: {
            sourceNodeId: string
            targetNodeId: string
            sourcePort: string
            targetPort: string
            deleteNodeId: string
          }) => void
        }
      }
    ).__ideaboardCanvasBenchmarkActions
    expect(hook).toBeDefined()
    expect(flagged.persistence.recoverBoardSession).toHaveBeenCalledTimes(1)

    hook!.connectDelete({
      sourceNodeId: 'image-1',
      targetNodeId: 'prompt-1',
      sourcePort: 'output',
      targetPort: 'input',
      deleteNodeId: 'fusion-1',
    })
    expect(flagged.benchmark.connectIntent).toHaveBeenCalledWith({
      sourceNodeId: 'image-1',
      targetNodeId: 'prompt-1',
      sourcePort: 'output',
      targetPort: 'input',
    })
    expect(flagged.benchmark.onNodesChange).toHaveBeenCalledWith([{ id: 'fusion-1', type: 'remove' }])

    await flagged.view.rerender({ visible: false })
    expect(
      (window as unknown as Record<string, unknown>).__ideaboardCanvasBenchmarkActions,
    ).toBeUndefined()
  })

  it('leaves benchmark actions uninstalled without the flag', async () => {
    const plain = mountLifecycle({})
    expect(
      (window as unknown as Record<string, unknown>).__ideaboardCanvasBenchmarkActions,
    ).toBeUndefined()
    expect(plain.persistence.recoverBoardSession).toHaveBeenCalledTimes(1)
    await plain.view.rerender({ visible: false })
  })
})
