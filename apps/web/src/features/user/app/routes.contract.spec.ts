import type { Component } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { userRoutes } from './routes'

const routeComponents = vi.hoisted(() => ({
  layout: { name: 'UserLayoutRouteTarget' },
}))

vi.mock('../layout/UserLayout.vue', () => ({ default: routeComponents.layout }))

type LazyRouteModule = {
  default?: Component
}

function requireRoute(routes: RouteRecordRaw[], path: string): RouteRecordRaw {
  const route = routes.find((candidate) => candidate.path === path)
  expect(route, `route ${path} should exist`).toBeDefined()
  return route as RouteRecordRaw
}

async function loadRouteComponent(route: RouteRecordRaw): Promise<Component> {
  expect(route.component).toEqual(expect.any(Function))
  const loaded = await (route.component as () => Promise<LazyRouteModule | Component>)()
  return (loaded as LazyRouteModule).default ?? (loaded as Component)
}

describe('userRoutes contract', () => {
  it('preserves route paths, names, and access metadata', () => {
    const termsRoute = requireRoute(userRoutes, '/terms')
    const refundPolicyRoute = requireRoute(userRoutes, '/refund-policy')
    const privacyPolicyRoute = requireRoute(userRoutes, '/privacy-policy')
    const acceptableUseRoute = requireRoute(userRoutes, '/acceptable-use')
    const shareRoute = requireRoute(userRoutes, '/share')
    const userRootRoute = requireRoute(userRoutes, '/user')

    expect({
      path: shareRoute.path,
      name: shareRoute.name,
      meta: shareRoute.meta,
    }).toEqual({
      path: '/share',
      name: 'ShareJoin',
      meta: { requiresAuth: true },
    })
    expect({ name: termsRoute.name, meta: termsRoute.meta }).toEqual({
      name: 'Terms',
      meta: { public: true, seoKey: 'terms' },
    })
    expect({ name: refundPolicyRoute.name, meta: refundPolicyRoute.meta }).toEqual({
      name: 'RefundPolicy',
      meta: { public: true, seoKey: 'refund' },
    })
    expect({ name: privacyPolicyRoute.name, meta: privacyPolicyRoute.meta }).toEqual({
      name: 'PrivacyPolicy',
      meta: { public: true, seoKey: 'privacy' },
    })
    expect({ name: acceptableUseRoute.name, meta: acceptableUseRoute.meta }).toEqual({
      name: 'AcceptableUse',
      meta: { public: true, seoKey: 'acceptable-use' },
    })
    expect(
      userRootRoute.children?.map((route) => ({
        path: route.path,
        name: route.name,
        meta: route.meta,
      })),
    ).toEqual([
      { path: 'login', name: 'UserLogin', meta: { guest: true } },
      { path: 'register', name: 'UserRegister', meta: { guest: true } },
      { path: 'auth/callback', name: 'UserAuthCallback', meta: { public: true } },
      { path: 'boards', name: 'UserBoards', meta: { requiresAuth: true } },
      { path: 'account', name: undefined, meta: { requiresAuth: true } },
      { path: 'settings', name: undefined, meta: { requiresAuth: true } },
      { path: 'billing', name: undefined, meta: { requiresAuth: true } },
      { path: 'recharge', name: undefined, meta: { requiresAuth: true } },
      { path: 'search', name: 'UserSearch', meta: { public: true } },
      { path: 'canvas', name: 'UserCanvas', meta: { requiresAuth: true } },
    ])

    expect(requireRoute(userRootRoute.children ?? [], 'account').redirect).toEqual({
      path: '/user/boards',
      query: { accountTab: 'profile' },
    })
    expect(requireRoute(userRootRoute.children ?? [], 'settings').redirect).toEqual({
      path: '/user/boards',
      query: { accountTab: 'security' },
    })
    expect(requireRoute(userRootRoute.children ?? [], 'billing').redirect).toEqual({
      path: '/user/boards',
      query: { accountTab: 'billing' },
    })
    expect(requireRoute(userRootRoute.children ?? [], 'recharge').redirect).toEqual({
      path: '/user/boards',
      query: { accountTab: 'recharge' },
    })
  })

  it('lazy-loads the real User layout and route page modules', async () => {
    const loadedTerms = await loadRouteComponent(requireRoute(userRoutes, '/terms'))
    const loadedRefundPolicy = await loadRouteComponent(requireRoute(userRoutes, '/refund-policy'))
    const loadedPrivacyPolicy = await loadRouteComponent(
      requireRoute(userRoutes, '/privacy-policy'),
    )
    const loadedAcceptableUse = await loadRouteComponent(
      requireRoute(userRoutes, '/acceptable-use'),
    )
    const userRootRoute = requireRoute(userRoutes, '/user')
    const children = userRootRoute.children ?? []

    const loadedLayout = await loadRouteComponent(userRootRoute)
    const loadedLogin = await loadRouteComponent(requireRoute(children, 'login'))
    const loadedRegister = await loadRouteComponent(requireRoute(children, 'register'))
    const loadedBoards = await loadRouteComponent(requireRoute(children, 'boards'))
    const loadedSearch = await loadRouteComponent(requireRoute(children, 'search'))
    const loadedCanvas = await loadRouteComponent(requireRoute(children, 'canvas'))

    expect(loadedLayout).toBe(routeComponents.layout)
    expect((loadedTerms as Component & { __name?: string }).__name).toBe('TermsPage')
    expect((loadedRefundPolicy as Component & { __name?: string }).__name).toBe('RefundPolicyPage')
    expect((loadedPrivacyPolicy as Component & { __name?: string }).__name).toBe(
      'PrivacyPolicyPage',
    )
    expect((loadedAcceptableUse as Component & { __name?: string }).__name).toBe(
      'AcceptableUsePage',
    )
    expect((loadedLogin as Component & { __name?: string }).__name).toBe('CanvasLandingPage')
    expect((loadedRegister as Component & { __name?: string }).__name).toBe('CanvasLandingPage')
    expect((loadedBoards as Component & { __name?: string }).__name).toBe('BoardsPage')
    expect((loadedSearch as Component & { __name?: string }).__name).toBe('SearchPage')
    expect((loadedCanvas as Component & { __name?: string }).__name).toBe('CanvasPage')
  }, 30_000)
})
