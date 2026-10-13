// @vitest-environment node
import postcss from 'postcss'
import { describe, expect, test } from 'vitest'
import {
  loadRetirementSources,
  validateRetirementSources,
  validateCanvasPurity,
  validateAccountPurity,
  type RetirementSources,
  type PurityModule,
} from '../../scripts/checkRetirementReplacements'

const valid = loadRetirementSources()
const app = 'src/app/style.css'
const theme = 'src/theme/theme.css'
const semantic = 'src/theme/tokens/semantic.css'
const admin = 'src/theme/tokens/admin.css'
const type = 'src/theme/tokens/type.css'
const canvas = 'src/theme/tokens/canvas.css'
const round = 'src/theme/atoms/round-control.css'
const chrome = 'src/theme/atoms/page-chrome.css'

function changeDeclaration(
  sources: RetirementSources,
  file: string,
  name: string,
  value: string | null,
  selector?: string,
) {
  const ast = postcss.parse(sources[file]!)
  let hasChanged = false
  ast.walkDecls(name, (decl) => {
    if (selector && (decl.parent?.type !== 'rule' || decl.parent.selector !== selector)) return
    if (hasChanged) return
    hasChanged = true
    if (value === null) decl.remove()
    else decl.value = value
  })
  if (!hasChanged) throw new Error(`Fixture declaration not found: ${file} ${name}`)
  sources[file] = ast.toString()
}

