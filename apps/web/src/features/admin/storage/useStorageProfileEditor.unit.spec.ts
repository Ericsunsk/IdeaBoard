import { computed, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mockAppToast } from '../../../test/toast'
import { useStorageProfileEditor } from './useStorageProfileEditor'
import {
  normalizeProfile,
  toProviderMetaMap,
  type StorageCard,
  type StorageProfilePayload,
  type StorageProviderMeta,
} from './profiles'

vi.mock('../../../core/composables/useAppToast')

vi.mock('../../../core/composables/useAsyncAction', () => ({
  useAsyncAction: () => ({
    runAsyncAction: async (
      action: () => unknown,
      options: { loading?: { value: boolean } } = {},
    ) => {
      if (options.loading) options.loading.value = true
      try {
        return await action()
      } finally {
        if (options.loading) options.loading.value = false
      }
    },
  }),
}))

describe('useStorageProfileEditor', () => {
  beforeEach(() => {
    mockAppToast()
  })

  it('applies vendor presets and clears saved secret fields from the editor form', async () => {
    const providers = ref([makeS3Provider()])
    const providerMap = computed(() => toProviderMetaMap(providers.value))
    const storages = ref<StorageCard[]>([])
    const saveProfile = vi.fn(async (input: { payload: StorageProfilePayload }) => ({
      message: 'saved',
      profile: {
        id: 'profile-1',
        name: input.payload.name,
        provider: input.payload.provider,
        enabled: input.payload.enabled,
        is_current: false,
        builtin: false,
        secret_configured: true,
        config: {
          ...input.payload.config,
          secret_access_key: 'server-secret',
        },
      },
    }))

    const editor = useStorageProfileEditor({
      creatableProviders: computed(() =>
        providers.value.filter((provider) => provider.create_allowed),
      ),
      providerMap,
      storages,
      saveProfile,
      fetchBucketDomains: vi.fn(),
    })

    editor.openCreate()
    editor.handleVendorSelect('minio')
    editor.updateConfigValue('bucket_name', 'images')
    editor.updateConfigValue('access_key_id', 'key')
    editor.updateConfigValue('secret_access_key', 'secret')
    await editor.saveDraft()

    expect(saveProfile).toHaveBeenCalledWith({
      id: undefined,
      payload: expect.objectContaining({
        name: 'MinIO 存储',
        provider: 's3-compatible',
        enabled: false,
        config: expect.objectContaining({
          bucket_name: 'images',
          endpoint: '127.0.0.1:9000',
          secret_access_key: 'secret',
        }),
      }),
    })
    expect(editor.form.value.secretConfigured).toBe(true)
    expect(editor.form.value.config.secret_access_key).toBe('')
  })

  it('exposes no workspace-owned operation commands or pending refs', () => {
    const providers = ref([makeS3Provider()])
    const providerMap = computed(() => toProviderMetaMap(providers.value))

    const editor = useStorageProfileEditor({
      creatableProviders: computed(() => providers.value),
      providerMap,
      storages: ref<StorageCard[]>([]),
      saveProfile: vi.fn(),
      fetchBucketDomains: vi.fn(),
    })

    expect(editor).not.toHaveProperty('saveAndApply')
    expect(editor).not.toHaveProperty('applyStorage')
    expect(editor).not.toHaveProperty('handleTest')
    expect(editor).not.toHaveProperty('removeProfile')
    expect(editor).not.toHaveProperty('applyingId')
    expect(editor).not.toHaveProperty('testingId')
    expect(editor).not.toHaveProperty('savingAndApplying')
  })

  it('omits an existing secret value when saving an edited profile with the secret field left blank', async () => {
    const providers = ref([makeS3Provider()])
    const providerMap = computed(() => toProviderMetaMap(providers.value))
    const savedProfile = normalizeProfile(
      {
        id: 'profile-1',
        name: '对象存储',
        provider: 's3-compatible',
        enabled: true,
        is_current: false,
        builtin: false,
        secret_configured: true,
        config: {
          bucket_name: 'images',
          endpoint: 'oss-cn-shanghai.aliyuncs.com',
          region_id: 'cn-shanghai',
          access_key_id: 'key',
          secret_access_key: 'server-secret',
          force_path_style: false,
        },
      },
      providerMap.value,
    )
    const saveProfile = vi.fn(
      async (input: { payload: StorageProfilePayload; id?: string | undefined }) => ({
        message: 'saved',
        profile: {
          id: input.id || savedProfile.id,
          name: input.payload.name,
          provider: input.payload.provider,
          enabled: input.payload.enabled,
          is_current: false,
          builtin: false,
          secret_configured: true,
          config: {
            ...input.payload.config,
            secret_access_key: 'server-secret',
          },
        },
      }),
    )

    const editor = useStorageProfileEditor({
      creatableProviders: computed(() =>
        providers.value.filter((provider) => provider.create_allowed),
      ),
      providerMap,
      storages: ref([savedProfile]),
      saveProfile,
      fetchBucketDomains: vi.fn(),
    })

    editor.openEdit(savedProfile.id)
    editor.updateConfigValue('secret_access_key', '')
    await editor.saveDraft()

    expect(saveProfile).toHaveBeenCalledWith({
      id: 'profile-1',
      payload: expect.objectContaining({
        config: expect.not.objectContaining({
          secret_access_key: expect.anything(),
        }),
      }),
    })
  })

  it('persists the current form before fetching bucket domains', async () => {
    const providers = ref([makeS3Provider()])
    const providerMap = computed(() => toProviderMetaMap(providers.value))
    const savedProfile = normalizeProfile(
      {
        id: 'profile-2',
        name: '对象存储',
        provider: 's3-compatible',
        enabled: true,
        is_current: false,
        builtin: false,
        secret_configured: true,
        config: {
          bucket_name: 'images',
          endpoint: 'oss-cn-shanghai.aliyuncs.com',
          region_id: 'cn-shanghai',
          access_key_id: 'key',
          secret_access_key: '',
          force_path_style: false,
        },
      },
      providerMap.value,
    )
    const saveProfile = vi.fn(async () => ({
      message: 'saved',
      profile: {
        id: savedProfile.id,
        name: savedProfile.name,
        provider: savedProfile.provider,
        enabled: savedProfile.enabled,
        is_current: savedProfile.isCurrent,
        builtin: savedProfile.builtin,
        secret_configured: savedProfile.secretConfigured,
        config: savedProfile.config,
      },
    }))
    const fetchBucketDomains = vi.fn(async () => ['cdn.example.com'])

    const editor = useStorageProfileEditor({
      creatableProviders: computed(() => providers.value),
      providerMap,
      storages: ref([savedProfile]),
      saveProfile,
      fetchBucketDomains,
    })

    editor.openEdit(savedProfile.id)
    await editor.fetchBucketDomains()

    expect(saveProfile).toHaveBeenCalledTimes(1)
    expect(fetchBucketDomains).toHaveBeenCalledWith('profile-2')
    expect(editor.bucketDomains.value).toEqual(['cdn.example.com'])
  })
})

function makeS3Provider(): StorageProviderMeta {
  return {
    provider: 's3-compatible',
    label: 'S3 兼容',
    description: '',
    builtin: false,
    create_allowed: true,
    supports_test: true,
    supports_bucket_info: true,
    fields: [
      { key: 'bucket_name', label: 'Bucket', type: 'text', required: true },
      { key: 'endpoint', label: 'Endpoint', type: 'text', required: true },
      { key: 'region_id', label: '地域', type: 'text' },
      { key: 'access_key_id', label: 'Access Key ID', type: 'text', required: true },
      {
        key: 'secret_access_key',
        label: 'Access Key Secret',
        type: 'password',
        required: true,
        secret: true,
      },
      { key: 'force_path_style', label: 'Path Style', type: 'switch' },
    ],
  }
}
