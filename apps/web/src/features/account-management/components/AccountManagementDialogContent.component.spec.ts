/* eslint-disable vue/one-component-per-file -- Local stubs keep the layout contract test focused. */
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import { useUserStore } from '../../../app/stores/user'
import type { User } from '../../../shared/api/userSession'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import type { AccountManagementTab } from '../useAccountManagementDialogController'
import AccountManagementDialogContent from './AccountManagementDialogContent.vue'

const logoutAndRedirectMock = vi.hoisted(() => vi.fn())

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
    error: vi.fn(),
  }),
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

function renderAccountManagementDialogContent(initialTab: AccountManagementTab = 'profile') {
  const store = useUserStore()
  store.setAuth(makeUser())

  return renderWithUser(AccountManagementDialogContent, {
    props: { initialTab },
    global: {
      stubs: {
        Avatar: AvatarStub,
        Button: ButtonStub,
        Message: MessageStub,
        Password: PasswordStub,
        Tag: TagStub,
        UserCreditsPane: {
          props: ['active'],
          template: '<section aria-label="点子账户">点子余额与流水</section>',
        },
        UserUsagePane: {
          props: ['active'],
          template: '<section aria-label="用量统计">Lifetime 数据</section>',
        },
      },
    },
  })
}

describe('AccountManagementDialogContent', () => {
  beforeEach(() => {
    logoutAndRedirectMock.mockReset()
    logoutAndRedirectMock.mockResolvedValue(true)
    useUserStore().clearAuth()
    localStorage.clear()
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
    expect(screen.getByRole('textbox', { name: '当前原密码' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: '设置新密码' })).toBeVisible()
    expect(screen.queryByText('auth-user-layout')).not.toBeInTheDocument()
  })

  it('opens the point wallet inside account management', async () => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: '账单记录' }))

    expect(screen.getByRole('button', { name: '账单记录' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: '点子账户' })).toHaveTextContent('点子余额与流水')
  })

  it('opens directly on the requested initial tab', () => {
    renderAccountManagementDialogContent('billing')

    expect(screen.getByRole('button', { name: '账单记录' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: '点子账户' })).toBeVisible()
    expect(screen.queryByRole('region', { name: '账户资料概览' })).not.toBeInTheDocument()
  })

  it.each([
    ['充值积分', '充值积分即将上线'],
    ['订阅套餐', '订阅套餐即将上线'],
    ['奖励中心', '奖励中心即将上线'],
  ])('opens the %s roadmap page', async (label, regionName) => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: label }))

    expect(screen.getByRole('button', { name: label })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('region', { name: regionName })).toBeVisible()
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
})
