import { beforeAll, describe, expect, it } from 'vitest'
import { i18n } from '@/core/i18n'
import { loadLocaleMessages } from '@/core/i18n/userLocale'
import { renderWithUser, screen } from '@/test/render'
import WalletRestrictionPanel from './WalletRestrictionPanel.vue'

describe('WalletRestrictionPanel', () => {
  beforeAll(async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
  })

  it('renders active restrictions with per-item release actions', async () => {
    const { emitted } = renderWithUser(WalletRestrictionPanel, {
      props: {
        restrictions: [
          {
            restriction_id: '00000000-0000-4000-8000-000000000001',
            type: 'ledger_integrity',
            source: 'reconciliation',
            related_object: { type: 'finding', id: '7' },
            reason: '',
            created_at: '2026-09-23T00:00:00Z',
          },
        ],
        isRestrictionPending: false,
      },
    })

    expect(screen.getByText('账本完整性')).toBeVisible()
    await emitted()
    expect(
      screen.getByRole('button', { name: '解除限制 00000000-0000-4000-8000-000000000001' }),
    ).toBeVisible()
  })

  it('validates the create form before emitting', async () => {
    const { emitted, user } = renderWithUser(WalletRestrictionPanel, {
      props: { restrictions: [], isRestrictionPending: false },
    })

    await user.click(screen.getByRole('button', { name: '新增限制' }))
    expect(screen.getByText('请选择限制类型')).toBeVisible()
    expect(emitted()['create']).toBeUndefined()
  })
})
