import { describe, expect, it } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { adminRoutes } from '../../features/admin/app/routes'
import { SESSION_TRUST_WINDOW_MS } from '../../features/auth'
import { resolveAuthGateRedirect, resolveAuthSessionRefresh } from './authGate'

const NOW = 2_000_000
const FRESH_VALIDATED_AT = NOW - 60_000
const STALE_VALIDATED_AT = NOW - SESSION_TRUST_WINDOW_MS - 1
const BOUNDARY_VALIDATED_AT = NOW - SESSION_TRUST_WINDOW_MS

describe('resolveAuthSessionRefresh', () => {
  it('trusts a fresh session on admin routes without revalidating', () => {
    expect(
      resolveAuthSessionRefresh({ isAdminRoute: true, validatedAt: FRESH_VALIDATED_AT, now: NOW }),
    ).toEqual({ shouldRefresh: false, needsFresh: false })
  })

  it('revalidates a stale session on admin routes', () => {
    expect(
      resolveAuthSessionRefresh({ isAdminRoute: true, validatedAt: STALE_VALIDATED_AT, now: NOW }),
    ).toEqual({ shouldRefresh: true, needsFresh: true })
  })

  it('revalidates at exactly the trust-window boundary', () => {
    expect(
      resolveAuthSessionRefresh({
        isAdminRoute: true,
        validatedAt: BOUNDARY_VALIDATED_AT,
        now: NOW,
      }),
    ).toEqual({ shouldRefresh: true, needsFresh: true })
  })

  it('revalidates an unverified session on admin routes', () => {
    expect(resolveAuthSessionRefresh({ isAdminRoute: true, validatedAt: 0, now: NOW })).toEqual({
      shouldRefresh: true,
      needsFresh: true,
    })
  })

  it('does not force freshness for non-admin protected routes', () => {
    expect(
      resolveAuthSessionRefresh({
        isAdminRoute: false,
        requiresAuth: true,
        validatedAt: STALE_VALIDATED_AT,
        now: NOW,
      }),
    ).toEqual({ shouldRefresh: true, needsFresh: false })
  })

  it('leaves fresh non-admin public navigations alone', () => {
    expect(
      resolveAuthSessionRefresh({ isAdminRoute: false, validatedAt: FRESH_VALIDATED_AT, now: NOW }),
    ).toEqual({ shouldRefresh: false, needsFresh: false })
  })

  it('refreshes guest routes to resolve the login state', () => {
    expect(
      resolveAuthSessionRefresh({
        isAdminRoute: false,
        guest: true,
        validatedAt: FRESH_VALIDATED_AT,
        now: NOW,
      }),
    ).toEqual({ shouldRefresh: true, needsFresh: false })
  })

  it('decides repeated navigations against an advancing clock', () => {
    const first = resolveAuthSessionRefresh({ isAdminRoute: true, validatedAt: 0, now: NOW })
    expect(first).toEqual({ shouldRefresh: true, needsFresh: true })

    // First navigation revalidated at NOW; a minute later the session is trusted.
    const second = resolveAuthSessionRefresh({
      isAdminRoute: true,
      validatedAt: NOW,
      now: NOW + 60_000,
    })
    expect(second).toEqual({ shouldRefresh: false, needsFresh: false })

    // Past the window the next admin navigation revalidates again.
    const third = resolveAuthSessionRefresh({
      isAdminRoute: true,
      validatedAt: NOW,
      now: NOW + SESSION_TRUST_WINDOW_MS,
    })
    expect(third).toEqual({ shouldRefresh: true, needsFresh: true })
  })
})

describe('resolveAuthGateRedirect', () => {
  it.each(['/admin', '/admin/credits/reward-codes'])(
    'requires authentication for Admin routes without an explicit auth flag: %s',
    (path) => {
      const router = createRouter({ history: createMemoryHistory(), routes: adminRoutes })
      const destination = router.resolve(path)

      expect(
        resolveAuthGateRedirect({
          isAdminRoute: true,
          user: null,
          currentPath: destination.path,
          currentFullPath: destination.fullPath,
        }),
      ).toBe('/user/login?redirect=' + encodeURIComponent(path))
    },
  )

  it('sends unauthenticated protected navigations to login with the destination preserved', () => {
    expect(
      resolveAuthGateRedirect({
        isAdminRoute: false,
        requiresAuth: true,
        user: null,
        currentPath: '/user/canvas',
        currentFullPath: '/user/canvas?boardId=board-1',
      }),
    ).toBe('/user/login?redirect=%2Fuser%2Fcanvas%3FboardId%3Dboard-1')
  })

  it('sends unauthenticated admin navigations to login with the destination preserved', () => {
    expect(
      resolveAuthGateRedirect({
        isAdminRoute: true,
        requiresAuth: true,
        user: null,
        currentPath: '/admin/users',
        currentFullPath: '/admin/users?page=1',
      }),
    ).toBe('/user/login?redirect=%2Fadmin%2Fusers%3Fpage%3D1')
  })

  it('redirects logged-in guests to the workspace default', () => {
    expect(
      resolveAuthGateRedirect({
        isAdminRoute: false,
        guest: true,
        user: { role: 'user' },
        currentPath: '/user/login',
        currentFullPath: '/user/login',
      }),
    ).toBe('/user/boards')
  })

  it('sends authenticated admins on guest routes to the workspace default', () => {
    expect(
      resolveAuthGateRedirect({
        isAdminRoute: false,
        guest: true,
        user: { role: 'admin' },
        currentPath: '/user/login',
        currentFullPath: '/user/login',
      }),
    ).toBe('/user/boards')
  })

  it('blocks non-admin users from admin routes (role gate)', () => {
    expect(
      resolveAuthGateRedirect({
        isAdminRoute: true,
        user: { role: 'user' },
        currentPath: '/admin/monitor',
        currentFullPath: '/admin/monitor',
      }),
    ).toBe('/user/boards')
  })

  it('lets admins continue on admin routes', () => {
    expect(
      resolveAuthGateRedirect({
        isAdminRoute: true,
        user: { role: 'admin' },
        currentPath: '/admin/monitor',
        currentFullPath: '/admin/monitor',
      }),
    ).toBe(true)
  })
})
