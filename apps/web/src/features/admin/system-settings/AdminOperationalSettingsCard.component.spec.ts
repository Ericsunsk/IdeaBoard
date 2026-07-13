import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import AdminOperationalSettingsCard from './AdminOperationalSettingsCard.vue'

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }),
}))

describe('AdminOperationalSettingsCard', () => {
  let updateRequests: unknown[]

  beforeEach(() => {
    updateRequests = []
    useMswHandlers(
      http.get('/api/admin/settings/operational', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: makeOperationalSettingsState() }),
      ),
      http.put('/api/admin/settings/operational', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        updateRequests.push(body)
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makeOperationalSettingsState({
            task_runtime: body.task_runtime,
            outbound_policy: body.outbound_policy,
            event_retention: body.event_retention,
          }),
        })
      }),
    )
  })

  it('lets an admin read and update operational settings with an audited reason', async () => {
    const { user } = renderCard()

    expect(await screen.findByText(/重启 API\/Worker 后生效/)).toBeVisible()
    expect(screen.getByRole('spinbutton', { name: 'Worker 并发' })).toHaveValue('8')
    expect(screen.getByRole('textbox', { name: /出站白名单/ })).toHaveValue('models.internal')
    expect(screen.getByText(/重启 API 后生效/)).toBeVisible()
    expect(screen.getByText(/保存后即时生效/)).toBeVisible()
    expect(screen.getByText(/当前生效：并发 5/)).toBeVisible()

    const concurrencyInput = screen.getByRole('spinbutton', { name: 'Worker 并发' })
    await user.clear(concurrencyInput)
    await user.type(concurrencyInput, '12')
    await user.clear(screen.getByRole('textbox', { name: /出站白名单/ }))
    await user.type(
      screen.getByRole('textbox', { name: /出站白名单/ }),
      'Models.Internal\n10.0.0.8:8443',
    )

    await user.click(screen.getByRole('button', { name: '保存运行参数' }))
    expect(screen.getByText('请输入操作原因')).toBeVisible()
    expect(updateRequests).toEqual([])

    await user.type(screen.getByRole('textbox', { name: /变更原因/ }), '  扩容并更新出口  ')
    await user.click(screen.getByRole('button', { name: '保存运行参数' }))

    await waitFor(() => {
      expect(updateRequests).toEqual([
        {
          task_runtime: {
            worker_concurrency: 12,
            task_timeout_seconds: 240,
            no_capacity_retry_seconds: 12,
            max_retry: 30,
          },
          outbound_policy: {
            allow_hosts: ['models.internal', '10.0.0.8:8443'],
          },
          event_retention: { nats_retention_days: 14 },
          reason: '扩容并更新出口',
        },
      ])
    })
  })
})

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return renderWithUser(AdminOperationalSettingsCard, {
    global: {
      plugins: [
        [PrimeVue, { unstyled: true }],
        [VueQueryPlugin, { queryClient }],
      ],
    },
  })
}

function makeOperationalSettingsState(configuredOverrides: Record<string, unknown> = {}) {
  const configured = {
    task_runtime: {
      worker_concurrency: 8,
      task_timeout_seconds: 240,
      no_capacity_retry_seconds: 12,
      max_retry: 30,
    },
    outbound_policy: { allow_hosts: ['models.internal'] },
    event_retention: { nats_retention_days: 14 },
    ...configuredOverrides,
  }
  return {
    task_runtime: {
      configured: configured.task_runtime,
      effective: {
        worker_concurrency: 5,
        task_timeout_seconds: 180,
        no_capacity_retry_seconds: 5,
        max_retry: 25,
      },
      restart_required: true,
    },
    outbound_policy: {
      configured: configured.outbound_policy,
      effective: configured.outbound_policy,
      restart_required: false,
    },
    event_retention: {
      configured: configured.event_retention,
      effective: { nats_retention_days: 7 },
      restart_required: true,
    },
  }
}
