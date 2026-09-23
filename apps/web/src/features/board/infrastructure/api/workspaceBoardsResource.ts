// Workspace-rooted boards transport (issue #802, frontend slice).
//
// Feature-owned resource over the Public Data Plane board endpoints. It calls
// `/api/v1/workspaces/{workspace_id}/boards` through the central
// `api.request` + `runtimeUrls` with thin snake_case transport types only:
// snake_case -> camelCase mapping lives in
// `application/sync/workspaceBoardSync` (keep this resource thin).
//
// TODO(#802-regen): move these calls onto `generated-query/public` once the
// merger stream regenerates it from the board operations documented in the
// #802 report. Never import `generated-query/admin|control` here, and route
// pages must consume the sync layer, never this module directly.

import { api } from '../../../../shared/api/client'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../shared/api/contractKernel'
import { resolveApiUrl } from '../../../../shared/config/runtimeUrls'

export type WorkspaceBoardItem = {
  id: string
  workspace_id: string
  name: string
  revision: number
  schema_version: number
  archived_at?: string | null | undefined
  created_at: string
  updated_at: string
}

export type WorkspaceBoardDocument = {
  board_id: string
  revision: number
  schema_version: number
  document: unknown
  updated_at: string
}

export type WorkspaceBoardRevision = {
  id: number
  revision: number
  schema_version: number
  source_revision: number
  created_by?: string | undefined
  created_at: string
  snapshot?: unknown | undefined
}

export type WorkspaceBoardMember = {
  user_id: string
  role: string
  created_at?: string | undefined
}

export type WorkspaceBoardShare = {
  token_id: string
  token?: string | undefined
  token_prefix: string
  role: string
  revoked: boolean
  usage_count: number
  created_at: string
  revoked_at?: string | null | undefined
}

export type WorkspaceBoardsRequestOptions = {
  signal?: AbortSignal | null | undefined
  requestId?: string | undefined
  idempotencyKey?: string | undefined
  ifMatch?: string | number | null | undefined
}

function toRequestConfig(options: WorkspaceBoardsRequestOptions | undefined): {
  signal?: AbortSignal | undefined
  headers: Record<string, string>
} {
  const ifMatch =
    options?.ifMatch == null || options.ifMatch === ''
      ? undefined
      : { 'If-Match': String(options.ifMatch) }
  return {
    ...(options?.signal == null ? {} : { signal: options.signal }),
    headers: {
      ...buildContractRequestHeaders({
        requestId: options?.requestId?.trim() ? options.requestId.trim() : newRequestId(),
        ...(options?.idempotencyKey == null ? {} : { idempotencyKey: options.idempotencyKey }),
      }),
      ...ifMatch,
    },
  }
}

function collectionUrl(workspaceId: string): string {
  return resolveApiUrl(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/boards`)
}

function itemUrl(workspaceId: string, boardId: string, suffix = ''): string {
  return resolveApiUrl(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/boards/${encodeURIComponent(boardId)}${suffix}`,
  )
}

