import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import type { ShareTransport } from './commands'
import { workspaceMembersQueryOptions, type WorkspaceMemberEntry } from '../../../workspace'

export type ShareMemberRow = WorkspaceMemberEntry & {
  isSelected: boolean
  isCreator: boolean
}

export function useShareRoster(deps: {
  transport: Pick<ShareTransport, 'fetchRoster'>
  scope: () => { workspaceId: string } | null
  enabled: () => boolean
  creatorId: () => string
  selectedIds: () => readonly string[]
}) {
  const query = useQuery(
    computed(() => ({
      ...workspaceMembersQueryOptions(deps.scope()?.workspaceId ?? '', deps.transport.fetchRoster),
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
