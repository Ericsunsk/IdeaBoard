// Workspace resource contract (issue #801).
//
// MSW-level contract: exercises the real Public generated transport against
// the AGREED envelopes, so backend drift surfaces here instead of in UI.
// No generated-query mocks by design (see checkTestGuardrails).
import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import {
  buildWorkspaceCreateHeaders,
  classifyWorkspaceAccessError,
  fetchAllWorkspaces,
  fetchPrincipal,
  fetchWorkspacePage,
  normalizeWorkspaceListParams,
  parsePrincipal,
  parseWorkspaceItem,
  parseWorkspacePage,
  provisionWorkspace,
  workspaceApiUrls,
  WORKSPACE_NAME_MAX_LENGTH,
  WORKSPACE_PAGE_SIZE_DEFAULT,
  WORKSPACE_PAGE_SIZE_MAX,
} from './workspaceResource'

const AGREED_PAGE = {
  data: [
    { public_id: 'ws_a1', name: 'Alpha', organization_id: null, archived_at: null },
    { public_id: 'ws_b2', name: 'Beta', organization_id: 'org_c3', archived_at: null },
  ],
  meta: { next_cursor: 'cursor2', has_more: true },
}

describe('workspace list envelope', () => {
  it('maps the agreed snake_case page to camelCase domain items', async () => {
    useMswHandlers(http.get('/api/v1/workspaces', () => HttpResponse.json(AGREED_PAGE)))

    const page = await fetchWorkspacePage()

    expect(page.items).toEqual([
      {
        publicId: 'ws_a1',
        name: 'Alpha',
        organizationId: null,
        archivedAt: null,
      },
      {
        publicId: 'ws_b2',
        name: 'Beta',
        organizationId: 'org_c3',
        archivedAt: null,
      },
    ])
    expect(page).toMatchObject({ nextCursor: 'cursor2', hasMore: true })
  })

  it('tolerates the kernel stub shape until the backend slice lands', () => {
    const page = parseWorkspacePage({
      data: [{ id: 'ws_stub1', created_at: '2026-09-01T00:00:00Z' }],
      meta: { next_cursor: null, has_more: false },
    })

    expect(page.items[0]).toMatchObject({ publicId: 'ws_stub1', name: '' })
    expect(page).toMatchObject({ nextCursor: null, hasMore: false })
  })

  it('walks cursor pagination and clamps page size to the agreed bounds', async () => {
    const seenUrls: string[] = []
    useMswHandlers(
      http.get('/api/v1/workspaces', ({ request }) => {
        seenUrls.push(request.url)
        const cursor = new URL(request.url).searchParams.get('cursor')
        if (cursor === 'cursor2') {
          return HttpResponse.json({
            data: [{ public_id: 'ws_c3', name: 'Gamma' }],
            meta: { next_cursor: null, has_more: false },
          })
        }
        return HttpResponse.json(AGREED_PAGE)
      }),
    )

    const items = await fetchAllWorkspaces({ pageSize: 999, maxPages: 5 })

    expect(items.map((item) => item.publicId)).toEqual(['ws_a1', 'ws_b2', 'ws_c3'])
    expect(seenUrls[0]).toContain(`page_size=${WORKSPACE_PAGE_SIZE_MAX}`)
    expect(seenUrls[1]).toContain('cursor=cursor2')
  })

  it('defaults to page size 20 and rejects opaque-ID violations', () => {
    expect(normalizeWorkspaceListParams()).toMatchObject({
      page_size: WORKSPACE_PAGE_SIZE_DEFAULT,
    })
    expect(normalizeWorkspaceListParams({ pageSize: 0 }).page_size).toBe(1)

    for (const raw of [
      { public_id: '123', name: 'Legacy' },
      { public_id: 'org_c3', name: 'Wrong prefix' },
      { public_id: 'ws_', name: 'Empty payload' },
      { id: 42, name: 'Numeric' },
    ]) {
      expect(() => parseWorkspaceItem(raw)).toThrow()
    }
    expect(() => parseWorkspacePage({ data: [{ public_id: 'usr_u1' }] })).toThrow()
    expect(() => parseWorkspacePage({ bogus: true })).toThrow()
  })

  it('propagates Request-Id on reads', async () => {
    let requestId: string | null = null
    useMswHandlers(
      http.get('/api/v1/workspaces', ({ request }) => {
        requestId = request.headers.get('request-id')
        return HttpResponse.json(AGREED_PAGE)
      }),
    )

    await fetchWorkspacePage(undefined, { requestId: 'req_workspace_1' })

    expect(requestId).toBe('req_workspace_1')
  })

  it('keeps hidden (404) distinct from forbidden (403)', async () => {
    useMswHandlers(
      http.get('/api/v1/workspaces', () =>
        HttpResponse.json(
          {
            error: {
              code: 'workspace_not_found',
              message: 'Not found',
              request_id: 'req_404',
            },
          },
          { status: 404 },
        ),
      ),
    )

    const hidden = await fetchWorkspacePage().catch((error: unknown) => error)
    expect(classifyWorkspaceAccessError(hidden)).toBe('hidden')

    expect(classifyWorkspaceAccessError({ status: 403 })).toBe('forbidden')
    expect(classifyWorkspaceAccessError({ status: 500 })).toBeNull()
    expect(classifyWorkspaceAccessError(new Error('boom'))).toBeNull()
  })

  it('redirects to login by default on a list 401', async () => {
    localStorage.setItem('user', JSON.stringify({ public_id: 'ws_owner1' }))
    window.history.pushState({}, '', '/user/boards')
    useMswHandlers(
      http.get('/api/v1/workspaces', () =>
        HttpResponse.json({ code: 401, msg: '' }, { status: 401 }),
      ),
    )

    await expect(fetchWorkspacePage()).rejects.toThrow()

    expect(localStorage.getItem('user')).toBeNull()
    expect(window.location.pathname).toBe('/user/login')
    window.history.pushState({}, '', '/')
  })

  it('returns a silent list 401 without navigating', async () => {
    localStorage.setItem('user', JSON.stringify({ public_id: 'ws_owner1' }))
    window.history.pushState({}, '', '/user/boards')
    useMswHandlers(
      http.get('/api/v1/workspaces', () =>
        HttpResponse.json({ code: 401, msg: '' }, { status: 401 }),
      ),
    )

    await expect(fetchWorkspacePage(undefined, { authFailure: 'silent' })).rejects.toThrow()

    expect(localStorage.getItem('user')).toBeNull()
    expect(window.location.pathname).toBe('/user/boards')
    window.history.pushState({}, '', '/')
  })
})

