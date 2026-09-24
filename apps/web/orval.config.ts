import { defineConfig } from 'orval'

// Post-#809: the legacy single-file `apps/api/openapi.yaml` was deleted in
// the hard cut, along with the `apiQuery` project that consumed it. The
// generation baseline is the Public/Control split specs, both required:
// `../api/` resolves from apps/web to apps/api (and to the staged copy in
// the drift-check workspace, which mirrors the same layout).
export default defineConfig({
  publicQuery: {
    input: {
      target: '../api/openapi.public.yaml',
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
          // Do NOT set `useQuery: true` here: it forces every operation
          // (including PUT/POST/DELETE) into cached query hooks, which fire on
          // mount and must never carry mutations. Orval defaults already do
          // the right thing: GET -> useQuery, non-GET -> useMutation.
          shouldExportKeys: true,
        },
      },
    },
  },
  // Issue #808: the Control client carries its own transport mutator so
  // generated relative paths resolve under /api/control/v1 (the shared
  // mutator only mounts /api). Generated code stays untouched.
  controlQuery: {
    input: {
      target: '../api/openapi.control.yaml',
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
          // Same rationale as publicQuery: Orval defaults already map
          // GET -> useQuery and non-GET -> useMutation.
          shouldExportKeys: true,
        },
      },
    },
  },
})