type Fixture = { rule: string; invalidate: (sources: RetirementSources) => void }
const fixtureCases: Fixture[] = [
  ...[
    ['token-closure', theme, '--color-destructive-soft'],
    ['token-alias', theme, '--color-canvas-text-control'],
    ['dark-required', semantic, '--color-home-hero-text'],
    ['dark-chart-alias', admin, '--color-admin-chart-axis-line'],
    ['primitive-closure', 'src/theme/tokens/primitives.css', '--primitive-neutral-950'],
  ].map(([rule, file, name]): Fixture => ({
    rule: rule!,
    invalidate: (sources) => {
      const ast = postcss.parse(sources[file!]!)
      let value = ''
      ast.walkDecls(name!, (decl) => {
        value = decl.value
        decl.remove()
      })
      ast.append(postcss.rule({ selector: '.unused-bait' }).append({ prop: name!, value }))
      sources[file!] = ast.toString()
    },
  })),
  ...[
    'false ? "page-chrome-surface" : "private-surface"',
    'isEnabled ? "page-chrome-surface" : "private-surface"',
    'false && "page-chrome-surface"',
    '{ "page-chrome-surface": false }',
    '{ "page-chrome-surface": isEnabled }',
    '[{ "page-chrome-surface": false }, "private-surface"]',
  ].map((expression): Fixture => ({
    rule: 'chrome-consumer',
    invalidate: (sources) => {
      sources['src/features/workspace/shell/components/WorkspaceNavigation.vue'] =
        `<template><nav :class='${expression}' /></template><script setup>const isEnabled = false</script>`
    },
  })),
  ...[
    ['ideaboard-color-mode', 'wrong-storage-key'],
    ['(prefers-color-scheme: dark)', '(prefers-color-scheme: light)'],
    ["classList.add('dark')", "classList.add('wrong-class')"],
  ].map(([original, replacement]): Fixture => ({
    rule: 'synchronous-theme-boot',
    invalidate: (sources) => {
      sources['index.html'] = sources['index.html']!.replace(original!, replacement!)
    },
  })),
  {
    rule: 'token-closure',
    invalidate: (s) => changeDeclaration(s, theme, '--color-destructive-soft', null),
  },
  {
    rule: 'scrollbar-role',
    invalidate: (s) => changeDeclaration(s, theme, '--color-scrollbar-thumb', 'transparent'),
  },
  {
    rule: 'token-namespace',
    invalidate: (s) => {
      s[app] += '\n@theme { --color-ib-scrollbar-thumb: black; }'
    },
  },
  {
    rule: 'token-alias',
    invalidate: (s) => changeDeclaration(s, theme, '--color-canvas-text-control', '#62626a'),
  },
  {
    rule: 'type-scale',
    invalidate: (s) => changeDeclaration(s, type, '--text-admin-body', '14px'),
  },
  {
    rule: 'type-scale',
    invalidate: (s) =>
      changeDeclaration(s, 'src/theme/tokens/primitives.css', '--primitive-type-size-13', '14px'),
  },
  {
    rule: 'literal-deduplication',
    invalidate: (s) => {
      s[theme] += '\n@theme { --color-extra-copy: #9278e6; }'
    },
  },
  {
    rule: 'bridge-fallback',
    invalidate: (s) => {
      const file = 'src/features/admin/shared/core/adminThemeTokens.ts'
      s[file] = s[file]!.replace(
        "'--color-admin-text-1', '#313131'",
        "'--color-admin-text-1', '#ffffff'",
      )
    },
  },
  {
    rule: 'token-uniqueness',
    invalidate: (s) => {
      s[admin] += '\nhtml.dark { --color-admin-bg: black; }'
    },
  },
  {
    rule: 'dark-base-closure',
    invalidate: (s) => {
      s[canvas] += '\nhtml.dark { --color-canvas-unregistered: black; }'
    },
  },
  {
    rule: 'dark-required',
    invalidate: (s) => changeDeclaration(s, semantic, '--color-home-hero-text', null),
  },
  {
    rule: 'dark-chart-alias',
    invalidate: (s) => {
      s[admin] = s[admin]!.replace(
        '--color-admin-chart-axis-line: var(--color-admin-border-hover)',
        '--color-admin-chart-axis-line: var(--color-admin-border)',
      )
    },
  },
  {
    rule: 'token-parity',
    invalidate: (s) => changeDeclaration(s, semantic, '--background', null, ':root'),
  },
  {
    rule: 'token-parity',
    invalidate: (s) => {
      s[semantic] += '\nhtml.dark { --new-only-dark: black; }'
    },
  },
  {
    rule: 'token-parity',
    invalidate: (s) => {
      s['src/theme/tokens/canvas.css'] += '\n:root { --color-canvas-new-only-light: black; }'
    },
  },
  {
    rule: 'primitive-closure',
    invalidate: (s) =>
      changeDeclaration(s, 'src/theme/tokens/primitives.css', '--primitive-neutral-950', null),
  },
  {
    rule: 'token-import-order',
    invalidate: (s) => {
      const ast = postcss.parse(s[theme]!)
      const firstImport = ast.nodes.find(
        (node): node is import('postcss').AtRule =>
          node.type === 'atrule' && node.name === 'import',
      )
      if (!firstImport) throw new Error('Fixture import not found: theme entry')
      firstImport.remove()
      ast.append(firstImport)
      s[theme] = ast.toString()
    },
  },
  {
    rule: 'chrome-surface',
    invalidate: (s) => changeDeclaration(s, chrome, 'background', 'white', '.page-chrome-surface'),
  },
  {
    rule: 'chrome-geometry',
    invalidate: (s) => {
      const ast = postcss.parse(s[chrome]!)
      ast.walkRules('.page-chrome-surface', (rule) => {
        rule.append({ prop: 'padding', value: '1rem' })
      })
      s[chrome] = ast.toString()
    },
  },
  {
    rule: 'round-semantic',
    invalidate: (s) =>
      changeDeclaration(
        s,
        round,
        '--round-control-bg',
        'var(--color-canvas-surface-bg)',
        '.round-control',
      ),
  },
  {
    rule: 'round-chrome-state',
    invalidate: (s) =>
      changeDeclaration(s, round, '--round-control-color-disabled', null, '.page-chrome-control'),
  },
  {
    rule: 'chrome-state-owner',
    invalidate: (s) => {
      s[chrome] += '\n.page-chrome-surface { --round-control-border: red; }'
    },
  },
  {
    rule: 'feature-surface-owner',
    invalidate: (s) => {
      s['src/features/search/ui/search.css'] +=
        '\n.duplicate { box-shadow: var(--shadow-page-chrome); }'
    },
  },
  {
    rule: 'chrome-consumer',
    invalidate: (s) => {
      const file = 'src/features/board/ui/gallery/skin.ts'
      s[file] = s[file]!.replaceAll('page-chrome-surface', 'private-surface')
    },
  },
  {
    rule: 'chrome-consumer',
    invalidate: (s) => {
      s['src/features/workspace/shell/components/WorkspaceNavigation.vue'] =
        '<template><nav title="page-chrome-surface" /></template>'
    },
  },
  {
    rule: 'native-chrome-control',
    invalidate: (s) => {
      const file = 'src/features/account/ui/chrome/UserCreditBalanceButton.vue'
      s[file] = s[file]!.replace('<button', '<UiButton').replace('</button>', '</UiButton>')
    },
  },
  {
    rule: 'readonly-controller',
    invalidate: (s) => {
      s['src/features/board/ui/gallery/BoardsPage.vue'] =
        '<template><input v-model="controller.viewModel.sort" /></template>'
    },
  },
  {
    rule: 'lightbox-shared-skin',
    invalidate: (s) => {
      const file = 'src/features/library/ui/LibraryAssetLightbox.vue'
      s[file] = s[file]!.replace(':class="lightboxScrimLinkClass"', 'class="bg-white/10"')
    },
  },
  {
    rule: 'persistent-form-error',
    invalidate: (s) => {
      s['src/features/auth/components/auth-fields/EmailField.vue'] =
        '<template><input :placeholder="inlineError || label" /></template>'
    },
  },
  {
    rule: 'persistent-form-error',
    invalidate: (s) => {
      s['src/features/auth/components/auth-fields/EmailField.vue'] =
        '<template><input :placeholder="fieldError || label" /></template>'
    },
  },
  {
    rule: 'announced-form-error',
    invalidate: (s) => {
      const file = 'src/core/components/IbFormField.vue'
      s[file] = s[file]!.replace('role="alert"', 'role="presentation"')
    },
  },
  {
    rule: 'announced-form-error',
    invalidate: (s) => {
      const file = 'src/core/components/IbFormField.vue'
      s[file] = s[file]!.replace('v-if="!showMessage"', 'v-if="showMessage"')
    },
  },
  {
    rule: 'synchronous-theme-boot',
    invalidate: (s) => {
      s['index.html'] = s['index.html']!.replace('<script>', '<script async>')
    },
  },
  {
    rule: 'synchronous-theme-boot',
    invalidate: (s) => {
      s['index.html'] =
        '<html><head><script>document.documentElement.classList.add("dark")</script></head><body></body></html>'
    },
  },
]

