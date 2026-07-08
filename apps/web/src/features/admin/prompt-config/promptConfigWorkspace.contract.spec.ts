import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import type {
  AdminPromptConfigResource,
  FeaturePrompt,
  FeaturePromptEvalReport,
  FeaturePromptRevision,
} from './promptConfigResource'
import { createAdminPromptConfigWorkspace } from './promptConfigWorkspace'

const promptSlug = 'sketch-to-image'
const evalPromptSlug = 'pattern-gen'
const promptTitle = '草图成图'

const mockResource = {
  evaluatePrompt: vi.fn(),
  fetchPrompts: vi.fn(),
  fetchLatestEvalReport: vi.fn(),
  publishPrompt: vi.fn(),
  rollbackPrompt: vi.fn(),
}

describe('prompt config workspace facade', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockResource.fetchPrompts.mockResolvedValue([makePrompt({ prompt: 'Initial prompt' })])
    mockResource.evaluatePrompt.mockResolvedValue(makeEvalReport({ passed: true }))
    mockResource.fetchLatestEvalReport.mockResolvedValue(null)
    mockResource.publishPrompt.mockResolvedValue(makePrompt({ prompt: 'Published prompt' }))
    mockResource.rollbackPrompt.mockResolvedValue(makePrompt({ prompt: 'Rollback prompt' }))
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('loads prompt drafts, variables, and current revision state', async () => {
    const { workspace } = createWorkspace()

    await workspace.loadPrompts()

    expect(mockResource.fetchPrompts).toHaveBeenCalledTimes(1)
    expect(workspace.promptMap[promptSlug]).toBe('Initial prompt')
    expect(workspace.enabledMap[promptSlug]).toBe(true)
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Initial prompt')
    expect(workspace.getPromptVariables(promptSlug)).toEqual([
      { key: '{{prompt}}', tone: 'prompt' },
    ])
    expect(workspace.getCurrentRevision(promptSlug)).toEqual({
      id: 1,
      schemaVersion: 1,
      publishedAt: null,
      updatedAt: '2026-01-01T00:00:00Z',
    })
    expect(workspace.getDraftValidation(promptSlug)).toEqual({
      canPublish: true,
      isDirty: false,
      message: '无未保存更改',
      severity: 'neutral',
    })
  })

  it('loads and exposes the latest prompt eval report from the workspace state', async () => {
    const { workspace } = createWorkspace()
    const report = makeEvalReport({ passed: false, summary: '0/1 golden fixtures passed' })
    mockResource.fetchPrompts.mockResolvedValueOnce([
      makePrompt({
        slug: evalPromptSlug,
        prompt: 'Initial eval prompt',
        eval_report: report,
      }),
    ])

    await workspace.loadPrompts()

    expect(workspace.canEvaluatePrompt(evalPromptSlug)).toBe(true)
    expect(workspace.getPromptEvalReport(evalPromptSlug)).toEqual(report)
  })

  it('hides a loaded prompt eval report once the local draft changes', async () => {
    const { workspace } = createWorkspace()
    const report = makeEvalReport({ passed: true })
    mockResource.fetchPrompts.mockResolvedValueOnce([
      makePrompt({
        slug: evalPromptSlug,
        prompt: 'Initial eval prompt',
        eval_report: report,
      }),
    ])

    await workspace.loadPrompts()

    expect(workspace.getPromptEvalReport(evalPromptSlug)).toEqual(report)

    workspace.updatePromptDraft(evalPromptSlug, 'Edited eval prompt')

    expect(workspace.getPromptEvalReport(evalPromptSlug)).toBeNull()

    workspace.resetDrafts([evalPromptSlug])

    expect(workspace.getPromptEvalReport(evalPromptSlug)).toEqual(report)
  })

  it('does not attach an eval response to a draft that changed while eval was running', async () => {
    const { workspace } = createWorkspace()
    let resolveEval: ((report: FeaturePromptEvalReport) => void) | undefined
    mockResource.fetchPrompts.mockResolvedValueOnce([
      makePrompt({ slug: evalPromptSlug, prompt: 'Initial eval prompt' }),
    ])
    mockResource.evaluatePrompt.mockReturnValueOnce(
      new Promise<FeaturePromptEvalReport>((resolve) => {
        resolveEval = resolve
      }),
    )

    await workspace.loadPrompts()
    workspace.updatePromptDraft(evalPromptSlug, 'Eval prompt v1')
    const evalPromise = workspace.runPromptEval(evalPromptSlug)
    workspace.updatePromptDraft(evalPromptSlug, 'Eval prompt v2')
    resolveEval?.(makeEvalReport({ passed: true }))
    await evalPromise

    expect(workspace.getPromptEvalReport(evalPromptSlug)).toBeNull()
  })

  it('edits draft state and exposes validation feedback', async () => {
    const { workspace, feedback } = createWorkspace()

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, '')
    workspace.updateEnabledDraft(promptSlug, false)

    expect(workspace.getDraftValidation(promptSlug)).toEqual({
      canPublish: true,
      isDirty: true,
      message: '提示词为空，发布后将使用空模板',
      severity: 'warning',
    })

    workspace.insertVariableIntoDraft(promptSlug, '{{prompt}}', 0)

    expect(workspace.promptMap[promptSlug]).toBe('{{prompt}}')
    expect(workspace.getDraftValidation(promptSlug).message).toBe('可发布')

    await workspace.publishPrompt('missing-slug')

    expect(feedback.promptDraftInvalid).toHaveBeenCalledWith('提示词配置已刷新，请重新选择')
    expect(mockResource.publishPrompt).not.toHaveBeenCalled()
  })

  it('publishes a prompt then refreshes through the workspace command sequence', async () => {
    const { workspace, feedback } = createWorkspace()
    mockResource.fetchPrompts
      .mockResolvedValueOnce([makePrompt({ prompt: 'Initial prompt' })])
      .mockResolvedValueOnce([
        makePrompt({
          enabled: false,
          prompt: 'Published prompt',
          schema_version: 2,
          published_at: '2026-01-02T00:00:00Z',
          updated_at: '2026-01-02T00:00:00Z',
        }),
      ])
    mockResource.publishPrompt.mockResolvedValueOnce(
      makePrompt({
        enabled: false,
        prompt: 'Published prompt',
        schema_version: 2,
        published_at: '2026-01-02T00:00:00Z',
        updated_at: '2026-01-02T00:00:00Z',
      }),
    )

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, 'Published prompt')
    workspace.updateEnabledDraft(promptSlug, false)

    await workspace.publishPrompt(promptSlug)

    expect(mockResource.publishPrompt).toHaveBeenCalledWith({
      slug: promptSlug,
      prompt: 'Published prompt',
      enabled: false,
    })
    expect(feedback.promptSaved).toHaveBeenCalledWith(promptTitle)
    expect(mockResource.fetchPrompts).toHaveBeenCalledTimes(2)
    expect(workspace.promptMap[promptSlug]).toBe('Published prompt')
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Published prompt')
    expect(workspace.enabledMap[promptSlug]).toBe(false)
    expect(workspace.getCurrentRevision(promptSlug)?.schemaVersion).toBe(2)
  })

  it('rolls back a prompt then refreshes active revision and saved draft state', async () => {
    const { workspace, feedback } = createWorkspace()
    const currentRevision = makeRevision({
      id: 22,
      prompt: 'Current prompt',
      schema_version: 2,
      is_current: true,
    })
    const safeRevision = makeRevision({
      id: 11,
      prompt: 'Safe prompt',
      schema_version: 1,
      is_current: false,
      eval_report: makeEvalReport({ passed: true }),
    })
    mockResource.fetchPrompts
      .mockResolvedValueOnce([
        makePrompt({
          prompt: 'Current prompt',
          current_revision_id: 22,
          schema_version: 2,
          recent_revisions: [currentRevision, safeRevision],
        }),
      ])
      .mockResolvedValueOnce([
        makePrompt({
          prompt: 'Safe prompt',
          current_revision_id: 11,
          schema_version: 1,
          recent_revisions: [
            { ...safeRevision, is_current: true },
            { ...currentRevision, is_current: false },
          ],
        }),
      ])
    mockResource.rollbackPrompt.mockResolvedValueOnce(
      makePrompt({
        prompt: 'Safe prompt',
        current_revision_id: 11,
        schema_version: 1,
        recent_revisions: [
          { ...safeRevision, is_current: true },
          { ...currentRevision, is_current: false },
        ],
      }),
    )

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, 'Dirty local draft')

    const result = await workspace.rollbackPrompt(promptSlug, 11)

    expect(result?.current_revision_id).toBe(11)
    expect(mockResource.rollbackPrompt).toHaveBeenCalledWith({
      slug: promptSlug,
      revision_id: 11,
    })
    expect(mockResource.fetchPrompts).toHaveBeenCalledTimes(2)
    expect(workspace.promptMap[promptSlug]).toBe('Safe prompt')
    expect(workspace.getSavedPrompt(promptSlug)).toBe('Safe prompt')
    expect(workspace.getCurrentRevision(promptSlug)).toMatchObject({
      id: 11,
      schemaVersion: 1,
    })
    expect(workspace.getRecentRevisions(promptSlug)[0]).toMatchObject({
      id: 11,
      is_current: true,
      eval_report: expect.objectContaining({ passed: true }),
    })
    expect(feedback.promptSaved).toHaveBeenCalledWith(promptTitle)
    expect(workspace.rollingBack[promptSlug]).toBe(false)
  })

  it('runs prompt eval and blocks publish when the required report fails', async () => {
    const { workspace, feedback } = createWorkspace()
    const failingReport = makeEvalReport({
      passed: false,
      summary: '0/1 golden fixtures passed',
      fixture_results: [
        {
          fixture_id: 'pattern-gen-user-intent',
          name: 'Pattern fixture',
          passed: false,
          expected_contains: ['red floral silk scarf pattern, flat lay'],
          missing_expected: [],
          forbidden_contains: ['{{'],
          found_forbidden: ['{{'],
          rendered_prompt: 'Create {{missing_token}} red floral silk scarf pattern, flat lay',
        },
      ],
    })
    mockResource.fetchPrompts.mockResolvedValueOnce([
      makePrompt({ slug: evalPromptSlug, prompt: 'Initial eval prompt' }),
    ])
    mockResource.evaluatePrompt.mockResolvedValueOnce(failingReport)

    await workspace.loadPrompts()
    workspace.updatePromptDraft(evalPromptSlug, 'Create {{missing_token}}')

    const result = await workspace.publishPrompt(evalPromptSlug)

    expect(result).toBeNull()
    expect(mockResource.evaluatePrompt).toHaveBeenCalledWith({
      slug: evalPromptSlug,
      prompt: 'Create {{missing_token}}',
      enabled: true,
    })
    expect(mockResource.publishPrompt).not.toHaveBeenCalled()
    expect(workspace.getPromptEvalReport(evalPromptSlug)).toEqual(failingReport)
    expect(feedback.promptDraftInvalid).toHaveBeenCalledWith(
      'Prompt eval 未通过：0/1 golden fixtures passed',
    )
  })

  it('runs prompt eval and allows publish when the required report passes', async () => {
    const { workspace, feedback } = createWorkspace()
    const passingReport = makeEvalReport({ passed: true, summary: '1/1 golden fixtures passed' })
    mockResource.fetchPrompts
      .mockResolvedValueOnce([makePrompt({ slug: evalPromptSlug, prompt: 'Initial eval prompt' })])
      .mockResolvedValueOnce([
        makePrompt({
          slug: evalPromptSlug,
          prompt: 'Create {{user_prompt}}',
          schema_version: 2,
          eval_report: passingReport,
        }),
      ])
    mockResource.evaluatePrompt.mockResolvedValueOnce(passingReport)
    mockResource.publishPrompt.mockResolvedValueOnce(
      makePrompt({
        slug: evalPromptSlug,
        prompt: 'Create {{user_prompt}}',
        schema_version: 2,
        eval_report: passingReport,
      }),
    )

    await workspace.loadPrompts()
    workspace.updatePromptDraft(evalPromptSlug, 'Create {{user_prompt}}')

    const result = await workspace.publishPrompt(evalPromptSlug)

    expect(result?.schema_version).toBe(2)
    expect(mockResource.evaluatePrompt).toHaveBeenCalledWith({
      slug: evalPromptSlug,
      prompt: 'Create {{user_prompt}}',
      enabled: true,
    })
    expect(mockResource.publishPrompt).toHaveBeenCalledWith({
      slug: evalPromptSlug,
      prompt: 'Create {{user_prompt}}',
      enabled: true,
    })
    expect(feedback.promptSaved).toHaveBeenCalledWith(promptTitle)
  })

  it('keeps the draft in place when publish fails', async () => {
    const { workspace, feedback } = createWorkspace()
    const error = new Error('publish failed')
    mockResource.publishPrompt.mockRejectedValueOnce(error)

    await workspace.loadPrompts()
    workspace.updatePromptDraft(promptSlug, 'Unsaved prompt')

    const result = await workspace.publishPrompt(promptSlug)

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

function makeRevision(overrides: Partial<FeaturePromptRevision> = {}): FeaturePromptRevision {
  return {
    id: 1,
    prompt: '',
    enabled: true,
    variables: [{ key: '{{prompt}}', tone: 'prompt' }],
    model_defaults: {},
    user_prompt_merge_policy: {
      append_base_prompt_without_template: true,
      append_user_prompt_when_template_misses_placeholder: true,
    },
    schema_version: 1,
    is_current: false,
    published_at: null,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeEvalReport(overrides: Partial<FeaturePromptEvalReport> = {}): FeaturePromptEvalReport {
  return {
    id: 1,
    feature_slug: evalPromptSlug,
    draft_hash: 'a'.repeat(64),
    suite_id: 'pattern-gen-golden-v1',
    passed: true,
    summary: '1/1 golden fixtures passed',
    fixture_results: [
      {
        fixture_id: 'pattern-gen-user-intent',
        name: 'Pattern fixture',
        passed: true,
        expected_contains: ['red floral silk scarf pattern, flat lay'],
        missing_expected: [],
        forbidden_contains: ['{{'],
        found_forbidden: [],
        rendered_prompt: 'Create red floral silk scarf pattern, flat lay',
      },
    ],
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}
