import { describe, expect, it } from 'vitest'
import {
  assertOpaqueId,
  isOpaqueIdWithPrefix,
  opaqueIdPattern,
  OPAQUE_ID_SUFFIX_SOURCE,
} from './opaqueIds'
import { assertWorkspaceId } from './workspaceId'
import { isOpaqueId, opaqueIdPrefix } from '../../api/contractKernel'
import {
  assertUploadId,
  assertWorkspaceAssetId,
  assertWorkspaceId as assertUploadWorkspaceId,
} from '../../api/upload'
import {
  assertWorkspaceTaskAssetId,
  assertWorkspaceTaskId,
  assertWorkspaceTaskWorkspaceId,
} from '../../api/workspaceTasks'

const ACCEPT = [
  'ws_abc123',
  'ws_a-b_c',
  'task_x-y_z',
  'upload_UP00',
  'asset_a1-2_3',
  'lasset_asset0001',
  'lfolder_folder-0001_x',
  'visq_op0001',
]

const REJECT = ['nope', 'ws_', '  ', '', 'task-', 'ws_a b', 'ws_a.b', 'ws_a/b', 'ws_a@b']

describe('opaqueIds', () => {
  it('mirrors the backend payload alphabet ([A-Za-z0-9_-])', () => {
    expect(OPAQUE_ID_SUFFIX_SOURCE).toBe('[A-Za-z0-9_-]+')
  })

  it('builds anchored per-prefix patterns', () => {
    expect(opaqueIdPattern('ws').source).toBe('^(?:ws)_[A-Za-z0-9_-]+$')
    expect(opaqueIdPattern(['lfolder', 'lasset']).source).toBe(
      '^(?:lfolder|lasset)_[A-Za-z0-9_-]+$',
    )
  })

  it('asserts fail-closed with trim + canonical messages', () => {
    expect(assertOpaqueId('  ws_abc123  ', 'ws', 'workspace id')).toBe('ws_abc123')
    expect(() => assertOpaqueId('task_abc123', 'ws', 'workspace id')).toThrow(
      'Invalid workspace id: expected ws_...',
    )
    expect(() => assertOpaqueId('ws_', 'ws', 'workspace id')).toThrow(
      'Invalid workspace id: expected ws_...',
    )
    expect(isOpaqueIdWithPrefix('ws_a-b', 'ws')).toBe(true)
    expect(isOpaqueIdWithPrefix('ws_a-b', 'task')).toBe(false)
    expect(isOpaqueIdWithPrefix(42, 'ws')).toBe(false)
  })

  it('keeps identical accept/reject outcomes across every migrated assert', () => {
    const asserts: Array<(value: string) => string> = [
      (value) => assertWorkspaceId(value),
      (value) => assertUploadWorkspaceId(value),
      (value) => assertWorkspaceTaskWorkspaceId(value),
    ]
    for (const assert of asserts) {
      expect(assert('  ws_a-b_c  ')).toBe('ws_a-b_c')
      for (const value of ['nope', 'ws_', 'task_abc123']) {
        expect(() => assert(value)).toThrow('Invalid workspace id: expected ws_...')
      }
    }
    expect(assertWorkspaceTaskId('task_x-y')).toBe('task_x-y')
    expect(() => assertWorkspaceTaskId('upload_x')).toThrow('Invalid task id: expected task_...')
    expect(assertWorkspaceTaskAssetId('asset_a-1')).toBe('asset_a-1')
    expect(assertWorkspaceAssetId('asset_a-1')).toBe('asset_a-1')
    expect(assertUploadId('upload_up-01')).toBe('upload_up-01')
    expect(() => assertUploadId('task_up-01')).toThrow('Invalid upload id: expected upload_...')
  })

  it('accepts backend-valid dash/underscore payloads through the contract kernel', () => {
    for (const id of ['ws_a-b_c', 'task_x-y_z']) {
      expect(isOpaqueId(id)).toBe(true)
      expect(opaqueIdPrefix(id)).toBe(id.slice(0, id.indexOf('_')))
    }
    for (const value of REJECT) {
      expect(isOpaqueId(value)).toBe(false)
    }
    for (const id of ACCEPT) {
      const prefix = id.slice(0, id.indexOf('_'))
      if (['ws', 'task', 'upload', 'asset'].includes(prefix)) {
        expect(isOpaqueId(id)).toBe(true)
      }
    }
  })
})
