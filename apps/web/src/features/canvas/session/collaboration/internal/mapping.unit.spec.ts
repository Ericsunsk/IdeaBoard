import type { BoardSnapshot } from '../../../../board/ports/canvas'
// Doc-binding mapping specs: v4 strict roundtrip + transient stripping.

import { describe, expect, test } from 'vitest'
import { DOC_EDGES_MAP, DOC_NODES_MAP, docEntriesToSnapshot, snapshotToDocEntries } from './mapping'

const transientFixture = Object.fromEntries(
  ['selected', 'dragging', 'resizing'].map((key) => [key, true]),
)

function buildSnapshot(overrides: Partial<BoardSnapshot> = {}): BoardSnapshot {
  return {
    schemaVersion: 4,
    imageNodes: [
      {
        id: 2,
        type: 'image-node',
        x: 30,
        y: 40,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'idle',
        outputImages: [],
        ...transientFixture,
      },
      {
        id: 1,
        type: 'image-node',
        x: 10,
        y: 20,
        width: 280,
        sourceImageUrl: '',
        sourceImageRatio: 1,
        outputStatus: 'idle',
        outputImages: [],
        ...transientFixture,
      },
    ],
    promptNodes: [
      {
        id: 7,
        type: 'prompt',
        x: 1,
        y: 2,
        width: 240,
        text: 'hello',
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ],
    fusionNodes: [],
    connections: [
      {
        id: 9,
        fromNodeType: 'image',
        fromNodeId: 1,
        fromPort: 'image',
        toNodeType: 'prompt',
        toNodeId: 7,
        toPort: 'image',
      },
    ],
    groups: [{ id: 'group-1', nodeIds: [], x: 0, y: 0, width: 10, height: 10 }],
    ...overrides,
  }
}

describe('doc mapping snapshot roundtrip', () => {
  test('uses the frozen nodes/edges map names and v4 strict version', () => {
    expect(DOC_NODES_MAP).toBe('nodes')
    expect(DOC_EDGES_MAP).toBe('edges')
    expect(buildSnapshot().schemaVersion).toBe(4)
  })

  test('round-trips nodes, edges, and groups through entries', () => {
    const entries = snapshotToDocEntries(buildSnapshot())
    expect(entries.nodes.length).toBe(4)
    expect(entries.edges.length).toBe(1)
    expect(entries.nodes.find(([key]) => key === 'image-1')?.[1]).toMatchObject({
      id: 'image-1',
      type: 'image',
      position: { x: 10, y: 20 },
      data: { id: 1 },
    })
    expect(entries.edges[0]?.[1]).toMatchObject({
      id: entries.edges[0]?.[0],
      source: 'image-1',
      target: 'prompt-7',
    })
    const rebuilt = docEntriesToSnapshot(new Map(entries.nodes), new Map(entries.edges))
    expect(rebuilt?.schemaVersion).toBe(4)
    expect(rebuilt?.imageNodes.map((node) => node.id)).toEqual([1, 2])
    expect(rebuilt?.promptNodes.map((node) => node.id)).toEqual([7])
    expect(rebuilt?.connections.map((edge) => edge.id)).toEqual([9])
    expect(rebuilt?.groups.map((group) => group.id)).toEqual(['group-1'])
  })

  test('strips selected/dragging/resizing before CRDT writes', () => {
    const entries = snapshotToDocEntries(buildSnapshot())
    for (const [, entry] of entries.nodes) {
      expect(entry.data).not.toHaveProperty('selected')
      expect(entry.data).not.toHaveProperty('dragging')
      expect(entry.data).not.toHaveProperty('resizing')
    }
  })

  test('fails closed on version mismatch and malformed frames', () => {
    expect(() =>
      snapshotToDocEntries(buildSnapshot({ schemaVersion: 3 as unknown as 4 })),
    ).toThrow()
    expect(docEntriesToSnapshot(new Map([['x', { nope: 1 }]]), new Map())).toBeNull()
    expect(docEntriesToSnapshot(new Map(), new Map([['e', null]]))).toBeNull()
  })
})
