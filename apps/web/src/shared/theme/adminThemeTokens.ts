type AdminThemeCssVar = `--color-admin-${string}`

function defineAdminThemeToken<CssVar extends AdminThemeCssVar, Fallback extends string>(
  cssVar: CssVar,
  fallback: Fallback,
) {
  return {
    cssVar,
    fallback,
    value: `var(${cssVar}, ${fallback})` as const,
  }
}

// CSS variables in app/style.css are the runtime source of truth; fallbacks keep
// third-party adapters and canvas rendering stable when CSS vars cannot resolve.
const adminThemeTokenBridge = {
  bg: defineAdminThemeToken('--color-admin-bg', '#eef3f8'),
  shell: defineAdminThemeToken('--color-admin-shell', 'rgba(255, 255, 255, 0.62)'),
  card: defineAdminThemeToken('--color-admin-card-strong', '#ffffff'),
  cardTranslucent: defineAdminThemeToken('--color-admin-card', 'rgba(255, 255, 255, 0.94)'),
  border: defineAdminThemeToken('--color-admin-border', '#d7e0eb'),
  borderSoft: defineAdminThemeToken('--color-admin-border-soft', '#e7edf4'),
  borderHover: defineAdminThemeToken('--color-admin-border-hover', '#c5d1df'),
  borderPressed: defineAdminThemeToken('--color-admin-border-pressed', '#b8c5d5'),
  borderStrong: defineAdminThemeToken('--color-admin-border-strong', '#b9c8da'),
  textPrimary: defineAdminThemeToken('--color-admin-text-1', '#1d2939'),
  textSecondary: defineAdminThemeToken('--color-admin-text-2', '#667085'),
  textSecondaryStrong: defineAdminThemeToken('--color-admin-text-2-strong', '#475467'),
  textTertiary: defineAdminThemeToken('--color-admin-text-3', '#98a2b3'),
  textDisabled: defineAdminThemeToken('--color-admin-text-disabled', '#b7c0cc'),
  textInverse: defineAdminThemeToken('--color-admin-text-inverse', '#ffffff'),
  accent: defineAdminThemeToken('--color-admin-accent', '#26364f'),
  accentHover: defineAdminThemeToken('--color-admin-accent-hover', '#1d2a3f'),
  accentPressed: defineAdminThemeToken('--color-admin-accent-pressed', '#111827'),
  accentSoft: defineAdminThemeToken('--color-admin-accent-soft', 'rgba(38, 54, 79, 0.09)'),
  accentSofter: defineAdminThemeToken('--color-admin-accent-softer', 'rgba(38, 54, 79, 0.055)'),
  focusRing: defineAdminThemeToken('--color-admin-focus-ring', 'rgba(38, 54, 79, 0.12)'),
  surfaceMuted: defineAdminThemeToken('--color-admin-surface-muted', '#f5f8fb'),
  surfaceHover: defineAdminThemeToken('--color-admin-surface-hover', '#f1f5f9'),
  surfacePressed: defineAdminThemeToken('--color-admin-surface-pressed', '#e8eef5'),
  surfaceDisabled: defineAdminThemeToken('--color-admin-surface-disabled', '#f0f4f8'),
  controlBg: defineAdminThemeToken('--color-admin-control-bg', '#f8fafc'),
  controlBgHover: defineAdminThemeToken('--color-admin-control-bg-hover', '#f1f5f9'),
  controlBorder: defineAdminThemeToken('--color-admin-control-border', '#d7e0eb'),
  controlBorderHover: defineAdminThemeToken('--color-admin-control-border-hover', '#c5d1df'),
  controlBorderPressed: defineAdminThemeToken('--color-admin-control-border-pressed', '#b8c5d5'),
  navActiveBg: defineAdminThemeToken('--color-admin-nav-active-bg', '#22304a'),
  navActiveBgHover: defineAdminThemeToken('--color-admin-nav-active-bg-hover', '#19243a'),
  previewBg: defineAdminThemeToken('--color-admin-preview-bg', '#f3f6fa'),
  divider: defineAdminThemeToken('--color-admin-divider', 'rgba(29, 41, 57, 0.08)'),
  success: defineAdminThemeToken('--color-admin-success', '#2f9e73'),
  warning: defineAdminThemeToken('--color-admin-warning', '#d48830'),
  warningStrong: defineAdminThemeToken('--color-admin-warning-strong', '#e79a38'),
  warningHover: defineAdminThemeToken('--color-admin-warning-hover', '#eba84e'),
  danger: defineAdminThemeToken('--color-admin-danger', '#d14d5a'),
  dangerHover: defineAdminThemeToken('--color-admin-danger-hover', '#df6370'),
  dangerPressed: defineAdminThemeToken('--color-admin-danger-pressed', '#b23a45'),
  info: defineAdminThemeToken('--color-admin-info', '#4775a8'),
  modelIndigo: defineAdminThemeToken('--color-admin-model-indigo', '#4f6f9f'),
  modelPurple: defineAdminThemeToken('--color-admin-model-purple', '#7b668f'),
  modelCyan: defineAdminThemeToken('--color-admin-model-cyan', '#3d8b9a'),
  primeNeutralStrong: defineAdminThemeToken('--color-admin-prime-neutral-strong', '#344054'),
  chartTooltipBorder: defineAdminThemeToken('--color-admin-chart-tooltip-border', '#d9e2ef'),
  chartTooltipText: defineAdminThemeToken('--color-admin-chart-tooltip-text', '#0f172a'),
  chartAxisLine: defineAdminThemeToken('--color-admin-chart-axis-line', '#d5deea'),
  chartSplitLine: defineAdminThemeToken('--color-admin-chart-split-line', '#edf2f7'),
  chartLegendText: defineAdminThemeToken('--color-admin-chart-legend-text', '#475569'),
  chartMist: defineAdminThemeToken('--color-admin-chart-mist', '#6f879f'),
  chartOlive: defineAdminThemeToken('--color-admin-chart-olive', '#6f8b71'),
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

export const adminThemeTokens = mapAdminThemeTokenField('value')
export const adminThemeTokenFallbacks = mapAdminThemeTokenField('fallback')
export const adminThemeTokenCssVars = mapAdminThemeTokenField('cssVar')
