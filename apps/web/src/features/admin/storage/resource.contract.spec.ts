import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createAdminStorageResource } from './resource'
import type { StorageProfilePayload } from './helpers'

type StorageRequestLog = {
  method: string
  path: string
  search: string
  body?: unknown
}

describe('createAdminStorageResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads storage DTOs and routes commands through the feature resource boundary', async () => {
    const receivedRequests: StorageRequestLog[] = []
    useMswHandlers(...storageQueryHandlers(), ...storageMutationHandlers(receivedRequests))
    const resource = createAdminStorageResource()
    const payload = makeStoragePayload()

    await expect(resource.fetchProviders()).resolves.toEqual([makeS3Provider()])
    await expect(resource.fetchProfiles()).resolves.toEqual([
      makeGeneratedProfile('profile-listed'),
    ])
    await expect(resource.saveProfile({ payload })).resolves.toMatchObject({
      message: 'created',
      profile: {
        id: 'profile-created',
      },
    })
    await expect(
      resource.saveProfile({ id: 'profile-1', payload, apply: true }),
    ).resolves.toMatchObject({
      message: 'updated',
      profile: {
        id: 'profile-updated',
      },
    })
    await expect(resource.testProfile('profile-1')).resolves.toEqual({ message: 'tested' })
    await expect(resource.applyProfile('profile-1')).resolves.toMatchObject({
      message: 'applied',
      profile: {
        id: 'profile-applied',
      },
    })
    await expect(resource.fetchBucketDomains('profile-1')).resolves.toEqual([
      'cdn.example.com',
      'assets.example.com',
    ])
    await expect(resource.deleteProfile('profile-1')).resolves.toEqual({ message: 'deleted' })

    expect(receivedRequests).toEqual([
      {
        method: 'POST',
        path: '/api/admin/storage/profiles',
        search: '',
        body: payload,
      },
      {
        method: 'PUT',
        path: '/api/admin/storage/profiles/profile-1',
        search: '?apply=true',
        body: payload,
      },
      {
        method: 'POST',
        path: '/api/admin/storage/profiles/profile-1/test',
        search: '',
      },
      {
        method: 'POST',
        path: '/api/admin/storage/profiles/profile-1/apply',
        search: '',
      },
      {
        method: 'GET',
        path: '/api/admin/storage/profiles/profile-1/bucket-info',
        search: '',
      },
      {
        method: 'DELETE',
        path: '/api/admin/storage/profiles/profile-1',
        search: '',
      },
    ])
  })

  it('surfaces storage API failures from the HTTP response body', async () => {
    useMswHandlers(
      ...storageQueryHandlers(),
      http.post('/api/admin/storage/profiles', () => {
        return HttpResponse.json({ code: 409, msg: '存储名称已存在', message_safe: true })
      }),
    )
    const resource = createAdminStorageResource()

    await expect(resource.saveProfile({ payload: makeStoragePayload() })).rejects.toThrow(
      '存储名称已存在',
    )
  })

  it('uses the resource fallback when a successful response omits required data', async () => {
    useMswHandlers(
      http.get('/api/admin/storage/providers', () => {
        return HttpResponse.json({ code: 0, msg: 'success' })
      }),
    )
    const resource = createAdminStorageResource()

    await expect(resource.fetchProviders()).rejects.toThrow('加载存储供应商失败')
  })

  it('normalizes provider field lists when the API returns null for providers without fields', async () => {
    useMswHandlers(
      http.get('/api/admin/storage/providers', () => {
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            providers: [
              {
                provider: 'local',
                label: '本地存储',
                description: 'Local uploads',
                builtin: true,
                create_allowed: false,
                supports_test: false,
                supports_bucket_info: false,
                fields: null,
              },
            ],
          },
        })
      }),
    )
    const resource = createAdminStorageResource()

    await expect(resource.fetchProviders()).resolves.toEqual([
      expect.objectContaining({
        provider: 'local',
        fields: [],
      }),
    ])
  })
})

function storageQueryHandlers() {
  return [
    http.get('/api/admin/storage/providers', () => {
      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          providers: [makeS3Provider()],
        },
      })
    }),
    http.get('/api/admin/storage/profiles', () => {
      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          profiles: [makeGeneratedProfile('profile-listed')],
        },
      })
    }),
  ]
}

function storageMutationHandlers(receivedRequests: StorageRequestLog[]) {
  return [
    http.post('/api/admin/storage/profiles', async ({ request }) => {
      recordRequest(receivedRequests, request, await readRequestJson(request))

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          message: 'created',
          profile: makeGeneratedProfile('profile-created'),
        },
      })
    }),
    http.put('/api/admin/storage/profiles/:id', async ({ request }) => {
      recordRequest(receivedRequests, request, await readRequestJson(request))

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          message: 'updated',
          profile: makeGeneratedProfile('profile-updated'),
        },
      })
    }),
    http.post('/api/admin/storage/profiles/:id/test', ({ request }) => {
      recordRequest(receivedRequests, request)

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          message: 'tested',
        },
      })
    }),
    http.post('/api/admin/storage/profiles/:id/apply', ({ request }) => {
      recordRequest(receivedRequests, request)

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          message: 'applied',
          profile: makeGeneratedProfile('profile-applied'),
        },
      })
    }),
    http.get('/api/admin/storage/profiles/:id/bucket-info', ({ request }) => {
      recordRequest(receivedRequests, request)

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          info: {
            domains: ['cdn.example.com', 'assets.example.com'],
          },
        },
      })
    }),
    http.delete('/api/admin/storage/profiles/:id', ({ request }) => {
      recordRequest(receivedRequests, request)

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          message: 'deleted',
        },
      })
    }),
  ]
}

function recordRequest(receivedRequests: StorageRequestLog[], request: Request, body?: unknown) {
  const url = new URL(request.url)
  receivedRequests.push({
    method: request.method,
    path: url.pathname,
    search: url.search,
    ...(body === undefined ? {} : { body }),
  })
}

async function readRequestJson(request: Request) {
  const text = await request.text()
  return text ? JSON.parse(text) : undefined
}

function makeS3Provider() {
  return {
    provider: 's3-compatible',
    label: 'S3 兼容存储',
    description: 'S3 compatible object storage',
    builtin: false,
    create_allowed: true,
    supports_test: true,
    supports_bucket_info: true,
    fields: [
      {
        key: 'bucket_name',
        label: 'Bucket',
        type: 'text',
        required: true,
      },
      {
        key: 'access_key',
        label: 'Access Key',
        type: 'text',
        secret: true,
      },
    ],
  }
}

function makeStoragePayload(): StorageProfilePayload {
  return {
    name: '对象存储',
    provider: 's3-compatible',
    enabled: true,
    config: {
      bucket_name: 'images',
      access_key: 'ak-test',
    },
  }
}

function makeGeneratedProfile(id: string) {
  return {
    id,
    name: '对象存储',
    provider: 's3-compatible',
    enabled: true,
    is_current: false,
    builtin: false,
    secret_configured: false,
    config: {
      bucket_name: 'images',
    },
  }
}
