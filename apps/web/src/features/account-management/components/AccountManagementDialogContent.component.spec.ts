/* eslint-disable vue/one-component-per-file -- Local stubs keep the layout contract test focused. */
import { defineComponent, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import { useUserStore } from '../../../app/stores/user'
import type { User } from '../../../shared/api/userSession'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { accountTabRegistry, type AccountManagementTab } from '../accountTabRegistry'
import AccountManagementDialogContent from './AccountManagementDialogContent.vue'

const logoutAndRedirectMock = vi.hoisted(() => vi.fn())
const useAvatarUploadWorkflowMock = vi.hoisted(() => vi.fn())
const toastErrorMock = vi.hoisted(() => vi.fn())

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({
    fallbackText: 'IB',
    logoURL: '',
    projectName: 'IdeaBoard',
  }),
}))

vi.mock('../../../shared/composables/useLogoutRedirect', () => ({
  useLogoutRedirect: () => ({
    logoutAndRedirect: logoutAndRedirectMock,
  }),
}))

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({
    error: toastErrorMock,
  }),
}))

vi.mock('../useAvatarUploadWorkflow', () => ({
  useAvatarUploadWorkflow: useAvatarUploadWorkflowMock,
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

const accountTabCases: Array<{ key: AccountManagementTab; label: string }> =
  accountTabRegistry.flatMap(({ tabs }) => tabs.map(({ key, label }) => ({ key, label })))

function makeUser(): User {
  return {
    id: 1,
    uid: 'USM001',
    username: 'mock-user',
    auth_user_id: 'auth-user-layout',
    avatar: '',
    role: 'user',
    status: 1,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  }
}

function renderAccountManagementDialogContent(initialTab?: AccountManagementTab) {
  const store = useUserStore()
  store.setAuth(makeUser())

  return renderWithUser(AccountManagementDialogContent, {
    props: initialTab === undefined ? {} : { initialTab },
    global: {
      stubs: {
        Avatar: AvatarStub,
        Button: ButtonStub,
        Message: MessageStub,
        Password: PasswordStub,
        Tag: TagStub,
        transition: TransitionStub,
        UserBillingRecordsPane: {
          props: ['active'],
          template: '<section aria-label="账单记录">最终收支记录</section>',
        },
        UserUsagePane: {
          props: ['active'],
          template: '<section aria-label="用量统计">Lifetime 数据</section>',
        },
        UserRewardsPane: {
          template: '<section aria-label="奖励中心">输入兑换码</section>',
        },
      },
    },
  })
}

describe('AccountManagementDialogContent', () => {
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
    useAvatarUploadWorkflowMock.mockReset()
    useAvatarUploadWorkflowMock.mockReturnValue({
      uploading: ref(false),
      handleFileChange: vi.fn(),
    })
    useUserStore().clearAuth()
    localStorage.clear()
  })

  it('uses the shared Motion Policy while switching account panes', async () => {
    const { user } = renderAccountManagementDialogContent()

    expect(screen.getByTestId('account-pane-transition')).toHaveAttribute(
      'data-transition-name',
      'account-pane',
    )
    expect(screen.getByTestId('account-pane-transition')).toHaveAttribute(
      'data-transition-css',
      'true',
    )

    await user.click(screen.getByRole('button', { name: '账号安全' }))
    expect(screen.getByRole('heading', { name: '账号安全' })).toBeVisible()
  })

  it('switches panes immediately under reduced motion', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: true })),
    )
    const { user } = renderAccountManagementDialogContent()

    expect(screen.getByTestId('account-pane-transition')).toHaveAttribute(
      'data-transition-css',
      'false',
    )

    await user.click(screen.getByRole('button', { name: '账号安全' }))
    expect(screen.getByRole('heading', { name: '账号安全' })).toBeVisible()
  })

  it('renders a left navigation rail with profile content in the right pane', () => {
    renderAccountManagementDialogContent()

    expect(screen.getByRole('navigation', { name: '账户管理导航' })).toBeVisible()
    expect(screen.getByRole('region', { name: '账户设置' })).toBeVisible()
    expect(screen.getByRole('region', { name: '订阅与套餐' })).toBeVisible()
    expect(screen.getByRole('region', { name: '权益和账单' })).toBeVisible()
    expect(screen.getByRole('button', { name: '个人资料' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('button', { name: '充值积分' })).toBeVisible()
    expect(screen.getByRole('button', { name: '订阅套餐' })).toBeVisible()
    expect(screen.getByRole('button', { name: '奖励中心' })).toBeVisible()
    expect(screen.getByRole('button', { name: '账单记录' })).toBeVisible()
    expect(screen.getByRole('button', { name: '用量统计' })).toBeVisible()
    expect(screen.getByText('IdeaBoard')).toBeVisible()
    expect(screen.getByRole('button', { name: '退出登录' })).toBeVisible()
    expect(screen.getAllByText('mock-user')[0]).toBeVisible()
    expect(screen.getAllByText('USM001')[0]).toBeVisible()
    expect(screen.getByRole('region', { name: '账户资料概览' })).toBeVisible()
    expect(screen.getAllByText('auth-user-layout')[0]).toBeVisible()
  })

  it('switches the right pane from profile to account security', async () => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: '账号安全' }))

    expect(screen.getByRole('button', { name: '账号安全' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('heading', { name: '账号安全' })).toBeVisible()
    expect(screen.queryByText('auth-user-layout')).not.toBeInTheDocument()
  })

  it('keeps the profile workflow mounted while visiting another account pane', async () => {
    const { user } = renderAccountManagementDialogContent()

    expect(useAvatarUploadWorkflowMock).toHaveBeenCalledOnce()
    await user.click(screen.getByRole('button', { name: '账号安全' }))
    expect(screen.getByRole('heading', { name: '账号安全' })).toBeVisible()
    await user.click(screen.getByRole('button', { name: '个人资料' }))

    expect(screen.getByRole('heading', { name: '个人资料' })).toBeVisible()
    expect(useAvatarUploadWorkflowMock).toHaveBeenCalledOnce()
  })

  it('opens billing records inside account management', async () => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: '账单记录' }))

    expect(screen.getByRole('button', { name: '账单记录' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: '账单记录' })).toHaveTextContent('最终收支记录')
  })

  it('opens directly on the requested initial tab', () => {
    renderAccountManagementDialogContent('billing')

    expect(screen.getByRole('button', { name: '账单记录' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: '账单记录' })).toBeVisible()
    expect(screen.queryByRole('region', { name: '账户资料概览' })).not.toBeInTheDocument()
  })

  it.each(accountTabCases)(
    'opens the $label tab from the canonical registry',
    async ({ label }) => {
      const { user } = renderAccountManagementDialogContent()

      await user.click(screen.getByRole('button', { name: label }))

      expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-current', 'page')
    },
  )

  it('reacts to an initial tab update and falls back for an unknown tab', async () => {
    const { rerender } = renderAccountManagementDialogContent()

    await rerender({ initialTab: 'usage' })
    expect(screen.getByRole('button', { name: '用量统计' })).toHaveAttribute('aria-current', 'page')

    await rerender({ initialTab: 'unknown-tab' as AccountManagementTab })
    expect(screen.getByRole('button', { name: '个人资料' })).toHaveAttribute('aria-current', 'page')
  })

  it.each([
    ['充值积分', '充值积分即将上线'],
    ['订阅套餐', '订阅套餐即将上线'],
  ])('opens the %s roadmap page', async (label, regionName) => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: label }))

    expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: regionName })).toBeVisible()
  })

  it('opens the rewards redemption pane', async () => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: '奖励中心' }))

    expect(screen.getByRole('button', { name: '奖励中心' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: '奖励中心' })).toHaveTextContent('输入兑换码')
  })

  it('opens lifetime usage inside account management', async () => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: '用量统计' }))

    expect(screen.getByRole('button', { name: '用量统计' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: '用量统计' })).toHaveTextContent('Lifetime 数据')
  })

  it('logs out directly from the left rail without extra confirmation', async () => {
    const { user } = renderAccountManagementDialogContent()

    expect(screen.queryByRole('group', { name: '退出登录确认' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '退出登录' }))

    await vi.waitFor(() => {
      expect(logoutAndRedirectMock).toHaveBeenCalledWith(USER_HOME_PATH)
    })
    expect(screen.queryByRole('group', { name: '退出登录确认' })).not.toBeInTheDocument()
  })

  it('keeps logout disabled and ignores duplicate clicks until the request settles', async () => {
    const logoutRequest = Promise.withResolvers<boolean>()
    logoutAndRedirectMock.mockReturnValueOnce(logoutRequest.promise)
    const { user } = renderAccountManagementDialogContent()
    const logoutButton = screen.getByRole('button', { name: '退出登录' })

    await user.click(logoutButton)

    expect(logoutButton).toBeDisabled()
    expect(logoutButton).toHaveAccessibleName('正在退出登录')
    await user.click(logoutButton)
    expect(logoutAndRedirectMock).toHaveBeenCalledOnce()

    logoutRequest.resolve(false)

    await waitFor(() => expect(logoutButton).toBeEnabled())
    expect(logoutButton).toHaveAccessibleName('退出登录')
    expect(toastErrorMock).toHaveBeenCalledWith('退出登录失败，请稍后重试')
  })

  it('restores logout controls and shows safe feedback after a rejected request', async () => {
    logoutAndRedirectMock.mockRejectedValueOnce(new Error('sensitive provider failure'))
    const { user } = renderAccountManagementDialogContent()
    const logoutButton = screen.getByRole('button', { name: '退出登录' })

    await user.click(logoutButton)

    await waitFor(() => expect(logoutButton).toBeEnabled())
    expect(toastErrorMock).toHaveBeenCalledWith('退出登录失败，请稍后重试')
  })
})
