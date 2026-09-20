import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { renderWithUser, screen } from '../../../test/render'
import AdminLayout from './AdminLayout.vue'

vi.mock('../shared/composables/useAnomalyRadar', () => ({
  useAnomalyRadar: () => ({ anomalies: [], anomalyCount: 0, hasAnomalies: false }),
}))

vi.mock('../../../shared/composables/useSiteBranding', () => ({
  useSiteBranding: () => ({
    fallbackText: 'IdeaBoard',
    logoURL: '',
    projectName: 'IdeaBoard',
  }),
}))

vi.mock('../shared/logos/useBrandLogoOverrides', () => ({
  useBrandLogoOverrides: () => undefined,
}))

const routes = [
  { path: '/admin/statistics', component: { template: '<div>stats</div>' } },
  { path: '/admin/users', component: { template: '<div>users</div>' } },
  { path: '/admin/credits', component: { template: '<div>credits</div>' } },
  { path: '/admin/monitor', component: { template: '<div>tasks</div>' } },
  { path: '/admin/models', component: { template: '<div>models</div>' } },
  { path: '/admin/feature-prompts', component: { template: '<div>prompts</div>' } },
  { path: '/admin/settings', component: { template: '<div>settings</div>' } },
  { path: '/admin/login', component: { template: '<div>login</div>' } },
]

async function renderLayout(path = '/admin/users') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const view = renderWithUser(AdminLayout, {
    global: { plugins: [router, [VueQueryPlugin, { queryClient }]] },
  })
  return { ...view, router }
}

describe('AdminLayout', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('renders grouped navigation with all sections', async () => {
    await renderLayout()

    for (const group of ['业务监控', '运营管理', '系统配置']) {
      expect(await screen.findByText(group)).toBeVisible()
    }
    for (const label of [
      '数据仪表盘',
      '任务监控',
      '用户管理',
      '积分运营',
      '模型管理',
      '系统提示词',
      '系统设置',
    ]) {
      expect(screen.getByRole('link', { name: label })).toBeVisible()
    }
  })

  it('renders the page head with title and subtitle', async () => {
    await renderLayout('/admin/monitor')

    expect(await screen.findByRole('heading', { name: '任务监控' })).toBeVisible()
    expect(screen.getByText('任务执行轨迹、耗时与错误诊断')).toBeVisible()
  })

  it('marks the active section with aria-current', async () => {
    await renderLayout('/admin/monitor')

    expect(screen.getByRole('link', { name: '任务监控' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('link', { name: '用户管理' })).not.toHaveAttribute('aria-current')
  })

  it('collapses the sidebar and persists the preference', async () => {
    const { user } = await renderLayout()
    const aside = await screen.findByLabelText('后台导航')

    expect(aside).toHaveClass('w-52')
    await user.click(screen.getByRole('button', { name: '折叠侧边栏' }))

    expect(localStorage.getItem('admin_sidebar_collapsed')).toBe('true')
    await user.hover(document.body)
    expect(aside).toHaveClass('lg:w-14')
    expect(screen.queryByText('业务监控')).not.toBeInTheDocument()
  })

  it('does not auto-expand when hovering the collapse button area while collapsed', async () => {
    const { user } = await renderLayout()
    const aside = await screen.findByLabelText('后台导航')

    await user.click(screen.getByRole('button', { name: '折叠侧边栏' }))
    expect(aside).toHaveClass('lg:w-14')

    const expandButton = screen.getByRole('button', { name: '展开并固定侧边栏' })
    await user.hover(expandButton)
    expect(aside).toHaveClass('lg:w-14')
    expect(screen.queryByText('业务监控')).not.toBeInTheDocument()
  })

  it('reveals the sticky topbar title after scrolling on standard pages', async () => {
    Object.defineProperty(window, 'scrollY', { value: 120, configurable: true })
    await renderLayout('/admin/monitor')
    window.dispatchEvent(new Event('scroll'))

    const title = await screen.findByTitle('回到顶部')
    expect(title).toBeVisible()
    expect(title).toHaveTextContent('任务监控')
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
  })

  it('pins the topbar title permanently for system settings without scrolling', async () => {
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
    await renderLayout('/admin/settings/general')

    const title = await screen.findByTitle('回到顶部')
    expect(title).toBeVisible()
    expect(title).toHaveTextContent('系统设置')
  })

  it('opens the mobile drawer from the topbar menu button', async () => {
    const { user } = await renderLayout()
    const nav = await screen.findByLabelText('后台导航')

    expect(nav).toHaveClass('-translate-x-full')
    await user.click(screen.getByRole('button', { name: '切换导航菜单' }))
    expect(nav).not.toHaveClass('-translate-x-full')
  })
})
