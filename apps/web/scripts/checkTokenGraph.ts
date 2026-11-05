import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import postcss from 'postcss'

/**
 * Guard: token graph integrity, exact mode parity, and contrast floors.
 *
 * - Integrity: every `var()` reference resolves to a declared token (or a
 *   framework/runtime allowlist entry), the owner graph is acyclic in both
 *   modes, and references flow down-tier only (Tier 1 -> Tier 2 -> Tier 3).
 * - Parity: every color role assigned in light has an explicit dark
 *   assignment and vice versa.
 * - Contrast: the finite role x surface inventory below clears its floors in
 *   both modes, compositing alpha the way a browser does.
 *
 * Consumer-boundary rules (no raw colors, no Tier 1 leaks in components)
 * live in `checkStyleGovernance.ts`; this script owns the token engine
 * itself. Authored color ingredients belong to Tier 1; scoped roles
 * must reference semantic roles. All violations fail the lint lane.
 */

export type TokenViolation = { rule: string; file: string; message: string }

type DeclScope = 'light' | 'dark' | 'scoped'
export type TokenDecl = {
  file: string
  name: string
  value: string
  scope: DeclScope
  isThemeKey: boolean
  isInlineTheme?: boolean
}
type Tier = 't1' | 't2' | 't3' | 'app' | 'consumer'

const WEB_ROOT = fileURLToPath(new URL('..', import.meta.url))
const SRC_ROOT = join(WEB_ROOT, 'src')

const OWNER_FILES = [
  'theme/theme.css',
  'theme/tokens/primitives.css',
  'theme/tokens/semantic.css',
  'theme/tokens/admin.css',
  'theme/tokens/canvas.css',
  'theme/tokens/motion.css',
  'theme/tokens/type.css',
  'theme/tokens/z.css',
  'theme/base.css',
  'theme/atoms/board-page-background.css',
  'theme/atoms/brand-mark.css',
  'theme/atoms/interactive-control-state.css',
  'theme/atoms/page-chrome.css',
  'theme/atoms/round-control.css',
  'theme/atoms/touch-target.css',
  'theme/atoms/user-avatar-button.css',
  'app/style.css',
] as const

function tierOfFile(file: string): Tier {
  if (file === 'theme/tokens/primitives.css') return 't1'
  if (
    file === 'theme/theme.css' ||
    file === 'theme/tokens/semantic.css' ||
    file === 'theme/tokens/admin.css' ||
    file === 'theme/tokens/motion.css' ||
    file === 'theme/tokens/type.css' ||
    file === 'theme/tokens/z.css' ||
    file === 'theme/base.css'
  )
    return 't2'
  if (file === 'theme/tokens/canvas.css') return 't3'
  if (file.startsWith('theme/atoms/') || file === 'app/style.css') return 'app'
  return 'consumer'
}

/** Framework-provided references (Tailwind defaults, never declared here). */
const FRAMEWORK_REFS = new Set([
  '--shadow-xs',
  '--shadow-sm',
  '--shadow-md',
  '--shadow-lg',
  '--shadow-xl',
  '--shadow-2xl',
  '--font-mono',
])

/**
 * Vendor runtime variables: provided by third-party primitives at runtime
 * (measured geometry), never declared in source.
 */
const VENDOR_RUNTIME_REFS = new Set(['--reka-select-trigger-width'])

/**
 * Fallback-governed customization points: consumed as `var(--x, default)`
 * theming hooks that hosts may override; the default is the effective value
 * when unset, so no static declaration exists by design.
 */
const CUSTOMIZATION_POINT_REFS = new Set([
  '--canvas-range-track-color',
  '--canvas-range-thumb-color',
  '--canvas-range-focus-color',
  '--editing-controls-label-width',
  '--editing-controls-field-gap',
  // Unset means currentColor (invalid-at-computed-value-time initial).
  '--canvas-handle-border',
  // Unset means the dialog enters without a transform offset.
  '--dialog-motion-from-transform',
  // Unset means the danger tone inherits the control color.
  '--control-danger-color',
])

