import { vi } from 'vitest'
import type { BoardCommandDependencies } from '../session/catalog/commands'
import type { WorkspaceBoardMeta } from '../session/adapters/boardsResource'
import { makeBoardMeta } from './factories'

export function makeServerMeta(
  id = 'board-server-1',
  name = 'Untitled',
  overrides: Partial<WorkspaceBoardMeta> = {},
): WorkspaceBoardMeta {
  return {
    id,
    name,
    revision: 1,
    schemaVersion: 4,
    createdAt: 100,
    updatedAt: 200,
    contentVersion: '',
    previewScene: null,
    capabilities: { read: true, write: true, manage: true, generate: true },
    accessState: 'private',
    workspaceAccess: 'none',
    isCreator: true,
    hasDirectGrant: false,
    creatorUserId: 'usr_creator',
    ...overrides,
  }
}

export function makeCommandDeps(
  overrides: Partial<BoardCommandDependencies> = {},
): BoardCommandDependencies {
  const deps: BoardCommandDependencies = {
    createBoard: vi.fn(() => makeBoardMeta('created-board', 'Untitled')),
    getBoard: vi.fn((boardId: string) => makeBoardMeta(boardId, 'Board')),
    renameBoard: vi.fn((boardId: string, nextName: string) =>
      makeBoardMeta(boardId, nextName.trim()),
    ),
    duplicateBoard: vi.fn(() => makeBoardMeta('copy-board', 'Source Copy')),
    deleteBoard: vi.fn(() => true),
    isNameTaken: vi.fn(() => false),
    duplicateBoardState: vi.fn(() => true),
    deleteBoardState: vi.fn(() => true),
    syncBoard: vi.fn(async () => undefined),
    syncRenameBoard: vi.fn(async () => undefined),
    syncDeleteBoard: vi.fn(async () => undefined),
    hydrateBoards: vi.fn(async () => ({ isOk: true as const })),
    getSyncConflictKind: vi.fn((error: unknown) => {
      if (typeof error !== 'object' || !error) return null
      const record = error as { status?: unknown; code?: unknown }
      if (record.code === 'board_idempotency_conflict') return 'idempotency-conflict' as const
      if (record.status === 409 || record.code === 'board_revision_conflict') {
        return 'revision-conflict' as const
      }
      return null
    }),
    isNotFoundError: vi.fn(
      (error: unknown) =>
        typeof error === 'object' && !!error && 'status' in error && error.status === 404,
    ),
    clearSyncRollback: vi.fn(),
    isMutationCurrent: vi.fn(() => true),
    resolveWorkspace: vi.fn(() => 'ws_test123'),
    fetchSourceDocument: vi.fn(async () => ({
      boardId: 'source-board',
      revision: 2,
      schemaVersion: 4,
      document: {},
      updatedAt: 300,
    })),
    createServerBoard: vi.fn(
      async (_workspaceId: string, name: string): Promise<WorkspaceBoardMeta> =>
        makeServerMeta('board-server-1', name),
    ),
    initializeServerBoard: vi.fn(async (_workspaceId, board) => ({ ...board, revision: 2 })),
    archiveServerBoard: vi.fn(async (): Promise<WorkspaceBoardMeta> => makeServerMeta()),
    registerServerBoard: vi.fn((input: { id: string; name: string; revision?: number }) =>
      makeBoardMeta(input.id, input.name),
    ),
  }
  return { ...deps, ...overrides } as BoardCommandDependencies
}
