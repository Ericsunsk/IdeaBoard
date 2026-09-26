import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { createDeferred } from '../../../test/deferred'
import type { AdminPromptConfigResource, FeaturePrompt } from './resource'
import { createAdminPromptConfigWorkspace } from './workspace'

const featureSlug = 'variations'
const featureTitle = '风格延展'

const mockResource = {
  fetchPrompts: vi.fn(),
  savePrompt: vi.fn(),
}

describe('prompt config workspace facade', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockResource.fetchPrompts.mockResolvedValue([
      makeFeaturePrompt({ prompt: 'Initial {{variation_strength}}' }),
    ])
    mockResource.savePrompt.mockResolvedValue(makeFeaturePrompt({ prompt: 'Saved prompt' }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads the current prompt while variables remain owned by the workflow registry', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadPrompts()

    expect(workspace.promptMap[featureSlug]).toBe('Initial {{variation_strength}}')
    expect(workspace.getPromptVariables(featureSlug)).toEqual([
      { key: '{{user_prompt}}', tone: 'prompt' },
      { key: '{{variation_strength}}', tone: 'control' },
    ])
    expect(workspace.getDraftValidation(featureSlug)).toEqual({
      canSave: true,
      isDirty: false,
      message: '无未保存更改',
      severity: 'neutral',
    })
  })

  it('saves only the current prompt and allows an empty prompt', async () => {
    const { workspace, feedback } = createWorkspace()
    mockResource.fetchPrompts
      .mockResolvedValueOnce([makeFeaturePrompt({ prompt: 'Initial prompt' })])
      .mockResolvedValueOnce([makeFeaturePrompt({ prompt: '' })])
    mockResource.savePrompt.mockResolvedValueOnce(makeFeaturePrompt({ prompt: '' }))

    await workspace.loadPrompts()
    workspace.updatePromptDraft(featureSlug, '')

    expect(workspace.getDraftValidation(featureSlug)).toMatchObject({
      canSave: true,
      isDirty: true,
      severity: 'warning',
    })

    await workspace.savePrompt(featureSlug)

    expect(mockResource.savePrompt).toHaveBeenCalledWith(
      {
        slug: featureSlug,
        prompt: '',
      },
      { operatorReason: 'prompt review' },
    )
    expect(feedback.promptSaved).toHaveBeenCalledWith(featureTitle)
    expect(workspace.promptMap[featureSlug]).toBe('')
  })

  it('preserves a dirty prompt across refresh and resets explicitly', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadPrompts()
    workspace.updatePromptDraft(featureSlug, 'Dirty local prompt')
    mockResource.fetchPrompts.mockResolvedValueOnce([
      makeFeaturePrompt({ prompt: 'Server refreshed prompt' }),
    ])

    await workspace.refreshPrompts()

    expect(workspace.promptMap[featureSlug]).toBe('Dirty local prompt')
    expect(workspace.getSavedPrompt(featureSlug)).toBe('Server refreshed prompt')

    workspace.resetDrafts([featureSlug])

    expect(workspace.promptMap[featureSlug]).toBe('Server refreshed prompt')
  })

  it('keeps the local prompt when saving fails', async () => {
    const { workspace, feedback } = createWorkspace()
    const error = new Error('save failed')
    mockResource.savePrompt.mockRejectedValueOnce(error)

    await workspace.loadPrompts()
    workspace.updatePromptDraft(featureSlug, 'Unsaved prompt')

    const result = await workspace.savePrompt(featureSlug)

    expect(result).toBeNull()
    expect(feedback.savePromptFailed).toHaveBeenCalledWith(error)
    expect(workspace.promptMap[featureSlug]).toBe('Unsaved prompt')
    expect(workspace.saving[featureSlug]).toBe(false)
  })

  it('blocks duplicate prompt saves and settles saving state once', async () => {
    const saveRequest = createDeferred<FeaturePrompt>()
    const { workspace, feedback } = createWorkspace()

    await workspace.loadPrompts()
    workspace.updatePromptDraft(featureSlug, 'Committed prompt')
    mockResource.savePrompt.mockReturnValueOnce(saveRequest.promise)

    const firstSave = workspace.savePrompt(featureSlug)
    const duplicateSave = workspace.savePrompt(featureSlug)

    // Operator-reason collection yields a microtask before the transport call.
    await vi.waitFor(() => expect(workspace.saving[featureSlug]).toBe(true))
    await vi.waitFor(() => expect(mockResource.savePrompt).toHaveBeenCalledOnce())
    await expect(duplicateSave).resolves.toBeNull()

    saveRequest.resolve(makeFeaturePrompt({ prompt: 'Committed prompt' }))

    await expect(firstSave).resolves.toMatchObject({ prompt: 'Committed prompt' })
    expect(workspace.saving[featureSlug]).toBe(false)
    expect(feedback.promptSaved).toHaveBeenCalledOnce()
  })

  it('inserts a code-owned variable at the cursor position', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadPrompts()
    workspace.updatePromptDraft(featureSlug, 'tail')
    workspace.insertVariableIntoDraft(featureSlug, '{{variation_strength}}', 0)

    expect(workspace.promptMap[featureSlug]).toBe('{{variation_strength}}tail')
  })
})

function createWorkspace() {
  const feedback = {
    loadPromptsFailed: vi.fn(),
    promptDraftInvalid: vi.fn(),
    promptSaved: vi.fn(),
    savePromptFailed: vi.fn(),
  }
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminPromptConfigWorkspace({
    feedback,
    confirm: {
      confirm: vi.fn().mockResolvedValue(true),
      readReason: () => 'prompt review',
    },
    getFeatureTitle: () => featureTitle,
    runAsyncAction,
    resource: mockResource as unknown as AdminPromptConfigResource,
  })
  return { workspace, feedback }
}

function makeFeaturePrompt(overrides: Partial<FeaturePrompt> = {}): FeaturePrompt {
  return {
    feature_slug: featureSlug,
    prompt: '',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}
