/* eslint-disable vue/one-component-per-file -- Local stubs keep the layout contract test focused. */
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import { useUserStore } from '../../../app/stores/user'
import type { User } from '../../../shared/api/userSession'
import AccountManagementDialogContent from './AccountManagementDialogContent.vue'

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({
    fallbackText: 'IB',
    logoURL: '',
    projectName: 'IdeaBoard',
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

function renderAccountManagementDialogContent() {
  const store = useUserStore()
  store.setAuth(makeUser())

  return renderWithUser(AccountManagementDialogContent, {
    global: {
      stubs: {
        Avatar: AvatarStub,
        Button: ButtonStub,
        Message: MessageStub,
        Password: PasswordStub,
        Tag: TagStub,
      },
    },
  })
}

describe('AccountManagementDialogContent', () => {
  beforeEach(() => {
    useUserStore().clearAuth()
    localStorage.clear()
  })

  it('renders a left navigation rail with profile content in the right pane', () => {
    renderAccountManagementDialogContent()

    expect(screen.getByRole('navigation', { name: '账户管理导航' })).toBeVisible()
    expect(screen.getByRole('button', { name: '个人资料' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(screen.getByText('IdeaBoard')).toBeVisible()
    expect(screen.getByRole('button', { name: '退出登录' })).toBeVisible()
    expect(screen.getAllByText('mock-user')[0]).toBeVisible()
    expect(screen.getAllByText('USM001')[0]).toBeVisible()
    expect(screen.getByRole('heading', { name: 'mock-user' })).toBeVisible()
    expect(screen.getAllByText('auth-user-layout')[0]).toBeVisible()
  })

  it('switches the right pane from profile to account security', async () => {
    const { user } = renderAccountManagementDialogContent()

    await user.click(screen.getByRole('button', { name: '账号安全' }))

    expect(screen.getByRole('button', { name: '账号安全' })).toHaveAttribute(
      'aria-current',
      'page',
    )
    expect(screen.getByRole('heading', { name: '账号安全' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: '当前原密码' })).toBeVisible()
    expect(screen.queryByRole('heading', { name: 'mock-user' })).not.toBeInTheDocument()
  })

  it('asks for confirmation before logging out from the left rail', async () => {
    const { user } = renderAccountManagementDialogContent()

    expect(screen.queryByRole('group', { name: '退出登录确认' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '退出登录' }))

    expect(screen.getByRole('group', { name: '退出登录确认' })).toBeVisible()
    expect(screen.getByRole('button', { name: '取消' })).toBeVisible()
    expect(screen.getByRole('button', { name: '确认退出' })).toBeVisible()
  })
})
