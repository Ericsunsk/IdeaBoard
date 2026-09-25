import { defineComponent, ref } from 'vue'
import { IconoirProvider } from '@iconoir/vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { createDeferred } from '../../../../test/deferred'
import { useUserStore } from '../../../auth/session/userSession'
import type { User } from '../../../../shared/api/sessionUser'
import { HOME_PATH } from '../../../../shared/constants/routePaths'
import { accountTabRegistry, type AccountTab } from '../common/tabRegistry'
import AccountDialogContent from './AccountDialogContent.vue'

const logoutAndRedirectMock = vi.hoisted(() => vi.fn())
const useAvatarUploadMock = vi.hoisted(() => vi.fn())
const toastErrorMock = vi.hoisted(() => vi.fn())

vi.mock('../../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({
    fallbackText: 'IB',
    logoURL: '',
    projectName: 'IdeaBoard',
  }),
}))

vi.mock('../../../../shared/composables/useLogoutRedirect', () => ({
  useLogoutRedirect: () => ({
    logoutAndRedirect: logoutAndRedirectMock,
  }),
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({
    error: toastErrorMock,
  }),
}))

vi.mock('../../application/profile/useAvatarUpload', () => ({
  useAvatarUpload: useAvatarUploadMock,
}))

const ButtonStub = defineComponent({
  props: {
    disabled: { type: Boolean, default: false },
    label: { type: String, default: '' },
    type: { type: String, default: 'button' },
  },
  template: `
    <button :type="type" :disabled="disabled">
      <slot />
      <span v-if="label">{{ label }}</span>
    </button>
  `,
})

const AvatarStub = defineComponent({
  props: {
    label: { type: String, default: '' },
    image: { type: String, default: '' },
  },
  template: '<span>{{ label || image }}</span>',
})

const TagStub = defineComponent({
  props: {
    value: { type: String, required: true },
  },
  template: '<span>{{ value }}</span>',
})

const PasswordStub = defineComponent({
  props: {
    modelValue: { type: String, default: '' },
  },
  emits: ['update:modelValue', 'blur'],
  methods: {
    handleInput(event: Event) {
      this.$emit('update:modelValue', (event.target as HTMLInputElement).value)
    },
  },
  template: `
    <input
      :value="modelValue"
      @input="handleInput"
      @blur="$emit('blur')"
    />
  `,
})

const MessageStub = defineComponent({
  template: '<p><slot /></p>',
})

const TransitionStub = defineComponent({
  props: {
    name: { type: String, default: '' },
    css: { type: Boolean, default: true },
  },
  template: `
    <div
      data-testid="account-pane-transition"
      :data-transition-name="name"
      :data-transition-css="String(css)"
    >
      <slot />
    </div>
  `,
})

const TestProviders = defineComponent({
  components: { AccountDialogContent, IconoirProvider },
  props: {
    initialTab: { type: String, default: 'profile' },
  },
  template: '<IconoirProvider><AccountDialogContent :initial-tab="initialTab" /></IconoirProvider>',
})

// Resolve expected visible labels through the same locale the component uses.
const enAccountMessages = (
  (await import('../../../../shared/i18n/locales/en/account')) as {
    default: { tabs: Record<string, string> }
  }
).default

const accountTabCases: Array<{ key: AccountTab; label: string }> = accountTabRegistry.flatMap(
  ({ tabs }) =>
    tabs.map(({ key, labelKey }) => ({
      key,
      label: enAccountMessages.tabs[labelKey.replace('account.tabs.', '')] ?? key,
    })),
)

function makeUser(): User {
  return {
    public_id: '00000000-0000-0000-0000-000000000001',
    display_name: 'mock-user',
    storage_namespace: 'USM001',
    avatar: '',
    role: 'user',
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  }
}

