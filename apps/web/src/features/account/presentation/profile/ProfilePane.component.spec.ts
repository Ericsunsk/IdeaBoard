import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import ProfilePane from './ProfilePane.vue'

const mockUser = ref({ display_name: 'Ada', role: 'creator', avatar: '' })

vi.mock('../../../auth', () => ({
  useUserStore: () => ({ user: mockUser.value }),
}))

vi.mock('../../application/profile/useAvatarUpload', () => ({
  useAvatarUpload: () => ({ uploading: ref(false), handleFileChange: vi.fn() }),
}))

describe('ProfilePane role badge', () => {
  it('renders creator role on the neutral shared status base without a dot', () => {
    mockUser.value = { display_name: 'Ada', role: 'creator', avatar: '' }
    const { container } = renderWithUser(ProfilePane)

    const badge = screen.getByText('Creator')
    const root = badge.parentElement!
    expect(root).toHaveClass('bg-muted', 'text-muted-foreground')
    expect(container.querySelector('[aria-hidden="true"] span:last-child')).toBeNull()
  })

  it('renders admin role on the info shared status base', () => {
    mockUser.value = { display_name: 'Ada', role: 'admin', avatar: '' }
    renderWithUser(ProfilePane)

    const badge = screen.getByText('Admin')
    const root = badge.parentElement!
    expect(root).toHaveClass('bg-info-soft', 'text-info')
  })
})
