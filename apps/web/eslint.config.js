import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import vue from 'eslint-plugin-vue'
import vueA11y from 'eslint-plugin-vuejs-accessibility'
import prettierConfig from 'eslint-config-prettier'
import { functionLengthRule } from './scripts/eslint-function-length.mjs'

export default tseslint.config(
  {
    ignores: ['dist/**', 'node_modules/**', 'output/**', 'src/shared/api/generated-query/**'],
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
    plugins: { governance: { rules: { 'function-length': functionLengthRule } } },
    languageOptions: {
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
    files: ['src/**/*.{ts,tsx}'],
    ignores: ['src/shared/api/generated-query/**'],
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
