import type { HttpHandler } from 'msw'
import { http, HttpResponse } from '../../../test/msw'

// Shared MSW builder for the outbound-policy settings surface
// (`/api/control/v1/platform/settings/outbound-policy`). State stays
// caller-owned (board `testing/factories.ts` pattern): GET serves the current
// hosts through `getAllowHosts`, PUT captures body + operator reason and
// answers through `respondToUpdate`, with `onUpdate` syncing caller fixtures.
export type OutboundPolicyUpdate = {
  allow_hosts: string[]
}

export type OutboundPolicyMswState = {
  getAllowHosts: () => string[] | Promise<string[]>
  loadRequests: number
  loadError: { status: number; body: Record<string, unknown> } | null
  updateRequests: Array<{ body: unknown; operatorReason: string | null }>
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
        operatorReason: request.headers.get('X-Operator-Reason'),
      })
      if (state.updateError) {
        return HttpResponse.json(state.updateError.body, { status: state.updateError.status })
      }
      state.onUpdate(body)
      return HttpResponse.json({ data: state.respondToUpdate(body) })
    }),
  ]
}
