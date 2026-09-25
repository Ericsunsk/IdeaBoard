import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createAdminPromptConfigResource } from './resource'
import type { FeaturePrompt } from './resource'

const featureSlug = 'variations'

describe('feature prompt resource', () => {
  it('maps the canonical feature-prompts API behind the Admin resource seam', async () => {
    const requests: Array<{ method: string; path: string; body?: unknown }> = []
    useMswHandlers(
      http.get('/api/control/v1/platform/feature-prompts', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            prompts: [makeFeaturePrompt({ prompt: 'Initial prompt' })],
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
          data: makeFeaturePrompt({ prompt: 'Saved prompt' }),
        })
      }),
    )

    const resource = createAdminPromptConfigResource()
    const prompts = await resource.fetchPrompts()
    const saved = await resource.savePrompt({
      slug: featureSlug,
      prompt: 'Saved prompt',
    })

    expect(prompts[0]?.prompt).toBe('Initial prompt')
    expect(requests).toEqual([
      {
        method: 'PUT',
        path: `/api/admin/feature-prompts/${featureSlug}`,
        body: { prompt: 'Saved prompt' },
      },
    ])
    expect(saved.prompt).toBe('Saved prompt')
  })
})

async function readRequestJson(request: Request) {
  const text = await request.text()
  return text ? JSON.parse(text) : undefined
}

function makeFeaturePrompt(overrides: Partial<FeaturePrompt> = {}): FeaturePrompt {
  return {
    feature_slug: featureSlug,
    prompt: '',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}
