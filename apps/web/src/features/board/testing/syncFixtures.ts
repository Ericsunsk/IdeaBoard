import { setStoredAuth } from '../../../core/utils/auth'
import { getWorkspaceSelectionStore, resetWorkspaceSelectionStore } from '../../workspace'
import { BOARD_STATE_SCHEMA_VERSION, type BoardState } from '../core'
import { makeBoardState, type MockWorkspaceBoardApiState } from './factories'

export function primeAuthenticatedWorkspace(workspaceId: string) {
  setStoredAuth({
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'tester',
    avatar: '',
    role: 'user',
  })
  resetWorkspaceSelectionStore()
  getWorkspaceSelectionStore().select(workspaceId)
}

export function makePromptBoardState(updatedAt: number, promptText: string): BoardState {
  return makeBoardState({
    updatedAt,
    promptNodes: [
      {
        id: 1,
        type: 'prompt',
        x: 0,
        y: 0,
        width: 280,
        text: promptText,
        outputCount: 1,
        aspectRatio: 'auto',
        inputImages: [],
      },
    ],
  })
}

export function addRemoteBoardFixture(
  apiState: MockWorkspaceBoardApiState,
  workspaceId: string,
  params: {
    id: string
    name: string
    createdAt: number
    updatedAt: number
    revision?: number | undefined
    state: BoardState
  },
) {
  apiState.boardsResponse.data.push({
    id: params.id,
    workspace_id: workspaceId,
    name: params.name,
    revision: params.revision ?? 1,
    schema_version: BOARD_STATE_SCHEMA_VERSION,
    archived_at: null,
    created_at: new Date(params.createdAt).toISOString(),
    updated_at: new Date(params.updatedAt).toISOString(),
  })
  const { schemaVersion, ...payload } = params.state
  apiState.documentById[params.id] = {
    board_id: params.id,
    revision: params.revision ?? 1,
    schema_version: schemaVersion,
    document: { schema_version: schemaVersion, payload },
    updated_at: new Date(params.state.updatedAt).toISOString(),
  }
}
