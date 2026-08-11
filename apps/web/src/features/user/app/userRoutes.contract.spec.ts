import type { Component } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { describe, expect, it, vi } from 'vitest'
import { userRoutes } from './userRoutes'
import { PUBLIC_HOME_PATH, USER_HOME_PATH } from '../../../shared/constants/routePaths'

const routeComponents = vi.hoisted(() => ({
  layout: { name: 'UserLayoutRouteTarget' },
  home: { name: 'UserHomeRouteTarget' },
}))

vi.mock('../layout/UserLayout.vue', () => ({ default: routeComponents.layout }))
vi.mock('../../canvas/pages/CanvasLandingPage.vue', () => ({ default: routeComponents.home }))

type LazyRouteModule = {
  default: Component
}

function requireRoute(routes: RouteRecordRaw[], path: string): RouteRecordRaw {
  const route = routes.find((candidate) => candidate.path === path)
  expect(route, `route ${path} should exist`).toBeDefined()
  return route as RouteRecordRaw
}

async function loadRouteComponent(route: RouteRecordRaw): Promise<Component> {
  expect(route.component).toEqual(expect.any(Function))
  const loaded = await (route.component as () => Promise<LazyRouteModule>)()
  return loaded.default
}

describe('userRoutes contract', () => {
  it('preserves route paths, names, and access metadata', () => {
    const termsRoute = requireRoute(userRoutes, '/terms')
    const refundPolicyRoute = requireRoute(userRoutes, '/refund-policy')
    const acceptableUseRoute = requireRoute(userRoutes, '/acceptable-use')
    const shareRoute = requireRoute(userRoutes, '/share')
    const legacyHomeRoute = requireRoute(userRoutes, USER_HOME_PATH)
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
      meta: { public: true },
    })
    expect({ name: refundPolicyRoute.name, meta: refundPolicyRoute.meta }).toEqual({
      name: 'RefundPolicy',
      meta: { public: true },
    })
    expect({ name: acceptableUseRoute.name, meta: acceptableUseRoute.meta }).toEqual({
      name: 'AcceptableUse',
      meta: { public: true },
    })
    expect({
      name: legacyHomeRoute.name,
      redirect: legacyHomeRoute.redirect,
      meta: legacyHomeRoute.meta,
    }).toEqual({
      name: 'LegacyUserHome',
      redirect: PUBLIC_HOME_PATH,
      meta: { public: true },
    })
    expect(
      userRootRoute.children?.map((route) => ({
        path: route.path,
        name: route.name,
        meta: route.meta,
      })),
    ).toEqual([
      { path: 'home', name: 'UserHome', meta: undefined },
      { path: 'login', name: 'UserLogin', meta: { guest: true } },
      { path: 'register', name: 'UserRegister', meta: { guest: true } },
      { path: 'auth/callback', name: 'UserAuthCallback', meta: { public: true } },
      { path: 'boards', name: 'UserBoards', meta: { requiresAuth: true } },
      { path: 'canvas', name: 'UserCanvas', meta: { requiresAuth: true } },
    ])
  })

  it('lazy-loads the real User layout and route page modules', async () => {
    const loadedTerms = await loadRouteComponent(requireRoute(userRoutes, '/terms'))
    const loadedRefundPolicy = await loadRouteComponent(requireRoute(userRoutes, '/refund-policy'))
    const loadedAcceptableUse = await loadRouteComponent(
      requireRoute(userRoutes, '/acceptable-use'),
    )
    const userRootRoute = requireRoute(userRoutes, '/user')
    const children = userRootRoute.children ?? []

    const loadedLayout = await loadRouteComponent(userRootRoute)
    const loadedHome = await loadRouteComponent(requireRoute(children, 'home'))
    const loadedLogin = await loadRouteComponent(requireRoute(children, 'login'))
    const loadedRegister = await loadRouteComponent(requireRoute(children, 'register'))
    const loadedBoards = await loadRouteComponent(requireRoute(children, 'boards'))
    const loadedCanvas = await loadRouteComponent(requireRoute(children, 'canvas'))

    expect(loadedLayout).toBe(routeComponents.layout)
    expect((loadedTerms as Component & { __name?: string }).__name).toBe('TermsPage')
    expect((loadedRefundPolicy as Component & { __name?: string }).__name).toBe('RefundPolicyPage')
    expect((loadedAcceptableUse as Component & { __name?: string }).__name).toBe(
      'AcceptableUsePage',
    )
    expect(loadedHome).toBe(routeComponents.home)
    expect((loadedLogin as Component & { __name?: string }).__name).toBe('CanvasLandingPage')
    expect((loadedRegister as Component & { __name?: string }).__name).toBe('CanvasLandingPage')
    expect((loadedBoards as Component & { __name?: string }).__name).toBe('BoardsPage')
    expect((loadedCanvas as Component & { __name?: string }).__name).toBe('CanvasPage')
  }, 15_000)
})
