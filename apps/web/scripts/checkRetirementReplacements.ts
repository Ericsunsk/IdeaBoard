import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'
import vm from 'node:vm'
import postcss, { type Root } from 'postcss'
import { parse as parseSfc } from '@vue/compiler-sfc'
import { baseParse, NodeTypes, type ElementNode } from '@vue/compiler-dom'
import ts from 'typescript'

export type RetirementSources = Record<string, string>
export type RetirementViolation = { rule: string; file: string; message: string }
const webRoot = fileURLToPath(new URL('..', import.meta.url))
const app = 'src/app/style.css'
const themeEntry = 'src/theme/theme.css'
const tokens = 'src/theme/tokens/'
const tokenOwners = [
  app,
  themeEntry,
  'src/theme/base.css',
  ...[
    'semantic.css',
    'admin.css',
    'canvas.css',
    'primitives.css',
    'motion.css',
    'type.css',
    'z.css',
  ].map((file) => tokens + file),
]
const chrome = 'src/theme/atoms/page-chrome.css'
const round = 'src/theme/atoms/round-control.css'
const bridge = 'src/features/admin/shared/core/adminThemeTokens.ts'

const requiredTokens = [
  '--color-scrollbar-thumb',
  '--color-destructive-soft',
  '--color-admin-status-wash',
  ...['default', 'pending', 'running', 'success', 'danger'].flatMap((status) => [
    `--color-admin-status-${status}-glow`,
    `--shadow-admin-status-dot-${status}`,
  ]),
  ...['purple', 'orange', 'blue', 'rose'].map((provider) => `--color-provider-${provider}`),
  '--color-scrim',
  '--color-scrim-soft',
  ...[
    'faint',
    'faint-soft',
    'faint-strong',
    'faintest',
    'line',
    'line-strong',
    'line-emphasis',
    'line-bold',
  ].map((tier) => `--color-primary-${tier}`),
  ...['dialog', 'drawer', 'popover'].map((role) => `--shadow-admin-${role}`),
  ...['sticky', 'header', 'overlay', 'dialog'].map((role) => `--z-index-admin-${role}`),
  ...['content', 'popover', 'banner', 'lightbox'].map((role) => `--z-index-${role}`),
  ...['strong', 'emphasis', 'display'].map((role) => `--font-weight-admin-${role}`),
  ...['shell', 'popover', 'drawer', 'empty-state'].map((role) => `--spacing-admin-${role}`),
  '--color-overlay-btn-bg',
  '--color-overlay-btn-bg-hover',
  '--color-canvas-accent-wash',
  '--color-dialog-mask',
  ...['bg', 'border', 'item', 'item-hover-bg', 'danger', 'danger-hover-bg'].map(
    (role) => `--color-overlay-${role}`,
  ),
  '--shadow-overlay-menu',
]
const aliases: Record<string, string> = {
  '--color-canvas-text-control': '--color-text-control',
  '--color-canvas-text-option': '--color-text-option',
  '--color-admin-chart-tooltip-border': '--color-admin-border',
  '--color-admin-chart-axis-line': '--color-admin-border',
  '--color-admin-chart-split-line': '--color-admin-surface-hover',
  '--color-admin-accent': '--color-brand-creative-accent',
  '--color-admin-model-purple': '--color-brand-creative-accent',
  '--color-canvas-accent': '--color-tool-accent',
  '--color-admin-accent-hover': '--color-brand-creative-accent-hover',
  '--color-admin-accent-pressed': '--color-brand-creative-accent-pressed',
  '--color-canvas-accent-soft': '--color-tool-accent-soft',
  '--color-admin-chart-legend-text': '--color-admin-text-2',
  '--color-board-create-title': '--color-board-card-title',
  '--color-board-control-text': '--color-board-card-title',
  '--color-board-control-input-text': '--color-board-card-title',
  '--color-board-dialog-title': '--color-board-control-input-text',
}
const typeScale = {
  micro: 10,
  caption: 11,
  meta: 12,
  body: 13,
  content: 14,
  'title-sm': 15,
  title: 16,
  heading: 18,
  icon: 20,
  'modal-title-mobile': 22,
  'modal-title': 26,
  'summary-value': 28,
}
const darkRequired = [
  ...['hero-text', 'hero-subtle', 'note-bg', 'note-border', 'note-text'].map(
    (role) => `--color-home-${role}`,
  ),
  ...[
    'workspace-bg',
    'surface-bg',
    'neutral-line',
    'dot',
    'edge',
    'edge-selected',
    'node-bg',
    'node-border',
    'text-primary',
    'text-secondary',
    'text-muted',
  ].map((role) => `--color-canvas-${role}`),
  ...[
    'page',
    'page-text',
    'page-text-muted',
    'surface',
    'surface-strong',
    'control-bg',
    'card-border',
    'card-bg',
    'card-title',
    'card-text',
  ].map((role) => `--color-board-${role}`),
]
const primitiveRequired = [
  ...['0', '50', '100', '800', '900', '950'].map((role) => `--primitive-neutral-${role}`),
  '--primitive-violet-500',
  '--primitive-violet-650',
  ...['8', '12', '16', 'pill'].map((role) => `--primitive-radius-${role}`),
  ...['sm', 'md', 'lg'].map((role) => `--primitive-shadow-${role}`),
]
const chromeConsumers = [
  'features/workspace/shell/components/WorkspaceNavigation.vue',
  'features/search/ui/pages/SearchPage.vue',
  'features/search/ui/components/SearchBatchActionDock.vue',
  'features/board/ui/gallery/skin.ts',
  'features/library/ui/LibraryFloatingIsland.vue',
  'features/canvas/ui/chrome/components/CanvasChrome.vue',
  'features/canvas/ui/chrome/components/CanvasChromeBoardHeader.vue',
  'features/canvas/ui/chrome/CanvasMiniMap.vue',
  'features/account/ui/chrome/UserAccountMenu.vue',
  'features/account/ui/chrome/UserCreditBalanceButton.vue',
].map((file) => `src/${file}`)
const featureSkins = [
  'src/features/search/ui/search.css',
  'src/features/canvas/ui/styles/canvas-chrome.css',
  'src/features/canvas/ui/styles/canvas-vue-flow-adapter-skin.css',
]
const lightboxes = [
  'src/features/search/ui/components/SearchLightboxModal.vue',
  'src/features/library/ui/LibraryAssetLightbox.vue',
]
const formFields = [
  'src/features/auth/components/auth-fields/EmailField.vue',
  'src/features/auth/components/auth-fields/PasswordField.vue',
  'src/features/auth/components/auth-fields/OtpField.vue',
  'src/features/auth/components/RegisterAuthPanel.vue',
  'src/core/components/IbFormField.vue',
]

