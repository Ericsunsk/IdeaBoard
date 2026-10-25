import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  acceptWorkspaceInviteByToken as acceptWorkspaceInviteByTokenRequest,
  declineWorkspaceInviteByToken as declineWorkspaceInviteByTokenRequest,
  getGetWorkspaceInviteByTokenQueryKey,
  getWorkspaceInviteByToken as getWorkspaceInviteByTokenRequest,
} from '../../../api/generated-query/public/workspace-members/workspace-members'
import type { WorkspaceInvitePreview } from '../../../api/generated-query/public/model/workspaceInvitePreview'
import type { WorkspaceInviteAck } from '../../../api/generated-query/public/model/workspaceInviteAck'
import type { WorkspaceMember } from '../../../api/generated-query/public/model/workspaceMember'
import { noRetry } from '../../../core/query/queryDefaults'
import { withSignal } from '../../../core/utils/request'

export type { WorkspaceInvitePreview, WorkspaceInviteAck }

/** Loads the public preview of a workspace invitation by raw token (no auth required). */
export async function fetchWorkspaceInviteByToken(
  token: string,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<WorkspaceInvitePreview> {
  const trimmed = token.trim()
  if (!trimmed) throw new Error('Token is required')
  const response = await getWorkspaceInviteByTokenRequest(trimmed, { ...withSignal(options?.signal) })
  return response.data
}

/** Accepts a workspace invitation by token (authenticated). */
export async function acceptWorkspaceInviteByToken(
  token: string,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<WorkspaceMember> {
  const trimmed = token.trim()
  if (!trimmed) throw new Error('Token is required')
  const response = await acceptWorkspaceInviteByTokenRequest(trimmed, { ...withSignal(options?.signal) })
  return response.data
}

/** Declines a workspace invitation by token (authenticated). */
export async function declineWorkspaceInviteByToken(
  token: string,
  options?: { signal?: AbortSignal | null | undefined },
): Promise<WorkspaceInviteAck> {
  const trimmed = token.trim()
  if (!trimmed) throw new Error('Token is required')
  const response = await declineWorkspaceInviteByTokenRequest(trimmed, { ...withSignal(options?.signal) })
  return response.data
}

/** Query options for fetching invitation details by token. */
export function workspaceInviteByTokenQueryOptions(
  token: MaybeRefOrGetter<string>,
  fetch: typeof fetchWorkspaceInviteByToken = fetchWorkspaceInviteByToken,
) {
  type QueryContext = { signal?: AbortSignal }
  const tokenValue = computed(() => (toValue(token) || '').trim())
  const queryFn = ({ signal }: QueryContext) => fetch(tokenValue.value, { signal: signal ?? null })
  return {
    // RISK (audit): the raw invite token rides the Vue Query key via the
    // generated helper. Treat keys as sensitive (devtools/persisted caches
    // can expose the token): keep them in memory only, never log or persist
    // query keys. The generated helper shape stays unchanged.
    queryKey: computed(() => getGetWorkspaceInviteByTokenQueryKey(tokenValue.value)),
    queryFn,
    // eslint-disable-next-line governance/boolean-prefix
    enabled: computed(() => Boolean(tokenValue.value)),
    ...noRetry(),
  }
}