describe('workspace creation', () => {
  it('injects Idempotency-Key and Request-Id on every create', async () => {
    const seen: Array<{ idempotencyKey: string | null; requestId: string | null; body: unknown }> =
      []
    useMswHandlers(
      http.post('/api/v1/workspaces', async ({ request }) => {
        seen.push({
          idempotencyKey: request.headers.get('idempotency-key'),
          requestId: request.headers.get('request-id'),
          body: await request.json(),
        })
        return HttpResponse.json({
          data: { public_id: 'ws_new1', name: 'New', organization_id: null, archived_at: null },
        })
      }),
    )

    const created = await provisionWorkspace(
      { name: 'New' },
      { idempotencyKey: 'workspace-create:retry-1', requestId: 'req_create_1' },
    )

    expect(created).toMatchObject({ publicId: 'ws_new1', name: 'New' })
    expect(seen).toHaveLength(1)
    expect(seen[0]?.idempotencyKey).toBe('workspace-create:retry-1')
    expect(seen[0]?.requestId).toBe('req_create_1')
    expect(seen[0]?.body).toEqual({ data: { name: 'New' } })
  })

  it('generates a fresh idempotency key per new intent', () => {
    const first = buildWorkspaceCreateHeaders()
    const second = buildWorkspaceCreateHeaders()

    expect(first['Idempotency-Key']).toMatch(/^workspace-create:/)
    expect(second['Idempotency-Key']).toMatch(/^workspace-create:/)
    expect(first['Idempotency-Key']).not.toBe(second['Idempotency-Key'])
    expect(first['Request-Id']).toBeTruthy()
  })

  it('validates name and organization inputs before touching the network', async () => {
    await expect(provisionWorkspace({ name: '   ' })).rejects.toThrow()
    await expect(
      provisionWorkspace({ name: 'x'.repeat(WORKSPACE_NAME_MAX_LENGTH + 1) }),
    ).rejects.toThrow()
    await expect(provisionWorkspace({ name: 'Ok', organizationId: 'nope' })).rejects.toThrow()
  })
})

describe('principal envelope', () => {
  it('maps the agreed principal shape to camelCase', async () => {
    useMswHandlers(
      http.get('/api/v1/user', () =>
        HttpResponse.json({
          data: { public_id: 'usr_u1', display_name: 'Ada', avatar: 'https://cdn.example/a.png' },
        }),
      ),
    )

    await expect(fetchPrincipal()).resolves.toEqual({
      publicId: 'usr_u1',
      displayName: 'Ada',
      avatar: 'https://cdn.example/a.png',
    })
  })

  it('tolerates the stub User shape and rejects non-user IDs', () => {
    expect(parsePrincipal({ data: { id: 'usr_stub9' } })).toEqual({
      publicId: 'usr_stub9',
      displayName: '',
    })
    expect(() => parsePrincipal({ data: { id: 'ws_a1' } })).toThrow()
    expect(() => parsePrincipal({ data: null })).toThrow()
  })
})

describe('workspace URLs', () => {
  it('builds canonical agreed-contract URLs without legacy user routes', () => {
    const urls = [
      workspaceApiUrls.user(),
      workspaceApiUrls.workspaces(),
      workspaceApiUrls.workspaces({ cursor: 'c 1', pageSize: 50 }),
      workspaceApiUrls.workspaceDetail('ws_a1'),
      workspaceApiUrls.archiveWorkspace('ws_a1'),
      workspaceApiUrls.organizations(),
      workspaceApiUrls.organizationDetail('org_c3'),
      workspaceApiUrls.organizationWorkspaces('org_c3'),
    ]

    for (const url of urls) {
      expect(url).toContain('/api/v1/')
      expect(url).not.toContain('/api/user')
      expect(url).not.toContain('/api/v1/me')
    }
    expect(workspaceApiUrls.workspaces({ cursor: 'c 1', pageSize: 50 })).toContain(
      'cursor=c+1&page_size=50',
    )
    expect(workspaceApiUrls.workspaceDetail('ws a/1')).toContain('ws%20a%2F1')
  })
})
