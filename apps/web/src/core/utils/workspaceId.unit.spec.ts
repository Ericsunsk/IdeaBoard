import { describe, expect, it } from 'vitest'
import { assertWorkspaceId, requireWorkspaceId } from './workspaceId'

describe('assertWorkspaceId', () => {
  it('trims and accepts canonical workspace ids', () => {
    expect(assertWorkspaceId('  ws_abc123  ')).toBe('ws_abc123')
    expect(assertWorkspaceId('ws_a-b_c')).toBe('ws_a-b_c')
  })

  it('rejects malformed ids without network access', () => {
    expect(() => assertWorkspaceId('nope')).toThrow('Invalid workspace id: expected ws_...')
    expect(() => assertWorkspaceId('ws_')).toThrow('Invalid workspace id: expected ws_...')
    expect(() => assertWorkspaceId('task_abc123')).toThrow('Invalid workspace id: expected ws_...')
    expect(() => assertWorkspaceId('   ')).toThrow('Invalid workspace id: expected ws_...')
  })
})

describe('requireWorkspaceId', () => {
  it('returns the trimmed explicit value', () => {
    expect(requireWorkspaceId('  ws_abc123  ', 'missing')).toBe('ws_abc123')
  })

  it('throws the caller-supplied message when unset', () => {
    expect(() => requireWorkspaceId(null, 'Library workspace is unavailable.')).toThrow(
      'Library workspace is unavailable.',
    )
    expect(() => requireWorkspaceId('   ', 'Library workspace is unavailable.')).toThrow(
      'Library workspace is unavailable.',
    )
    expect(() => requireWorkspaceId(undefined, 'Library workspace is unavailable.')).toThrow(
      'Library workspace is unavailable.',
    )
  })

  it('throws the caller-supplied error factory to preserve typed errors', () => {
    class WorkspaceUnavailableError extends Error {
      readonly code = 'workspace_selection_required'
    }
    const missing = () => new WorkspaceUnavailableError('Select a workspace.')
    try {
      requireWorkspaceId(null, missing)
      expect.unreachable()
    } catch (error) {
      expect(error).toBeInstanceOf(WorkspaceUnavailableError)
      expect((error as WorkspaceUnavailableError).code).toBe('workspace_selection_required')
    }
  })

  it('fails closed on blank values without inferring a workspace', () => {
    // Wrappers resolve store fallbacks before delegating; a blank explicit
    // must fail closed with the caller message, never an inferred workspace.
    expect(() => requireWorkspaceId('', 'missing')).toThrow('missing')
  })
})
