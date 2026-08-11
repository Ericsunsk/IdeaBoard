import { describe, expect, it, vi } from 'vitest'
import { shallowRef } from 'vue'
import { useFlowHistory } from './useFlowHistory'

function createHistoryHarness(
  initialNodes: unknown[] = [],
  initialEdges: unknown[] = [],
  initialGroups: unknown[] = [],
) {
  const nodes = shallowRef<unknown[]>(initialNodes)
  const edges = shallowRef<unknown[]>(initialEdges)
  const groups = shallowRef<unknown[]>(initialGroups)
  const onRestore = vi.fn()
  const history = useFlowHistory({
    nodes,
    edges,
    groups,
    onRestore,
    commitNodes: (next) => {
      nodes.value = next
    },
    commitEdges: (next) => {
      edges.value = next
    },
    commitGroups: (next) => {
      groups.value = next
    },
  })

  return { nodes, edges, groups, history, onRestore }
}

describe('useFlowHistory', () => {
  it('undoes, redoes, and clears the baseline', () => {
    const { nodes, edges, history } = createHistoryHarness(
      [{ id: 'n1', position: { x: 0, y: 0 } }],
      [{ id: 'e1' }],
    )

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
    const { nodes, history } = createHistoryHarness([{ id: 'n1', value: 1 }])

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
    const n1 = { id: 'n1' }
    const n2 = { id: 'n2' }
    const { nodes, history, onRestore } = createHistoryHarness([n1, n2])

    const before = history.capture()
    nodes.value = [n2, n1]
    const pushed = history.push(before)
    expect(pushed).toBe(true)

    history.undo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n1')
    expect(nodes.value[0]).toBe(n1)
    expect(onRestore).toHaveBeenCalledTimes(1)

    history.redo()
    expect((nodes.value[0] as { id: string })?.id).toBe('n2')
    expect(nodes.value[0]).toBe(n2)
    expect(onRestore).toHaveBeenCalledTimes(2)
  })

  it('does not push no-op snapshots when arrays change but item values do not', () => {
    const { nodes, history, onRestore } = createHistoryHarness([{ id: 'n1', value: 1 }])

    const before = history.capture()
    nodes.value = [{ id: 'n1', value: 1 }]
    const pushed = history.push(before)

    expect(pushed).toBe(false)
    history.undo()
    expect((nodes.value[0] as { value: number })?.value).toBe(1)
    expect(onRestore).not.toHaveBeenCalled()
  })

  it('returns opaque history snapshots', () => {
    const { history } = createHistoryHarness([{ id: 'n1', value: 1 }])

    const captured = history.capture()

    expect(Object.keys(captured)).toEqual([])
    expect(Object.isFrozen(captured)).toBe(true)
  })

  it('tracks value changes and restores cloned before and after values', () => {
    const { nodes, history } = createHistoryHarness([{ id: 'n1', data: { text: 'before' } }])

    const before = history.capture()
    nodes.value = [{ id: 'n1', data: { text: 'after' } }]
    const pushed = history.push(before)
    nodes.value = [{ id: 'n1', data: { text: 'mutated after push' } }]

    expect(pushed).toBe(true)
    history.undo()
    expect((nodes.value[0] as { data: { text: string } })?.data.text).toBe('before')

    history.redo()
    expect((nodes.value[0] as { data: { text: string } })?.data.text).toBe('after')
  })

  it('tracks in-place item mutations after capture', () => {
    const node = { id: 'n1', position: { x: 0, y: 0 }, data: { text: 'before' } }
    const { nodes, history } = createHistoryHarness([node])

    const before = history.capture()
    node.position.x = 20
    node.data.text = 'after'
    const pushed = history.push(before)
    node.position.x = 40
    node.data.text = 'mutated after push'

    expect(pushed).toBe(true)
    history.undo()
    expect(nodes.value[0]).toMatchObject({
      id: 'n1',
      position: { x: 0, y: 0 },
      data: { text: 'before' },
    })

    history.redo()
    expect(nodes.value[0]).toMatchObject({
      id: 'n1',
      position: { x: 20, y: 0 },
      data: { text: 'after' },
    })
  })

  it('tracks added and removed nodes and edges', () => {
    const { nodes, edges, history } = createHistoryHarness(
      [
        { id: 'n1', value: 1 },
        { id: 'n2', value: 2 },
      ],
      [{ id: 'e1', source: 'n1', target: 'n2' }],
    )

    const before = history.capture()
    nodes.value = [
      { id: 'n2', value: 2 },
      { id: 'n3', value: 3 },
    ]
    edges.value = [{ id: 'e2', source: 'n2', target: 'n3' }]
    const pushed = history.push(before)

    expect(pushed).toBe(true)
    history.undo()
    expect(nodes.value.map((node) => (node as { id: string }).id)).toEqual(['n1', 'n2'])
    expect(edges.value.map((edge) => (edge as { id: string }).id)).toEqual(['e1'])

    history.redo()
    expect(nodes.value.map((node) => (node as { id: string }).id)).toEqual(['n2', 'n3'])
    expect(edges.value.map((edge) => (edge as { id: string }).id)).toEqual(['e2'])
  })

  it('excludes runtime edge animation from history snapshots and restores', () => {
    const { edges, history, onRestore } = createHistoryHarness(
      [],
      [{ id: 'e1', source: 'n1', target: 'n2', animated: false, label: 'before' }],
    )

    const projectionOnlyBefore = history.capture()
    edges.value = [{ id: 'e1', source: 'n1', target: 'n2', animated: true, label: 'before' }]
    expect(history.push(projectionOnlyBefore)).toBe(false)

    const semanticBefore = history.capture()
    edges.value = [{ id: 'e1', source: 'n1', target: 'n2', animated: true, label: 'after' }]
    expect(history.push(semanticBefore)).toBe(true)

    history.undo()
    expect(edges.value).toEqual([{ id: 'e1', source: 'n1', target: 'n2', label: 'before' }])
    history.redo()
    expect(edges.value).toEqual([{ id: 'e1', source: 'n1', target: 'n2', label: 'after' }])
    expect(onRestore).toHaveBeenCalledTimes(2)
  })

  it('tracks optional groups as a history sidecar', () => {
    const { groups, history, onRestore } = createHistoryHarness(
      [{ id: 'n1' }, { id: 'n2' }],
      [],
      [],
    )

    const before = history.capture()
    groups.value = [{ id: 'group-1', nodeIds: ['n1', 'n2'] }]
    const pushed = history.push(before)

    expect(pushed).toBe(true)

    history.undo()
    expect(groups.value).toEqual([])
    expect(onRestore).toHaveBeenCalledTimes(1)

    history.redo()
    expect(groups.value).toEqual([{ id: 'group-1', nodeIds: ['n1', 'n2'] }])
    expect(onRestore).toHaveBeenCalledTimes(2)
  })
})
