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
  bg: defineAdminThemeToken('--color-admin-bg', '#fbfbfb'),
  shell: defineAdminThemeToken('--color-admin-shell', 'rgba(255, 255, 255, 0.62)'),
  card: defineAdminThemeToken('--color-admin-card-strong', '#ffffff'),
  cardTranslucent: defineAdminThemeToken('--color-admin-card', 'rgba(255, 255, 255, 0.94)'),
  border: defineAdminThemeToken('--color-admin-border', '#e5e7eb'),
  borderSoft: defineAdminThemeToken('--color-admin-border-soft', '#ececec'),
  borderHover: defineAdminThemeToken('--color-admin-border-hover', '#d4d4d4'),
  borderPressed: defineAdminThemeToken('--color-admin-border-pressed', '#bfbfbf'),
  textPrimary: defineAdminThemeToken('--color-admin-text-1', '#313131'),
  textSecondary: defineAdminThemeToken('--color-admin-text-2', '#62626a'),
  textLabel: defineAdminThemeToken('--color-admin-text-label', '#52525b'),
  // Former name kept as an alias so existing call sites and canvas adapters keep resolving.
  textSecondaryStrong: defineAdminThemeToken('--color-admin-text-2-strong', '#52525b'),
  textTertiary: defineAdminThemeToken('--color-admin-text-3', '#696971'),
  textDisabled: defineAdminThemeToken('--color-admin-text-disabled', '#d4d4d4'),
  textInverse: defineAdminThemeToken('--color-admin-text-inverse', '#ffffff'),
  accent: defineAdminThemeToken('--color-admin-accent', '#9278e6'),
  accentHover: defineAdminThemeToken('--color-admin-accent-hover', '#725fd0'),
  accentPressed: defineAdminThemeToken('--color-admin-accent-pressed', '#614ec2'),
  accentSoft: defineAdminThemeToken('--color-admin-accent-soft', 'rgba(146, 120, 230, 0.08)'),
  accentSofter: defineAdminThemeToken('--color-admin-accent-softer', 'rgba(146, 120, 230, 0.05)'),
  focusRing: defineAdminThemeToken('--color-admin-focus-ring', 'rgba(146, 120, 230, 0.18)'),
  surfaceMuted: defineAdminThemeToken('--color-admin-surface-muted', '#f5f5f5'),
  surfaceHover: defineAdminThemeToken('--color-admin-surface-hover', '#f0f0f0'),
  surfacePressed: defineAdminThemeToken('--color-admin-surface-pressed', '#e5e5e5'),
  surfaceDisabled: defineAdminThemeToken('--color-admin-surface-disabled', '#f5f5f5'),
  controlBg: defineAdminThemeToken('--color-admin-control-bg', '#fafafa'),
  controlBgHover: defineAdminThemeToken('--color-admin-control-bg-hover', '#f0f0f0'),
  controlBorder: defineAdminThemeToken('--color-admin-control-border', '#e5e7eb'),
  controlBorderHover: defineAdminThemeToken('--color-admin-control-border-hover', '#d4d4d4'),
  controlBorderPressed: defineAdminThemeToken('--color-admin-control-border-pressed', '#bfbfbf'),
  navActiveBg: defineAdminThemeToken('--color-admin-nav-active-bg', '#22304a'),
  navActiveBgHover: defineAdminThemeToken('--color-admin-nav-active-bg-hover', '#19243a'),
  previewBg: defineAdminThemeToken('--color-admin-preview-bg', '#f5f5f5'),
  divider: defineAdminThemeToken('--color-admin-divider', 'rgba(0, 0, 0, 0.08)'),
  success: defineAdminThemeToken('--color-admin-success', '#288661'),
  successSoft: defineAdminThemeToken('--color-admin-success-soft', 'rgba(40, 134, 97, 0.12)'),
  warning: defineAdminThemeToken('--color-admin-warning', '#a46925'),
  warningSoft: defineAdminThemeToken('--color-admin-warning-soft', 'rgba(164, 105, 37, 0.12)'),
  danger: defineAdminThemeToken('--color-admin-danger', '#c94a56'),
  dangerSoft: defineAdminThemeToken('--color-admin-danger-soft', 'rgba(201, 74, 86, 0.12)'),
  dangerHover: defineAdminThemeToken('--color-admin-danger-hover', '#df6370'),
  dangerPressed: defineAdminThemeToken('--color-admin-danger-pressed', '#b23a45'),
  info: defineAdminThemeToken('--color-admin-info', '#4775a8'),
  infoSoft: defineAdminThemeToken('--color-admin-info-soft', 'rgba(71, 117, 168, 0.12)'),
  modelIndigo: defineAdminThemeToken('--color-admin-model-indigo', '#4f6f9f'),
  modelPurple: defineAdminThemeToken('--color-admin-model-purple', '#9278e6'),
  modelCyan: defineAdminThemeToken('--color-admin-model-cyan', '#3d8b9a'),
  chartTooltipBorder: defineAdminThemeToken('--color-admin-chart-tooltip-border', '#e5e7eb'),
  chartTooltipText: defineAdminThemeToken('--color-admin-chart-tooltip-text', '#262626'),
  chartAxisLine: defineAdminThemeToken('--color-admin-chart-axis-line', '#e5e7eb'),
  chartSplitLine: defineAdminThemeToken('--color-admin-chart-split-line', '#f0f0f0'),
  chartLegendText: defineAdminThemeToken('--color-admin-chart-legend-text', '#737373'),
  chartMist: defineAdminThemeToken('--color-admin-chart-mist', '#8f8f8f'),
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
