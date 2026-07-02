import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import { useCanvasTaskRuntime } from './useCanvasTaskRuntime'

const mocks = vi.hoisted(() => ({
  planner: { kind: 'planner' },
  workflow: { kind: 'workflow' },
  commands: { kind: 'commands' },
  createImageExecutionPlanner: vi.fn(),
  useFlowImageWorkflowExecution: vi.fn(),
  createImageWorkflowCommands: vi.fn(),
}))

vi.mock('../execution', () => ({
  createImageExecutionPlanner: mocks.createImageExecutionPlanner,
}))

vi.mock('../execution/runtime/flowImageWorkflowExecution', () => ({
  useFlowImageWorkflowExecution: mocks.useFlowImageWorkflowExecution,
}))

vi.mock('../execution/runtime/imageWorkflowCommandRuntime', () => ({
  createImageWorkflowCommands: mocks.createImageWorkflowCommands,
}))

describe('useCanvasTaskRuntime', () => {
  it('owns the image workflow execution seam for canvas task feedback', () => {
    const nodes = ref([{ id: 'node-a' }]) as Ref<Node[]>
    const edges = ref([{ id: 'edge-a' }]) as Ref<Edge[]>
    const flowTransactions = {
      commitGraphMutation: vi.fn(),
      commitNodeMutation: vi.fn(),
      commitEdgeMutation: vi.fn(),
      commitDirty: vi.fn(),
    }
    const flushAutosave = vi.fn()
    const currentBoardId = ref('board-a')
    const refreshNodeInternals = vi.fn()
    const feedback = {
      succeeded: vi.fn(),
      warning: vi.fn(),
    }
    mocks.createImageExecutionPlanner.mockReturnValue(mocks.planner)
    mocks.useFlowImageWorkflowExecution.mockReturnValue(mocks.workflow)
    mocks.createImageWorkflowCommands.mockReturnValue(mocks.commands)

    const runtime = useCanvasTaskRuntime({
      nodes,
      edges,
      flowTransactions,
      flushAutosave,
      currentBoardId,
      refreshNodeInternals,
      feedback,
    })
    const graphReader = mocks.createImageExecutionPlanner.mock.calls[0]?.[0]

    expect(graphReader()).toEqual({
      nodes: nodes.value,
      edges: edges.value,
    })
    expect(mocks.useFlowImageWorkflowExecution).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes,
        edges,
        commitGraphMutation: flowTransactions.commitGraphMutation,
        commitNodeMutation: flowTransactions.commitNodeMutation,
        commitEdgeMutation: flowTransactions.commitEdgeMutation,
        flushAutosave,
        refreshNodeInternals,
        onSuccess: feedback.succeeded,
        onWarning: feedback.warning,
      }),
    )
    const workflowOptions = mocks.useFlowImageWorkflowExecution.mock.calls[0]?.[0]
    expect(workflowOptions.getCurrentBoardId()).toBe('board-a')
    expect(mocks.createImageWorkflowCommands).toHaveBeenCalledWith({
      planner: mocks.planner,
      workflow: mocks.workflow,
      onWarning: feedback.warning,
    })
    expect(runtime).toBe(mocks.commands)
  })
})
