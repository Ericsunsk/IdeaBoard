// Runtime roles (bare Tier 2 names such as `--success`) and domain roles
// (`--color-*`) use the same bridge.
type ThemeCssVar = `--${string}`

function defineThemeToken<CssVar extends ThemeCssVar, Fallback extends string>(
  cssVar: CssVar,
  fallback: Fallback,
) {
  return {
    cssVar,
    fallback,
  }
}

// Runtime roles in theme/theme.css, theme/tokens/semantic.css, and
// theme/tokens/admin.css are the source of truth; fallbacks keep
// third-party adapters and canvas rendering stable when CSS vars cannot
// resolve.
const adminThemeTokenBridge = {
  bg: defineThemeToken('--color-admin-bg', '#0f0f0f'),
  shell: defineThemeToken('--color-admin-shell', 'rgba(20, 20, 20, 0.75)'),
  card: defineThemeToken('--color-admin-card-strong', '#141414'),
  cardTranslucent: defineThemeToken('--color-admin-card', '#1d1d1d'),
  border: defineThemeToken('--color-admin-border', '#282828'),
  borderSoft: defineThemeToken('--color-admin-border-soft', '#1f1f1f'),
  borderHover: defineThemeToken('--color-admin-border-hover', '#3d3d3d'),
  borderPressed: defineThemeToken('--color-admin-border-pressed', '#525252'),
  textPrimary: defineThemeToken('--color-admin-text-1', '#f5f5f5'),
  textSecondary: defineThemeToken('--color-admin-text-2', '#a3a3a3'),
  textLabel: defineThemeToken('--color-admin-text-label', '#d4d4d4'),
  textTertiary: defineThemeToken('--color-admin-text-3', '#737373'),
  textDisabled: defineThemeToken('--color-admin-text-disabled', '#525252'),
  textInverse: defineThemeToken('--color-admin-text-inverse', '#0f0f0f'),
  accent: defineThemeToken('--color-admin-accent', '#fcfcfc'),
  accentHover: defineThemeToken('--color-admin-accent-hover', '#ebebeb'),
  accentPressed: defineThemeToken('--color-admin-accent-pressed', '#dedede'),
  accentSoft: defineThemeToken('--color-admin-accent-soft', 'rgba(255, 255, 255, 0.12)'),
  accentSofter: defineThemeToken('--color-admin-accent-softer', 'rgba(255, 255, 255, 0.06)'),
  focusRing: defineThemeToken('--color-admin-focus-ring', 'rgba(255, 255, 255, 0.25)'),
  surfaceMuted: defineThemeToken('--color-admin-surface-muted', '#181818'),
  surfaceHover: defineThemeToken('--color-admin-surface-hover', '#282828'),
  surfacePressed: defineThemeToken('--color-admin-surface-pressed', '#333333'),
  controlBg: defineThemeToken('--color-admin-control-bg', '#1d1d1d'),
  navActiveBg: defineThemeToken('--color-admin-nav-active-bg', 'rgba(255, 255, 255, 0.1)'),
  navActiveBgHover: defineThemeToken(
    '--color-admin-nav-active-bg-hover',
    'rgba(255, 255, 255, 0.15)',
  ),
  divider: defineThemeToken('--color-admin-divider', 'rgba(255, 255, 255, 0.08)'),
  success: defineThemeToken('--success', '#34d399'),
  warning: defineThemeToken('--warning', '#fbbf24'),
  danger: defineThemeToken('--destructive', '#f87171'),
  info: defineThemeToken('--info', '#60a5fa'),
  modelIndigo: defineThemeToken('--color-admin-model-indigo', '#818cf8'),
  modelPurple: defineThemeToken('--color-admin-model-purple', '#a78bfa'),
  modelCyan: defineThemeToken('--color-admin-model-cyan', '#38bdf8'),
  chartTooltipBorder: defineThemeToken('--color-admin-chart-tooltip-border', '#282828'),
  chartTooltipText: defineThemeToken('--color-admin-chart-tooltip-text', '#f5f5f5'),
  chartAxisLine: defineThemeToken('--color-admin-chart-axis-line', '#3d3d3d'),
  chartSplitLine: defineThemeToken('--color-admin-chart-split-line', '#282828'),
  chartLegendText: defineThemeToken('--color-admin-chart-legend-text', '#a3a3a3'),
  chartMist: defineThemeToken('--color-admin-chart-mist', '#737373'),
  chartOlive: defineThemeToken('--color-admin-chart-olive', '#86efac'),
} as const

type AdminThemeTokenBridge = typeof adminThemeTokenBridge
type AdminThemeTokenName = keyof AdminThemeTokenBridge
type AdminThemeTokenField = keyof AdminThemeTokenBridge[AdminThemeTokenName]
type AdminThemeTokenFieldMap<Field extends AdminThemeTokenField> = {
  readonly [Name in AdminThemeTokenName]: AdminThemeTokenBridge[Name][Field]
}

function mapAdminThemeTokenField<Field extends AdminThemeTokenField>(
  field: Field,
): AdminThemeTokenFieldMap<Field> {
  return Object.fromEntries(
    Object.entries(adminThemeTokenBridge).map(([name, token]) => [name, token[field]]),
  ) as AdminThemeTokenFieldMap<Field>
}

export const adminThemeTokenFallbacks = mapAdminThemeTokenField('fallback')

export const adminThemeTokenVars = mapAdminThemeTokenField('cssVar')
