import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import vue from 'eslint-plugin-vue'
import vueA11y from 'eslint-plugin-vuejs-accessibility'
import prettierConfig from 'eslint-config-prettier'
import { functionLengthRule } from './scripts/eslint-function-length.mjs'
import { nameReplacementsRule } from './scripts/eslint-name-replacements.mjs'
import { queryPlacementRule } from './scripts/eslint-query-placement.mjs'
import { booleanPrefixRule, noBarrelImportRule } from './scripts/eslint-p0-governance.mjs'

export default tseslint.config(
  {
    ignores: ['dist/**', 'node_modules/**', 'output/**', 'src/api/generated-query/**'],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...vue.configs['flat/recommended'],
  ...vueA11y.configs['flat/recommended'],
  {
    files: ['**/*.vue'],
    languageOptions: {
      parserOptions: {
        parser: tseslint.parser,
        extraFileExtensions: ['.vue'],
        ecmaVersion: 'latest',
        sourceType: 'module',
      },
    },
  },
  {
    files: ['src/**/*.{ts,tsx,vue}'],
    plugins: {
      governance: {
        rules: {
          'function-length': functionLengthRule,
          'name-replacements': nameReplacementsRule,
          'query-placement': queryPlacementRule,
          'boolean-prefix': booleanPrefixRule,
          'no-barrel-import': noBarrelImportRule,
        },
      },
    },
    languageOptions: {
      parserOptions: {
        project: ['tsconfig.app.json', 'tsconfig.test.json', 'tsconfig.test-node.json'],
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.vue'],
      },
      globals: {
        ...globals.browser,
        ...globals.es2024,
      },
    },
    rules: {
      'no-undef': 'off',
      'no-empty': 'error',
      'no-useless-assignment': 'error',
      // Nested functions receive their own budget; Vitest describe only groups tests.
      complexity: ['error', 15],
      'max-depth': ['error', 4],
      'governance/function-length': ['warn', { max: 100 }],
      // P0 naming subset (plan §3.5, warn-only): default camelCase, typeLike
      // PascalCase, `I-` interface ban (confirmed zero violations; the custom
      // regex below is a confirmation-only ratchet). `property` keeps
      // snake_case for backend-shaped DTOs (API vocabulary §3.6); `variable`
      // and `import` keep PascalCase for SFC/type imports. `default` is only
      // the fallback: any name matching a specific selector below is judged
      // by that selector, not by `default`.
      '@typescript-eslint/naming-convention': [
        'warn',
        {
          selector: 'default',
          format: ['camelCase'],
          leadingUnderscore: 'allow',
          trailingUnderscore: 'allow',
        },
        {
          selector: 'variable',
          format: ['camelCase', 'PascalCase', 'UPPER_CASE'],
          leadingUnderscore: 'allow',
          trailingUnderscore: 'allow',
        },
        { selector: 'import', format: ['camelCase', 'PascalCase', 'UPPER_CASE'] },
        // PascalCase functions are functional components in SFCs
        // (`SlotRenderer` in DataTableFrame.vue, called as `<SlotRenderer>`).
        { selector: 'function', format: ['camelCase', 'PascalCase'], leadingUnderscore: 'allow' },
        { selector: 'parameter', format: ['camelCase'], leadingUnderscore: 'allow' },
        {
          selector: 'property',
          format: ['camelCase', 'snake_case', 'UPPER_CASE'],
          leadingUnderscore: 'allowSingleOrDouble',
          trailingUnderscore: 'allowSingleOrDouble',
        },
        // Quoted external vocabularies are not identifiers: BCP 47 locale
        // codes (`zh-CN`), HTML/ARIA attributes (`aria-hidden`,
        // `data-testid`), lint rule names (`governance/function-length`),
        // and domain position keys (`top-left` in fusionBlend).
        { selector: 'property', modifiers: ['requiresQuotes'], format: null },
        { selector: 'method', modifiers: ['requiresQuotes'], format: null },
        // Component registration/stub objects key by SFC name (`{ CanvasScreen }`
        // in specs); interface/type members stay on the `property` formats above.
        // Quoted HTML/ARIA keys stay exempt here too: the specific
        // `objectLiteralProperty` selector otherwise shadows the `property`
        // requiresQuotes exemption above.
        { selector: 'objectLiteralProperty', modifiers: ['requiresQuotes'], format: null },
        {
          selector: 'objectLiteralProperty',
          format: ['camelCase', 'PascalCase', 'snake_case', 'UPPER_CASE'],
          leadingUnderscore: 'allowSingleOrDouble',
          trailingUnderscore: 'allowSingleOrDouble',
        },
        { selector: 'typeLike', format: ['PascalCase'] },
        { selector: 'typeParameter', format: ['PascalCase'] },
        {
          selector: 'interface',
          format: ['PascalCase'],
          custom: { regex: '^I[A-Z]', match: false },
        },
        { selector: 'enumMember', format: ['PascalCase', 'UPPER_CASE'] },
      ],
      // P0 placeholder ban (plan §3.5 id-denylist, warn-only). Kept to
      // unambiguous placeholders: `temp` scratch buffers are legitimate
      // image-math vocabulary in canvas/retouch/inpaint (same family as the
      // allowed single-letter math symbols, §10.5), so they stay out.
      'id-denylist': ['warn', 'foo', 'bar', 'baz', 'qux'],
      // unicorn/name-replacements equivalent without a new plugin (warn-only):
      // abbreviated identifiers must use the full word. `args`/`e` stay out
      // (idiomatic rest/event names); see the rule module for the subset.
      'governance/name-replacements': 'warn',
      'governance/boolean-prefix': 'warn',
      'governance/no-barrel-import': 'warn',
      // P0 scope migration remains advisory; Canvas overrides this below.
      'governance/query-placement': 'warn',
      // P0 Vue dual-end casing (plan §3.5, warn-only): PascalCase at
      // registration and in templates. Single-word SFCs stay allowed
      // (`vue/multi-word-component-names` remains off per §10.5).
      'vue/component-definition-name-casing': ['warn', 'PascalCase'],
      'vue/component-name-in-template-casing': ['warn', 'PascalCase'],
      'no-unused-vars': 'off',
      'preserve-caught-error': 'error',
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-empty-object-type': 'error',
      '@typescript-eslint/no-unused-expressions': 'error',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
          caughtErrorsIgnorePattern: '^_',
          ignoreRestSiblings: true,
        },
      ],
      'vue/attributes-order': 'error',
      'vue/block-order': ['error', { order: ['template', 'script', 'style'] }],
      'vue/define-macros-order': 'error',
      'vue/first-attribute-linebreak': 'off',
      'vue/html-closing-bracket-newline': 'off',
      'vue/html-closing-bracket-spacing': 'off',
      'vue/html-indent': 'off',
      'vue/html-self-closing': 'off',
      'vue/max-attributes-per-line': 'off',
      'vue/multi-word-component-names': 'off',
      'vue/multiline-html-element-content-newline': 'off',
      'vuejs-accessibility/aria-props': 'error',
      'vuejs-accessibility/form-control-has-label': 'error',
      'vuejs-accessibility/alt-text': 'error',
      'vuejs-accessibility/anchor-has-content': 'error',
      'vuejs-accessibility/heading-has-content': 'error',
      'vuejs-accessibility/no-autofocus': 'off',
      'vuejs-accessibility/no-static-element-interactions': 'off',
      'vuejs-accessibility/click-events-have-key-events': 'off',
      'vuejs-accessibility/label-has-for': 'off',
      'vue/no-duplicate-class-names': 'error',
      'vue/no-deprecated-filter': 'error',
      'vue/no-empty-component-block': 'error',
      'vue/no-useless-mustaches': 'error',
      'vue/no-useless-v-bind': 'error',
      'vue/no-use-v-if-with-v-for': 'error',
      'vue/no-v-html': 'error',
      'vue/require-default-prop': 'error',
      'vue/singleline-html-element-content-newline': 'off',
      'vue/v-bind-style': 'error',
      'vue/v-on-event-hyphenation': ['error', 'always', { autofix: true }],
      'vue/v-on-style': 'error',
      'vue/v-slot-style': 'error',
    },
  },
  {
    files: ['*.config.{js,ts}', 'eslint.config.js', '.dependency-cruiser.cjs'],
    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.es2024,
      },
    },
  },
  {
    files: [
      'src/app/**/*.{ts,tsx,vue}',
      'src/features/**/app/**/*.{ts,tsx,vue}',
      'src/features/**/pages/**/*.{ts,tsx,vue}',
    ],
    // Module-direction rules (shared -> features, pages -> generated-query,
    // circular, cross-feature deep imports) are owned by dependency-cruiser
    // (.dependency-cruiser.cjs + lint:dependencies). ESLint keeps only the
    // file-local Overlay adapter conventions below.
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['**/useOverlayInteractionLifecycle'],
              message:
                'Route, page, and app layers must use a dialog Overlay adapter or feature-owned public Overlay adapter instead of the base lifecycle directly.',
            },
            {
              group: ['**/overlay-lifecycle/useCanvasCustomLayerOverlay'],
              message:
                'Import Canvas Overlay lifecycle through the overlay-lifecycle public barrel instead of its implementation file.',
            },
          ],
        },
      ],
    },
  },
  {
    files: ['src/features/admin/**/*.{ts,tsx,vue}'],
    ignores: ['src/features/admin/shared/session/overlay/useAdminDialogInteraction.ts'],
    // Admin src-root imports ride the `@/` alias (first batch: five-level
    // `../../../../../api|core|test|...` converged). Deep relatives that
    // reach `src/` stay banned so the alias cannot silently regress.
    // Intra-admin `shared` and cross-feature `features/*` relatives stay
    // allowed; the Overlay base-lifecycle ban below mirrors the page/app
    // rule so admin pages keep both guards (the single adapter file above
    // stays exempt as the legitimate base-lifecycle owner).
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['**/useOverlayInteractionLifecycle'],
              message:
                'Route, page, and app layers must use a dialog Overlay adapter or feature-owned public Overlay adapter instead of the base lifecycle directly.',
            },
            {
              group: ['**/overlay-lifecycle/useCanvasCustomLayerOverlay'],
              message:
                'Import Canvas Overlay lifecycle through the overlay-lifecycle public barrel instead of its implementation file.',
            },
            {
              group: [
                '../../../../api',
                '../../../../api/**',
                '../../../../core',
                '../../../../core/**',
                '../../../../test',
                '../../../../test/**',
                '../../../../lib',
                '../../../../lib/**',
                '../../../../theme',
                '../../../../theme/**',
                '../../../../components',
                '../../../../components/**',
                '../../../../app',
                '../../../../app/**',
                '../../../../../api',
                '../../../../../api/**',
                '../../../../../core',
                '../../../../../core/**',
                '../../../../../test',
                '../../../../../test/**',
                '../../../../../lib',
                '../../../../../lib/**',
                '../../../../../theme',
                '../../../../../theme/**',
                '../../../../../components',
                '../../../../../components/**',
                '../../../../../app',
                '../../../../../app/**',
                '../../../../../../api',
                '../../../../../../api/**',
                '../../../../../../core',
                '../../../../../../core/**',
                '../../../../../../test',
                '../../../../../../test/**',
                '../../../../../../lib',
                '../../../../../../lib/**',
                '../../../../../../theme',
                '../../../../../../theme/**',
                '../../../../../../components',
                '../../../../../../components/**',
                '../../../../../../app',
                '../../../../../../app/**',
              ],
              message:
                'Use @/ alias for src-root imports in admin instead of deep relative paths.',
            },
          ],
        },
      ],
    },
  },
  {
    files: [
      'src/features/canvas/ui/screens/internal/useCanvasLibraryIsland.ts',
      'src/features/canvas/session/image-workflow/useImageWorkflowExecution.ts',
      'src/features/board/ui/gallery/cards/BoardCard.vue',
      'src/features/board/ui/share/ShareDialog.vue',
    ],
    // First-batch seam guard: these leaves receive workspace/auth via
    // ports/props injection from the page/screen seam (no direct store
    // imports). `core/utils/auth` stays allowed (different import names).
    rules: {
      'no-restricted-imports': [
        'error',
        {
          patterns: [
            {
              group: ['**/workspace', '**/workspace/**', '**/auth', '**/auth/**'],
              importNames: [
                'getWorkspaceSelectionStore',
                'resetWorkspaceSelectionStore',
                'installWorkspaceSelectionStore',
                'createWorkspaceSelectionStore',
                'useWorkspaceSelection',
                'subscribeWorkspaceSelectionChange',
                'useUserStore',
                'useAuthSession',
              ],
              message:
                'Board/Canvas leaves receive workspace/auth via ports/props injection from the page/screen seam instead of direct store imports.',
            },
          ],
        },
      ],
    },
  },
  {
    // P0 sole new error: Canvas interaction state must stay local. The
    // independent rule preserves the existing page/app Overlay restrictions.
    // Query types follow the same scope; test/provider harnesses are exempt.
    files: ['src/features/canvas/**/*.{ts,tsx,vue}'],
    rules: {
      'governance/query-placement': 'error',
    },
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['src/api/generated-query/**'],
    rules: {
      'no-restricted-globals': [
        'error',
        ...[
          'EffectScope',
          'computed',
          'createApp',
          'customRef',
          'defineAsyncComponent',
          'defineComponent',
          'effectScope',
          'getCurrentInstance',
          'getCurrentScope',
          'getCurrentWatcher',
          'h',
          'inject',
          'isProxy',
          'isReactive',
          'isReadonly',
          'isRef',
          'isShallow',
          'markRaw',
          'nextTick',
          'onActivated',
          'onBeforeMount',
          'onBeforeRouteLeave',
          'onBeforeRouteUpdate',
          'onBeforeUnmount',
          'onBeforeUpdate',
          'onDeactivated',
          'onErrorCaptured',
          'onMounted',
          'onRenderTracked',
          'onRenderTriggered',
          'onScopeDispose',
          'onServerPrefetch',
          'onUnmounted',
          'onUpdated',
          'onWatcherCleanup',
          'provide',
          'reactive',
          'readonly',
          'ref',
          'resolveComponent',
          'shallowReactive',
          'shallowReadonly',
          'shallowRef',
          'toRaw',
          'toRef',
          'toRefs',
          'toValue',
          'triggerRef',
          'unref',
          'useAttrs',
          'useCssModule',
          'useCssVars',
          'useId',
          'useLink',
          'useModel',
          'useMutation',
          'useQuery',
          'useQueryClient',
          'useRoute',
          'useRouter',
          'useSlots',
          'useTemplateRef',
          'watch',
          'watchEffect',
          'watchPostEffect',
          'watchSyncEffect',
        ].map((name) => ({
          name,
          message: `Explicitly import ${name} in .ts files instead of relying on auto-import globals.`,
        })),
      ],
    },
  },
  {
    // Test render stubs are colocated with the specs that shape them by design;
    // one-component-per-file governs production component organization, not tests.
    files: ['src/test/**/*.{ts,tsx}', 'src/**/*.spec.{ts,tsx}', 'src/**/*.test.{ts,tsx}'],
    rules: {
      'vue/one-component-per-file': 'off',
    },
  },
  prettierConfig,
)
