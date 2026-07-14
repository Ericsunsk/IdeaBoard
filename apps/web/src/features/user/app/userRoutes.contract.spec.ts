import type { Component } from 'vue'
import type { RouteRecordRaw } from 'vue-router'
import { describe, expect, it } from 'vitest'
import { userRoutes } from './userRoutes'

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
      { path: 'boards', name: 'UserBoards', meta: { requiresAuth: true } },
      { path: 'canvas', name: 'UserCanvas', meta: { requiresAuth: true } },
    ])
  })

  it('lazy-loads the real User layout and route page modules', async () => {
    const userRootRoute = requireRoute(userRoutes, '/user')
    const children = userRootRoute.children ?? []

    const [
      userLayoutModule,
      homeModule,
      boardsModule,
      canvasModule,
      loadedLayout,
      loadedHome,
      loadedLogin,
      loadedRegister,
      loadedBoards,
      loadedCanvas,
    ] = await Promise.all([
      import('../layout/UserLayout.vue'),
      import('../../canvas/pages/CanvasLandingPage.vue'),
      import('../../board-catalog/pages/BoardsPage.vue'),
      import('../../canvas/pages/CanvasPage.vue'),
      loadRouteComponent(userRootRoute),
      loadRouteComponent(requireRoute(children, 'home')),
      loadRouteComponent(requireRoute(children, 'login')),
      loadRouteComponent(requireRoute(children, 'register')),
      loadRouteComponent(requireRoute(children, 'boards')),
      loadRouteComponent(requireRoute(children, 'canvas')),
    ])

    expect(loadedLayout).toBe(userLayoutModule.default)
    expect(loadedHome).toBe(homeModule.default)
    expect(loadedLogin).toBe(homeModule.default)
    expect(loadedRegister).toBe(homeModule.default)
    expect(loadedBoards).toBe(boardsModule.default)
    expect(loadedCanvas).toBe(canvasModule.default)
  })
})
