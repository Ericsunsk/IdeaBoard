import { readFileSync } from 'node:fs'
import { join } from 'node:path'

/**
 * Guard: retouch shared-token contract.
 *
 * Replaces the 76 lint-planned source-string assertions retired from
 * `src/features/canvas/tools/retouch/ui/retouchShared.contract.spec.ts`.
 * Each policy below keeps the same intent as the retired `toContain` /
 * `not.toContain` / `not.toMatch` assertions, but `contains` checks run on
 * comment-stripped sources so a commented-out token, a dead code path, or a
 * prose mention can no longer satisfy a rule. `lacks` / `lacksPattern`
 * checks intentionally stay on the raw source (identical semantics to the
 * retired negated assertions): a retired marker must not reappear anywhere,
 * not even in a comment.
 *
 * Explicitly out of scope (kept in the spec, never deleted by this lane):
 *
 *   - 5 focus-ring assertions ("keeps focus rings on motion-safe outline
 *     tokens without layout shift") whose planned replacement is the browser
 *     policy `computed-style/3a1e8303`. A source lint cannot prove computed
 *     outline rendering, so those stay until the browser replacement lands.
 *   - 18 locale assertions (`labelKey` / call-time `t()` / `i18n.global.t`
 *     absence) frozen under `i18n/retouch-call-time-locale-switch`
 *     (tool-decision-frozen, decision-pending). They need the i18n
 *     governance tool decision first.
 *
 * Policy ledger (rule -> retired scenario -> inventory IDs):
 *
 *   - source-policy/retouchShared.contract/36d47aae: serves both histogram
 *     and curves frames from one contract. Ledger: source-fdce7286c24e9944-1,
 *     source-defab72b5a91e0db-1, source-06b78aaf141804ec-1,
 *     source-aec9e01591e7087f-1, source-6c61056858c8a0ff-1.
 *   - source-policy/retouchShared.contract/57f03081: keeps the veil and
 *     histogram wash on Canvas surface tokens. Ledger:
 *     source-65c4dbc9a7245988-1, source-a094aaedb3988626-1,
 *     source-33732885ae4ed90f-1.
 *   - source-policy/retouchShared.contract/b3ff8835: leaves grid strokes and
 *     marker accents on the surface contract. Ledger:
 *     source-7080d725339d5030-1, source-33089bafd852e074-1,
 *     source-cff379eec51315f1-1, source-5685e225c7d710a9-1.
 *   - source-policy/retouchShared.contract/4c9978a2: defines handle sizing
 *     once and reuses it in curves and histogram markers. Ledger:
 *     source-b1494287a7ee0b80-1, source-b7117d432dcd1bda-1,
 *     source-469f99e9a1158d84-1, source-4a87cf6246c6f457-1,
 *     source-9ab622f1920f4d29-1.
 *   - source-policy/retouchShared.contract/b9588c5b: rounds instrument dots
 *     with the pill token. Ledger: source-77069bdc47c63289-1,
 *     source-91d0365467e8a8b4-1.
 *   - source-policy/retouchShared.contract/1420557f: sources panel metrics
 *     from the Canvas surface contract. Ledger: source-838cf626a0aa9f79-1,
 *     source-0f5a2f3310242018-1, source-8f7370d5b82388f6-1,
 *     source-96fea834d69063f9-1, source-ca5efbf6d8478ef8-1.
 *   - source-policy/retouchShared.contract/6fa1c8ad: keeps both button groups
 *     on the shared action row. Ledger: source-f14285a2dac05058-1,
 *     source-3395c863148d8311-1.
 *   - source-policy/retouchShared.contract/66820f2a: derives the layer thumb
 *     radius from the panel control radius. Ledger: source-da8dd21e25fe47d8-1.
 *   - source-policy/retouchShared.contract/15f3f640: keeps apply as the
 *     single primary commit control without local overrides. Ledger:
 *     source-084b84aa499f6ee5-1, source-b9853b5899fe1eec-1,
 *     source-9db5daf9f26664b7-1.
 *   - source-policy/retouchShared.contract/aa277691: keeps Chrome wiring
 *     preview state to the commit control. Ledger: source-ab44d40a92fc344f-1,
 *     source-3f5e9c085b28ff4e-1, source-cec247ffaf1e7004-1.
 *   - source-policy/retouchShared.contract/20d5f590: injects the apply gate
 *     from the parent instead of reading the stack directly. Ledger:
 *     source-405667778cf9cdfc-1, source-e9f1797e907e8503-1,
 *     source-ed9626e5fc31dcf0-1, source-7f3513d53b59e08d-1.
 *   - source-policy/retouchShared.contract/1093d62f: keeps listbox, slider,
 *     and tooltip markers that the behavioral specs pin. Ledger:
 *     source-3bdbcec16b7499aa-1, source-51c0ee76f13e53da-1,
 *     source-da1058d0663d8951-1, source-242c860c14fef25c-1,
 *     source-31133286f5e690e1-1, source-5e4e7a1aaac2ba50-1.
 *   - source-policy/retouchShared.contract/f58ec046: folds the
 *     threshold/levels marker pair with zero-specificity :where(). Ledger:
 *     source-7572f5f4c52446d7-1, source-6231a99803d49893-1,
 *     source-ee68696bfa692cf7-1.
 *   - source-policy/retouchShared.contract/371d3c69: keeps marker modes
 *     distinct across Levels button and Threshold range. Ledger:
 *     source-f81387fc46a798b9-1, source-3a5139e9b4138568-1.
 *   - source-policy/retouchShared.contract/24efd8bc: renders the four effect
 *     controls from one kind map behind component :is. Ledger:
 *     source-119a9f456f23434a-1, source-800e4dd916113b91-1,
 *     source-7604bed0714f18bb-1, source-9b0d501922a1fe82-1,
 *     source-8dae562ac7c69bf6-1, source-2816b96c908f89e7-1,
 *     source-d627abfa492712b4-1, source-a20f37fe64df1d34-1.
 *   - source-policy/retouchShared.contract/45774b8f: keeps the hsl branch and
 *     the sharpen histogram exception. Ledger: source-c812cc480428e08b-1,
 *     source-6fb079cabb880071-1.
 *   - source-policy/retouchShared.contract/0a8bb689: derives kind
 *     icon/label/rail tables from definitions with call-time t() (structural
 *     half: icon/rail tables). Ledger: source-074610845d97cec0-1,
 *     source-2c78b78ef0298ced-1, source-46af5e716e82b700-1.
 *   - source-policy/retouchShared.contract/7f127e21: keeps LayersPanel
 *     table-driven with a generic fallback instead of throwing. Ledger:
 *     source-0eccbdb43577aa22-1, source-7d4333d63f5e2855-1,
 *     source-257b2e9cbe7caeac-1, source-44dead6e867fab94-1,
 *     source-80bdf79275ef0f12-1.
 *   - source-policy/retouchShared.contract/78ec4640: resolves Chrome rail
 *     and list selection through one view-model function. Ledger:
 *     source-e414524d1dcaa0ed-1, source-4d96252c7f8cfa34-1,
 *     source-c92342a786df9acc-1, source-251c43b210b837f7-1.
 *   - source-policy/retouchShared.contract/bf9a5ad7: exposes Chrome as
 *     RetouchChrome through the unique exit. Ledger:
 *     source-a2dc0f8b9fb9c3e2-1, source-3cd14ebc85f78833-1,
 *     source-b802473409410b92-1.
 *   - source-policy/retouchShared.contract/4a90da6a: keeps levels/threshold
 *     markers locale-free with call-time t() (structural half: no baked
 *     ariaLabels, reactive field labels). Ledger: source-b166e4f1708fe1f8-1,
 *     source-3e9b5b463dbe287e-1, source-2247529231fc456f-1.
 */

