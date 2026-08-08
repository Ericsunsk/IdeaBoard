import { describe, expect, expectTypeOf, it, vi } from 'vitest'
import type { Node, NodeChange } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { CanvasWorkspaceRuntime } from './types'
import { useCanvasWorkspaceRuntime } from './useCanvasWorkspaceRuntime'

const mocks = vi.hoisted(() => ({
  createCanvasWorkspaceRuntime: vi.fn(),
  provideCanvasBillingReadiness: vi.fn(),
}))

vi.mock('./internal/createCanvasWorkspaceRuntimeParts', () => ({
  createCanvasWorkspaceRuntime: mocks.createCanvasWorkspaceRuntime,
}))

vi.mock('../execution', () => ({
  provideCanvasBillingReadiness: mocks.provideCanvasBillingReadiness,
}))

describe('workspace-runtime useCanvasWorkspaceRuntime', () => {
  it('returns the stable runtime facade assembled internally', () => {
    const assembledRuntime = {} as CanvasWorkspaceRuntime
    mocks.createCanvasWorkspaceRuntime.mockReturnValue(assembledRuntime)

    expect(useCanvasWorkspaceRuntime({} as never)).toBe(assembledRuntime)
    expect(mocks.provideCanvasBillingReadiness).toHaveBeenCalledOnce()
    expect(mocks.createCanvasWorkspaceRuntime).toHaveBeenCalledWith({})
  })

  it('keeps the Canvas read and write contract available at the public seam', () => {
    expectTypeOf<CanvasWorkspaceRuntime['viewModel']['flowPane']['nodes']>().toEqualTypeOf<
      Ref<Node[]>
    >()
    expectTypeOf<
      CanvasWorkspaceRuntime['commands']['flowPane']['onNodesChange']
    >().toEqualTypeOf<(changes: NodeChange[]) => void>()
    expectTypeOf<
      CanvasWorkspaceRuntime['commands']['flowPane']['connectionAdapter']['vueFlowBindings']
    >().toHaveProperty('onConnect')
    expectTypeOf<
      CanvasWorkspaceRuntime['commands']['graphEditing']['createNodeAtPosition']
    >().toBeFunction()
    expectTypeOf<
      CanvasWorkspaceRuntime['commands']['imageWorkflow']['runImageDirectAction']
    >().toBeFunction()
  })
})
