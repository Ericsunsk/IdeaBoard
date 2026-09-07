import { Linter } from 'eslint'
import tseslint from 'typescript-eslint'
import vueParser from 'vue-eslint-parser'
import { describe, expect, it } from 'vitest'
import { functionLengthRule } from '../../scripts/eslint-function-length.mjs'

function verify(code: string, filename = 'example.ts', max = 5) {
  return new Linter().verify(
    code,
    {
      files: ['**/*.ts', '**/*.vue'],
      languageOptions: {
        parser: filename.endsWith('.vue') ? vueParser : tseslint.parser,
        parserOptions: { parser: tseslint.parser, ecmaVersion: 2022, sourceType: 'module' },
      },
      plugins: { governance: { rules: { 'function-length': functionLengthRule } } },
      rules: { 'governance/function-length': ['warn', { max }] },
    },
    filename,
  )
}

const longBody = Array.from({ length: 6 }, () => '  work();').join('\n')

describe('function length governance', () => {
  it('reports ordinary functions, methods and IIFEs exceeding the budget', () => {
    for (const code of [
      `function run() {\n${longBody}\n}`,
      `class Runner { run() {\n${longBody}\n} }`,
      `(() => {\n${longBody}\n})();`,
    ]) {
      expect(verify(code)).toEqual([expect.objectContaining({ messageId: 'tooLong' })])
    }
  })

  it('ignores blank and comment-only lines but counts multiline literals', () => {
    expect(verify('function run() {\n// comment\n\n/* comment */\nwork();\n}')).toEqual([])
    expect(verify('function run() {\nconst text = `a\nb\nc\nd\ne`;\n}')).toHaveLength(1)
  })

  it('checks nested functions independently without counting their bodies twice', () => {
    const messages = verify(
      `function factory() {\nfunction child() {\n${longBody}\n}\nreturn child;\n}`,
    )
    expect(messages).toEqual([expect.objectContaining({ line: 2, messageId: 'tooLong' })])
  })

  it('exempts imported Vitest suites but still checks their tests', () => {
    const messages = verify(
      `import { describe as suite, it } from 'vitest';\nsuite.only('group', () => {\nit('case', () => {\n${longBody}\n});\n});`,
      'example.spec.ts',
    )
    expect(messages).toEqual([expect.objectContaining({ line: 3, messageId: 'tooLong' })])
    expect(
      verify(
        `import { describe } from 'vitest';\ndescribe.each([1])('group', () => {\n${longBody}\n});`,
        'example.spec.ts',
      ),
    ).toEqual([])
  })

  it('does not exempt similarly named application functions or shadowed bindings', () => {
    for (const code of [
      `function describe(name, callback) {}\ndescribe('group', () => {\n${longBody}\n});`,
      `import { describe } from 'other-library';\ndescribe('group', () => {\n${longBody}\n});`,
      `import { describe } from 'vitest';\nfunction factory(describe) { describe('group', () => {\n${longBody}\n}); }`,
    ])
      expect(verify(code, 'example.spec.ts')).toHaveLength(1)
    expect(
      verify(`import { describe } from 'vitest';\ndescribe('group', () => {\n${longBody}\n});`),
    ).toHaveLength(1)
  })

  it('checks Vue script setup and TypeScript functions', () => {
    expect(
      verify(
        `<script setup lang="ts">\nfunction run(value: number) {\n${longBody}\n}\n</script>`,
        'Example.vue',
      ),
    ).toEqual([expect.objectContaining({ messageId: 'tooLong' })])
  })
})
