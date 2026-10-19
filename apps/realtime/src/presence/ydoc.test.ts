import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { buildYCompactEnvelope, sanitizeYSnapshot, stripYEdge, stripYNode } from './ydoc.js'

describe('stripYNode', () => {
  it('keeps the whitelist and drops transient and unknown keys', () => {
    const stripped = stripYNode('n1', {
      id: 'n1',
      type: 'image',
      position: { x: 10, y: 20 },
      data: { prompt: 'cat' },
      style: { color: 'red' },
      selected: true,
      dragging: true,
      resizing: true,
      bogus: 1,
    })
    assert.ok(stripped !== null)
    assert.deepEqual(Object.keys(stripped!).sort(), ['data', 'id', 'position', 'style', 'type'])
  })

  it('drops entries with mismatched id or missing type', () => {
    assert.equal(stripYNode('n1', 'n1'), null)
    assert.equal(stripYNode('n1', { id: 'n2', type: 'image' }), null)
    assert.equal(stripYNode('n1', { id: 'n1' }), null)
  })

  it('drops a malformed position but keeps the entry', () => {
    const stripped = stripYNode('n1', {
      id: 'n1',
      type: 'image',
      position: { x: 'far', y: 1 },
    })
    assert.ok(stripped !== null)
    assert.ok(!('position' in stripped!))
  })
})

describe('stripYEdge', () => {
  it('keeps endpoints and drops transient keys', () => {
    const stripped = stripYEdge('e1', {
      id: 'e1',
      type: 'link',
      source: 'n1',
      target: 'n2',
      selected: true,
    })
    assert.deepEqual(stripped, {
      id: 'e1',
      source: 'n1',
      target: 'n2',
      type: 'link',
    })
  })

  it('drops edges without both endpoints', () => {
    assert.equal(stripYEdge('e1', { id: 'e1', source: 'n1' }), null)
  })
})

describe('sanitizeYSnapshot', () => {
  it('reduces maps and builds a valid envelope', () => {
    const snapshot = sanitizeYSnapshot({ n1: { id: 'n1', type: 'image', selected: true } }, {})
    assert.ok(!('selected' in snapshot.nodes['n1']!))
    const envelope = JSON.parse(buildYCompactEnvelope(1, snapshot, new Uint8Array([0, 0]))) as {
      schema_version: number
      payload: { nodes: unknown; edges: unknown }
    }
    assert.equal(envelope.schema_version, 1)
    assert.ok(typeof envelope.payload.nodes === 'object')
    assert.ok(typeof envelope.payload.edges === 'object')
  })
})
