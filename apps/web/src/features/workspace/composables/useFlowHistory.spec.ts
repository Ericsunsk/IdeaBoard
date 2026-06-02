import { ref } from 'vue'
import { useFlowHistory } from './useFlowHistory'

function expect(condition: unknown, message: string) {
  if (!condition) throw new Error(message)
}

function testUndoRedoAndBaseline() {
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
  expect(pushed, 'expected changed snapshot to be pushed')

  history.undo()
  expect((nodes.value[0] as { id: string })?.id === 'n1', 'expected undo to restore nodes')
  expect(nodes.value.length === 1, 'expected undo to remove added node')
  expect((edges.value[0] as { id: string })?.id === 'e1', 'expected undo to restore edges')

  history.redo()
  expect((nodes.value[0] as { id: string })?.id === 'n2', 'expected redo to restore node order')
  expect(
    ((nodes.value[1] as { position: { x: number } })?.position.x ?? 0) === 10,
    'expected redo to restore patched node fields',
  )
  expect((edges.value[0] as { id: string })?.id === 'e2', 'expected redo to restore edges')

  history.clearBaseline()
  nodes.value = [{ id: 'n3' }]
  history.undo()
  expect(
    (nodes.value[0] as { id: string })?.id === 'n3',
    'expected baseline clear to reset history',
  )
}

function testRedoStackClearedAfterNewPush() {
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

  expect(
    ((nodes.value[0] as { value: number })?.value ?? 0) === 3,
    'expected new push after undo to clear redo stack',
  )
}

function testOrderOnlyMutation() {
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
  expect(pushed, 'expected order-only mutation to be pushed')

  history.undo()
  expect((nodes.value[0] as { id: string })?.id === 'n1', 'expected undo to restore node order')

  history.redo()
  expect((nodes.value[0] as { id: string })?.id === 'n2', 'expected redo to restore node order')
}

function run() {
  testUndoRedoAndBaseline()
  testRedoStackClearedAfterNewPush()
  testOrderOnlyMutation()
  console.log('useFlowHistory.spec.ts passed')
}

run()