export const workspaceBoardsApiUrls = {
  collection: collectionUrl,
  item: (workspaceId: string, boardId: string): string => itemUrl(workspaceId, boardId),
  document: (workspaceId: string, boardId: string): string =>
    itemUrl(workspaceId, boardId, '/document'),
  revisions: (workspaceId: string, boardId: string): string =>
    itemUrl(workspaceId, boardId, '/revisions'),
  revisionDetail: (workspaceId: string, boardId: string, revisionId: string | number): string =>
    itemUrl(workspaceId, boardId, `/revisions/${encodeURIComponent(String(revisionId))}`),
  members: (workspaceId: string, boardId: string): string =>
    itemUrl(workspaceId, boardId, '/members'),
  memberDetail: (workspaceId: string, boardId: string, memberId: string): string =>
    itemUrl(workspaceId, boardId, `/members/${encodeURIComponent(memberId)}`),
  shares: (workspaceId: string, boardId: string): string => itemUrl(workspaceId, boardId, '/shares'),
  shareDetail: (workspaceId: string, boardId: string, tokenId: string): string =>
    itemUrl(workspaceId, boardId, `/shares/${encodeURIComponent(tokenId)}`),
  join: (workspaceId: string): string =>
    resolveApiUrl(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/boards/join`),
  archive: (workspaceId: string, boardId: string): string =>
    itemUrl(workspaceId, boardId, '/archive'),
  restore: (workspaceId: string, boardId: string): string =>
    itemUrl(workspaceId, boardId, '/restore'),
}

export async function listWorkspaceBoards(
  workspaceId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  const search = new URLSearchParams()
  if (params?.cursor?.trim()) search.set('cursor', params.cursor.trim())
  search.set('page_size', String(params?.pageSize ?? 20))
  return api.request<unknown>(
    `${collectionUrl(workspaceId)}?${search.toString()}`,
    toRequestConfig(options),
  )
}

export async function createWorkspaceBoard(
  workspaceId: string,
  input: { name: string },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    collectionUrl(workspaceId),
    { ...toRequestConfig(options), method: 'POST', body: { data: input } as never },
  )
}

export async function fetchWorkspaceBoard(
  workspaceId: string,
  boardId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(itemUrl(workspaceId, boardId), toRequestConfig(options))
}

export async function renameWorkspaceBoard(
  workspaceId: string,
  boardId: string,
  input: { name: string },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId),
    { ...toRequestConfig(options), method: 'PATCH', body: { data: input } as never },
  )
}

export async function archiveWorkspaceBoard(
  workspaceId: string,
  boardId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, '/archive'),
    { ...toRequestConfig(options), method: 'POST' },
  )
}

export async function restoreWorkspaceBoard(
  workspaceId: string,
  boardId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, '/restore'),
    { ...toRequestConfig(options), method: 'POST' },
  )
}

export async function fetchWorkspaceBoardDocument(
  workspaceId: string,
  boardId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(itemUrl(workspaceId, boardId, '/document'), toRequestConfig(options))
}

export async function replaceWorkspaceBoardDocument(
  workspaceId: string,
  boardId: string,
  input: { schema_version: number; payload: unknown },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, '/document'),
    { ...toRequestConfig(options), method: 'PUT', body: { data: input } as never },
  )
}

export async function listWorkspaceBoardRevisions(
  workspaceId: string,
  boardId: string,
  params?: { cursor?: string | null | undefined; pageSize?: number | undefined },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  const search = new URLSearchParams()
  if (params?.cursor?.trim()) search.set('cursor', params.cursor.trim())
  search.set('page_size', String(params?.pageSize ?? 20))
  return api.request<unknown>(
    `${itemUrl(workspaceId, boardId, '/revisions')}?${search.toString()}`,
    toRequestConfig(options),
  )
}

export async function fetchWorkspaceBoardRevision(
  workspaceId: string,
  boardId: string,
  revisionId: string | number,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, `/revisions/${encodeURIComponent(String(revisionId))}`),
    toRequestConfig(options),
  )
}

export async function listWorkspaceBoardMembers(
  workspaceId: string,
  boardId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<WorkspaceBoardMember[]> {
  const response = await api.request<unknown>(
    itemUrl(workspaceId, boardId, '/members'),
    toRequestConfig(options),
  )
  return requireContractData<WorkspaceBoardMember[]>(response, 'Board members request failed')
}

export async function addWorkspaceBoardMember(
  workspaceId: string,
  boardId: string,
  input: { user_id: string; role: string },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, '/members'),
    { ...toRequestConfig(options), method: 'POST', body: { data: input } as never },
  )
}

export async function updateWorkspaceBoardMember(
  workspaceId: string,
  boardId: string,
  memberId: string,
  input: { role: string },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, `/members/${encodeURIComponent(memberId)}`),
    { ...toRequestConfig(options), method: 'PATCH', body: { data: input } as never },
  )
}

export async function removeWorkspaceBoardMember(
  workspaceId: string,
  boardId: string,
  memberId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, `/members/${encodeURIComponent(memberId)}`),
    { ...toRequestConfig(options), method: 'DELETE' },
  )
}

export async function listWorkspaceBoardShares(
  workspaceId: string,
  boardId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<WorkspaceBoardShare[]> {
  const response = await api.request<unknown>(
    itemUrl(workspaceId, boardId, '/shares'),
    toRequestConfig(options),
  )
  return requireContractData<WorkspaceBoardShare[]>(response, 'Board shares request failed')
}

export async function createWorkspaceBoardShare(
  workspaceId: string,
  boardId: string,
  input: { role: string },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, '/shares'),
    { ...toRequestConfig(options), method: 'POST', body: { data: input } as never },
  )
}

export async function revokeWorkspaceBoardShare(
  workspaceId: string,
  boardId: string,
  tokenId: string,
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    itemUrl(workspaceId, boardId, `/shares/${encodeURIComponent(tokenId)}`),
    { ...toRequestConfig(options), method: 'DELETE' },
  )
}

export async function joinWorkspaceBoard(
  workspaceId: string,
  input: { token: string },
  options?: WorkspaceBoardsRequestOptions,
): Promise<unknown> {
  return api.request<unknown>(
    resolveApiUrl(`/api/v1/workspaces/${encodeURIComponent(workspaceId)}/boards/join`),
    { ...toRequestConfig(options), method: 'POST', body: { data: input } as never },
  )
}
