import { beforeEach, describe, expect, test } from 'vitest'
import { clearStoredAuth } from '../../../../core/utils/auth'
import { primeAuthenticatedWorkspace, selectWorkspace } from '../../testing'
import { useSharePanel } from './useSharePanel'

const WORKSPACE = 'ws_share123'

function primeScope() {
  primeAuthenticatedWorkspace(WORKSPACE)
}

describe('useSharePanel', () => {
  beforeEach(() => {
    localStorage.clear()
    clearStoredAuth()
    primeScope()
  })

  test('opens a board target and closes without clearing until hidden', () => {
    const panel = useSharePanel()
    expect(panel.open('board-1', 'Campaign')).toBe(true)
    expect(panel.isOpen.value).toBe(true)
    expect(panel.target.value).toEqual({ boardId: 'board-1', boardName: 'Campaign' })

    panel.close()
    expect(panel.isOpen.value).toBe(false)
    expect(panel.target.value).toEqual({ boardId: 'board-1', boardName: 'Campaign' })

    panel.handleHidden()
    expect(panel.target.value).toBeNull()
    panel.dispose()
  })

  test('refuses to open without a complete scope', () => {
    const panel = useSharePanel()
    expect(panel.open('  ', 'Campaign')).toBe(false)
    expect(panel.isOpen.value).toBe(false)
    panel.dispose()
  })

  test('scope switches close the panel', () => {
    const panel = useSharePanel()
    expect(panel.open('board-1', 'Campaign')).toBe(true)
    const generation = panel.generation.value

    selectWorkspace('ws_other456')

    expect(panel.isOpen.value).toBe(false)
    expect(panel.target.value).toBeNull()
    expect(panel.generation.value).toBeGreaterThan(generation)
    panel.dispose()
  })

  test('reopening bumps the generation to separate panel opens', () => {
    const panel = useSharePanel()
    expect(panel.open('board-1', 'Campaign')).toBe(true)
    const first = panel.generation.value
    panel.close()
    panel.handleHidden()
    expect(panel.open('board-2', 'Launch')).toBe(true)
    expect(panel.generation.value).toBeGreaterThan(first)
    expect(panel.target.value).toEqual({ boardId: 'board-2', boardName: 'Launch' })
    panel.dispose()
  })
})
