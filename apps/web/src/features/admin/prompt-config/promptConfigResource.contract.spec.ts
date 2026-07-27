import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createAdminWorkflowFeatureResource } from './promptConfigResource'
import type { WorkflowFeature } from './promptConfigResource'

const featureSlug = 'sketch-to-image'

describe('workflow feature resource', () => {
  it('maps generated workflow feature APIs behind the Admin resource seam', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    useMswHandlers(
      http.get('/api/admin/workflow-features', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            workflow_features: [makeWorkflowFeature({ system_prompt: 'Initial prompt' })],
          },
        }),
      ),
      http.put('/api/admin/workflow-features/:slug', async ({ request }) => {
        const body = await readRequestJson(request)
        const url = new URL(request.url)
        requests.push({ method: request.method, path: url.pathname, body })

        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makeWorkflowFeature({
            canonical_model: 'image-beta',
            system_prompt: 'Saved prompt',
            enabled: false,
          }),
        })
      }),
    )

    const resource = createAdminWorkflowFeatureResource()
    const workflowFeatures = await resource.fetchWorkflowFeatures()
    const saved = await resource.saveWorkflowFeature({
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

    expect(workflowFeatures[0]?.system_prompt).toBe('Initial prompt')
    expect(requests).toEqual([
      {
        method: 'PUT',
        path: `/api/admin/workflow-features/${featureSlug}`,
        body: {
          canonical_model: 'image-beta',
          system_prompt: 'Saved prompt',
          enabled: false,
          variables: [{ key: '{{prompt}}', tone: 'prompt' }],
          user_prompt_merge_policy: {
            append_base_prompt_without_template: true,
            append_user_prompt_when_template_misses_placeholder: true,
          },
        },
      },
    ])
    expect(saved.canonical_model).toBe('image-beta')
    expect(saved.enabled).toBe(false)
  })
})

async function readRequestJson(request: Request) {
  const text = await request.text()
  return text ? JSON.parse(text) : undefined
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
