import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminPromptConfigResource, FeaturePrompt } from './promptConfigResource'
import { createAdminPromptConfigWorkspace } from './promptConfigWorkspace'

const promptSlug = 'sketch-to-image'
const promptTitle = '草图成图'

const mockResource = {
  fetchPrompts: vi.fn(),
  savePrompt: vi.fn(),
}

describe('prompt config workspace facade', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockResource.fetchPrompts.mockResolvedValue([makePrompt({ prompt: 'Initial prompt' })])
    mockResource.savePrompt.mockResolvedValue(makePrompt({ prompt: 'Saved prompt' }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads prompt drafts, variables, and save validation state', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadPrompts()

    expect(mockResource.fetchPrompts).toHaveBeenCalledTimes(1)
    expect(workspace.promptMap[promptSlug]).toBe('Initial prompt')
    expect(workspace.enabledMap[promptSlug]).toBe(true)
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Initial prompt')
    expect(workspace.getPromptVariables(promptSlug)).toEqual([
      { key: '{{prompt}}', tone: 'prompt' },
    ])
    expect(workspace.getDraftValidation(promptSlug)).toEqual({
      canSave: true,
      isDirty: false,
      message: '无未保存更改',
      severity: 'neutral',
    })
  })

  it('edits draft state and exposes validation feedback', async () => {
    const { workspace, feedback } = createWorkspace()

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, '')
    workspace.updateEnabledDraft(promptSlug, false)

    expect(workspace.getDraftValidation(promptSlug)).toEqual({
      canSave: true,
      isDirty: true,
      message: '提示词为空，保存后将使用空模板',
      severity: 'warning',
    })

    workspace.insertVariableIntoDraft(promptSlug, '{{prompt}}', 0)

    expect(workspace.promptMap[promptSlug]).toBe('{{prompt}}')
    expect(workspace.getDraftValidation(promptSlug).message).toBe('可保存')

    await workspace.savePrompt('missing-slug')

    expect(feedback.promptDraftInvalid).toHaveBeenCalledWith('提示词配置已刷新，请重新选择')
    expect(mockResource.savePrompt).not.toHaveBeenCalled()
  })

  it('saves a prompt then refreshes through the workspace command sequence', async () => {
    const { workspace, feedback } = createWorkspace()
    mockResource.fetchPrompts
      .mockResolvedValueOnce([makePrompt({ prompt: 'Initial prompt' })])
      .mockResolvedValueOnce([
        makePrompt({
          enabled: false,
          prompt: 'Saved prompt',
          schema_version: 2,
          published_at: '2026-01-02T00:00:00Z',
          updated_at: '2026-01-02T00:00:00Z',
        }),
      ])
    mockResource.savePrompt.mockResolvedValueOnce(
      makePrompt({
        enabled: false,
        prompt: 'Saved prompt',
        schema_version: 2,
        published_at: '2026-01-02T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
      }),
    )

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, 'Saved prompt')
    workspace.updateEnabledDraft(promptSlug, false)

    await workspace.savePrompt(promptSlug)

    expect(mockResource.savePrompt).toHaveBeenCalledWith({
      slug: promptSlug,
      prompt: 'Saved prompt',
      enabled: false,
    })
    expect(feedback.promptSaved).toHaveBeenCalledWith(promptTitle)
    expect(mockResource.fetchPrompts).toHaveBeenCalledTimes(2)
    expect(workspace.promptMap[promptSlug]).toBe('Saved prompt')
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Saved prompt')
    expect(workspace.enabledMap[promptSlug]).toBe(false)
  })

  it('keeps the draft in place when save fails', async () => {
    const { workspace, feedback } = createWorkspace()
    const error = new Error('save failed')
    mockResource.savePrompt.mockRejectedValueOnce(error)

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, 'Unsaved prompt')

    const result = await workspace.savePrompt(promptSlug)

    expect(result).toBeNull()
    expect(feedback.savePromptFailed).toHaveBeenCalledWith(error)
    expect(feedback.promptSaved).not.toHaveBeenCalled()
    expect(mockResource.fetchPrompts).toHaveBeenCalledTimes(1)
    expect(workspace.promptMap[promptSlug]).toBe('Unsaved prompt')
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Initial prompt')
    expect(workspace.saving[promptSlug]).toBe(false)
  })

  it('refreshes saved state while preserving dirty drafts until reset', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, 'Dirty local draft')
    mockResource.fetchPrompts.mockResolvedValueOnce([
      makePrompt({
        enabled: false,
        prompt: 'Server refreshed prompt',
        updated_at: '2026-01-03T00:00:00Z',
      }),
    ])

    await workspace.refreshPrompts()

    expect(workspace.promptMap[promptSlug]).toBe('Dirty local draft')
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Server refreshed prompt')

    workspace.resetDrafts([promptSlug])

    expect(workspace.promptMap[promptSlug]).toBe('Server refreshed prompt')
    expect(workspace.enabledMap[promptSlug]).toBe(false)
  })
})

function createWorkspace() {
  const feedback = createPromptConfigFeedback()
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminPromptConfigWorkspace({
    feedback,
    getFeatureTitle: () => promptTitle,
    runAsyncAction,
    resource: mockResource as unknown as AdminPromptConfigResource,
  })
  return { workspace, feedback }
}

function createPromptConfigFeedback() {
  return {
    loadPromptsFailed: vi.fn(),
    promptDraftInvalid: vi.fn(),
    promptSaved: vi.fn(),
    savePromptFailed: vi.fn(),
  }
}

function makePrompt(overrides: Partial<FeaturePrompt> = {}): FeaturePrompt {
  return {
    slug: promptSlug,
    prompt: '',
    enabled: true,
    variables: [{ key: '{{prompt}}', tone: 'prompt' }],
    model_defaults: {},
    user_prompt_merge_policy: {
      append_base_prompt_without_template: true,
      append_user_prompt_when_template_misses_placeholder: true,
    },
    schema_version: 1,
    current_revision_id: 1,
    published_at: null,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}
