import type { HttpHandler } from 'msw'
import { http, HttpResponse } from '@/test/msw'

type OutboundPolicyUpdate = {
  allow_hosts: string[]
}

export type OutboundPolicyMswState = {
  getAllowHosts: () => string[] | Promise<string[]>
  loadRequests: number
  loadError: { status: number; body: Record<string, unknown> } | null
  updateRequests: Array<{ body: unknown }>
  updateError: { status: number; body: Record<string, unknown> } | null
  respondToUpdate: (body: OutboundPolicyUpdate) => OutboundPolicyUpdate
  onUpdate: (body: OutboundPolicyUpdate) => void
}

export function createOutboundPolicyMswState(
  overrides: Partial<OutboundPolicyMswState> = {},
): OutboundPolicyMswState {
  return {
    getAllowHosts: () => ['models.internal'],
    loadRequests: 0,
    loadError: null,
    updateRequests: [],
    updateError: null,
    respondToUpdate: (body) => ({ allow_hosts: body.allow_hosts }),
    onUpdate: () => {},
    ...overrides,
  }
}

export function createOutboundPolicyHandlers(state: OutboundPolicyMswState): HttpHandler[] {
  return [
    http.get('/api/control/v1/platform/settings/outbound-policy', async () => {
      state.loadRequests += 1
      if (state.loadError) {
        return HttpResponse.json(state.loadError.body, { status: state.loadError.status })
      }
      return HttpResponse.json({ data: { allow_hosts: await state.getAllowHosts() } })
    }),
    http.put('/api/control/v1/platform/settings/outbound-policy', async ({ request }) => {
      const body = (await request.json()) as OutboundPolicyUpdate
      state.updateRequests.push({
        body,
      })
      if (state.updateError) {
        return HttpResponse.json(state.updateError.body, { status: state.updateError.status })
      }
      state.onUpdate(body)
      return HttpResponse.json({ data: state.respondToUpdate(body) })
    }),
  ]
}
