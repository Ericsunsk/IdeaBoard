import js from '@eslint/js'
import globals from 'globals'
import tseslint from 'typescript-eslint'
import vue from 'eslint-plugin-vue'
import prettierConfig from 'eslint-config-prettier'

export default tseslint.config(
  {
    ignores: [
      'dist/**',
      'node_modules/**',
      'output/**',
      'src/auto-imports.d.ts',
      'src/components.d.ts',
      'src/shared/api/generated-query/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...vue.configs['flat/recommended'],
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
      'no-unused-vars': 'off',
      'preserve-caught-error': 'error',
      '@typescript-eslint/no-explicit-any': 'off',
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
    files: ['*.config.{js,ts}', 'eslint.config.js'],
    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.es2024,
      },
    },
  },
  prettierConfig,
)