const COLOR_ROLE_PATTERN =
  /^--(?:color|background|foreground|card|popover|primary|secondary|muted|accent|destructive|success|warning|info|border|input|ring|surface|sidebar|chart|workspace|tool|tooltip|progress)(?:-|$)/

/**
 * Single owner of the retired product-prefix token vocabulary.
 * `checkStyleGovernance` (usage scans) and `checkRetirementReplacements`
 * (declaration scans) both derive their matchers from this fragment so a new
 * `ib-` alias cannot slip through one lane while the other stays green.
 * The compatibility aliases in `checkRetirementReplacements` (`aliases`,
 * e.g. `--color-canvas-text-control`) stay live: the admin theme-token bridge
 * and canvas surface skin still resolve through them, so they are NOT deleted
 * here — removing them would break closure and bridge-fallback contracts.
 * Expiry: delete an alias only when the bridge fallback, the canvas surface
 * skin, and `textRampContrast` no longer resolve through it (all three).
 */
export const RETIRED_PRODUCT_TOKEN_FRAGMENT =
  'color-ib-[a-z0-9-]+|text-ib-[a-z0-9-]+|shadow-ib-(?:control|overlay|focus)(?=[\\s:),])'

/** Declaration-side retired-namespace test (token-owner files). */
export function isRetiredProductTokenDecl(prop: string): boolean {
  return /^--(?:color-ib-|text-ib-|shadow-ib-(?:control|overlay|focus)|color-dialog-overlay)/.test(
    prop,
  )
}
const MECHANICAL_KEYWORD_PATTERN = /^(?:transparent|inherit|currentColor|none)$/i

/**
 * Single-source chrome roles: these resolve through mode-aware board-card /
 * accent roles, so one dark declaration serves both modes and an explicit
 * light twin would be byte-identical redundancy (see the single-source note
 * over the page-chrome block in `tokens/semantic.css`). The exemption only
 * applies while the dark value carries no raw design color of its own.
 */
const SINGLE_SOURCE_NO_LIGHT_TWIN = new Set([
  '--color-page-chrome-bg',
  '--color-page-chrome-bg-hover',
  '--color-page-chrome-border',
  '--color-page-chrome-border-hover',
  '--color-page-chrome-color',
  '--color-page-chrome-color-hover',
  '--color-page-chrome-color-disabled',
  '--color-page-chrome-focus-ring',
])
const RAW_COLOR_PATTERN =
  /#[0-9a-f]{3,8}\b|(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb|color)\(|\b(?:white|black)\b/i

const COLOR_EXPRESSION_WORDS = new Set([
  'var',
  'color-mix',
  'in',
  'srgb',
  'srgb-linear',
  'display-p3',
  'a98-rgb',
  'prophoto-rgb',
  'rec2020',
  'lab',
  'oklab',
  'xyz',
  'xyz-d50',
  'xyz-d65',
  'hsl',
  'hwb',
  'lch',
  'oklch',
  'shorter',
  'longer',
  'increasing',
  'decreasing',
  'hue',
  'transparent',
  'currentcolor',
  'inherit',
  'none',
])
const SHADOW_GEOMETRY_WORDS = new Set([
  'inset',
  'calc',
  'min',
  'max',
  'clamp',
  'px',
  'rem',
  'em',
  'ex',
  'ch',
  'cap',
  'ic',
  'lh',
  'rlh',
  'vw',
  'vh',
  'vmin',
  'vmax',
  'vi',
  'vb',
  'svw',
  'svh',
  'lvw',
  'lvh',
  'dvw',
  'dvh',
  'cm',
  'mm',
  'q',
  'in',
  'pt',
  'pc',
])

