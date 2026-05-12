import { defineConfig } from 'orval'

export default defineConfig({
  apiQuery: {
    input: '../api/openapi.yaml',
    output: {
      target: 'src/shared/api/generated-query/endpoints.ts',
      schemas: 'src/shared/api/generated-query/model',
      mode: 'tags-split',
      client: 'vue-query',
      httpClient: 'axios',
      clean: true,
      urlEncodeParameters: true,
      override: {
        mutator: {
          path: './src/shared/api/orvalMutator.ts',
          name: 'orvalMutator',
        },
        query: {
          useQuery: true,
          shouldExportQueryKey: true,
        },
      },
    },
  },
})
