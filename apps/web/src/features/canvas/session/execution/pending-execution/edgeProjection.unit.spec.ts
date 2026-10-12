import { describe, expect, it } from 'vitest'
import { projectPendingImageExecutionEdges } from './edgeProjection'

describe('projectPendingImageExecutionEdges', () => {
  it('derives image-input animation only from the Core snapshot', () => {
    const result = projectPendingImageExecutionEdges(
      [
        { id: 'active', source: 'image-1', target: 'image-2', targetHandle: 'in:image' },
        { id: 'other', source: 'image-1', target: 'prompt-1', targetHandle: 'in:prompt' },
      ],
      {
        version: 1,
        attachment: { boardId: 'board-a', epoch: 1 },
        executions: [{ resultNodeId: 'image-2', status: 'running', taskId: 'task-1' }],
      },
    )

    expect(result).toMatchObject({ didChange: true })
    expect(result.edges[0]).toMatchObject({ animated: true } satisfies Partial<
      import('@vue-flow/core').Edge
    >)
    expect(result.edges[1]).not.toHaveProperty('animated')

    const completed = projectPendingImageExecutionEdges(result.edges, {
      version: 2,
      attachment: { boardId: 'board-a', epoch: 1 },
      executions: [{ resultNodeId: 'image-2', status: 'succeeded', taskId: 'task-1' }],
    })

    expect(completed.edges[0]).toMatchObject({ animated: false } satisfies Partial<
      import('@vue-flow/core').Edge
    >)
  })
})