function hasRawDesignColor(decl: TokenDecl): boolean {
  const expression = decl.value.replace(/--[\w-]+/g, '')
  if (RAW_COLOR_PATTERN.test(expression)) return true
  const isShadow = /^--(?:shadow|text-shadow)(?:-|$)/.test(decl.name)
  if (!COLOR_ROLE_PATTERN.test(decl.name) && !isShadow) return false
  // Only references, mechanical keywords and composition syntax belong here.
  // Unknown words include every CSS named color, including future additions.
  const words = expression.toLowerCase().match(/[a-z][a-z0-9-]*/g) ?? []
  return words.some(
    (word) => !COLOR_EXPRESSION_WORDS.has(word) && !(isShadow && SHADOW_GEOMETRY_WORDS.has(word)),
  )
}

function isLightSelector(selector: string): boolean {
  return selector.split(',').some((part) => /^(?:html)?\.light$/.test(part.trim()))
}

function scopesOfDecl(decl: postcss.Declaration): {
  scopes: DeclScope[]
  isThemeKey: boolean
  isInlineTheme?: boolean
} {
  let parent: postcss.Container | postcss.Document | undefined = decl.parent
  let inTheme = false
  let inInlineTheme = false
  let light = false
  let root = false
  while (parent) {
    if (parent.type === 'atrule' && (parent as postcss.AtRule).name === 'theme') {
      inTheme = true
      if ((parent as postcss.AtRule).params.includes('inline')) inInlineTheme = true
    }
    if (parent.type === 'rule') {
      const selector = (parent as postcss.Rule).selector
      if (isLightSelector(selector)) light = true
      if (selector.split(',').some((part) => part.trim() === ':root')) root = true
    }
    parent = parent.parent
  }
  // `@theme inline` keys are compile-time utility mappings, never runtime
  // assignments; they carry no mode value for parity purposes.
  if (inInlineTheme) return { scopes: ['scoped'], isThemeKey: false, isInlineTheme: true }
  if (light) return { scopes: root ? ['dark', 'light'] : ['light'], isThemeKey: false }
  if (inTheme && !inInlineTheme) return { scopes: ['dark'], isThemeKey: true }
  if (root) return { scopes: ['dark'], isThemeKey: false }
  return { scopes: ['scoped'], isThemeKey: false }
}

function refsOf(value: string): string[] {
  const names: string[] = []
  for (const match of value.matchAll(/var\(\s*(--[\w-]+)/g)) {
    if (match[1]) names.push(match[1])
  }
  return [...new Set(names)]
}

function declsOfSource(source: string, file: string): TokenDecl[] {
  const decls: TokenDecl[] = []
  try {
    const root = postcss.parse(source, { from: file })
    root.walkDecls((decl) => {
      if (decl.prop.startsWith('--')) {
        const { scopes, ...metadata } = scopesOfDecl(decl)
        for (const scope of scopes) {
          decls.push({ file, name: decl.prop, value: decl.value.trim(), scope, ...metadata })
        }
      }
    })
  } catch {
    for (const match of source.matchAll(/(--[\w-]+)\s*:\s*([^;{}]+);/g)) {
      if (match[1]) {
        decls.push({
          file,
          name: match[1],
          value: (match[2] ?? '').trim(),
          scope: 'scoped',
          isThemeKey: false,
        })
      }
    }
  }
  return decls
}

export function parseOwnerDecls(): TokenDecl[] {
  return OWNER_FILES.flatMap((file) =>
    declsOfSource(readFileSync(join(SRC_ROOT, file), 'utf8'), file),
  )
}

function listSourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) return listSourceFiles(full)
    return /\.(css|vue|ts)$/.test(entry) && !/[.](spec|test)[.]/.test(entry) ? [full] : []
  })
}

function stripComments(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/(^|\s)\/\/.*$/gm, '$1')
}

