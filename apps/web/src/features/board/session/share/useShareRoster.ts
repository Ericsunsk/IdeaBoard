import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import type { ShareTransport } from './commands'
import {
  workspaceMembersQueryOptions as defaultWorkspaceMembersQueryOptions,
  type WorkspaceMemberEntry,
} from '../../../workspace'

export type ShareMemberRow = WorkspaceMemberEntry & {
  isSelected: boolean
  isCreator: boolean
}

export type ShareRosterOptionsBuilder = typeof defaultWorkspaceMembersQueryOptions

export function useShareRoster(deps: {
  transport: Pick<ShareTransport, 'fetchRoster'>
  scope: () => { workspaceId: string } | null
  enabled: () => boolean
  creatorId: () => string
  selectedIds: () => readonly string[]
  /** Ports seam: roster query options builder (default: workspace members query options). */
  buildRosterOptions?: ShareRosterOptionsBuilder
}) {
  const buildRosterOptions = deps.buildRosterOptions ?? defaultWorkspaceMembersQueryOptions
  const query = useQuery(
    computed(() => ({
      ...buildRosterOptions(deps.scope()?.workspaceId ?? '', deps.transport.fetchRoster),
      enabled: deps.enabled,
    })),
  )
  const memberRows = computed<ShareMemberRow[]>(() => {
    const selected = new Set(deps.selectedIds())
    return (query.data.value ?? [])
      .map((member) => ({
        ...member,
        isCreator: member.userId === deps.creatorId(),
        isSelected: member.userId === deps.creatorId() || selected.has(member.userId),
      }))
      .sort((a, b) => Number(b.isCreator) - Number(a.isCreator))
  })
  return { query, memberRows }
}
