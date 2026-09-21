import { defineConfig } from 'orval'

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
})
