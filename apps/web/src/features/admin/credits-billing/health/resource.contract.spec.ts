import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { useAdminCreditHealthResource } from './resource'

describe('Admin Credit Health resource', () => {
  it('shares one query key and request across multiple consumers', async () => {
    let requests = 0
    useMswHandlers(
      http.get('/api/admin/credits/health', () => {
        requests += 1
        return HttpResponse.json({ code: 0, msg: 'ok', data: makeHealth() })
      }),
    )
    const Consumer = defineComponent({
      setup() {
        const first = useAdminCreditHealthResource()
        const second = useAdminCreditHealthResource()
        return { firstHealth: first.health, secondHealth: second.health }
      },
      template: `
        <div>
          <span data-testid="first">{{ firstHealth?.status }}</span>
          <span data-testid="second">{{ secondHealth?.status }}</span>
        </div>
      `,
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(Consumer, {
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })

    await waitFor(() => expect(screen.getByTestId('first')).toHaveTextContent('healthy'))
    expect(screen.getByTestId('second')).toHaveTextContent('healthy')
    expect(requests).toBe(1)
  })
})

function makeHealth() {
  return {
    status: 'healthy',
    open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
    restricted_wallet_count: 0,
  }
}