function declaredNames(): Set<string> {
  const names = new Set<string>()
  for (const file of listSourceFiles(SRC_ROOT)) {
    const content = stripComments(readFileSync(file, 'utf8'))
    for (const match of content.matchAll(/(--[\w-]+)['"]?\s*:/g)) {
      if (match[1]) names.add(match[1])
    }
  }
  return names
}

function checkIntegrity(decls: readonly TokenDecl[]): TokenViolation[] {
  const issues: TokenViolation[] = []
  const defined = declaredNames()
  for (const decl of decls) defined.add(decl.name)
  for (const file of listSourceFiles(SRC_ROOT)) {
    const rel = relative(SRC_ROOT, file).split(sep).join('/')
    const content = stripComments(readFileSync(file, 'utf8'))
    for (const name of refsOf(content)) {
      if (
        defined.has(name) ||
        FRAMEWORK_REFS.has(name) ||
        VENDOR_RUNTIME_REFS.has(name) ||
        CUSTOMIZATION_POINT_REFS.has(name)
      ) {
        continue
      }
      issues.push({
        rule: 'token-undefined',
        file: rel,
        message: `${name} is referenced but never declared.`,
      })
    }
  }
  return issues
}

export function scopeValues(
  decls: readonly TokenDecl[],
  mode: 'light' | 'dark',
): Map<string, TokenDecl> {
  const values = new Map<string, TokenDecl>()
  // Runtime declarations override Tailwind's theme-layer defaults. Within
  // each scope, the last declaration wins, as it does in the stylesheet.
  for (const isThemeKey of [true, false]) {
    for (const scope of ['scoped', 'dark', ...(mode === 'light' ? ['light'] : [])]) {
      for (const decl of decls) {
        if (decl.isThemeKey === isThemeKey && !decl.isInlineTheme && decl.scope === scope) {
          values.set(decl.name, decl)
        }
      }
    }
  }
  return values
}

export function checkAcyclic(decls: readonly TokenDecl[]): TokenViolation[] {
  const issues: TokenViolation[] = []
  for (const mode of ['light', 'dark'] as const) {
    const values = scopeValues(decls, mode)
    const visiting = new Set<string>()
    const visited = new Set<string>()
    const visit = (name: string, chain: string[]): void => {
      if (visited.has(name)) return
      if (visiting.has(name)) {
        issues.push({
          rule: 'token-cycle',
          file: values.get(name)?.file ?? 'theme',
          message: `Cyclic token reference in ${mode} scope: ${[...chain, name].join(' -> ')}.`,
        })
        return
      }
      visiting.add(name)
      for (const ref of refsOf(values.get(name)?.value ?? '')) {
        if (values.has(ref)) visit(ref, [...chain, name])
      }
      visiting.delete(name)
      visited.add(name)
    }
    for (const name of values.keys()) visit(name, [])
  }
  return issues
}

function tierOfName(values: ReadonlyMap<string, TokenDecl>, name: string): Tier | undefined {
  const decl = values.get(name)
  return decl === undefined ? undefined : tierOfFile(decl.file)
}

export function checkTierDirection(decls: readonly TokenDecl[]): TokenViolation[] {
  const issues: TokenViolation[] = []
  const lightValues = scopeValues(decls, 'light')
  const darkValues = scopeValues(decls, 'dark')
  const fail = (rule: string, file: string, message: string) => {
    issues.push({ rule, file, message })
  }
  for (const decl of decls) {
    if (decl.isInlineTheme) continue
    const fromTier = tierOfFile(decl.file)
    for (const ref of refsOf(decl.value)) {
      const ownerValues = decl.scope === 'light' ? [lightValues] : [darkValues, lightValues]
      for (const toTier of new Set(ownerValues.map((values) => tierOfName(values, ref)))) {
        if (toTier === undefined || FRAMEWORK_REFS.has(ref)) continue
        if (toTier === 't1') {
          if (fromTier === 't2') continue
          fail(
            'tier-direction',
            decl.file,
            `${decl.name} references Tier 1 ${ref}; route through a Tier 2 role.`,
          )
          continue
        }
        if (fromTier === 't1') {
          fail(
            'tier-direction',
            decl.file,
            `Tier 1 ${decl.name} must not reference ${ref} (tier ${toTier}).`,
          )
        } else if (fromTier === 't2' && toTier !== 't2') {
          fail(
            'tier-direction',
            decl.file,
            `Tier 2 ${decl.name} must not reference ${ref} (tier ${toTier}).`,
          )
        } else if (fromTier === 't3' && toTier !== 't2') {
          fail(
            'tier-direction',
            decl.file,
            `Tier 3 ${decl.name} must reference a Tier 2 role, not ${ref} (tier ${toTier}).`,
          )
        }
      }
    }
    if (hasRawDesignColor(decl) && (fromTier === 't2' || fromTier === 't3')) {
      fail(
        'tier-raw-literal',
        decl.file,
        `${decl.name} carries a raw design color; route through a Tier 1 ingredient via a Tier 2 role.`,
      )
    }
  }
  return issues
}

export function checkParity(decls: readonly TokenDecl[]): TokenViolation[] {
  const issues: TokenViolation[] = []
  const light = new Map<string, TokenDecl>()
  const dark = new Map<string, TokenDecl>()
  for (const decl of decls) {
    if (decl.isInlineTheme) continue
    if (!COLOR_ROLE_PATTERN.test(decl.name)) continue
    if (decl.scope === 'light') light.set(decl.name, decl)
    else if (decl.scope === 'dark') dark.set(decl.name, decl)
  }
  for (const [name, decl] of light) {
    if (MECHANICAL_KEYWORD_PATTERN.test(decl.value)) continue
    if (!dark.has(name)) {
      issues.push({
        rule: 'mode-parity',
        file: decl.file,
        message: `${name} has a light assignment but no explicit dark assignment.`,
      })
    }
  }
  for (const [name, decl] of dark) {
    if (MECHANICAL_KEYWORD_PATTERN.test(decl.value)) continue
    if (
      SINGLE_SOURCE_NO_LIGHT_TWIN.has(name) &&
      !RAW_COLOR_PATTERN.test(decl.value)
    )
      continue
    if (!light.has(name)) {
      issues.push({
        rule: 'mode-parity',
        file: decl.file,
        message: `${name} has a dark assignment but no light assignment.`,
      })
    }
  }
  return issues
}

type Rgb = readonly [number, number, number]

function parseComponent(part: string, base: number): number {
  if (!/^[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?%?$/i.test(part)) {
    throw new Error(`Unsupported color component: ${part}`)
  }
  const value = part.endsWith('%') ? (Number.parseFloat(part) / 100) * base : Number(part)
  if (!Number.isFinite(value)) throw new Error(`Non-finite color component: ${part}`)
  return Math.min(base, Math.max(0, value))
}

function resolveVariables(value: string, resolve: (name: string) => string): string {
  let current = value.trim()
  const seen = new Set<string>()
  for (;;) {
    const match = current.match(/^var\(\s*(--[\w-]+)\s*(?:,([\s\S]*))?\)$/)
    if (!match?.[1]) return current
    const name = match[1]
    if (seen.has(name)) throw new Error(`Unresolvable token reference: ${value}`)
    seen.add(name)
    try {
      current = resolve(name).trim()
    } catch (error) {
      if (match[2] === undefined) throw error
      current = match[2].trim()
    }
  }
}

function parseRgb(value: string): { rgb: Rgb; alpha: number } | undefined {
  const match = value.match(/^rgba?\(\s*(.+?)\s*\)$/i)
  if (!match?.[1]) return undefined
  const body = match[1]
  const parts = body.includes(',') ? body.split(',').map((part) => part.trim()) : body.split('/')
  const channels = body.includes(',') ? parts.slice(0, 3) : (parts[0] ?? '').trim().split(/\s+/)
  const alpha = body.includes(',') ? parts[3] : parts[1]?.trim()
  if (channels.length !== 3 || parts.length > (body.includes(',') ? 4 : 2)) {
    throw new Error(`Unsupported color value: ${value}`)
  }
  return {
    rgb: channels.map((part) => parseComponent(part, 255)) as unknown as Rgb,
    alpha: alpha === undefined ? 1 : parseComponent(alpha, 1),
  }
}

export function parseColor(value: string, resolve: (name: string) => string): Rgb {
  return resolveAlphaColor(value, resolve).rgb
}

function parseHexColor(value: string): Rgb {
  const hex = value.startsWith('#') ? value.slice(1) : ''
  if (/^[0-9a-f]{6}$/i.test(hex)) {
    return [
      Number.parseInt(hex.slice(0, 2), 16),
      Number.parseInt(hex.slice(2, 4), 16),
      Number.parseInt(hex.slice(4, 6), 16),
    ]
  }
  if (/^[0-9a-f]{3}$/i.test(hex)) {
    return [hex[0]!, hex[1]!, hex[2]!].map((c) => Number.parseInt(c + c, 16)) as unknown as Rgb
  }
  throw new Error(`Unsupported color value: ${value}`)
}

function composite(top: Rgb, alpha: number, bottom: Rgb): Rgb {
  return [
    top[0] * alpha + bottom[0] * (1 - alpha),
    top[1] * alpha + bottom[1] * (1 - alpha),
    top[2] * alpha + bottom[2] * (1 - alpha),
  ]
}

function channel(raw: number): number {
  const c = raw / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

function luminance([r, g, b]: Rgb): number {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

export function contrastOf(foreground: Rgb, background: Rgb): number {
  if ([...foreground, ...background].some((value) => !Number.isFinite(value))) {
    throw new Error('Non-finite contrast color channels')
  }
  const a = luminance(foreground)
  const b = luminance(background)
  const [lighter, darker] = a > b ? [a, b] : [b, a]
  const ratio = (lighter + 0.05) / (darker + 0.05)
  if (!Number.isFinite(ratio)) throw new Error('Non-finite contrast ratio')
  return ratio
}

type ContrastPair = { foreground: string; background: string; floor: number; label: string }

export const CONTRAST_INVENTORY: readonly ContrastPair[] = [
  { foreground: '--foreground', background: '--background', floor: 7, label: 'body text' },
  { foreground: '--card-foreground', background: '--card', floor: 7, label: 'card text' },
  ...(['primary', 'destructive'] as const).flatMap((role) =>
    ['', '-hover', '-pressed'].map((state) => ({
      foreground: `--${role}-foreground`,
      background: `--${role}${state}`,
      floor: 4.5,
      label: `${role}${state} action text`,
    })),
  ),
  {
    foreground: '--secondary-foreground',
    background: '--secondary',
    floor: 4.5,
    label: 'secondary text',
  },
  { foreground: '--accent-foreground', background: '--accent', floor: 4.5, label: 'accent text' },
  {
    foreground: '--sidebar-foreground',
    background: '--sidebar',
    floor: 7,
    label: 'sidebar text',
  },
  ...['primary', 'accent'].map((role) => ({
    foreground: `--sidebar-${role}-foreground`,
    background: `--sidebar-${role}`,
    floor: 4.5,
    label: `sidebar ${role} text`,
  })),
  { foreground: '--sidebar-ring', background: '--sidebar', floor: 3, label: 'sidebar focus ring' },
  ...[1, 2, 3, 4, 5].flatMap((chart) =>
    ['background', 'card'].map((surface) => ({
      foreground: `--chart-${chart}`,
      background: `--${surface}`,
      floor: 3,
      label: `chart ${chart} on ${surface}`,
    })),
  ),
]

export function resolveAlphaColor(
  value: string,
  resolve: (name: string) => string,
): { rgb: Rgb; alpha: number } {
  const current = resolveVariables(value, resolve)
  const mix = current.match(
    /^color-mix\(\s*in srgb\s*,\s*(.+?)\s+([\d.]+)%\s*,\s*transparent\s*\)$/,
  )
  if (mix?.[1] && mix?.[2]) {
    const percentage = Number(mix[2])
    if (!Number.isFinite(percentage) || percentage < 0 || percentage > 100) {
      throw new Error(`Unsupported color-mix percentage: ${mix[2]}`)
    }
    const color = resolveAlphaColor(mix[1].trim(), resolve)
    return { rgb: color.rgb, alpha: color.alpha * (percentage / 100) }
  }
  return parseRgb(current) ?? { rgb: parseHexColor(current), alpha: 1 }
}

export function checkContrast(decls: readonly TokenDecl[]): TokenViolation[] {
  return contrastInventoryReport(decls).flatMap((entry) => {
    if (entry.error !== undefined) {
      return [
        {
          rule: 'contrast-unresolvable',
          file: 'theme/tokens/semantic.css',
          message: `${entry.mode} ${entry.label}: ${entry.error}`,
        },
      ]
    }
    if (entry.ratio < entry.floor) {
      return [
        {
          rule: 'contrast-floor',
          file: 'theme/tokens/semantic.css',
          message: `${entry.mode} ${entry.label} (${entry.foreground} on ${entry.background}) is ${entry.ratio.toFixed(2)}:1, below the ${entry.floor}:1 floor.`,
        },
      ]
    }
    return []
  })
}

export type ContrastReportEntry = {
  mode: 'light' | 'dark'
  label: string
  foreground: string
  background: string
  floor: number
  ratio: number
  error?: string
}

export function contrastInventoryReport(decls: readonly TokenDecl[]): ContrastReportEntry[] {
  const entries: ContrastReportEntry[] = []
  for (const mode of ['light', 'dark'] as const) {
    const values = scopeValues(decls, mode)
    const resolve = (name: string): string => {
      const value = values.get(name)?.value
      if (value === undefined) throw new Error(`Token ${name} not found in ${mode} scope`)
      return value
    }
    for (const pair of CONTRAST_INVENTORY) {
      const foregroundValue = values.get(pair.foreground)?.value
      const backgroundValue = values.get(pair.background)?.value
      if (foregroundValue === undefined || backgroundValue === undefined) {
        entries.push({
          mode,
          ...pair,
          ratio: 0,
          error: `missing ${pair.foreground} or ${pair.background}`,
        })
        continue
      }
      try {
        const foreground = resolveAlphaColor(foregroundValue, resolve)
        const background = resolveAlphaColor(backgroundValue, resolve)
        const backdrop = resolveAlphaColor(values.get('--background')?.value ?? '', resolve)
        const backing = composite(backdrop.rgb, backdrop.alpha, [255, 255, 255])
        const effectiveBackground = composite(background.rgb, background.alpha, backing)
        const ratio = contrastOf(
          composite(foreground.rgb, foreground.alpha, effectiveBackground),
          effectiveBackground,
        )
        entries.push({ mode, ...pair, ratio })
      } catch (error) {
        entries.push({
          mode,
          ...pair,
          ratio: 0,
          error: error instanceof Error ? error.message : String(error),
        })
      }
    }
  }
  return entries
}

export function checkTokenGraph(): TokenViolation[] {
  const decls = parseOwnerDecls()
  return [
    ...checkIntegrity(decls),
    ...checkAcyclic(decls),
    ...checkTierDirection(decls),
    ...checkParity(decls),
    ...checkContrast(decls),
  ].sort((left, right) => left.file.localeCompare(right.file))
}

function main(): void {
  const violations = checkTokenGraph()
  if (violations.length === 0) {
    console.log('Token graph integrity, mode parity, and contrast floors passed.')
    return
  }
  console.error(`Token graph guardrails failed (${violations.length} violation(s)).\n`)
  for (const violation of violations) {
    console.error(`- ${violation.file}\n  [${violation.rule}] ${violation.message}`)
  }
  console.error('')
  process.exitCode = 1
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main()
}
