import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { renderWithUser, waitFor } from '../../../../test/render'
import {
  useAdminModelCatalogWorkspace,
} from './useAdminModelCatalogWorkspace'
import type { AdminGatewayCatalog } from '../infrastructure/adminGatewayResource'
import type { AdminModelItem } from '../domain/catalogData'

const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
}))

vi.mock('../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/toast')>()) as object,
  useAdminToast: () => toast,
}))

function model(overrides: Partial<AdminModelItem> & { id: string }): AdminModelItem {
  return {
    name: overrides.id,
    provider: 'OpenAI',
    mode: 'chat',
    description: '',
    contextOrResolution: '',
    capabilities: [],
    hosting: '第三方',
    termsUrl: '',
    docsUrl: '',
    ...overrides,
  }
}

function mountWorkspace(catalog: AdminGatewayCatalog) {
  let workspace!: ReturnType<typeof useAdminModelCatalogWorkspace>
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  renderWithUser(
    defineComponent({
      setup() {
        workspace = useAdminModelCatalogWorkspace(catalog)
        return () => null
      },
    }),
    { global: { plugins: [[VueQueryPlugin, { queryClient }]] } },
  )
  return {
    get workspace() {
      return workspace
    },
  }
}

describe('useAdminModelCatalogWorkspace refresh enrichment', () => {
  beforeEach(() => {
    toast.success.mockReset()
    toast.error.mockReset()
  })

  function fakeCatalog(
    stored: AdminModelItem[],
    detected: Array<{ id: string; description?: string; context_or_resolution?: string }>,
    saved: { items: AdminModelItem[][] } = { items: [] },
  ): AdminGatewayCatalog {
    return {
      list: async () => stored.map((m) => ({ ...m })),
      save: async (items: AdminModelItem[]) => {
        saved.items.push(items)
        stored.splice(0, stored.length, ...items.map((m) => ({ ...m })))
      },
      previewRoute: async () => {
        throw new Error('not needed')
      },
      listProviders: async () => [],
      detect: async () => ({
        models: detected.map((d) => ({
          id: d.id,
          name: d.id,
          provider: 'OpenAI',
          // exactOptionalPropertyTypes：显式 undefined 不可写入可选属性，按有无展开。
          ...(d.description !== undefined ? { description: d.description } : {}),
          ...(d.context_or_resolution !== undefined
            ? { context_or_resolution: d.context_or_resolution }
            : {}),
        })),
        configured_providers: 1,
      }),
    }
  }

  it('overwrites bit-marked import text with fresher detection', async () => {
    const saved = { items: [] as AdminModelItem[][] }
    const stored = [
      model({ id: 'openai/gpt-4o', description: '旧版官方简介', descriptionEnriched: true }),
    ]
    const { workspace } = mountWorkspace(
      fakeCatalog(stored, [
        { id: 'openai/gpt-4o', description: '实时官方简介', context_or_resolution: '128K' },
      ], saved),
    )
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))

    await workspace.handleRefresh()

    await waitFor(() => expect(saved.items).toHaveLength(1))
    expect(saved.items[0]![0]!.description).toBe('实时官方简介')
    expect(saved.items[0]![0]!.contextOrResolution).toBe('128K')
    expect(saved.items[0]![0]!.descriptionEnriched).toBe(true)
  })

  it('never touches operator-written descriptions', async () => {
    const saved = { items: [] as AdminModelItem[][] }
    const stored = [
      model({ id: 'openai/gpt-4o', description: '运营手写简介', descriptionEnriched: false }),
    ]
    const { workspace } = mountWorkspace(
      fakeCatalog(stored, [{ id: 'openai/gpt-4o', description: '实时官方简介' }], saved),
    )
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))

    await workspace.handleRefresh()

    expect(saved.items).toHaveLength(0)
    expect(toast.success).toHaveBeenCalledWith('模型库已更新至最新')
  })

  it('fills empty descriptions regardless of the bit', async () => {
    const saved = { items: [] as AdminModelItem[][] }
    const stored = [model({ id: 'openai/gpt-4o', description: '', descriptionEnriched: false })]
    const { workspace } = mountWorkspace(
      fakeCatalog(stored, [{ id: 'openai/gpt-4o', description: '官方简介' }], saved),
    )
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))

    await workspace.handleRefresh()

    await waitFor(() => expect(saved.items).toHaveLength(1))
    expect(saved.items[0]![0]!.description).toBe('官方简介')
  })

  it('leaves bit-less operator text alone', async () => {
    // 无位非空视为运营文本：占位文案传统已删除，不再有例外分支。
    const saved = { items: [] as AdminModelItem[][] }
    const stored = [model({ id: 'openai/gpt-4o', description: '运营手写简介' })]
    const { workspace } = mountWorkspace(
      fakeCatalog(stored, [{ id: 'openai/gpt-4o', description: '官方简介' }], saved),
    )
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))

    await workspace.handleRefresh()

    expect(saved.items).toHaveLength(0)
  })
})
