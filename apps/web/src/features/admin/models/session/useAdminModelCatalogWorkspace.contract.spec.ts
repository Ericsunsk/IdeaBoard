import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { renderWithUser, waitFor } from '@/test/render'
import { useAdminModelCatalogWorkspace } from './useAdminModelCatalogWorkspace'
import type { AdminGatewayCatalog } from '../resource'
import type { AdminModelItem } from '../core/catalogData'

const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('@/core/composables/useConfirmDialog', () => ({
  useConfirm: () => ({
    confirm: vi.fn().mockResolvedValue(true),
  }),
}))

vi.mock('../../shared/session/feedback/toast', async (importOriginal) => ({
  ...((await importOriginal<typeof import('../../shared/session/feedback/toast')>()) as object),
  useAdminToast: () => toast,
}))

const clipboardCopyText = vi.hoisted(() => ({ current: vi.fn() }))

vi.mock('../../../../core/composables/useAppClipboard', () => ({
  useAppClipboard: () => ({ copyText: clipboardCopyText.current }),
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

function fakeCatalog(
  stored: AdminModelItem[],
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
    listObserved: async () => [],
  }
}

describe('useAdminModelCatalogWorkspace refresh', () => {
  beforeEach(() => {
    toast.success.mockReset()
    toast.error.mockReset()
  })

  it('reloads the catalog list without local write-back', async () => {
    const saved = { items: [] as AdminModelItem[][] }
    const stored = [model({ id: 'gpt-4o', description: '运营手写简介' })]
    const catalog = fakeCatalog(stored, saved)
    const list = vi.spyOn(catalog, 'list')
    const { workspace } = mountWorkspace(catalog)
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))
    list.mockClear()
    stored[0] = model({ id: 'gpt-4o', description: '更新的运营简介' })

    await workspace.handleRefresh()

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('模型库已更新至最新'))
    expect(list).toHaveBeenCalledTimes(1)
    expect(workspace.allModels.value[0]?.description).toBe('更新的运营简介')
    expect(saved.items).toHaveLength(0)
  })

  it('keeps the cached catalog and reports refresh failure without success feedback', async () => {
    const catalog = fakeCatalog([model({ id: 'gpt-4o' })])
    const list = vi.spyOn(catalog, 'list')
    const { workspace } = mountWorkspace(catalog)
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))
    list.mockRejectedValueOnce(new Error('catalog refresh failed'))

    await workspace.handleRefresh()

    expect(workspace.allModels.value[0]?.id).toBe('gpt-4o')
    expect(workspace.isListStale.value).toBe(true)
    expect(toast.error).toHaveBeenCalledWith('catalog refresh failed')
    expect(toast.success).not.toHaveBeenCalled()
    expect(workspace.isRefreshing.value).toBe(false)
  })

  it('toasts a failed initial catalog load while keeping the inline anchor state', async () => {
    const { workspace } = mountWorkspace({
      ...fakeCatalog([]),
      list: async () => {
        throw new Error('catalog down')
      },
    })

    await waitFor(() => expect(workspace.isListError.value).toBe(true))
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('catalog down'))
  })
})

describe('useAdminModelCatalogWorkspace copy feedback contract', () => {
  beforeEach(() => {
    toast.success.mockReset()
    toast.error.mockReset()
    clipboardCopyText.current = vi.fn().mockResolvedValue(true)
  })

  function mountForCopy() {
    const stored = [model({ id: 'openai/gpt-4o' })]
    const catalog: AdminGatewayCatalog = {
      list: async () => stored.map((m) => ({ ...m })),
      save: async () => {},
      previewRoute: async () => {
        throw new Error('not needed')
      },
      listProviders: async () => [],
      listObserved: async () => [],
    }
    return mountWorkspace(catalog)
  }

  it('marks the id copied on clipboard success without error feedback', async () => {
    const { workspace } = mountForCopy()

    workspace.copyModelId('openai/gpt-4o')

    await waitFor(() => expect(workspace.copiedModelId.value).toBe('openai/gpt-4o'))
    expect(clipboardCopyText.current).toHaveBeenCalledWith('openai/gpt-4o')
    expect(toast.error).not.toHaveBeenCalled()
  })

  it('routes clipboard failure to copyFailed feedback and keeps copied state empty', async () => {
    clipboardCopyText.current = vi.fn().mockResolvedValue(false)
    const { workspace } = mountForCopy()

    workspace.copyModelId('openai/gpt-4o')

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('复制失败，请手动复制模型标识'))
    expect(workspace.copiedModelId.value).toBeNull()
  })
})

describe('provider/model catalog identity', () => {
  it('creates same-name models for different providers but rejects a duplicate pair', async () => {
    const openai = model({ provider: 'openai', id: 'gpt-image-1', name: 'OpenAI' })
    const azure = model({ provider: 'azureopenai', id: 'gpt-image-1', name: 'Azure' })
    const saved = { items: [] as AdminModelItem[][] }
    const { workspace } = mountWorkspace(fakeCatalog([openai], saved))
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(1))
    workspace.handleCreateManualModel()
    await workspace.handleSaveModel(azure)
    expect(saved.items[0]).toEqual([azure, openai])
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(2))
    workspace.handleCreateManualModel()
    await workspace.handleSaveModel({ ...azure, provider: ' AzureOpenAI ', id: 'GPT-IMAGE-1' })
    expect(saved.items).toHaveLength(1)
  })

  it('edits or moves exactly the original pair and prevents a conflicting provider change', async () => {
    const openai = model({ provider: 'openai', id: 'gpt-image-1', name: 'OpenAI' })
    const azure = model({ provider: 'azureopenai', id: 'gpt-image-1', name: 'Azure' })
    const saved = { items: [] as AdminModelItem[][] }
    const { workspace } = mountWorkspace(fakeCatalog([openai, azure], saved))
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(2))
    workspace.handleEditModel(azure)
    const moved = { ...azure, provider: 'custom', name: 'Custom' }
    await workspace.handleSaveModel(moved)
    expect(saved.items[0]).toEqual([openai, moved])
    await waitFor(() => expect(workspace.allModels.value[1]?.provider).toBe('custom'))
    workspace.handleEditModel(moved)
    await workspace.handleSaveModel({ ...moved, provider: 'openai' })
    expect(saved.items).toHaveLength(1)
    expect(workspace.editDialogVisible.value).toBe(true)
  })

  it('deletes only the selected provider/model pair', async () => {
    const openai = model({ provider: 'openai', id: 'gpt-image-1', name: 'OpenAI' })
    const azure = model({ provider: 'azureopenai', id: 'gpt-image-1', name: 'Azure' })
    const saved = { items: [] as AdminModelItem[][] }
    const { workspace } = mountWorkspace(fakeCatalog([openai, azure], saved))
    await waitFor(() => expect(workspace.allModels.value).toHaveLength(2))
    await workspace.handleDeleteModel(azure)
    expect(saved.items[0]).toEqual([openai])
  })
})
