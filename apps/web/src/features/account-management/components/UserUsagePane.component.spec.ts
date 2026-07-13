import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import UserUsagePane from './UserUsagePane.vue'

const usageState = vi.hoisted(() => ({
  usage: {
    value: undefined as
      | {
          timezone: 'Asia/Shanghai'
          lifetime: {
            total_credits: number
            peak_daily_credits: number
            average_daily_credits: number
            task_count: number
            active_days: number
          }
        }
      | undefined,
    __v_isRef: true,
  },
  isPending: { value: false, __v_isRef: true },
  error: { value: null as unknown, __v_isRef: true },
  refresh: vi.fn(),
}))

vi.mock('../useUserUsage', () => ({
  useUserUsage: () => usageState,
}))

const ButtonStub = defineComponent({
  template: '<button type="button"><slot /></button>',
})

function renderUsagePane() {
  return renderWithUser(UserUsagePane, {
    props: { active: true },
    global: { stubs: { Button: ButtonStub } },
  })
}

describe('UserUsagePane', () => {
  it('renders lifetime values in one consolidated summary panel', () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 1234,
        peak_daily_credits: 500,
        average_daily_credits: 12.5,
        task_count: 9,
        active_days: 4,
      },
    }
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('heading', { name: '用量统计' })).toBeVisible()
    expect(screen.getByRole('region', { name: '用量摘要' })).toHaveTextContent('1,234')
    expect(screen.getByRole('region', { name: '用量摘要' })).toHaveTextContent('12.5')
    expect(screen.getByText('调用次数')).toBeVisible()
    expect(screen.getByText('累计活跃天数')).toBeVisible()
  })

  it('keeps the responsive 2x2 plus spanning-row contract without viewport font sizing', () => {
    usageState.usage.value = {
      timezone: 'Asia/Shanghai',
      lifetime: {
        total_credits: 1234,
        peak_daily_credits: 500,
        average_daily_credits: 12.5,
        task_count: 9,
        active_days: 4,
      },
    }
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    const panel = screen.getByRole('region', { name: '用量摘要' })
    const cells = panel.querySelectorAll('article')
    expect(cells).toHaveLength(5)
    expect(cells[0]).toHaveClass('max-[640px]:border-b')
    expect(cells[4]).toHaveClass('max-[640px]:col-span-2')
    const value = cells[0]?.querySelector('p')
    expect(value).toHaveClass('text-[34px]', 'max-[900px]:text-[30px]', 'max-[640px]:text-[28px]')
    expect(value?.className).not.toContain('clamp')
    expect(value?.className).not.toContain('vw')
  })

  it('keeps the five zero metrics visible when no records exist', () => {
    usageState.usage.value = undefined
    usageState.isPending.value = false
    usageState.error.value = null

    renderUsagePane()

    expect(screen.getByRole('region', { name: '用量摘要' })).toHaveTextContent('总消耗')
    expect(screen.getAllByText('0')).toHaveLength(5)
  })

  it('shows a local skeleton while the first request is pending', () => {
    usageState.usage.value = undefined
    usageState.isPending.value = true
    usageState.error.value = null

    renderUsagePane()

    const skeleton = screen.getByLabelText('正在加载用量统计')
    expect(skeleton).toBeVisible()
    const cells = skeleton.querySelectorAll('article')
    expect(cells).toHaveLength(5)
    expect(cells[0]).toHaveClass('min-w-0', 'p-5', 'max-[640px]:p-4')
    expect(cells[4]).toHaveClass('max-[640px]:col-span-2')
    expect(screen.queryByRole('region', { name: '用量摘要' })).not.toBeInTheDocument()
  })

  it('shows an inline retry when loading fails', async () => {
    usageState.usage.value = undefined
    usageState.isPending.value = false
    usageState.error.value = new Error('网络异常')
    usageState.refresh.mockReset()

    const { user } = renderUsagePane()

    expect(screen.getByRole('alert')).toHaveTextContent('网络异常')
    await user.click(screen.getByRole('button', { name: '重新加载' }))
    expect(usageState.refresh).toHaveBeenCalledOnce()
  })
})
