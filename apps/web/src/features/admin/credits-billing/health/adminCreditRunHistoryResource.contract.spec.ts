import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { defineComponent, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { useAdminCreditRunHistoryResource } from './adminCreditRunHistoryResource'

describe('Admin Credit Run history resource', () => {
  it('loads only while enabled and keeps list and detail requests behind one resource boundary', async () => {
    const listRequests: Array<string | null> = []
    const detailRequests: string[] = []
    useMswHandlers(
      http.get('/api/admin/credits/reconciliation/runs', ({ request }) => {
        const beforeId = new URL(request.url).searchParams.get('before_id')
        listRequests.push(beforeId)
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            runs: [makeRun({ id: beforeId ? 20 : 21 })],
            next_before_id: beforeId ? undefined : 21,
          },
        })
      }),
      http.get('/api/admin/credits/reconciliation/runs/:runId', ({ params }) => {
        detailRequests.push(String(params.runId))
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makeRun({ id: Number(params.runId), reason: '人工复核' }),
        })
      }),
    )
    const Harness = defineComponent({
      setup() {
        const enabled = ref(false)
        const beforeId = ref<number>()
        const selectedRunId = ref<number | null>(null)
        const resource = useAdminCreditRunHistoryResource({ enabled, beforeId, selectedRunId })
        return {
          page: resource.page,
          selectedRun: resource.selectedRun,
          open: () => {
            enabled.value = true
          },
          next: () => {
            beforeId.value = 21
          },
          select: () => {
            selectedRunId.value = 21
          },
        }
      },
      template: `
        <button @click="open">open</button>
        <button @click="next">next</button>
        <button @click="select">select</button>
        <span data-testid="page-run">{{ page?.runs[0]?.id }}</span>
        <span data-testid="detail-reason">{{ selectedRun?.reason }}</span>
      `,
    })
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const { user } = renderWithUser(Harness, {
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })

    expect(listRequests).toHaveLength(0)
    await user.click(screen.getByRole('button', { name: 'open' }))
    await waitFor(() => expect(screen.getByTestId('page-run')).toHaveTextContent('21'))
    expect(listRequests).toEqual([null])

    await user.click(screen.getByRole('button', { name: 'select' }))
    await waitFor(() => expect(screen.getByTestId('detail-reason')).toHaveTextContent('人工复核'))
    expect(detailRequests).toEqual(['21'])

    await user.click(screen.getByRole('button', { name: 'next' }))
    await waitFor(() => expect(screen.getByTestId('page-run')).toHaveTextContent('20'))
    expect(listRequests).toEqual([null, '21'])
  })
})

function makeRun(overrides: Record<string, unknown> = {}) {
  return {
    id: 21,
    status: 'completed',
    trigger: 'scheduled',
    finding_count: 0,
    critical_count: 0,
    isolated_wallet_count: 0,
    attempt_count: 1,
    requested_at: '2026-07-17T02:00:00Z',
    started_at: '2026-07-17T02:00:01Z',
    completed_at: '2026-07-17T02:00:04Z',
    ...overrides,
  }
}
