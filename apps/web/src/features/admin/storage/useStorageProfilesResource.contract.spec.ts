import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { waitFor } from '@testing-library/vue'
import { createApp, defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { useStorageProfilesResource } from './useStorageProfilesResource'

type StorageProfilesResource = ReturnType<typeof useStorageProfilesResource>

type StorageRequestLog = {
  method: string
  path: string
  search: string
  body?: unknown
}

type StorageQueryCounts = {
  providers: number
  profiles: number
}

describe('useStorageProfilesResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses the HTTP boundary for storage queries and mutations', async () => {
    const receivedRequests: StorageRequestLog[] = []
    useMswHandlers(
      ...storageQueryHandlers(),
      ...storageMutationHandlers(receivedRequests),
    )
    const harness = mountStorageProfilesResource()
    const invalidateProfilesSpy = vi.spyOn(harness.queryClient, 'invalidateQueries')

    try {
      await waitForStorageQueries(harness.resource)

      expect(harness.resource.providers.value).toEqual([makeS3Provider()])
      expect(harness.resource.storages.value).toMatchObject([
        {
          id: 'profile-listed',
          name: '对象存储',
          provider: 's3-compatible',
          secretConfigured: false,
          config: {
            bucket_name: 'images',
          },
        },
      ])

      const payload = makeStoragePayload()
      await expect(harness.resource.saveProfile({ payload })).resolves.toMatchObject({
        message: 'created',
        profile: {
          id: 'profile-created',
        },
      })
      await expect(
        harness.resource.saveProfile({ id: 'profile-1', payload, apply: true }),
      ).resolves.toMatchObject({
        message: 'updated',
        profile: {
          id: 'profile-updated',
        },
      })
      await expect(harness.resource.applyProfile('profile-1')).resolves.toMatchObject({
        message: 'applied',
        profile: {
          id: 'profile-applied',
        },
      })
      await expect(harness.resource.fetchBucketDomains('profile-1')).resolves.toEqual([
        'cdn.example.com',
        'assets.example.com',
      ])
      await expect(harness.resource.deleteProfile('profile-1')).resolves.toEqual({
        message: 'deleted',
      })

      expect(invalidateProfilesSpy).toHaveBeenCalledTimes(4)
      expect(invalidateProfilesSpy).toHaveBeenNthCalledWith(1, {
        queryKey: ['admin', 'storage', 'profiles'],
      })
      expect(invalidateProfilesSpy).toHaveBeenNthCalledWith(2, {
        queryKey: ['admin', 'storage', 'profiles'],
      })
      expect(invalidateProfilesSpy).toHaveBeenNthCalledWith(3, {
        queryKey: ['admin', 'storage', 'profiles'],
      })
      expect(invalidateProfilesSpy).toHaveBeenNthCalledWith(4, {
        queryKey: ['admin', 'storage', 'profiles'],
      })
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
    } finally {
      await harness.unmount()
    }
  })

  it('surfaces storage API failures from the HTTP response body', async () => {
    useMswHandlers(
      ...storageQueryHandlers(),
      http.post('/api/admin/storage/profiles', () => {
        return HttpResponse.json({ code: 409, msg: '存储名称已存在' })
      }),
    )
    const harness = mountStorageProfilesResource()
    const invalidateProfilesSpy = vi.spyOn(harness.queryClient, 'invalidateQueries')

    try {
      await waitForStorageQueries(harness.resource)

      await expect(
        harness.resource.saveProfile({ payload: makeStoragePayload() }),
      ).rejects.toMatchObject({
        code: 409,
        msg: '存储名称已存在',
        message: '存储名称已存在',
        status: 200,
      })
      expect(invalidateProfilesSpy).not.toHaveBeenCalled()
    } finally {
      await harness.unmount()
    }
  })

  it('refetches providers and profiles when loadData runs the existing refresh path', async () => {
    const queryCounts: StorageQueryCounts = { providers: 0, profiles: 0 }
    useMswHandlers(...storageQueryHandlers(queryCounts))
    const harness = mountStorageProfilesResource()

    try {
      await waitFor(() => {
        expect(queryCounts).toEqual({ providers: 1, profiles: 1 })
      })

      await harness.resource.loadData()

      expect(queryCounts).toEqual({ providers: 2, profiles: 2 })
    } finally {
      await harness.unmount()
    }
  })
})

function mountStorageProfilesResource() {
  let resource: StorageProfilesResource | null = null
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 60_000,
        gcTime: 0,
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
  })
  const host = document.createElement('div')
  const app = createApp(
    defineComponent({
      setup() {
        resource = useStorageProfilesResource()
        return () => null
      },
    }),
  )

  document.body.appendChild(host)
  app.use(VueQueryPlugin, { queryClient })
  app.mount(host)

  const mountedResource = requireMountedStorageResource(resource)

  return {
    resource: mountedResource,
    queryClient,
    async unmount() {
      app.unmount()
      await queryClient.cancelQueries()
      queryClient.clear()
      host.remove()
    },
  }
}

function requireMountedStorageResource(
  resource: StorageProfilesResource | null,
): StorageProfilesResource {
  if (!resource) throw new Error('expected storage profiles resource to mount')
  return resource
}

async function waitForStorageQueries(resource: StorageProfilesResource) {
  await waitFor(() => {
    expect(resource.loading.value).toBe(false)
    expect(resource.providers.value).toHaveLength(1)
    expect(resource.storages.value).toHaveLength(1)
  })
}

function storageQueryHandlers(queryCounts?: StorageQueryCounts) {
  return [
    http.get('/api/admin/storage/providers', () => {
      if (queryCounts) queryCounts.providers += 1

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          providers: [makeS3Provider()],
        },
      })
    }),
    http.get('/api/admin/storage/profiles', () => {
      if (queryCounts) queryCounts.profiles += 1

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

function recordRequest(
  receivedRequests: StorageRequestLog[],
  request: Request,
  body?: unknown,
) {
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

function makeStoragePayload() {
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