export function loadRetirementSources(root = webRoot): RetirementSources {
  const files = [
    ...tokenOwners,
    chrome,
    round,
    bridge,
    'index.html',
    ...chromeConsumers,
    ...featureSkins,
    ...lightboxes,
    ...formFields,
    'src/features/board/ui/gallery/BoardsPage.vue',
  ]
  return Object.fromEntries(
    files.map((file) => [file, fs.readFileSync(path.join(root, file), 'utf8')]),
  )
}

function declarations(
  root: Root,
  filter: (name: string) => boolean = () => true,
  scope?: 'theme' | 'root' | 'dark',
): Map<string, string> {
  const result = new Map<string, string>()
  root.walkDecls((decl) => {
    const parent = decl.parent
    const inScope =
      scope === undefined ||
      (scope === 'theme' && parent?.type === 'atrule' && parent.name === 'theme') ||
      (parent?.type === 'rule' &&
        (scope === 'root'
          ? parent.selector.trim() === ':root'
          : scope === 'dark' &&
            parent.selector
              .split(',')
              .some((selector) => /^(?:html)?\.dark$/.test(selector.trim()))))
    if (inScope && filter(decl.prop)) result.set(decl.prop, decl.value.trim())
  })
  return result
}
function normalize(value: string): string {
  return value.replace(/\s+/g, '').toLowerCase()
}
function elements(source: string): ElementNode[] {
  const template = parseSfc(source).descriptor.template?.content ?? source
  const found: ElementNode[] = []
  const visit = (node: ReturnType<typeof baseParse> | ElementNode) => {
    for (const child of node.children) {
      if (child.type === NodeTypes.ELEMENT) {
        found.push(child)
        visit(child)
      }
    }
  }
  visit(baseParse(template))
  return found
}
function constantBoolean(
  node: ts.Node,
  constants: ReadonlyMap<string, boolean>,
): boolean | undefined {
  if (node.kind === ts.SyntaxKind.TrueKeyword) return true
  if (node.kind === ts.SyntaxKind.FalseKeyword) return false
  if (ts.isIdentifier(node)) return constants.get(node.text)
  if (ts.isParenthesizedExpression(node) || ts.isAsExpression(node))
    return constantBoolean(node.expression, constants)
  if (ts.isPrefixUnaryExpression(node) && node.operator === ts.SyntaxKind.ExclamationToken) {
    const value = constantBoolean(node.operand, constants)
    return value === undefined ? undefined : !value
  }
  return undefined
}
function booleanConstants(source: string): Map<string, boolean> {
  const constants = new Map<string, boolean>()
  const ast = ts.createSourceFile('fixture.ts', source, ts.ScriptTarget.Latest, true)
  for (const statement of ast.statements) {
    if (
      !ts.isVariableStatement(statement) ||
      !(statement.declarationList.flags & ts.NodeFlags.Const)
    )
      continue
    for (const declaration of statement.declarationList.declarations) {
      if (!ts.isIdentifier(declaration.name) || !declaration.initializer) continue
      const value = constantBoolean(declaration.initializer, constants)
      if (value !== undefined) constants.set(declaration.name.text, value)
    }
  }
  return constants
}
function stringValues(
  source: string,
  constants: ReadonlyMap<string, boolean> = booleanConstants(source),
  isClassExpression = false,
): string[] {
  const values: string[] = []
  const visit = (node: ts.Node) => {
    if (isClassExpression && ts.isPropertyAssignment(node)) {
      if (constantBoolean(node.initializer, constants) !== false) {
        if (ts.isStringLiteralLike(node.name) || ts.isIdentifier(node.name))
          values.push(node.name.text)
      }
      // Object class values enable the key; they never supply a class themselves.
      return
    }
    if (ts.isConditionalExpression(node)) {
      const condition = constantBoolean(node.condition, constants)
      if (condition !== undefined) {
        visit(condition ? node.whenTrue : node.whenFalse)
        return
      }
    }
    if (
      ts.isBinaryExpression(node) &&
      [ts.SyntaxKind.AmpersandAmpersandToken, ts.SyntaxKind.BarBarToken].includes(
        node.operatorToken.kind,
      )
    ) {
      const left = constantBoolean(node.left, constants)
      if (left !== undefined) {
        if ((node.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken) === left)
          visit(node.right)
        return
      }
    }
    if (ts.isStringLiteralLike(node)) values.push(node.text)
    ts.forEachChild(node, visit)
  }
  visit(
    ts.createSourceFile(
      'fixture.ts',
      isClassExpression ? `const classBinding = (${source})` : source,
      ts.ScriptTarget.Latest,
      true,
    ),
  )
  return values
}

