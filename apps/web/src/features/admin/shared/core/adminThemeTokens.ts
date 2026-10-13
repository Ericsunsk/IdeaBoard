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
    value: `var(${cssVar}, ${fallback})` as const,
  }
}

// Runtime roles in theme/theme.css and theme/tokens/semantic.css are the
// source of truth; fallbacks keep third-party adapters and canvas rendering
// stable when CSS vars cannot resolve.
const adminThemeTokenBridge = {
  bg: defineThemeToken('--color-admin-bg', '#fbfbfb'),
  shell: defineThemeToken('--color-admin-shell', 'rgba(255, 255, 255, 0.62)'),
  card: defineThemeToken('--color-admin-card-strong', '#ffffff'),
  cardTranslucent: defineThemeToken('--color-admin-card', 'rgba(255, 255, 255, 0.94)'),
  border: defineThemeToken('--color-admin-border', '#e5e7eb'),
  borderSoft: defineThemeToken('--color-admin-border-soft', '#ececec'),
  borderHover: defineThemeToken('--color-admin-border-hover', '#d4d4d4'),
  borderPressed: defineThemeToken('--color-admin-border-pressed', '#bfbfbf'),
  textPrimary: defineThemeToken('--color-admin-text-1', '#313131'),
  textSecondary: defineThemeToken('--color-admin-text-2', '#62626a'),
  textLabel: defineThemeToken('--color-admin-text-label', '#52525b'),
  textTertiary: defineThemeToken('--color-admin-text-3', '#696971'),
  textDisabled: defineThemeToken('--color-admin-text-disabled', '#d4d4d4'),
  textInverse: defineThemeToken('--color-admin-text-inverse', '#ffffff'),
  accent: defineThemeToken('--color-admin-accent', '#9278e6'),
  accentHover: defineThemeToken('--color-admin-accent-hover', '#725fd0'),
  accentPressed: defineThemeToken('--color-admin-accent-pressed', '#614ec2'),
  accentSoft: defineThemeToken('--color-admin-accent-soft', 'rgba(146, 120, 230, 0.08)'),
  accentSofter: defineThemeToken('--color-admin-accent-softer', 'rgba(146, 120, 230, 0.05)'),
  focusRing: defineThemeToken('--color-admin-focus-ring', 'rgba(146, 120, 230, 0.18)'),
  surfaceMuted: defineThemeToken('--color-admin-surface-muted', '#f5f5f5'),
  surfaceHover: defineThemeToken('--color-admin-surface-hover', '#f0f0f0'),
  surfacePressed: defineThemeToken('--color-admin-surface-pressed', '#e5e5e5'),
  surfaceDisabled: defineThemeToken('--color-admin-surface-disabled', '#f5f5f5'),
  controlBg: defineThemeToken('--color-admin-control-bg', '#fafafa'),
  controlBgHover: defineThemeToken('--color-admin-control-bg-hover', '#f0f0f0'),
  controlBorder: defineThemeToken('--color-admin-control-border', '#e5e7eb'),
  controlBorderHover: defineThemeToken('--color-admin-control-border-hover', '#d4d4d4'),
  controlBorderPressed: defineThemeToken('--color-admin-control-border-pressed', '#bfbfbf'),
  navActiveBg: defineThemeToken('--color-admin-nav-active-bg', '#22304a'),
  navActiveBgHover: defineThemeToken('--color-admin-nav-active-bg-hover', '#19243a'),
  previewBg: defineThemeToken('--color-admin-preview-bg', '#f5f5f5'),
  divider: defineThemeToken('--color-admin-divider', 'rgba(0, 0, 0, 0.08)'),
  success: defineThemeToken('--success', '#16a34a'),
  warning: defineThemeToken('--warning', '#d97706'),
  danger: defineThemeToken('--destructive', '#ef4444'),
  info: defineThemeToken('--info', '#0284c7'),
  modelIndigo: defineThemeToken('--color-admin-model-indigo', '#4f6f9f'),
  modelPurple: defineThemeToken('--color-admin-model-purple', '#9278e6'),
  modelCyan: defineThemeToken('--color-admin-model-cyan', '#3d8b9a'),
  chartTooltipBorder: defineThemeToken('--color-admin-chart-tooltip-border', '#e5e7eb'),
  chartTooltipText: defineThemeToken('--color-admin-chart-tooltip-text', '#262626'),
  chartAxisLine: defineThemeToken('--color-admin-chart-axis-line', '#e5e7eb'),
  chartSplitLine: defineThemeToken('--color-admin-chart-split-line', '#f0f0f0'),
  chartLegendText: defineThemeToken('--color-admin-chart-legend-text', '#737373'),
  chartMist: defineThemeToken('--color-admin-chart-mist', '#8f8f8f'),
  chartOlive: defineThemeToken('--color-admin-chart-olive', '#6f8b71'),
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

export const adminDarkThemeTokenFallbacks = {
  tooltipBg: '#18181b',
  tooltipBorder: '#27272a',
  tooltipText: '#f4f4f5',
  axisLine: '#3f3f46',
  axisText: '#a1a1aa',
  splitLine: '#27272a',
  legendText: '#a1a1aa',
  emptyText: '#71717a',
  cardBg: '#1d1d1d',
  segmentBorder: '#18181b',
  mainText: '#f4f4f5',
} as const
