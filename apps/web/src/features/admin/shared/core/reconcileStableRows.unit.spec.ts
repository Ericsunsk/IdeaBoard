import { describe, expect, it } from 'vitest'
import { reconcileStableRows } from './reconcileStableRows'

type Row = { id: string; enabled: boolean; label: string }

function isSameRow(cached: Row, incoming: Row): boolean {
  return (
    cached.id === incoming.id &&
    cached.enabled === incoming.enabled &&
    cached.label === incoming.label
  )
}

describe('reconcileStableRows', () => {
  it('reuses the cached reference when content is unchanged', () => {
    const cache = new Map<string, Row>()
    const first = reconcileStableRows(
      cache,
      [{ id: 'a', enabled: true, label: 'A' }],
      (row) => row.id,
      isSameRow,
    )
    const second = reconcileStableRows(
      cache,
      [{ id: 'a', enabled: true, label: 'A' }],
      (row) => row.id,
      isSameRow,
    )
    expect(second[0]).toBe(first[0])
  })

  it('returns a new reference only for the changed row', () => {
    const cache = new Map<string, Row>()
    const first = reconcileStableRows(
      cache,
      [
        { id: 'a', enabled: true, label: 'A' },
        { id: 'b', enabled: false, label: 'B' },
      ],
      (row) => row.id,
      isSameRow,
    )
    const second = reconcileStableRows(
      cache,
      [
        { id: 'a', enabled: false, label: 'A' },
        { id: 'b', enabled: false, label: 'B' },
      ],
      (row) => row.id,
      isSameRow,
    )
    expect(second[0]).not.toBe(first[0])
    expect(second[1]).toBe(first[1])
  })

  it('prunes keys that disappear from the next page', () => {
    const cache = new Map<string, Row>()
    reconcileStableRows(cache, [{ id: 'a', enabled: true, label: 'A' }], (row) => row.id, isSameRow)
    reconcileStableRows(cache, [], (row: Row) => row.id, isSameRow)
    expect(cache.size).toBe(0)
  })
})
