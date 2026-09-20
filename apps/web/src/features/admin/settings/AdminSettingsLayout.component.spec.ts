import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../test/render'
import {
  ADMIN_SETTINGS_GENERAL_PATH,
  ADMIN_SETTINGS_STORAGE_PATH,
} from '../../../shared/constants/routePaths'
import AdminSettingsLayout from './AdminSettingsLayout.vue'

const routerMocks = vi.hoisted(() => ({
  push: vi.fn(),
  route: { path: '/admin/settings/general' },
}))

vi.mock('vue-router', async (importOriginal) => {
  const original = await importOriginal<typeof import('vue-router')>()
  return {
    ...original,
    useRoute: () => routerMocks.route,
    useRouter: () => ({ push: routerMocks.push }),
  }
})

describe('AdminSettingsLayout', () => {
  beforeEach(() => {
    routerMocks.push.mockReset()
    routerMocks.route.path = ADMIN_SETTINGS_GENERAL_PATH
  })

  it('renders all 4 navigation tabs in the sub-header: 网站设置, 存储配置, 邮件服务, 安全与网络', async () => {
    renderWithUser(AdminSettingsLayout, {
      global: {
        stubs: { RouterView: true },
      },
    })

    expect(screen.getByText('网站设置')).toBeVisible()
    expect(screen.getByText('存储配置')).toBeVisible()
    expect(screen.getByText('邮件服务')).toBeVisible()
    expect(screen.getByText('安全与网络')).toBeVisible()
  })

  it('navigates to the clicked tab route', async () => {
    const { user } = renderWithUser(AdminSettingsLayout, {
      global: {
        stubs: { RouterView: true },
      },
    })

    const storageTab = screen.getByText('存储配置')
    await user.click(storageTab)

    expect(routerMocks.push).toHaveBeenCalledWith(ADMIN_SETTINGS_STORAGE_PATH)
  })
})