function renderAccountDialogContent(initialTab?: AccountTab) {
  const store = useUserStore()
  store.setAuth(makeUser())
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })

  return renderWithUser(TestProviders, {
    props: initialTab === undefined ? {} : { initialTab },
    global: {
      plugins: [router],
      stubs: {
        UiAvatar: AvatarStub,
        UiButton: ButtonStub,
        Message: MessageStub,
        UiPassword: PasswordStub,
        UiTag: TagStub,
        transition: TransitionStub,
        BillingPane: {
          props: ['active'],
          template: '<section aria-label="Transactions">最终收支记录</section>',
        },
        RechargePane: {
          props: ['active', 'paymentReturn'],
          template:
            '<section aria-label="Top up">选择充值数量<span v-if="paymentReturn">{{ paymentReturn.publicToken }}</span></section>',
        },
        UsagePane: {
          props: ['active'],
          template: '<section aria-label="Usage">Lifetime 数据</section>',
        },
        RewardsPane: {
          template: '<section aria-label="Redeem code">输入兑换码</section>',
        },
      },
    },
  })
}

describe('AccountDialogContent', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  beforeEach(() => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false })),
    )
    logoutAndRedirectMock.mockReset()
    logoutAndRedirectMock.mockResolvedValue(true)
    toastErrorMock.mockReset()
    useAvatarUploadMock.mockReset()
    useAvatarUploadMock.mockReturnValue({
      uploading: ref(false),
      handleFileChange: vi.fn(),
    })
    useUserStore().clearAuth()
    localStorage.clear()
  })

  it('uses the shared Motion Policy while switching account panes', async () => {
    const { user } = renderAccountDialogContent()

    expect(screen.getByTestId('account-pane-transition')).toHaveAttribute(
      'data-transition-name',
      'account-pane',
    )
    expect(screen.getByTestId('account-pane-transition')).toHaveAttribute(
      'data-transition-css',
      'true',
    )

    await user.click(screen.getByRole('button', { name: 'Security' }))
    expect(screen.getByRole('heading', { name: 'Security' })).toBeVisible()
  })

  it('switches panes immediately under reduced motion', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )
    const { user } = renderAccountDialogContent()

    expect(screen.getByTestId('account-pane-transition')).toHaveAttribute(
      'data-transition-css',
      'false',
    )

    await user.click(screen.getByRole('button', { name: 'Security' }))
    expect(screen.getByRole('heading', { name: 'Security' })).toBeVisible()
  })

  it('renders a left navigation rail with profile content in the right pane', () => {
    renderAccountDialogContent()

    expect(screen.getByRole('navigation', { name: 'Account navigation' })).toBeVisible()
    expect(screen.getByRole('region', { name: 'Account' })).toBeVisible()
    expect(screen.getByRole('region', { name: 'Billing' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Profile' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('button', { name: 'Top up' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Plans' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Redeem code' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Transactions' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Usage' })).toBeVisible()
    expect(screen.getByText('IdeaBoard')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeVisible()
    expect(screen.getAllByText('mock-user')[0]).toBeVisible()
    expect(screen.getAllByText('USM001')[0]).toBeVisible()
    expect(screen.getByRole('region', { name: 'Account profile overview' })).toBeVisible()
    expect(screen.queryByText('00000000-0000-0000-0000-000000000001')).not.toBeInTheDocument()
    expect(screen.queryByText('正常使用中')).not.toBeInTheDocument()
  })

  it('switches the right pane from profile to account security', async () => {
    const { user } = renderAccountDialogContent()

    await user.click(screen.getByRole('button', { name: 'Security' }))

    expect(screen.getByRole('button', { name: 'Security' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('heading', { name: 'Security' })).toBeVisible()
    expect(screen.queryByText('00000000-0000-0000-0000-000000000001')).not.toBeInTheDocument()
  })

  it('keeps the profile workflow mounted while visiting another account pane', async () => {
    const { user } = renderAccountDialogContent()

    expect(useAvatarUploadMock).toHaveBeenCalledOnce()
    await user.click(screen.getByRole('button', { name: 'Security' }))
    expect(screen.getByRole('heading', { name: 'Security' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'Profile' }))

    expect(screen.getByRole('heading', { name: 'Profile' })).toBeVisible()
    expect(useAvatarUploadMock).toHaveBeenCalledOnce()
  })

  it('opens billing records inside account management', async () => {
    const { user } = renderAccountDialogContent()

    await user.click(screen.getByRole('button', { name: 'Transactions' }))

    expect(screen.getByRole('button', { name: 'Transactions' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(screen.getByRole('region', { name: 'Transactions' })).toHaveTextContent('最终收支记录')
  })

  it('opens directly on the requested initial tab', () => {
    renderAccountDialogContent('billing')

    expect(screen.getByRole('button', { name: 'Transactions' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(screen.getByRole('region', { name: 'Transactions' })).toBeVisible()
    expect(
      screen.queryByRole('region', { name: 'Account profile overview' }),
    ).not.toBeInTheDocument()
  })

  it.each(accountTabCases)(
    'opens the $label tab from the canonical registry',
    async ({ label }) => {
      const { user } = renderAccountDialogContent()

      await user.click(screen.getByRole('button', { name: label }))

      expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-current', 'page')
    },
  )

  it('reacts to an initial tab update and falls back for an unknown tab', async () => {
    const { rerender } = renderAccountDialogContent()

    await rerender({ initialTab: 'usage' })
    expect(screen.getByRole('button', { name: 'Usage' })).toHaveAttribute('aria-current', 'page')

    await rerender({ initialTab: 'unknown-tab' as AccountTab })
    expect(screen.getByRole('button', { name: 'Profile' })).toHaveAttribute('aria-current', 'page')
  })

  it('opens the recharge workflow', async () => {
    const { user } = renderAccountDialogContent()

    await user.click(screen.getByRole('button', { name: 'Top up' }))

    expect(screen.getByRole('region', { name: 'Top up' })).toHaveTextContent('选择充值数量')
  })

  it.each([['Plans', 'Plans is coming soon']])(
    'opens the %s roadmap page',
    async (label, regionName) => {
      const { user } = renderAccountDialogContent()

      await user.click(screen.getByRole('button', { name: label }))

      expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-current', 'page')
      expect(screen.getByRole('region', { name: regionName })).toBeVisible()
    },
  )

  it('opens the rewards redemption pane', async () => {
    const { user } = renderAccountDialogContent()

    await user.click(screen.getByRole('button', { name: 'Redeem code' }))

    expect(screen.getByRole('button', { name: 'Redeem code' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(screen.getByRole('region', { name: 'Redeem code' })).toHaveTextContent('输入兑换码')
  })

  it('opens lifetime usage inside account management', async () => {
    const { user } = renderAccountDialogContent()

    await user.click(screen.getByRole('button', { name: 'Usage' }))

    expect(screen.getByRole('button', { name: 'Usage' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: 'Usage' })).toHaveTextContent('Lifetime 数据')
  })

  it('logs out directly from the left rail without extra confirmation', async () => {
    const { user } = renderAccountDialogContent()

    expect(screen.queryByRole('group', { name: '退出登录确认' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Sign out' }))

    await vi.waitFor(() => {
      expect(logoutAndRedirectMock).toHaveBeenCalledWith(HOME_PATH)
    })
    expect(screen.queryByRole('group', { name: '退出登录确认' })).not.toBeInTheDocument()
  })

  it('keeps logout disabled and ignores duplicate clicks until the request settles', async () => {
    const logoutRequest = createDeferred<boolean>()
    logoutAndRedirectMock.mockReturnValueOnce(logoutRequest.promise)
    const { user } = renderAccountDialogContent()
    const logoutButton = screen.getByRole('button', { name: 'Sign out' })

    await user.click(logoutButton)

    expect(logoutButton).toBeDisabled()
    expect(logoutButton).toHaveAccessibleName('Signing out…')
    await user.click(logoutButton)
    expect(logoutAndRedirectMock).toHaveBeenCalledOnce()

    logoutRequest.resolve(false)

    await waitFor(() => expect(logoutButton).toBeEnabled())
    expect(logoutButton).toHaveAccessibleName('Sign out')
    expect(toastErrorMock).toHaveBeenCalledWith('Sign-out failed. Please try again later.')
  })

  it('restores logout controls and shows safe feedback after a rejected request', async () => {
    logoutAndRedirectMock.mockRejectedValueOnce(new Error('sensitive provider failure'))
    const { user } = renderAccountDialogContent()
    const logoutButton = screen.getByRole('button', { name: 'Sign out' })

    await user.click(logoutButton)

    await waitFor(() => expect(logoutButton).toBeEnabled())
    expect(toastErrorMock).toHaveBeenCalledWith('Sign-out failed. Please try again later.')
  })
})