/** Semantic CSS/SFC/HTML policies run against real sources and fixture workspaces alike. */
export function validateRetirementSources(sources: RetirementSources): RetirementViolation[] {
  const issues: RetirementViolation[] = []
  const fail = (rule: string, file: string, message: string) => {
    issues.push({ rule, file, message })
  }
  const stylesheets = new Map<string, Root>()
  const css = (file: string): Root => {
    let root = stylesheets.get(file)
    if (!root) {
      root = postcss.parse(sources[file] ?? '', { from: file })
      stylesheets.set(file, root)
    }
    return root
  }
  const themeScopeFiles = [themeEntry, tokens + 'admin.css', tokens + 'type.css', tokens + 'z.css']
  const base = new Map<string, string>()
  for (const file of themeScopeFiles)
    for (const [name, value] of declarations(css(file), () => true, 'theme')) base.set(name, value)
  for (const [name, value] of declarations(css(tokens + 'semantic.css'), () => true, 'root'))
    base.set(name, value)
  // Bridge fallbacks resolve through Tier 2 aliases down to Tier 1 leaves.
  const resolveScope = new Map(base)
  for (const [name, value] of declarations(css(tokens + 'primitives.css'), () => true, 'root')) {
    if (!resolveScope.has(name)) resolveScope.set(name, value)
  }
  const resolveToken = (name: string, seen = new Set<string>()): string | undefined => {
    if (seen.has(name)) return undefined
    seen.add(name)
    const value = resolveScope.get(name),
      alias = value?.match(/^var\(\s*(--[\w-]+)\s*\)$/)?.[1]
    return alias ? resolveToken(alias, seen) : value
  }
  const darkScopeFiles = [tokens + 'admin.css', tokens + 'semantic.css', tokens + 'canvas.css']
  const dark = new Map<string, string>()
  for (const file of darkScopeFiles)
    for (const [name, value] of declarations(css(file), () => true, 'dark')) dark.set(name, value)
  const requireKeys = (
    rule: string,
    file: string,
    values: Map<string, string>,
    names: string[],
  ) => {
    for (const name of names) if (!values.get(name)) fail(rule, file, `Missing nonempty ${name}`)
  }
  const requireValues = (
    rule: string,
    file: string,
    values: Map<string, string>,
    expected: Record<string, string>,
  ) => {
    for (const [name, value] of Object.entries(expected))
      if (normalize(values.get(name) ?? '') !== normalize(value))
        fail(rule, file, `${name} must resolve through ${value}`)
  }
  requireKeys('token-closure', themeEntry, base, requiredTokens)
  requireValues('scrollbar-role', themeEntry, base, {
    '--color-scrollbar-thumb': 'color-mix(in srgb, var(--foreground) 58%, transparent)',
  })
  for (const file of tokenOwners) {
    css(file).walkDecls((decl) => {
      if (
        /^--(?:color-ib-|text-ib-|shadow-ib-(?:control|overlay|focus)|color-dialog-overlay)/.test(
          decl.prop,
        )
      )
        fail('token-namespace', file, `Retired namespace ${decl.prop}`)
    })
  }
  requireValues(
    'token-alias',
    themeEntry,
    base,
    Object.fromEntries(Object.entries(aliases).map(([name, target]) => [name, `var(${target})`])),
  )
  for (const [name, size] of Object.entries(typeScale)) {
    if (normalize(resolveToken(`--text-admin-${name}`) ?? '') !== `${size}px`)
      fail('type-scale', tokens + 'type.css', `--text-admin-${name} must resolve to ${size}px`)
  }
  // Physical color literals have one owner; semantic declarations reference it.
  for (const file of [themeEntry, tokens + 'admin.css']) {
    css(file).walkDecls((decl) => {
      if (decl.prop.startsWith('--') && /^#[a-f\d]{6}$/i.test(decl.value)) {
        fail('literal-deduplication', file, `${decl.prop} must reference a Tier 1 color`)
      }
    })
  }
  const bridgeSource = ts.createSourceFile(
    bridge,
    sources[bridge] ?? '',
    ts.ScriptTarget.Latest,
    true,
  )
  const bridged = new Set<string>()
  const visitBridge = (node: ts.Node) => {
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === 'defineThemeToken'
    ) {
      const [token, fallback] = node.arguments
      if (
        token &&
        fallback &&
        ts.isStringLiteral(token) &&
        ts.isStringLiteral(fallback) &&
        [
          '--color-admin-text-1',
          '--color-admin-text-label',
          '--color-admin-text-2',
          '--color-admin-text-3',
        ].includes(token.text)
      ) {
        bridged.add(token.text)
        if (normalize(resolveToken(token.text) ?? '') !== normalize(fallback.text))
          fail('bridge-fallback', bridge, `${token.text} fallback differs from CSS`)
      }
    }
    ts.forEachChild(node, visitBridge)
  }
  visitBridge(bridgeSource)
  for (const name of [
    '--color-admin-text-1',
    '--color-admin-text-label',
    '--color-admin-text-2',
    '--color-admin-text-3',
  ])
    if (!bridged.has(name)) fail('bridge-fallback', bridge, `Missing bridge ${name}`)
  const scopedTokens = (
    root: Root,
    scope: 'root' | 'dark',
    prefix: RegExp,
  ): Map<string, string> => {
    const values = new Map<string, string>()
    root.walkRules((rule) => {
      if (
        scope === 'dark'
          ? !rule.selector.split(',').some((selector) => /^(?:html)?\.dark$/.test(selector.trim()))
          : rule.selector !== ':root'
      )
        return
      rule.walkDecls((decl) => {
        if (decl.parent !== rule) return
        if (!prefix.test(decl.prop)) return
        if (values.has(decl.prop))
          fail(
            'token-uniqueness',
            root.source?.input.file ?? 'css',
            `${decl.prop} is redeclared in ${scope}`,
          )
        values.set(decl.prop, decl.value)
      })
    })
    return values
  }
  const parity = (file: string, a: Map<string, string>, b: Map<string, string>) => {
    if (
      !a.size ||
      !b.size ||
      [...a.keys()].some((name) => !b.has(name)) ||
      [...b.keys()].some((name) => !a.has(name))
    )
      fail('token-parity', file, 'Light and dark scopes must declare identical nonempty token sets')
  }
  const darkDomain = new Map<string, string>()
  for (const file of darkScopeFiles)
    for (const [name, value] of scopedTokens(
      css(file),
      'dark',
      /^--color-(?:admin|board|canvas|home|auth|page-chrome|overlay|toast|brand-creative|page|dialog|scrollbar)-/,
    ).entries())
      darkDomain.set(name, value)
  if (!darkDomain.size || [...darkDomain.keys()].some((name) => !base.has(name)))
    fail(
      'dark-base-closure',
      tokens + 'semantic.css',
      'Every dark domain token must exist in app theme',
    )
  requireKeys('dark-required', tokens + 'semantic.css', dark, darkRequired)
  requireValues('dark-chart-alias', tokens + 'admin.css', dark, {
    '--color-admin-chart-axis-line': 'var(--color-admin-border-hover)',
    '--color-admin-chart-split-line': 'var(--color-admin-border)',
  })
  const semanticRoot = css(tokens + 'semantic.css')
  parity(
    tokens + 'semantic.css',
    declarations(
      semanticRoot,
      (name) =>
        name.startsWith('--') && !name.startsWith('--color-') && !name.startsWith('--shadow-'),
      'root',
    ),
    declarations(
      semanticRoot,
      (name) =>
        name.startsWith('--') && !name.startsWith('--color-') && !name.startsWith('--shadow-'),
      'dark',
    ),
  )
  const canvas = css(tokens + 'canvas.css')
  parity(
    tokens + 'canvas.css',
    scopedTokens(canvas, 'root', /^--color-canvas-/),
    scopedTokens(canvas, 'dark', /^--color-canvas-/),
  )
  requireKeys(
    'primitive-closure',
    tokens + 'primitives.css',
    declarations(css(tokens + 'primitives.css'), () => true, 'root'),
    primitiveRequired,
  )
  const imports: string[] = []
  css(themeEntry).walkAtRules('import', (rule) => {
    imports.push(rule.params.replace(/^['"]|['"]$/g, ''))
  })
  if (
    imports.join('|') !==
    [
      'tailwindcss',
      './tokens/primitives.css',
      './tokens/semantic.css',
      './tokens/admin.css',
      './tokens/canvas.css',
      './tokens/motion.css',
      './tokens/type.css',
      './tokens/z.css',
    ].join('|')
  )
    fail('token-import-order', themeEntry, 'Token layers must load in their declared order')
  const surface = new Map<string, string>()
  css(chrome).walkRules((rule) => {
    if (rule.selector === '.page-chrome-surface')
      rule.walkDecls((decl) => {
        surface.set(decl.prop, decl.value)
      })
  })
  requireValues('chrome-surface', chrome, surface, {
    border: '1px solid var(--color-page-chrome-border)',
    background: 'var(--color-page-chrome-bg)',
    'box-shadow': 'var(--shadow-page-chrome)',
    'backdrop-filter': 'blur(var(--blur-glass))',
  })
  for (const name of surface.keys())
    if (/^(?:border-radius|(?:min-|max-)?(?:width|height)|padding(?:-|$))/.test(name))
      fail('chrome-geometry', chrome, `Shared surface cannot own ${name}`)
  const roundBase = new Map<string, string>(),
    roundChrome = new Map<string, string>()
  css(round).walkRules((rule) => {
    if (['.round-control', '.page-chrome-control'].includes(rule.selector))
      rule.walkDecls((decl) => {
        ;(rule.selector === '.round-control' ? roundBase : roundChrome).set(decl.prop, decl.value)
      })
  })
  requireValues('round-semantic', round, roundBase, {
    '--round-control-border': 'var(--border)',
    '--round-control-bg': 'var(--card)',
    '--round-control-color': 'var(--muted-foreground)',
    '--round-control-focus-ring': 'var(--ring)',
    '--round-control-shadow': 'var(--shadow-raised)',
  })
  const roundAliases: Record<string, string> = {
    border: 'border',
    'border-hover': 'border-hover',
    bg: 'bg',
    'bg-hover': 'bg-hover',
    'bg-disabled': 'bg',
    color: 'color',
    'color-hover': 'color-hover',
    'color-disabled': 'color-disabled',
    'focus-ring': 'focus-ring',
  }
  requireValues('round-chrome-state', round, roundChrome, {
    ...Object.fromEntries(
      Object.entries(roundAliases).map(([name, target]) => [
        `--round-control-${name}`,
        `var(--color-page-chrome-${target})`,
      ]),
    ),
    '--round-control-shadow': 'var(--shadow-page-chrome)',
  })
  css(round).walkDecls((decl) => {
    if (/--color-(?:board|canvas)-|--control-/.test(decl.prop + ' ' + decl.value))
      fail('round-semantic', round, 'Round controls cannot use feature color families')
  })
  css(chrome).walkDecls((decl) => {
    if (/--round-control-/.test(decl.prop + ' ' + decl.value))
      fail('chrome-state-owner', chrome, 'Round control file owns state mappings')
  })
  for (const file of featureSkins)
    css(file).walkDecls((decl) => {
      if (
        (decl.prop === 'box-shadow' && normalize(decl.value) === 'var(--shadow-page-chrome)') ||
        (decl.prop === 'border' &&
          normalize(decl.value) === '1pxsolidvar(--color-page-chrome-border)')
      )
        fail('feature-surface-owner', file, 'Consume page-chrome-surface instead of redeclaring it')
      if (
        file.includes('canvas-chrome.css') &&
        (decl.prop === '--round-control-border' || decl.prop.startsWith('--page-chrome-account-'))
      )
        fail('chrome-state-owner', file, 'Canvas cannot redefine shared chrome state')
    })
  for (const file of chromeConsumers) {
    const descriptor = file.endsWith('.vue') ? parseSfc(sources[file] ?? '').descriptor : undefined
    const constants = booleanConstants(
      (descriptor?.script?.content ?? '') + (descriptor?.scriptSetup?.content ?? ''),
    )
    const source = sources[file] ?? '',
      strings = file.endsWith('.vue')
        ? elements(source).flatMap((el) =>
            el.props.flatMap((prop) => {
              if (prop.type === NodeTypes.ATTRIBUTE)
                return prop.name === 'class' ? [prop.value?.content ?? ''] : []
              return prop.name === 'bind' &&
                prop.arg?.type === NodeTypes.SIMPLE_EXPRESSION &&
                prop.arg.content === 'class' &&
                prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION
                ? stringValues(prop.exp.content, constants, true)
                : []
            }),
          )
        : stringValues(source)
    if (!strings.some((value) => value.split(/\s+/).includes('page-chrome-surface')))
      fail('chrome-consumer', file, 'Use the shared surface class in executable class bindings')
    if (file.includes('/account/ui/chrome/')) {
      const descriptor = parseSfc(source).descriptor
      const script = ts.createSourceFile(
        file,
        (descriptor.script?.content ?? '') + (descriptor.scriptSetup?.content ?? ''),
        ts.ScriptTarget.Latest,
        true,
      )
      if (
        elements(source).some((el) => el.tag === 'UiButton') ||
        script.statements.some(
          (node) =>
            ts.isImportDeclaration(node) &&
            ts.isStringLiteral(node.moduleSpecifier) &&
            node.moduleSpecifier.text.endsWith('/components/ui/button'),
        )
      )
        fail(
          'native-chrome-control',
          file,
          'Fully skinned account controls must use native buttons',
        )
    }
  }
  const boards = 'src/features/board/ui/gallery/BoardsPage.vue'
  if (
    elements(sources[boards] ?? '').some((el) =>
      el.props.some((prop) => prop.type === NodeTypes.DIRECTIVE && prop.name === 'model'),
    )
  )
    fail('readonly-controller', boards, 'Readonly controller values need explicit commands')
  for (const file of lightboxes) {
    const source = sources[file] ?? '',
      nodes = elements(source),
      bindings = nodes.flatMap((el) =>
        el.props.flatMap((prop) =>
          prop.type === NodeTypes.DIRECTIVE && prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION
            ? [prop.exp.content]
            : [],
        ),
      )
    for (const name of ['lightboxScrimLinkClass', 'lightboxScrimIconButtonClass'])
      if (!bindings.some((binding) => binding === name))
        fail('lightbox-shared-skin', file, `Bind ${name}`)
    for (const el of nodes)
      for (const prop of el.props) {
        const value =
          prop.type === NodeTypes.ATTRIBUTE
            ? (prop.value?.content ?? '')
            : prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION
              ? stringValues(prop.exp.content).join(' ')
              : ''
        if (value.split(/\s+/).some((token) => ['bg-white/10', 'border-white/15'].includes(token)))
          fail('lightbox-shared-skin', file, 'Lightbox consumers must not duplicate the scrim skin')
      }
  }
  for (const file of formFields)
    for (const el of elements(sources[file] ?? ''))
      for (const prop of el.props)
        if (
          prop.type === NodeTypes.DIRECTIVE &&
          prop.name === 'bind' &&
          prop.arg?.type === NodeTypes.SIMPLE_EXPRESSION &&
          prop.arg.content === 'placeholder' &&
          prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION &&
          /(?:error|invalid)/i.test(prop.exp.content)
        )
          fail('persistent-form-error', file, 'An error must not become placeholder text')
  const field = 'src/core/components/IbFormField.vue',
    fieldNodes = elements(sources[field] ?? '')
  const errorBranch = fieldNodes.find((el) =>
    el.props.some(
      (prop) =>
        prop.type === NodeTypes.DIRECTIVE &&
        prop.name === 'else-if' &&
        prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION &&
        prop.exp.content === 'invalid',
    ),
  )
  let earlierBranchHidesError = false
  if (errorBranch) {
    const siblings = fieldNodes.filter((el) => el.loc.end.offset <= errorBranch.loc.start.offset)
    const previous = siblings.at(-1)
    const condition = previous?.props.find(
      (prop) => prop.type === NodeTypes.DIRECTIVE && ['if', 'else-if'].includes(prop.name),
    )
    if (
      condition?.type === NodeTypes.DIRECTIVE &&
      condition.exp?.type === NodeTypes.SIMPLE_EXPRESSION
    ) {
      try {
        earlierBranchHidesError = Boolean(
          vm.runInNewContext(
            condition.exp.content,
            { invalid: true, showMessage: true, shouldShowMessage: true },
            { timeout: 100 },
          ),
        )
      } catch {
        earlierBranchHidesError = true
      }
    }
  }
  if (
    !errorBranch ||
    earlierBranchHidesError ||
    !elements(`<template>${errorBranch.loc.source}</template>`).some((el) =>
      el.props.some(
        (prop) =>
          prop.type === NodeTypes.ATTRIBUTE &&
          prop.name === 'role' &&
          prop.value?.content === 'alert',
      ),
    ) ||
    fieldNodes.some((el) =>
      el.props.some(
        (prop) =>
          prop.type === NodeTypes.DIRECTIVE &&
          prop.name === 'else-if' &&
          prop.exp?.type === NodeTypes.SIMPLE_EXPRESSION &&
          prop.exp.content === 'shouldShowMessage',
      ),
    )
  )
    fail('announced-form-error', field, 'Invalid branch must persist and announce its error')
  const html = sources['index.html'] ?? '',
    scripts = elements(html).filter((el) => el.tag === 'script' && !el.props.length)
  const headEnd = html.indexOf('</head>'),
    script = scripts.find((el) => el.loc.start.offset < headEnd)
  if (!script)
    fail('synchronous-theme-boot', 'index.html', 'Synchronous theme policy must execute in head')
  else {
    // Preserve line breaks for JavaScript automatic semicolon insertion.
    const code = script.loc.source.slice(
      script.loc.source.indexOf('>') + 1,
      script.loc.source.lastIndexOf('</'),
    )
    for (const [mode, systemDark, expected] of [
      ['dark', false, true],
      ['light', true, false],
      ['system', true, true],
      ['system', false, false],
      [null, true, true],
    ] as const) {
      const classes = new Set<string>()
      try {
        vm.runInNewContext(
          code,
          {
            localStorage: {
              getItem: (key: string) => (key === 'ideaboard-color-mode' ? mode : null),
            },
            window: {
              matchMedia: (query: string) => ({
                matches: query === '(prefers-color-scheme: dark)' && systemDark,
              }),
            },
            document: {
              documentElement: {
                classList: {
                  add: (name: string) => {
                    classes.add(name)
                  },
                  remove: (name: string) => {
                    classes.delete(name)
                  },
                },
              },
            },
          },
          { timeout: 100 },
        )
        if (classes.has('dark') !== expected)
          fail('synchronous-theme-boot', 'index.html', `Theme boot fails mode ${mode}`)
      } catch {
        fail('synchronous-theme-boot', 'index.html', 'Theme policy does not execute synchronously')
      }
    }
  }
  return issues
}

export type PurityModule = {
  source: string
  dependencies?: Array<{ module: string; resolved?: string; dependencyTypes?: string[] }>
}
export function validateCanvasPurity(modules: PurityModule[]): RetirementViolation[] {
  const issues: RetirementViolation[] = []
  const roots = modules.filter(
    (module) =>
      /^src\/features\/canvas\/tools\/[^/]+\/core\//.test(module.source) &&
      !/\.spec\.ts$/.test(module.source),
  )
  const graph = new Map(modules.map((module) => [module.source, module.dependencies ?? []]))
  if (!roots.length)
    issues.push({
      rule: 'canvas-purity',
      file: 'src/features/canvas/tools',
      message: 'No tool core sources were analyzed',
    })
  const forbidden = (target: string) =>
    /^(?:vue(?:-router|-i18n)?|pinia|axios|ofetch|better-auth)(?:$|\/)|^@(?:tanstack|vue-flow)\//.test(
      target,
    ) ||
    /^src\/(?:api\/generated-query|features\/(?:library|search)|features\/canvas\/(?:session|ui)|features\/canvas\/tools\/image-tools(?!\/core\/))(?:\/|$)/.test(
      target,
    ) ||
    (/^src\/features\/canvas\/tools\//.test(target) && !target.includes('/core/')) ||
    (/^src\/(?:core|shared)\/(?:auth|query|i18n)(?:\/|$)/.test(target) &&
      target !== 'src/core/i18n/localeCore.ts')
  for (const root of roots) {
    const seen = new Set<string>()
    const visit = (source: string, chain: string[]) => {
      if (seen.has(source)) return
      seen.add(source)
      for (const dep of graph.get(source) ?? []) {
        if (dep.dependencyTypes?.includes('type-only')) continue
        const target = dep.resolved ?? dep.module
        if (forbidden(dep.module) || forbidden(target))
          issues.push({
            rule: 'canvas-purity',
            file: root.source,
            message: [...chain, target].join(' -> '),
          })
        else if (graph.has(target)) visit(target, [...chain, target])
      }
    }
    visit(root.source, [root.source])
  }
  for (const module of modules)
    if (module.source.startsWith('src/features/canvas/core/graph/application/'))
      issues.push({
        rule: 'canvas-runtime-location',
        file: module.source,
        message: 'Graph runtime belongs outside domain',
      })
  return issues
}

/** Account core stays deterministic, including dependencies hidden behind barrels. */
export function validateAccountPurity(
  modules: PurityModule[],
  sources: Record<string, string> = {},
): RetirementViolation[] {
  const issues: RetirementViolation[] = []
  const roots = modules.filter(
    (module) =>
      module.source.startsWith('src/features/account/core/') && !/\.spec\.ts$/.test(module.source),
  )
  const graph = new Map(modules.map((module) => [module.source, module.dependencies ?? []]))
  if (!roots.length)
    issues.push({
      rule: 'account-purity',
      file: 'src/features/account/core',
      message: 'No Account core sources were analyzed',
    })
  const forbidden = (target: string) =>
    /^(?:vue(?:-router|-i18n)?|pinia|axios|ofetch|better-auth)(?:$|\/)|^@(?:tanstack|vue-flow|vueuse)\//.test(
      target,
    ) ||
    /^src\/(?:api|components|theme)(?:\/|$)/.test(target) ||
    (/^src\/features\//.test(target) && !target.startsWith('src/features/account/core/')) ||
    (/^src\/(?:core|shared)\/(?:auth|query|i18n)(?:\/|$)/.test(target) &&
      target !== 'src/core/i18n/localeCore.ts')
  const domGlobals = new Set([
    'document',
    'window',
    'getComputedStyle',
    'localStorage',
    'sessionStorage',
    'navigator',
    'fetch',
    'XMLHttpRequest',
  ])
  for (const root of roots) {
    const seen = new Set<string>()
    const visit = (source: string, chain: string[]) => {
      if (seen.has(source)) return
      seen.add(source)
      const sourceText = sources[source]
      if (sourceText !== undefined) {
        const file = ts.createSourceFile(source, sourceText, ts.ScriptTarget.Latest, true)
        const checkGlobals = (node: ts.Node) => {
          if (ts.isTypeNode(node)) return
          if (
            (ts.isIdentifier(node) &&
              domGlobals.has(node.text) &&
              !(
                (ts.isPropertyAccessExpression(node.parent) && node.parent.name === node) ||
                (ts.isPropertyAssignment(node.parent) && node.parent.name === node)
              )) ||
            (ts.isPropertyAccessExpression(node) &&
              ts.isIdentifier(node.expression) &&
              node.expression.text === 'globalThis' &&
              domGlobals.has(node.name.text)) ||
            (ts.isElementAccessExpression(node) &&
              ts.isIdentifier(node.expression) &&
              node.expression.text === 'globalThis' &&
              ts.isStringLiteral(node.argumentExpression) &&
              domGlobals.has(node.argumentExpression.text))
          )
            issues.push({
              rule: 'account-purity',
              file: root.source,
              message: [...chain, `DOM/network global ${node.getText(file)}`].join(' -> '),
            })
          ts.forEachChild(node, checkGlobals)
        }
        checkGlobals(file)
      }
      for (const dep of graph.get(source) ?? []) {
        if (dep.dependencyTypes?.includes('type-only')) continue
        const target = dep.resolved ?? dep.module
        if (forbidden(dep.module) || forbidden(target))
          issues.push({
            rule: 'account-purity',
            file: root.source,
            message: [...chain, target].join(' -> '),
          })
        else if (graph.has(target)) visit(target, [...chain, target])
      }
    }
    visit(root.source, [root.source])
  }
  return issues
}

export function runCanvasPurity(root = webRoot): RetirementViolation[] {
  const result = spawnSync(
    'npx',
    ['depcruise', '--config', '.dependency-cruiser.cjs', '--output-type', 'json', 'src'],
    { cwd: root, encoding: 'utf8', maxBuffer: 50 * 1024 * 1024 },
  )
  if (result.error || result.status !== 0 || !result.stdout)
    throw new Error(
      `dependency-cruiser did not produce a clean graph (exit ${result.status ?? 'unknown'})`,
    )
  const output = JSON.parse(result.stdout) as { modules?: PurityModule[] }
  if (!output.modules) throw new Error('dependency-cruiser graph has no modules')
  const sources = Object.fromEntries(
    output.modules
      .filter((module) => module.source.startsWith('src/') && /\.ts$/.test(module.source))
      .map((module) => [module.source, fs.readFileSync(path.join(root, module.source), 'utf8')]),
  )
  return [
    ...validateCanvasPurity(output.modules),
    ...validateAccountPurity(output.modules, sources),
  ]
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const issues = [...validateRetirementSources(loadRetirementSources()), ...runCanvasPurity()]
  for (const issue of issues) console.error(`${issue.file}: ${issue.rule}: ${issue.message}`)
  if (issues.length) process.exitCode = 1
  else
    console.log(
      'Retirement replacements: PostCSS, Vue/TypeScript AST, synchronous theme boot and Canvas/Account graph policies passed.',
    )
}
