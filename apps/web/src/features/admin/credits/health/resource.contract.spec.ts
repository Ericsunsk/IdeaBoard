import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'
import { useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { useAdminCreditHealthResource } from './resource'
import { createCreditHealthHandlers, createCreditHealthMswState } from './testing'

describe('Admin Credit Health resource', () => {
  it('shares one query key and request across multiple consumers', async () => {
    const state = createCreditHealthMswState()
    useMswHandlers(...createCreditHealthHandlers(state))
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
    expect(state.healthRequests).toBe(1)
  })
})