describe('effective retirement lint policies', () => {
  test('accepts canonical declarations, runtime bridge values and executable class bindings', () => {
    expect(validateRetirementSources(valid)).toEqual([])
  })
  test.each([
    app,
    theme,
    'src/theme/base.css',
    semantic,
    admin,
    type,
    'src/theme/tokens/z.css',
    'src/theme/tokens/motion.css',
    'src/theme/tokens/canvas.css',
    'src/theme/tokens/primitives.css',
  ])('rejects retired namespace declarations in token owner %s', (file) => {
    for (const name of [
      '--color-ib-surface',
      '--text-ib-caption',
      '--shadow-ib-overlay',
      '--color-dialog-overlay',
    ]) {
      const fixture = { ...valid }
      fixture[file] += `\n@layer base { :root { ${name}: black; } }`
      expect(validateRetirementSources(fixture)).toContainEqual({
        rule: 'token-namespace',
        file,
        message: `Retired namespace ${name}`,
      })
    }
  })
  test('accepts applied object and array classes when their constant predicates are true', () => {
    for (const expression of [
      '{ "page-chrome-surface": true }',
      '[{ "page-chrome-surface": isEnabled }, false && "unused"]',
    ]) {
      const fixture = { ...valid }
      fixture['src/features/workspace/shell/components/WorkspaceNavigation.vue'] =
        `<template><nav :class='${expression}' /></template><script setup>const isEnabled = true</script>`
      expect(validateRetirementSources(fixture)).toEqual([])
    }
  })
  test('accepts harmless comments, declaration formatting and CSS variable whitespace', () => {
    const fixture = { ...valid }
    fixture[theme] = postcss
      .parse(fixture[theme]!)
      .toString()
      .replace('var(--color-canvas-text-secondary)', 'var( --color-canvas-text-secondary )')
    fixture[app] += '\n/* --color-ib-scrollbar-x: black; missing fake tokens do not count */'
    fixture['src/features/board/ui/gallery/BoardsPage.vue'] +=
      '\n<!-- <input v-model="controller.viewModel.sort" /> -->'
    expect(validateRetirementSources(fixture)).toEqual([])
  })
  test.each(fixtureCases)('rejects a semantic violation of $rule', ({ rule, invalidate }) => {
    const fixture = { ...valid }
    invalidate(fixture)
    expect(validateRetirementSources(fixture).map((issue) => issue.rule)).toContain(rule)
  })
})

