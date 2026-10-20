import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { fireEvent } from '@testing-library/vue'
import { renderWithUser, screen } from '../../../../test/render'
import { makeSessionUser } from '../../../../test/factories'
import type { User } from '../../../../api/sessionUserType'
import ProfilePane from './ProfilePane.vue'

const mockUser = ref<User>(makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' }))

const renameControls = {
  isEditing: ref(false),
  isRenaming: ref(false),
  draftName: ref(''),
  startRename: vi.fn(),
  cancelRename: vi.fn(),
  commitRename: vi.fn(),
}

vi.mock('../../../auth', () => ({
  useUserStore: () => ({ user: mockUser.value }),
}))

vi.mock('../../session/profile/useDisplayNameRename', () => ({
  useDisplayNameRename: () => renameControls,
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

describe('ProfilePane display-name rename', () => {
  it('renders a pencil rename button next to the name', () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' })
    renameControls.isEditing.value = false
    renderWithUser(ProfilePane)

    expect(screen.getByText('Ada')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Rename Ada' })).not.toBeNull()
  })

  it('starts the inline rename from the pencil button', async () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' })
    renameControls.isEditing.value = false
    renameControls.startRename.mockClear()
    const view = renderWithUser(ProfilePane)

    await view.user.click(screen.getByRole('button', { name: 'Rename Ada' }))

    expect(renameControls.startRename).toHaveBeenCalledOnce()
  })

  it('commits on Enter and cancels on Escape', async () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada', role: 'creator', avatar: '' })
    renameControls.isEditing.value = true
    renameControls.isRenaming.value = false
    renameControls.draftName.value = 'Ada'
    renameControls.commitRename.mockClear()
    renameControls.cancelRename.mockClear()
    const view = renderWithUser(ProfilePane)

    const input = screen.getByLabelText('Rename Ada')
    await view.user.clear(input)
    await view.user.type(input, 'New')
    await view.user.keyboard('{Enter}')
    expect(renameControls.commitRename).toHaveBeenCalledOnce()

    await view.user.keyboard('{Escape}')
    expect(renameControls.cancelRename).toHaveBeenCalledOnce()
  })

  it('waits for IME composition to finish before submitting the final draft', async () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada' })
    renameControls.isEditing.value = true
    renameControls.isRenaming.value = false
    renameControls.draftName.value = 'Ada'
    renameControls.commitRename.mockClear()
    renderWithUser(ProfilePane)
    const input = screen.getByLabelText('Rename Ada')

    await fireEvent.compositionStart(input)
    await fireEvent.input(input, { target: { value: '新名字' } })
    await fireEvent.keyDown(input, { key: 'Enter', isComposing: true })
    expect(renameControls.commitRename).not.toHaveBeenCalled()

    await fireEvent.compositionEnd(input)
    expect(renameControls.draftName.value).toBe('新名字')
    await fireEvent.keyDown(input, { key: 'Enter' })
    expect(renameControls.commitRename).toHaveBeenCalledOnce()
  })

  it('accepts 100 non-BMP characters without a UTF-16 input ceiling', async () => {
    mockUser.value = makeSessionUser({ display_name: 'Ada' })
    renameControls.isEditing.value = true
    renameControls.isRenaming.value = false
    renameControls.draftName.value = ''
    const view = renderWithUser(ProfilePane)
    const input = screen.getByLabelText('Rename Ada')
    const name = '😀'.repeat(100)

    await view.user.click(input)
    await view.user.paste(name)

    expect(input).not.toHaveAttribute('maxlength')
    expect(input).toHaveValue(name)
    expect(renameControls.draftName.value).toBe(name)
  })

  it.each([
    { key: 'Enter', keyCode: 229 },
    { key: 'Escape', isComposing: true },
  ])('ignores composition key events: %j', async (event) => {
    mockUser.value = makeSessionUser({ display_name: 'Ada' })
    renameControls.isEditing.value = true
    renameControls.isRenaming.value = false
    renameControls.commitRename.mockClear()
    renameControls.cancelRename.mockClear()
    renderWithUser(ProfilePane)

    await fireEvent.keyDown(screen.getByLabelText('Rename Ada'), event)

    expect(renameControls.commitRename).not.toHaveBeenCalled()
    expect(renameControls.cancelRename).not.toHaveBeenCalled()
  })
})
