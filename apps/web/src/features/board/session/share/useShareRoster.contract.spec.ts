import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { QueryClient, VueQueryPlugin, useQuery } from '@tanstack/vue-query'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { fetchWorkspaceMembers, workspaceMembersQueryOptions } from '../../../workspace'
import { useShareRoster } from './useShareRoster'

const WORKSPACE = 'ws_roster123'
const listMembers = vi.fn()

const views: VueWrapper[] = []
const clients: QueryClient[] = []

function observe<T>(client: QueryClient, setup: () => T): T {
  let result!: T
  views.push(
    mount(
      {
        setup() {
          result = setup()
          return () => null
        },
      },
      { global: { plugins: [[VueQueryPlugin, { queryClient: client }]] } },
    ),
  )
  return result
}

function observeSettings(client: QueryClient) {
  return observe(client, () => useQuery(workspaceMembersQueryOptions(WORKSPACE)))
}

function observeShare(client: QueryClient) {
  return observe(client, () =>
    useShareRoster({
      transport: { fetchRoster: fetchWorkspaceMembers },
      scope: () => ({ workspaceId: WORKSPACE }),
      enabled: () => true,
      creatorId: () => 'usr_creator',
      selectedIds: () => ['usr_member'],
    }),
  )
}

function createClient() {
  const client = new QueryClient()
  clients.push(client)
  return client
}

beforeEach(() => {
  useMswHandlers(
    http.get(`/api/v1/workspaces/${WORKSPACE}/members`, async () =>
      HttpResponse.json(await listMembers()),
    ),
  )
  listMembers.mockReset()
  listMembers.mockResolvedValue({
    data: [
      { user_id: 'usr_member', role: 'member', avatar: ' /member.png ' },
      { user_id: 'usr_creator', role: 'owner', email: 'creator@example.test' },
    ],
  })
})

afterEach(() => {
  views.splice(0).forEach((view) => view.unmount())
  clients.splice(0).forEach((client) => client.clear())
})

describe('Workspace settings and Share roster cache', () => {
  test.each(['settings', 'share'] as const)(
    'preserves profiles and membership roles when %s opens first',
    async (first) => {
      const client = createClient()
      const initial = first === 'settings' ? observeSettings(client) : observeShare(client).query
      await vi.waitFor(() => expect(initial.isSuccess.value).toBe(true))

      const settings = first === 'settings' ? initial : observeSettings(client)
      const share = observeShare(client)
      await vi.waitFor(() => expect(share.query.isSuccess.value).toBe(true))

      expect(settings.data.value).toEqual([
        { userId: 'usr_member', role: 'member', avatar_url: '/member.png' },
        { userId: 'usr_creator', role: 'owner', email: 'creator@example.test' },
      ])
      expect(share.query.data.value).toBe(settings.data.value)
      expect(share.memberRows.value).toEqual([
        {
          userId: 'usr_creator',
          role: 'owner',
          email: 'creator@example.test',
          isCreator: true,
          isSelected: true,
        },
        {
          userId: 'usr_member',
          role: 'member',
          avatar_url: '/member.png',
          isCreator: false,
          isSelected: true,
        },
      ])
      expect(listMembers).toHaveBeenCalledOnce()
    },
  )

  test.each(['settings', 'share'] as const)(
    '%s rejects invalid membership facts without retrying or caching them',
    async (surface) => {
      listMembers.mockResolvedValue({ data: [{ user_id: 'usr_member', role: 'super' }] })
      const client = createClient()
      const query = surface === 'settings' ? observeSettings(client) : observeShare(client).query
      await vi.waitFor(() => expect(query.isError.value).toBe(true))
      expect(query.data.value).toBeUndefined()
      expect(listMembers).toHaveBeenCalledOnce()
    },
  )
})
