import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createAdminPromptConfigResource } from './promptConfigResource'
import type { FeaturePrompt, FeaturePromptEvalReport } from './promptConfigResource'

const promptSlug = 'sketch-to-image'

describe('prompt config resource', () => {
  it('maps generated prompt APIs behind the feature resource seam', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    useMswHandlers(
      http.get('/api/admin/feature-prompts', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            prompts: [makePrompt({ prompt: 'Initial prompt' })],
          },
        }),
      ),
      http.put('/api/admin/feature-prompts/:slug', async ({ request }) => {
        const body = await readRequestJson(request)
        const url = new URL(request.url)
        requests.push({ method: request.method, path: url.pathname, body })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makePrompt({
            enabled: false,
            prompt: 'Published prompt',
            schema_version: 2,
          }),
        })
      }),
      http.post('/api/admin/feature-prompts/:slug/eval', async ({ request }) => {
        const body = await readRequestJson(request)
        const url = new URL(request.url)
        requests.push({ method: request.method, path: url.pathname, body })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makeEvalReport({ passed: true }),
        })
      }),
      http.post('/api/admin/feature-prompts/:slug/rollback', async ({ request }) => {
        const body = await readRequestJson(request)
        const url = new URL(request.url)
        requests.push({ method: request.method, path: url.pathname, body })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makePrompt({
            prompt: 'Rollback prompt',
            schema_version: 1,
            current_revision_id: 11,
          }),
        })
      }),
      http.get('/api/admin/feature-prompts/:slug/eval', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makeEvalReport({ passed: false }),
        }),
      ),
    )

    const resource = createAdminPromptConfigResource()
    const prompts = await resource.fetchPrompts()
    const published = await resource.publishPrompt({
      slug: promptSlug,
      prompt: 'Published prompt',
      enabled: false,
    })
    const evalReport = await resource.evaluatePrompt({
      slug: promptSlug,
      prompt: 'Eval prompt',
      enabled: true,
    })
    const rolledBack = await resource.rollbackPrompt({
      slug: promptSlug,
      revision_id: 11,
    })
    const latestEvalReport = await resource.fetchLatestEvalReport(promptSlug)

    expect(prompts[0]?.prompt).toBe('Initial prompt')
    expect(requests).toEqual([
      {
        method: 'PUT',
        path: `/api/admin/feature-prompts/${promptSlug}`,
        body: {
          prompt: 'Published prompt',
          enabled: false,
        },
      },
      {
        method: 'POST',
        path: `/api/admin/feature-prompts/${promptSlug}/eval`,
        body: {
          prompt: 'Eval prompt',
          enabled: true,
        },
      },
      {
        method: 'POST',
        path: `/api/admin/feature-prompts/${promptSlug}/rollback`,
        body: {
          revision_id: 11,
        },
      },
    ])
    expect(published.prompt).toBe('Published prompt')
    expect(published.enabled).toBe(false)
    expect(published.schema_version).toBe(2)
    expect(evalReport.passed).toBe(true)
    expect(rolledBack.current_revision_id).toBe(11)
    expect(rolledBack.prompt).toBe('Rollback prompt')
    expect(latestEvalReport?.passed).toBe(false)
  })
})

async function readRequestJson(request: Request) {
  const text = await request.text()
  return text ? JSON.parse(text) : undefined
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
    published_at: null,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function makeEvalReport(overrides: Partial<FeaturePromptEvalReport> = {}): FeaturePromptEvalReport {
  return {
    id: 1,
    feature_slug: promptSlug,
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
