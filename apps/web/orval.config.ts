import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'orval'

const configDir = path.dirname(fileURLToPath(import.meta.url))

// Issue #800: the Public/Control OpenAPI split lands in parallel. A project is
// only registered when its spec stub exists and carries paths, so
// `npm run api:generate` and existing builds keep passing before the split.
// Candidates prefer `../api/` (apps/web -> apps/api, mirroring apiQuery);
// `../../api/` is kept as a literal fallback per the issue text.
function resolveUsableSpec(candidates: readonly string[]): string | null {
  for (const candidate of candidates) {
    try {
      const content = readFileSync(path.resolve(configDir, candidate), 'utf8')
      if (content.includes('paths')) return candidate
    } catch {
      // Missing or unreadable stub: try the next candidate.
    }
  }
  return null
}

const publicSpecTarget = resolveUsableSpec([
  '../api/openapi.public.yaml',
  '../../api/openapi.public.yaml',
])
const controlSpecTarget = resolveUsableSpec([
  '../api/openapi.control.yaml',
  '../../api/openapi.control.yaml',
])

export default defineConfig({
  apiQuery: {
    input: {
      target: '../api/openapi.yaml',
      filters: {
        mode: 'exclude',
        tags: ['PaymentWebhooks', 'UserEvents'],
      },
    },
    output: {
      target: 'src/shared/api/generated-query/endpoints.ts',
      schemas: 'src/shared/api/generated-query/model',
      mode: 'tags-split',
      client: 'vue-query',
      httpClient: 'fetch',
      clean: true,
      formatter: 'prettier',
      urlEncodeParameters: true,
      override: {
        mutator: {
          path: './src/shared/api/orvalMutator.ts',
          name: 'orvalMutator',
        },
        fetch: {
          includeHttpResponseReturnType: false,
        },
        query: {
          // Do NOT set `useQuery: true` here: it forces every operation
          // (including PUT/POST/DELETE) into cached query hooks, which fire on
          // mount and must never carry mutations. Orval defaults already do
          // the right thing: GET -> useQuery, non-GET -> useMutation.
          shouldExportKeys: true,
        },
      },
    },
  },
  // Issue #800: Public/Control dual-client skeleton. Registered only when the
  // matching split spec stub exists (see resolveUsableSpec above); no feature
  // migrates to the new envelope yet (migration is #801-#808).
  ...(publicSpecTarget
    ? {
        publicQuery: {
          input: {
            target: publicSpecTarget,
          },
          output: {
            target: 'src/shared/api/generated-query/public/',
            schemas: 'src/shared/api/generated-query/public/model',
            mode: 'tags-split',
            client: 'vue-query',
            httpClient: 'fetch',
            clean: true,
            formatter: 'prettier',
            urlEncodeParameters: true,
            override: {
              mutator: {
                path: './src/shared/api/orvalMutator.ts',
                name: 'orvalMutator',
              },
              fetch: {
                includeHttpResponseReturnType: false,
              },
              query: {
                // Same rationale as apiQuery: Orval defaults already map
                // GET -> useQuery and non-GET -> useMutation.
                shouldExportKeys: true,
              },
            },
          },
        },
      }
    : {}),
  // Issue #808: the Control client carries its own transport mutator so
  // generated relative paths resolve under /api/control/v1 (the shared
  // mutator only mounts /api). Generated code stays untouched.
  ...(controlSpecTarget
    ? {
        controlQuery: {
          input: {
            target: controlSpecTarget,
          },
          output: {
            target: 'src/shared/api/generated-query/control/',
            schemas: 'src/shared/api/generated-query/control/model',
            mode: 'tags-split',
            client: 'vue-query',
            httpClient: 'fetch',
            clean: true,
            formatter: 'prettier',
            urlEncodeParameters: true,
            override: {
              mutator: {
                path: './src/shared/api/controlMutator.ts',
                name: 'controlMutator',
              },
              fetch: {
                includeHttpResponseReturnType: false,
              },
              query: {
                // Same rationale as apiQuery: Orval defaults already map
                // GET -> useQuery and non-GET -> useMutation.
                shouldExportKeys: true,
              },
            },
          },
        },
      }
    : {}),
})
