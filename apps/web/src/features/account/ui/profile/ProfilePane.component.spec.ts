import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import { makeSessionUser } from '../../../../test/factories'
import type { User } from '../../../../api/sessionUserType'
import ProfilePane from './ProfilePane.vue'

const mockUser = ref<User>(makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' }))

vi.mock('../../../auth', () => ({
  useUserStore: () => ({ user: mockUser.value }),
}))

vi.mock('../../session/profile/useAvatarUpload', () => ({
  useAvatarUpload: () => ({ uploading: ref(false), handleFileChange: vi.fn() }),
}))

describe('ProfilePane role badge', () => {
  it('renders creator role on the neutral shared status base without a dot', () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' })
    const { container } = renderWithUser(ProfilePane)

    const badge = screen.getByText('Creator')
    const root = badge.parentElement!
    expect(root).toHaveClass('bg-muted', 'text-muted-foreground')
    expect(container.querySelector('[aria-hidden="true"] span:last-child')).toBeNull()
  })

  it('renders admin role on the info shared status base', () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada', role: 'admin', avatar: '' })
    renderWithUser(ProfilePane)

    const badge = screen.getByText('Admin')
    const root = badge.parentElement!
    expect(root).toHaveClass('bg-info-soft', 'text-info')
  })
})

describe('ProfilePane account info', () => {
  it('renders storage namespace and dates from session profile facts', () => {
    mockUser.value = makeSessionUser({
      display_name: 'Ada',
      role: 'creator',
      avatar: '',
      storage_namespace: 'USER0001',
      status: 1,
      created_at: '2026-01-02T03:04:05Z',
      updated_at: '2026-02-03T04:05:06Z',
    })
    const { container } = renderWithUser(ProfilePane)

    const info = container.querySelector('[aria-label="Account information"]')
    expect(info).not.toBeNull()
    expect(screen.getByText('Storage namespace')).not.toBeNull()
    const namespace = screen.getByText('USER0001')
    expect(namespace).toHaveClass('font-mono')
    expect(namespace.getAttribute('title')).toBe('USER0001')
    expect(screen.getByText('2026.01.02')).not.toBeNull()
    expect(screen.getByText('2026.02.03')).not.toBeNull()
    expect(screen.queryByText('Banned')).toBeNull()
  })

  it('renders a dash fallback when profile facts are missing', () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' })
    renderWithUser(ProfilePane)

    expect(screen.getAllByText('-')).toHaveLength(3)
  })
})

describe('ProfilePane avatar', () => {
  it('sizes the public route URL to the next variant up (88px at dpr 1 → 128)', () => {
    mockUser.value = makeSessionUser({
      display_name: 'Ada',
      role: 'creator',
      avatar: '/api/v1/public/users/AVTR01/avatar',
    })
    const { container } = renderWithUser(ProfilePane)

    const image = container.querySelector('img')
    expect(image?.getAttribute('src')).toBe('/api/v1/public/users/AVTR01/avatar?size=128')
  })

  it('passes provisioned external avatars through unsized', () => {
    mockUser.value = makeSessionUser({
      display_name: 'Ada',
      role: 'creator',
      avatar: 'https://cdn.example/avatar.png',
    })
    const { container } = renderWithUser(ProfilePane)

    const image = container.querySelector('img')
    expect(image?.getAttribute('src')).toBe('https://cdn.example/avatar.png')
  })
})

describe('ProfilePane banned badge', () => {
  it('renders the banned badge on the danger shared status base without a dot', () => {
    mockUser.value = makeSessionUser({
      display_name: 'Ada',
      role: 'creator',
      avatar: '',
      storage_namespace: 'USER0001',
      status: 0,
      created_at: '2026-01-02T03:04:05Z',
      updated_at: '2026-02-03T04:05:06Z',
    })
    const { container } = renderWithUser(ProfilePane)

    const badge = screen.getByText('Banned')
    const root = badge.parentElement!
    expect(root).toHaveClass('bg-destructive-soft', 'text-destructive')
    expect(root).not.toHaveClass('bg-destructive')
    expect(container.querySelector('[aria-hidden="true"] span:last-child')).toBeNull()
  })
})