const core = 'src/features/canvas/tools/retouch/core/value.ts'
const pure = 'src/core/contracts/generationContract.ts'
const runtimeTargets = [
  'vue',
  'vue-router',
  'vue-i18n',
  'pinia',
  '@tanstack/vue-query',
  '@vue-flow/core',
  'ofetch',
  'axios',
  'better-auth/vue',
  'src/api/generated-query/public/index.ts',
  'src/features/canvas/tools/retouch/session/session.ts',
  'src/features/canvas/session/board-session/useBoardAutosave.ts',
  'src/core/auth/client.ts',
  'src/core/query/queryClient.ts',
  'src/core/i18n/index.ts',
]
describe('Canvas runtime dependency graph policy', () => {
  test('accepts pure contracts and type-only references while walking cyclic pure graphs', () => {
    expect(
      validateCanvasPurity([
        {
          source: core,
          dependencies: [
            { module: pure, resolved: pure },
            { module: 'vue', dependencyTypes: ['type-only'] },
            { module: 'src/core/i18n/localeCore.ts' },
          ],
        },
        { source: pure, dependencies: [{ module: core }] },
      ]),
    ).toEqual([])
  })
  test.each(runtimeTargets)('rejects direct runtime dependency %s', (target) => {
    expect(
      validateCanvasPurity([
        { source: core, dependencies: [{ module: target, resolved: target }] },
      ]).map((issue) => issue.rule),
    ).toContain('canvas-purity')
  })
  test.each(runtimeTargets)(
    'rejects transitive runtime dependency through a pure-looking barrel to %s',
    (target) => {
      const fixture: PurityModule[] = [
        { source: core, dependencies: [{ module: pure }] },
        { source: pure, dependencies: [{ module: target }] },
      ]
      expect(validateCanvasPurity(fixture).map((issue) => issue.rule)).toContain('canvas-purity')
    },
  )
  test('rejects a missing graph instead of silently passing empty core coverage', () => {
    expect(validateCanvasPurity([]).map((issue) => issue.rule)).toContain('canvas-purity')
  })
  test('rejects a relocated graph application runtime inside domain', () => {
    expect(
      validateCanvasPurity([
        { source: core },
        { source: 'src/features/canvas/core/graph/application/runtime.ts' },
      ]).map((issue) => issue.rule),
    ).toContain('canvas-runtime-location')
  })
})

describe('Account runtime dependency graph policy', () => {
  const accountCore = 'src/features/account/core/money.ts'
  test.each(runtimeTargets)(
    'rejects direct and transitive runtime dependencies on %s',
    (target) => {
      for (const intermediary of [accountCore, pure]) {
        const modules: PurityModule[] = [
          { source: accountCore, dependencies: [{ module: intermediary }] },
          { source: intermediary, dependencies: [{ module: target }] },
        ]
        expect(validateAccountPurity(modules).map((issue) => issue.rule)).toContain(
          'account-purity',
        )
      }
    },
  )
  test('accepts pure core cycles and type-only runtime references', () => {
    expect(
      validateAccountPurity([
        {
          source: accountCore,
          dependencies: [{ module: pure }, { module: 'vue', dependencyTypes: ['type-only'] }],
        },
        { source: pure, dependencies: [{ module: accountCore }] },
      ]),
    ).toEqual([])
  })
  test('rejects DOM globals in direct sources and transitive helpers', () => {
    const modules = [{ source: accountCore, dependencies: [{ module: pure }] }, { source: pure }]
    for (const source of [accountCore, pure])
      for (const expression of [
        'getComputedStyle(document.documentElement)',
        'globalThis.document.documentElement',
        'globalThis["document"].documentElement',
      ])
        expect(
          validateAccountPurity(modules, {
            [source]: `export const style = ${expression}`,
          }).map((issue) => issue.rule),
        ).toContain('account-purity')
    expect(
      validateAccountPurity(modules, {
        [accountCore]:
          'export type Styles = CSSStyleDeclaration; // document\nexport const key = "window"',
      }),
    ).toEqual([])
  })
  test('rejects missing Account coverage', () => {
    expect(validateAccountPurity([]).map((issue) => issue.rule)).toContain('account-purity')
  })
})