export type RetouchSharedViolation = { rule: string; file: string; message: string }

type ContainsCheck = { file: string; kind: 'contains'; text: string }
type LacksCheck = { file: string; kind: 'lacks'; text: string }
type LacksPatternCheck = {
  file: string
  kind: 'lacksPattern'
  pattern: RegExp
  description: string
}
export type RetouchSharedCheck = ContainsCheck | LacksCheck | LacksPatternCheck
export type RetouchSharedPolicy = {
  rule: string
  scenario: string
  checks: readonly RetouchSharedCheck[]
}

const UI = 'features/canvas/tools/retouch/ui'
const PLOT_FRAME = `${UI}/shared/styles/retouch-plot-frame.css`
const HISTOGRAM_PLOT_CSS = `${UI}/shared/styles/retouch-histogram-plot.css`
const CURVES_CSS = `${UI}/shared/styles/retouch-curves.css`
const HISTOGRAM_CONTROLS_CSS = `${UI}/shared/styles/retouch-histogram-controls.css`
const CONTROL_POINTS_CSS = `${UI}/shared/styles/retouch-control-points.css`
const PANEL_CSS = `${UI}/shared/styles/retouch-panel.css`
const LAYER_ACTIONS = `${UI}/layers/LayerActions.vue`
const EFFECT_ACTIONS = `${UI}/chrome/EffectActions.vue`
const LAYERS_PANEL = `${UI}/layers/LayersPanel.vue`
const HISTOGRAM_PLOT_VUE = `${UI}/histogram/HistogramPlot.vue`
const CHROME = `${UI}/chrome/Chrome.vue`
const THRESHOLD_CONTROL = `${UI}/threshold/ThresholdControl.vue`
const LEVELS_CONTROL = `${UI}/levels/LevelsControl.vue`
const LEVELS_FIELDS = `${UI}/levels/LevelsFields.vue`
const PROPERTIES_PANEL = `${UI}/chrome/PropertiesPanel.vue`
const RETOUCH_BARREL = 'features/canvas/tools/retouch/index.ts'
const VIEW_MODEL = 'features/canvas/tools/retouch/session/retouchViewModel.ts'
const LEVELS_CORE = 'features/canvas/tools/retouch/core/kinds/levels.ts'
const THRESHOLD_CORE = 'features/canvas/tools/retouch/core/kinds/threshold.ts'

