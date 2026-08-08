import { defineConfig } from 'orval'

export default defineConfig({
  apiQuery: {
    input: {
      target: '../api/openapi.yaml',
      filters: {
        mode: 'exclude',
        tags: ['PaymentWebhooks'],
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
          useQuery: true,
          shouldExportQueryKey: true,
        },
      },
    },
  },
})
