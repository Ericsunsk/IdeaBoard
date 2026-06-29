import { describe, expect, it } from 'vitest'
import { ref } from 'vue'
import { useFlowHistory } from './useFlowHistory'

describe('useFlowHistory', () => {
  it('undoes, redoes, and clears the baseline', () => {
    const nodes = ref<unknown[]>([{ id: 'n1', position: { x: 0, y: 0 } }])
    const edges = ref<unknown[]>([{ id: 'e1' }])
    const history = useFlowHistory({
      nodes,
      edges,
      commitNodes: (next) => {
        nodes.value = next
      },
      commitEdges: (next) => {
        edges.value = next
      },
    })

    const before = history.capture()
    nodes.value = [
      { id: 'n2', position: { x: 20, y: 0 } },
      { id: 'n1', position: { x: 10, y: 0 } },
    ]
    edges.value = [{ id: 'e2' }]
    const pushed = history.push(before)
    expect(pushed).toBe(true)

    history.undo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n1')
    expect(nodes.value.length).toBe(1)
    expect((edges.value[0] as { id: string })?.id).toBe('e1')

    history.redo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n2')
    expect((nodes.value[1] as { position: { x: number } })?.position.x ?? 0).toBe(10)
    expect((edges.value[0] as { id: string })?.id).toBe('e2')

    history.clearBaseline()
    nodes.value = [{ id: 'n3' }]
    history.undo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n3')
  })

  it('clears the redo stack after a new push', () => {
    const nodes = ref<unknown[]>([{ id: 'n1', value: 1 }])
    const edges = ref<unknown[]>([])
    const history = useFlowHistory({
      nodes,
      edges,
      commitNodes: (next) => {
        nodes.value = next
      },
      commitEdges: (next) => {
        edges.value = next
      },
    })

    const firstBefore = history.capture()
    nodes.value = [{ id: 'n1', value: 2 }]
    history.push(firstBefore)
    history.undo()

    const secondBefore = history.capture()
    nodes.value = [{ id: 'n1', value: 3 }]
    history.push(secondBefore)
    history.redo()

    expect((nodes.value[0] as { value: number })?.value ?? 0).toBe(3)
  })

  it('tracks order-only mutations', () => {
    const nodes = ref<unknown[]>([{ id: 'n1' }, { id: 'n2' }])
    const edges = ref<unknown[]>([])
    const history = useFlowHistory({
      nodes,
      edges,
      commitNodes: (next) => {
        nodes.value = next
      },
      commitEdges: (next) => {
        edges.value = next
      },
    })

    const before = history.capture()
    nodes.value = [{ id: 'n2' }, { id: 'n1' }]
    const pushed = history.push(before)
    expect(pushed).toBe(true)

    history.undo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n1')

    history.redo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n2')
  })
})