const contains = (file: string, text: string): RetouchSharedCheck => ({
  file,
  kind: 'contains',
  text,
})
const lacks = (file: string, text: string): RetouchSharedCheck => ({ file, kind: 'lacks', text })

export const RETOUCH_SHARED_POLICIES: readonly RetouchSharedPolicy[] = [
  {
    rule: 'source-policy/retouchShared.contract/36d47aae',
    scenario: 'serves both histogram and curves frames from one contract',
    checks: [
      contains(HISTOGRAM_PLOT_CSS, "@import './retouch-plot-frame.css'"),
      contains(CURVES_CSS, "@import './retouch-plot-frame.css'"),
      contains(PLOT_FRAME, '.retouch-histogram-plot'),
      contains(PLOT_FRAME, '.retouch-curves__graph'),
      contains(PLOT_FRAME, '--retouch-plot-frame-radius:'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/57f03081',
    scenario: 'keeps the veil and histogram wash on Canvas surface tokens',
    checks: [
      contains(PLOT_FRAME, '--canvas-glass-bg'),
      contains(PLOT_FRAME, '--canvas-surface-muted'),
      contains(PLOT_FRAME, '--canvas-text-subtle'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/b3ff8835',
    scenario: 'leaves grid strokes and marker accents on the surface contract',
    checks: [
      contains(PLOT_FRAME, '--canvas-surface-border-soft'),
      contains(PLOT_FRAME, '--canvas-accent-outline'),
      contains(HISTOGRAM_PLOT_CSS, '--canvas-accent-strong'),
      contains(HISTOGRAM_CONTROLS_CSS, '--canvas-accent-outline'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/4c9978a2',
    scenario: 'defines handle sizing once and reuses it in curves and histogram markers',
    checks: [
      contains(CONTROL_POINTS_CSS, '--retouch-control-point-size:'),
      contains(CONTROL_POINTS_CSS, '--retouch-control-point-bg:'),
      contains(CURVES_CSS, 'var(--retouch-control-point-bg)'),
      contains(HISTOGRAM_PLOT_CSS, 'var(--retouch-control-point-bg'),
      contains(HISTOGRAM_CONTROLS_CSS, 'var(--retouch-control-point-'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/b9588c5b',
    scenario: 'rounds instrument dots with the pill token',
    checks: [
      contains(HISTOGRAM_PLOT_CSS, 'var(--radius-pill)'),
      contains(CURVES_CSS, 'var(--radius-pill)'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/1420557f',
    scenario: 'sources panel metrics from the Canvas surface contract',
    checks: [
      contains(PANEL_CSS, '--retouch-panel-inset: var(--canvas-tool-button-gap)'),
      contains(PANEL_CSS, '--retouch-panel-control-radius: var(--radius-control)'),
      contains(PANEL_CSS, '--retouch-panel-text-size: var(--canvas-font-size-caption)'),
      contains(PANEL_CSS, '.retouch-action-row'),
      contains(PANEL_CSS, 'repeat(5, var(--node-tool-button-size))'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/6fa1c8ad',
    scenario: 'keeps both button groups on the shared action row',
    checks: [
      contains(LAYER_ACTIONS, 'retouch-action-row'),
      contains(EFFECT_ACTIONS, 'retouch-action-row'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/66820f2a',
    scenario: 'derives the layer thumb radius from the panel control radius',
    checks: [contains(LAYERS_PANEL, 'calc(var(--retouch-panel-control-radius) - 3px)')],
  },
  {
    rule: 'source-policy/retouchShared.contract/15f3f640',
    scenario: 'keeps apply as the single primary commit control without local overrides',
    checks: [
      contains(LAYER_ACTIONS, 'primary'),
      contains(LAYER_ACTIONS, ':is-disabled="applyDisabled"'),
      contains(LAYER_ACTIONS, "emit('apply-retouch')"),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/aa277691',
    scenario: 'keeps Chrome wiring preview state to the commit control',
    checks: [
      contains(CHROME, ':apply-disabled'),
      contains(CHROME, '@apply-retouch="emit(\'apply-retouch\')"'),
      contains(CHROME, ':histogram-source'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/20d5f590',
    scenario: 'injects the apply gate from the parent instead of reading the stack directly',
    checks: [
      contains(CHROME, 'canApply: boolean'),
      contains(CHROME, '!canApply || applyingRetouch'),
      lacks(CHROME, 'hasRenderableRetouchLayerStack'),
      lacks(CHROME, 'canApplyRetouch'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/1093d62f',
    scenario: 'keeps listbox, slider, and tooltip markers that the behavioral specs pin',
    checks: [
      contains(LAYERS_PANEL, 'role="listbox"'),
      contains(LAYERS_PANEL, 'hit-target-32-inline'),
      contains(LAYERS_PANEL, 'canvas-tool-tooltip-trigger'),
      contains(HISTOGRAM_PLOT_VUE, 'role="slider"'),
      contains(HISTOGRAM_PLOT_CSS, ':focus-visible'),
      contains(CURVES_CSS, ':focus-visible'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/f58ec046',
    scenario: 'folds the threshold/levels marker pair with zero-specificity :where()',
    checks: [
      contains(
        HISTOGRAM_CONTROLS_CSS,
        ':where(.retouch-threshold-control, .retouch-levels-control)',
      ),
      contains(HISTOGRAM_CONTROLS_CSS, '.retouch-histogram-plot__marker'),
      contains(HISTOGRAM_CONTROLS_CSS, 'var(--retouch-control-point-'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/371d3c69',
    scenario: 'keeps marker modes distinct across Levels button and Threshold range',
    checks: [
      contains(THRESHOLD_CONTROL, 'marker-mode="range"'),
      lacks(LEVELS_CONTROL, 'marker-mode="range"'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/24efd8bc',
    scenario: 'renders the four effect controls from one kind map behind component :is',
    checks: [
      contains(PROPERTIES_PANEL, 'RETOUCH_CONTROL_COMPONENTS'),
      contains(PROPERTIES_PANEL, 'component'),
      contains(PROPERTIES_PANEL, ':is="activeControl"'),
      contains(PROPERTIES_PANEL, 'threshold'),
      contains(PROPERTIES_PANEL, 'curves'),
      contains(PROPERTIES_PANEL, 'levels'),
      contains(PROPERTIES_PANEL, 'sharpen'),
      contains(PROPERTIES_PANEL, ':histogram-source'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/45774b8f',
    scenario: 'keeps the hsl branch and the sharpen histogram exception',
    checks: [
      contains(PROPERTIES_PANEL, "kind === 'hsl'"),
      contains(PROPERTIES_PANEL, "kind === 'sharpen'"),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/0a8bb689',
    scenario: 'derives kind icon/label/rail tables from definitions (structural half)',
    checks: [
      contains(VIEW_MODEL, 'RETOUCH_KIND_ICONS'),
      contains(VIEW_MODEL, 'RETOUCH_EFFECT_RAIL'),
      contains(EFFECT_ACTIONS, 'RETOUCH_EFFECT_RAIL'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/7f127e21',
    scenario: 'keeps LayersPanel table-driven with a generic fallback instead of throwing',
    checks: [
      contains(VIEW_MODEL, 'RETOUCH_UNKNOWN_KIND_ICON'),
      contains(VIEW_MODEL, 'resolveRetouchLayerIcon'),
      contains(LAYERS_PANEL, 'resolveRetouchLayerIcon'),
      lacks(LAYERS_PANEL, 'assertNever'),
      lacks(LAYERS_PANEL, 'Unhandled retouch layer kind'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/78ec4640',
    scenario: 'resolves Chrome rail and list selection through one view-model function',
    checks: [
      contains(VIEW_MODEL, 'resolveRetouchChromeSelection'),
      contains(CHROME, 'resolveRetouchChromeSelection'),
      contains(CHROME, 'chromeSelection.selectedKind'),
      contains(CHROME, 'chromeSelection.selectedLayerId'),
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/bf9a5ad7',
    scenario: 'exposes Chrome as RetouchChrome through the unique exit',
    checks: [
      contains(RETOUCH_BARREL, 'RetouchChrome'),
      contains(RETOUCH_BARREL, './ui/chrome/Chrome.vue'),
      {
        file: RETOUCH_BARREL,
        kind: 'lacksPattern',
        pattern: /export\s*\{\s*default\s+as\s+Chrome\s*[,}]/,
        description: 'RetouchChrome must stay the unique exit (no default-as-Chrome re-export)',
      },
    ],
  },
  {
    rule: 'source-policy/retouchShared.contract/4a90da6a',
    scenario: 'keeps levels/threshold markers locale-free (structural half)',
    checks: [
      lacks(LEVELS_CORE, 'ariaLabel:'),
      lacks(THRESHOLD_CORE, 'ariaLabel:'),
      contains(LEVELS_FIELDS, 'computed('),
    ],
  },
]

/**
 * Best-effort comment stripping so `contains` checks only match live code.
 * Preserves newlines to keep diagnostics stable. `lineComments` stays off
 * for CSS (where `//` is not a comment and appears in URLs).
 */
function stripComments(source: string, lineComments: boolean): string {
  let result = ''
  let index = 0
  let quote = ''
  while (index < source.length) {
    const current = source[index]
    const pair = source.slice(index, index + 2)
    if (quote !== '') {
      result += current
      if (current === '\\') {
        result += source[index + 1] ?? ''
        index += 2
        continue
      }
      if (current === quote) quote = ''
      index += 1
      continue
    }
    if (source.startsWith('<!--', index)) {
      const end = source.indexOf('-->', index + 4)
      const stop = end === -1 ? source.length : end + 3
      result += source.slice(index, stop).replace(/[^\n]/g, '')
      index = stop
      continue
    }
    if (pair === '/*') {
      const end = source.indexOf('*/', index + 2)
      const stop = end === -1 ? source.length : end + 2
      result += source.slice(index, stop).replace(/[^\n]/g, '')
      index = stop
      continue
    }
    if (lineComments && pair === '//') {
      const end = source.indexOf('\n', index + 2)
      index = end === -1 ? source.length : end
      continue
    }
    if (current === '"' || current === "'" || current === '`') quote = current
    result += current
    index += 1
  }
  return result
}

function strippedOf(
  sources: Record<string, string>,
  cache: Map<string, string>,
  file: string,
): string {
  const cached = cache.get(file)
  if (cached !== undefined) return cached
  const stripped = stripComments(sources[file] ?? '', !file.endsWith('.css'))
  cache.set(file, stripped)
  return stripped
}

/** Validate one policy against a file map (real sources or fixtures). */
export function checkRetouchSharedPolicy(
  policy: RetouchSharedPolicy,
  sources: Record<string, string>,
): RetouchSharedViolation[] {
  const violations: RetouchSharedViolation[] = []
  const cache = new Map<string, string>()
  for (const check of policy.checks) {
    const raw = sources[check.file] ?? ''
    if (raw === '') {
      violations.push({ rule: policy.rule, file: check.file, message: 'Missing source map entry' })
      continue
    }
    if (check.kind === 'contains') {
      if (!strippedOf(sources, cache, check.file).includes(check.text))
        violations.push({
          rule: policy.rule,
          file: check.file,
          message: `Missing live ${JSON.stringify(check.text)} (comments do not count)`,
        })
    } else if (check.kind === 'lacks') {
      if (raw.includes(check.text))
        violations.push({
          rule: policy.rule,
          file: check.file,
          message: `Retired ${JSON.stringify(check.text)} must not reappear`,
        })
    } else if (check.pattern.test(raw)) {
      violations.push({ rule: policy.rule, file: check.file, message: check.description })
    }
  }
  return violations
}

/** Validate every policy against a full source map. */
export function checkRetouchSharedSources(
  sources: Record<string, string>,
): RetouchSharedViolation[] {
  return RETOUCH_SHARED_POLICIES.flatMap((policy) => checkRetouchSharedPolicy(policy, sources))
}

/* ------------------------------------------------------------------ */
/* Self-test: every check above must reject the shape it exists to     */
/* reject. Runs before the production check so a green run proves      */
/* each rule is armed, not vacuous. Fixtures are derived from the      */
/* policy table itself: one canonical pass plus one negative per       */
/* check (required marker removed, retired marker reintroduced).       */
/* ------------------------------------------------------------------ */

type Fixture = {
  name: string
  policy: RetouchSharedPolicy
  sources: Record<string, string>
  expectPass: boolean
}

const FIXTURE_FALLBACK = 'retouch shared fixture placeholder'

function positiveSources(policy: RetouchSharedPolicy): Record<string, string> {
  const required = new Map<string, string[]>()
  for (const check of policy.checks) {
    if (check.kind !== 'contains') continue
    required.set(check.file, [...(required.get(check.file) ?? []), check.text])
  }
  const sources: Record<string, string> = {}
  for (const check of policy.checks) {
    if (check.file in sources) continue
    sources[check.file] = required.get(check.file)?.join('\n') ?? FIXTURE_FALLBACK
  }
  return sources
}

function buildFixtures(): Fixture[] {
  const fixtures: Fixture[] = []
  for (const policy of RETOUCH_SHARED_POLICIES) {
    fixtures.push({
      name: `${policy.rule} canonical sources pass`,
      policy,
      sources: positiveSources(policy),
      expectPass: true,
    })
    for (const check of policy.checks) {
      const sources = positiveSources(policy)
      if (check.kind === 'contains') {
        sources[check.file] = (sources[check.file] ?? '')
          .split('\n')
          .filter((line) => !line.includes(check.text))
          .join('\n')
        fixtures.push({
          name: `${policy.rule} without ${JSON.stringify(check.text)} fails`,
          policy,
          sources,
          expectPass: false,
        })
      } else if (check.kind === 'lacks') {
        sources[check.file] = `${sources[check.file] ?? ''}\n${check.text}\n`
        fixtures.push({
          name: `${policy.rule} reintroducing ${JSON.stringify(check.text)} fails`,
          policy,
          sources,
          expectPass: false,
        })
      } else {
        sources[check.file] = `${sources[check.file] ?? ''}\nexport { default as Chrome };\n`
        fixtures.push({
          name: `${policy.rule} reintroducing a default-as-Chrome exit fails`,
          policy,
          sources,
          expectPass: false,
        })
      }
    }
  }
  const decoy = (rule: string, file: string, content: string, name: string): void => {
    const policy = RETOUCH_SHARED_POLICIES.find((entry) => entry.rule === rule)
    if (!policy) throw new Error(`Unknown policy ${rule}`)
    const sources = positiveSources(policy)
    sources[file] = content
    fixtures.push({ name, policy, sources, expectPass: false })
  }
  decoy(
    'source-policy/retouchShared.contract/57f03081',
    PLOT_FRAME,
    '/* veil tokens were here: --canvas-glass-bg, --canvas-surface-muted, --canvas-text-subtle */\n.retouch-plot-frame { display: block; }\n',
    'commented-out veil tokens do not satisfy the veil rule',
  )
  decoy(
    'source-policy/retouchShared.contract/1093d62f',
    LAYERS_PANEL,
    '<!-- <div role="listbox" class="hit-target-32-inline canvas-tool-tooltip-trigger"> -->\n<div>layers</div>\n',
    'commented-out listbox markers do not satisfy the marker rule',
  )
  decoy(
    'source-policy/retouchShared.contract/20d5f590',
    CHROME,
    '/* canApply: boolean */\nconst gate = !canApply || applyingRetouch\n',
    'commented-out apply gate prop does not satisfy the gate rule',
  )
  decoy(
    'source-policy/retouchShared.contract/20d5f590',
    CHROME,
    'const gate = !canApply || applyingRetouch\n// canApply: boolean (see hasRenderableRetouchLayerStack history)\n',
    'retired stack reader in a comment still trips the raw-source ban',
  )
  return fixtures
}

function runSelfTest(): void {
  let failed = 0
  let negatives = 0
  for (const fixture of buildFixtures()) {
    const passed = checkRetouchSharedPolicy(fixture.policy, fixture.sources).length === 0
    if (!fixture.expectPass) negatives += 1
    if (passed !== fixture.expectPass) {
      failed += 1
      console.error(
        `[retouch-shared-self-test] ${fixture.name}: expected ${fixture.expectPass ? 'pass' : 'fail'}, got ${passed ? 'pass' : 'fail'}.`,
      )
    }
  }
  const total = buildFixtures().length
  if (failed > 0) {
    console.error(`\nRetouch shared self-test failed: ${failed} case(s) of ${total}.`)
    process.exit(1)
  }
  console.log(
    `Retouch shared self-test passed: ${total} case(s) (${negatives} negative, all armed).`,
  )
}

function loadRealSources(root: string): Record<string, string> {
  const files = [
    ...new Set(
      RETOUCH_SHARED_POLICIES.flatMap((policy) => policy.checks.map((check) => check.file)),
    ),
  ]
  return Object.fromEntries(files.map((file) => [file, readFileSync(join(root, file), 'utf8')]))
}

function main(): void {
  runSelfTest()
  const root = join(import.meta.dirname, '..', 'src')
  const violations = checkRetouchSharedSources(loadRealSources(root))
  if (violations.length > 0) {
    console.error('Retouch shared contract failed.\n')
    for (const violation of violations)
      console.error(`- ${violation.file}: ${violation.rule}: ${violation.message}`)
    console.error('')
    process.exit(1)
  }
  console.log(
    'Retouch shared contract passed: plot frame, control points, panel shell, preview/commit wiring, markers, kind map, view model, and barrel alias policies hold.',
  )
}

main()
