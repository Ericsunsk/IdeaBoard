import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { i18n } from '../../../../shared/i18n'
import { loadLocaleMessages } from '../../../../shared/i18n/userLocale'
import { renderWithUser, screen } from '../../../../test/render'
import RewardCodeFormDialog from './RewardCodeFormDialog.vue'
import type { RewardCode } from './resource'

function makeMockRewardCode(overrides: Partial<RewardCode> = {}): RewardCode {
  return {
    id: 1,
    displayName: '老用户奖励',
    maskedCodeHint: '•••• 1234',
    creditAmount: 500,
    maxRedemptions: 10,
    redemptionCount: 3,
    status: 'enabled',
    enabled: true,
    expiresAt: '2026-12-31T23:59:59+08:00',
    createdAt: '2026-01-01T00:00:00+08:00',
    updatedAt: '2026-01-01T00:00:00+08:00',
    ...overrides,
  }
}

describe('RewardCodeFormDialog with Schema Validation', () => {
  beforeAll(async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
  })

  afterAll(() => {
    ;(i18n.global.locale as { value: string }).value = 'en'
  })
  it('renders create mode by default with generated code and default credits', async () => {
    renderWithUser(RewardCodeFormDialog, {
      props: { visible: true },
    })

    expect(await screen.findByRole('heading', { name: '创建兑换码' })).toBeVisible()
    const nameInput = screen.getByLabelText(/名称/)
    const codeInput = screen.getByLabelText(/Code/)
    const creditInput = screen.getByLabelText(/奖励 Idea/)

    expect(nameInput).toHaveValue('')
    expect(codeInput).not.toHaveValue('')
    expect(creditInput).toHaveValue('100')
  })

  it('triggers valibot schema validation on empty submit and displays error messages', async () => {
    const { user } = renderWithUser(RewardCodeFormDialog, {
      props: { visible: true },
    })

    const submitBtn = await screen.findByRole('button', { name: '创建兑换码' })
    await user.clear(screen.getByLabelText(/Code/))
    await user.click(submitBtn)

    expect(await screen.findByText('请输入名称')).toBeVisible()
    expect(await screen.findByText('请输入 Code')).toBeVisible()
  })

  it('validates minimum cap when editing existing reward code', async () => {
    const mockCode = makeMockRewardCode({ maxRedemptions: 10, redemptionCount: 5 })
    const { user } = renderWithUser(RewardCodeFormDialog, {
      props: { visible: true, rewardCode: mockCode },
    })

    expect(await screen.findByRole('heading', { name: '编辑兑换码' })).toBeVisible()
    const capInput = screen.getByLabelText(/兑换上限/)
    await user.clear(capInput)
    await user.type(capInput, '3')

    const submitBtn = screen.getByRole('button', { name: '保存' })
    await user.click(submitBtn)

    expect(await screen.findByText('兑换上限不能低于当前兑换次数 5')).toBeVisible()
  })

  it('submits valid form data with schema conformity', async () => {
    const onSubmit = vi.fn()
    const { user } = renderWithUser(RewardCodeFormDialog, {
      props: { visible: true, onSubmit },
    })

    await screen.findByRole('dialog')
    await user.type(screen.getByLabelText(/名称/), '新用户专享')
    const submitBtn = screen.getByRole('button', { name: '创建兑换码' })
    await user.click(submitBtn)

    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        displayName: '新用户专享',
        creditAmount: 100,
        maxRedemptions: 0,
      }),
    )
  })
})
