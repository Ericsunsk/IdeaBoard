import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type { AdminWorkflowFeatureResource, WorkflowFeature } from './promptConfigResource'
import { createAdminWorkflowFeatureWorkspace } from './promptConfigWorkspace'

const featureSlug = 'sketch-to-image'
const featureTitle = '草图成图'

const mockResource = {
  fetchWorkflowFeatures: vi.fn(),
  saveWorkflowFeature: vi.fn(),
}

describe('workflow feature workspace facade', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockResource.fetchWorkflowFeatures.mockResolvedValue([
      makeWorkflowFeature({ system_prompt: 'Initial prompt' }),
    ])
    mockResource.saveWorkflowFeature.mockResolvedValue(
      makeWorkflowFeature({ system_prompt: 'Saved prompt' }),
    )
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads the complete current workflow feature configuration', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadWorkflowFeatures()

    expect(mockResource.fetchWorkflowFeatures).toHaveBeenCalledTimes(1)
    expect(workspace.getDraft(featureSlug)).toEqual({
      canonicalModel: 'image-alpha',
      systemPrompt: 'Initial prompt',
      enabled: true,
      variables: [{ key: '{{prompt}}', tone: 'prompt' }],
      userPromptMergePolicy: {
        append_base_prompt_without_template: true,
        append_user_prompt_when_template_misses_placeholder: true,
      },
    })
    expect(workspace.getWorkflowFeatureVariables(featureSlug)).toEqual([
      { key: '{{prompt}}', tone: 'prompt' },
    ])
    expect(workspace.getDraftValidation(featureSlug)).toEqual({
      canSave: true,
      isDirty: false,
      message: '无未保存更改',
      severity: 'neutral',
    })
    expect(workspace.loadFailed.value).toBe(false)
  })

  it('exposes a recoverable initial load failure state', async () => {
    const { workspace, feedback } = createWorkspace()
    const error = new Error('load failed')
    mockResource.fetchWorkflowFeatures.mockRejectedValueOnce(error)

    await workspace.loadWorkflowFeatures()

    expect(workspace.loadFailed.value).toBe(true)
    expect(feedback.loadWorkflowFeaturesFailed).toHaveBeenCalledWith(error)

    mockResource.fetchWorkflowFeatures.mockResolvedValueOnce([makeWorkflowFeature()])
    await workspace.loadWorkflowFeatures()

    expect(workspace.loadFailed.value).toBe(false)
    expect(workspace.workflowFeatures.value).toHaveLength(1)
  })

  it('validates Canonical Model and allows an empty System Prompt', async () => {
    const { workspace, feedback } = createWorkspace()

    await workspace.loadWorkflowFeatures()
    workspace.updateCanonicalModelDraft(featureSlug, '   ')

    expect(workspace.getDraftValidation(featureSlug)).toEqual({
      canSave: false,
      isDirty: true,
      message: '请输入 Canonical Model',
      severity: 'error',
    })

    workspace.updateCanonicalModelDraft(featureSlug, 'image-alpha')
    workspace.updateSystemPromptDraft(featureSlug, '')

    expect(workspace.getDraftValidation(featureSlug)).toEqual({
      canSave: true,
      isDirty: true,
      message: 'System Prompt 为空，执行时将不注入系统提示词',
      severity: 'warning',
    })

    await workspace.saveWorkflowFeature('missing-slug')

    expect(feedback.workflowFeatureDraftInvalid).toHaveBeenCalledWith(
      '工作流配置已刷新，请重新选择',
    )
    expect(mockResource.saveWorkflowFeature).not.toHaveBeenCalled()
  })

  it('saves canonical model, prompt, enabled state, variables, and merge policy together', async () => {
    const { workspace, feedback } = createWorkspace()
    mockResource.fetchWorkflowFeatures
      .mockResolvedValueOnce([makeWorkflowFeature({ system_prompt: 'Initial prompt' })])
      .mockResolvedValueOnce([
        makeWorkflowFeature({
          canonical_model: 'image-beta',
          system_prompt: 'Saved prompt',
          enabled: false,
          updated_at: '2026-01-02T00:00:00Z',
        }),
      ])
    mockResource.saveWorkflowFeature.mockResolvedValueOnce(
      makeWorkflowFeature({
        canonical_model: 'image-beta',
        system_prompt: 'Saved prompt',
        enabled: false,
        updated_at: '2026-01-02T00:00:00Z',
      }),
    )

    await workspace.loadWorkflowFeatures()
    workspace.updateCanonicalModelDraft(featureSlug, ' image-beta ')
    workspace.updateSystemPromptDraft(featureSlug, 'Saved prompt')
    workspace.updateEnabledDraft(featureSlug, false)

    await workspace.saveWorkflowFeature(featureSlug)

    expect(mockResource.saveWorkflowFeature).toHaveBeenCalledWith({
      slug: featureSlug,
      canonical_model: 'image-beta',
      system_prompt: 'Saved prompt',
      enabled: false,
      variables: [{ key: '{{prompt}}', tone: 'prompt' }],
      user_prompt_merge_policy: {
        append_base_prompt_without_template: true,
        append_user_prompt_when_template_misses_placeholder: true,
      },
    })
    expect(feedback.workflowFeatureSaved).toHaveBeenCalledWith(featureTitle)
    expect(mockResource.fetchWorkflowFeatures).toHaveBeenCalledTimes(2)
    expect(workspace.getDraft(featureSlug).canonicalModel).toBe('image-beta')
    expect(workspace.getDraft(featureSlug).systemPrompt).toBe('Saved prompt')
    expect(workspace.getDraft(featureSlug).enabled).toBe(false)
  })

  it('keeps the complete local draft when save fails', async () => {
    const { workspace, feedback } = createWorkspace()
    const error = new Error('save failed')
    mockResource.saveWorkflowFeature.mockRejectedValueOnce(error)

    await workspace.loadWorkflowFeatures()
    workspace.updateCanonicalModelDraft(featureSlug, 'image-beta')
    workspace.updateSystemPromptDraft(featureSlug, 'Unsaved prompt')
    workspace.updateEnabledDraft(featureSlug, false)

    const result = await workspace.saveWorkflowFeature(featureSlug)

    expect(result).toBeNull()
    expect(feedback.saveWorkflowFeatureFailed).toHaveBeenCalledWith(error)
    expect(feedback.workflowFeatureSaved).not.toHaveBeenCalled()
    expect(mockResource.fetchWorkflowFeatures).toHaveBeenCalledTimes(1)
    expect(workspace.getDraft(featureSlug)).toMatchObject({
      canonicalModel: 'image-beta',
      systemPrompt: 'Unsaved prompt',
      enabled: false,
    })
    expect(workspace.saving[featureSlug]).toBe(false)
  })

  it('preserves a dirty complete draft across refresh and resets explicitly', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadWorkflowFeatures()
    workspace.updateCanonicalModelDraft(featureSlug, 'local-model')
    workspace.updateSystemPromptDraft(featureSlug, 'Dirty local prompt')
    mockResource.fetchWorkflowFeatures.mockResolvedValueOnce([
      makeWorkflowFeature({
        canonical_model: 'server-model',
        system_prompt: 'Server refreshed prompt',
        updated_at: '2026-01-03T00:00:00Z',
      }),
    ])

    await workspace.refreshWorkflowFeatures()

    expect(workspace.getDraft(featureSlug).canonicalModel).toBe('local-model')
    expect(workspace.getDraft(featureSlug).systemPrompt).toBe('Dirty local prompt')
    expect(workspace.getSavedWorkflowFeature(featureSlug)?.canonical_model).toBe('server-model')

    workspace.resetDrafts([featureSlug])

    expect(workspace.getDraft(featureSlug).canonicalModel).toBe('server-model')
    expect(workspace.getDraft(featureSlug).systemPrompt).toBe('Server refreshed prompt')
  })

  it('inserts variables into the System Prompt draft', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadWorkflowFeatures()
    workspace.updateSystemPromptDraft(featureSlug, 'tail')
    workspace.insertVariableIntoDraft(featureSlug, '{{prompt}}', 0)

    expect(workspace.getDraft(featureSlug).systemPrompt).toBe('{{prompt}}tail')
  })
})

function createWorkspace() {
  const feedback = createWorkflowFeatureFeedback()
  const { runAsyncAction } = useAsyncAction()
  const workspace = createAdminWorkflowFeatureWorkspace({
    feedback,
    getFeatureTitle: () => featureTitle,
    runAsyncAction,
    resource: mockResource as unknown as AdminWorkflowFeatureResource,
  })
  return { workspace, feedback }
}

function createWorkflowFeatureFeedback() {
  return {
    loadWorkflowFeaturesFailed: vi.fn(),
    workflowFeatureDraftInvalid: vi.fn(),
    workflowFeatureSaved: vi.fn(),
    saveWorkflowFeatureFailed: vi.fn(),
  }
}

function makeWorkflowFeature(overrides: Partial<WorkflowFeature> = {}): WorkflowFeature {
  return {
    feature_slug: featureSlug,
    canonical_model: 'image-alpha',
    system_prompt: '',
    variables: [{ key: '{{prompt}}', tone: 'prompt' }],
    user_prompt_merge_policy: {
      append_base_prompt_without_template: true,
      append_user_prompt_when_template_misses_placeholder: true,
    },
    enabled: true,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}
