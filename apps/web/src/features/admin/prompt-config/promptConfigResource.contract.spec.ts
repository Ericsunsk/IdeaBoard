import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { waitFor } from '@testing-library/vue'
import { createApp, defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { usePromptConfigResource } from './promptConfigResource'
import type { FeaturePrompt } from './promptConfigResource'

type PromptConfigResource = ReturnType<typeof usePromptConfigResource>

type PromptRequestLog = {
  method: string
  path: string
  body?: unknown
}

type PromptServerState = {
  currentPrompt: FeaturePrompt
  requests: PromptRequestLog[]
  promptListRequests: number
}

const promptSlug = 'sketch-to-image'
const promptTitle = '草图成图'

describe('usePromptConfigResource', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('saves prompts through the resource boundary and invalidates prompt caches', async () => {
    const state = createPromptServerState()
    useMswHandlers(...promptConfigHandlers(state))
    const harness = mountPromptConfigResource()
    const invalidateSpy = vi.spyOn(harness.queryClient, 'invalidateQueries')

    try {
      await waitForPrompt(harness.resource, 'Initial prompt')

      harness.resource.promptMap[promptSlug] = 'Saved prompt'
      harness.resource.enabledMap[promptSlug] = false

      await harness.resource.savePrompt(promptSlug)

      expect(state.requests).toEqual([
        {
          method: 'PUT',
          path: `/api/admin/feature-prompts/${promptSlug}`,
          body: {
            prompt: 'Saved prompt',
            enabled: false,
          },
        },
      ])
      expect(harness.resource.promptMap[promptSlug]).toBe('Saved prompt')
      expect(harness.resource.getSavedPrompt(promptSlug)).toBe('Saved prompt')
      expect(harness.resource.enabledMap[promptSlug]).toBe(false)
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['admin', 'feature-prompts'] })
      expect(harness.feedback.promptSaved).toHaveBeenCalledWith(promptTitle)
    } finally {
      await harness.unmount()
    }
  })
})

function mountPromptConfigResource() {
  let resource: PromptConfigResource | null = null
  const feedback = createPromptConfigFeedback()
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 60_000,
        gcTime: 0,
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
  })
  const host = document.createElement('div')
  const app = createApp(
    defineComponent({
      setup() {
        resource = usePromptConfigResource({
          feedback,
          getFeatureTitle: () => promptTitle,
        })
        return () => null
      },
    }),
  )

  document.body.appendChild(host)
  app.use(VueQueryPlugin, { queryClient })
  app.mount(host)

  const mountedResource = requireMountedPromptResource(resource)

  return {
    feedback,
    queryClient,
    resource: mountedResource,
    async unmount() {
      app.unmount()
      await queryClient.cancelQueries()
      queryClient.clear()
      host.remove()
    },
  }
}

function requireMountedPromptResource(resource: PromptConfigResource | null): PromptConfigResource {
  if (!resource) throw new Error('expected prompt config resource to mount')
  return resource
}

async function waitForPrompt(resource: PromptConfigResource, prompt: string) {
  await waitFor(() => {
    expect(resource.promptMap[promptSlug]).toBe(prompt)
  })
}

function promptConfigHandlers(state: PromptServerState) {
  return [
    http.get('/api/admin/feature-prompts', () => {
      state.promptListRequests += 1

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: {
          prompts: [state.currentPrompt],
        },
      })
    }),
    http.put('/api/admin/feature-prompts/:slug', async ({ request }) => {
      const body = await readRequestJson(request)
      recordRequest(state.requests, request, body)
      const payload = body as { enabled?: boolean; prompt?: string }
      state.currentPrompt = makePrompt({
        enabled: payload.enabled ?? true,
        prompt: payload.prompt ?? '',
        schema_version: 2,
      })

      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: state.currentPrompt,
      })
    }),
  ]
}

function createPromptServerState(overrides: Partial<PromptServerState> = {}): PromptServerState {
  return {
    currentPrompt: makePrompt({ prompt: 'Initial prompt' }),
    requests: [],
    promptListRequests: 0,
    ...overrides,
  }
}

function recordRequest(receivedRequests: PromptRequestLog[], request: Request, body?: unknown) {
  const url = new URL(request.url)
  receivedRequests.push({
    method: request.method,
    path: url.pathname,
    ...(body === undefined ? {} : { body }),
  })
}

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

function createPromptConfigFeedback() {
  return {
    loadPromptsFailed: vi.fn(),
    promptSaved: vi.fn(),
    savePromptFailed: vi.fn(),
  }
}
