import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import postcss, { type Rule } from 'postcss'
import { parse as parseSfc } from '@vue/compiler-sfc'
import { NodeTypes, parse as parseTemplate } from '@vue/compiler-dom'

/**
 * Guard: canvas ui-area style contract retirements (P5 ui-area styles +
 * overlays lint-planned replacements).
 *
 * Replaces 45 source-string assertions retired from the canvas style
 * contract specs:
 *
 *   - `src/features/canvas/ui/styles/canvasSurfaceSkin.contract.spec.ts`
 *     (38 assertions, 6 replacement groups):
 *     - "mounts one feature-local surface owner ..." (5) ->
 *       `source-policy/canvasSurfaceSkin.contract/da951dfd`
 *       Ledger: source-cbb53e8d751f7d90-1, source-f863f8e6fd196af2-1,
 *       source-2e7c73d0621703cc-1, source-2261bcbec46a22ec-1,
 *       source-c4b3e602dd038600-1. Rule `surface-owner`.
 *     - "maps Canvas surface semantics to app tokens ..." (11) ->
 *       `source-policy/canvasSurfaceSkin.contract/7fbea095`
 *       Ledger: source-e37d78538a4e6369-1, source-21782940f9c55895-1,
 *       source-94863a14606f0588-1, source-f72164aecac9fdd5-1,
 *       source-0831b086ceea2f29-1, source-ba66c20605992e5d-1,
 *       source-f8b03acbfe464962-1, source-fffdba1e51a2839f-1,
 *       source-4040ff9363eb8d3c-1, source-6f4542e6a258101e-1,
 *       source-f21e2607de460d53-1. Rule `surface-token-mapping`.
 *     - "sources the tonal-range wash ..." (2) ->
 *       `source-policy/canvasSurfaceSkin.contract/bf5c696a`
 *       Ledger: source-cd7b3c0014b358fc-1, source-b3c62ab940a4c28e-1.
 *       Rule `surface-wash`.
 *     - "keeps Canvas consumers on the named surface contract ..." (3 x 9
 *       consumers, looped as one matcher set) ->
 *       `source-policy/canvasSurfaceSkin.contract/c9d2db74`
 *       Ledger: source-23d573d40df1d45d-1, source-aa1d6400dbb4b8ff-1,
 *       source-b2566bde23ad4ae0-1. Rule `consumer-surface-contract`.
 *     - "owns tooltips at the Canvas feature boundary ..." (7) ->
 *       `source-policy/canvasSurfaceSkin.contract/54661dc6`
 *       Ledger: source-46fb4d5c3334fedd-1, source-c5d71481f7b76c30-1,
 *       source-6033358578151d51-1, source-f9160f6f138a2416-1,
 *       source-30a9df87eae66bad-1, source-71b3a7af8c09204b-1,
 *       source-7335b64819300a32-1. Rule `tooltip-boundary`.
 *     - "contains rendered retouch previews ..." (10) ->
 *       `source-policy/canvasSurfaceSkin.contract/5d107033`
 *       Ledger: source-118580e359fff593-1, source-5fa05fdccce90709-1,
 *       source-9b897e3fc0067b5c-1, source-833d8577061054d5-1,
 *       source-93606640160e47f4-1, source-fc95dcb455956a8a-1,
 *       source-62a98523c616ac0e-1, source-ca6da6876e67de05-1,
 *       source-3e3d974edb3ce067-1, source-14e1038f88bbb394-1.
 *       Rule `retouch-preview-geometry`.
 *   - `src/features/canvas/ui/overlays/CanvasGroupFramesOverlay.component.spec.ts`
 *     (7 assertions, 1 group):
 *     - "uses the cursor selection stroke width ..." (7) ->
 *       `source-policy/CanvasGroupFramesOverlay.component/89c121de`
 *       Ledger: source-d880bb7135287592-1, source-6afca5dc03ee30d2-1,
 *       source-2f051ca903761065-1, source-e488601f448c871a-1,
 *       source-521edb4dab2d2fa4-1, source-88b49d390f914daa-1,
 *       source-cf435087c7b3738e-1. Rule `group-frame-ring`.
 *
 * Why a script instead of the specs: the specs asserted raw substrings of
 * the implementation (`toContain`). That passes for commented-out markup,
 * dead rules, or an unrelated match elsewhere in the file. The checks below
 * keep the same intent but assert parsed linkage:
 *
 *   - Class ownership is read from the parsed template AST (comments and
 *     script strings cannot satisfy a rule).
 *   - Token mappings are read as PostCSS declarations (selector + prop +
 *     resolved value), so a commented-out line or a mention in prose fails.
 *   - Consumer boundaries are enforced per declaration value, so a renamed
 *     variable or a dropped `var()` indirection fails.
 *
 * The checks parse sources rather than searching text. Fixture workspaces
 * below prove every rule rejects the shape it exists to reject (red-proof).
 */

export type CanvasUiStyleViolation = { rule: string; file: string; message: string }

export const SURFACE_OWNER_RULE = 'surface-owner'
export const SURFACE_TOKEN_MAPPING_RULE = 'surface-token-mapping'
export const SURFACE_WASH_RULE = 'surface-wash'
export const CONSUMER_SURFACE_CONTRACT_RULE = 'consumer-surface-contract'
export const TOOLTIP_BOUNDARY_RULE = 'tooltip-boundary'
export const RETOUCH_PREVIEW_GEOMETRY_RULE = 'retouch-preview-geometry'
export const GROUP_FRAME_RING_RULE = 'group-frame-ring'

export const CANVAS_SCREEN_FILE = 'features/canvas/ui/screens/CanvasScreen.vue'
export const SURFACE_SKIN_FILE = 'features/canvas/ui/styles/canvas-surface-skin.css'
export const APP_STYLE_FILE = 'app/style.css'
export const CHROME_SKIN_FILE = 'features/canvas/ui/styles/canvas-chrome.css'
export const STYLE_CONTRACTS_FILE = 'features/canvas/ui/styles/canvas-style-contracts.css'
export const TOOLTIP_SKIN_FILE = 'features/canvas/ui/styles/canvas-tool-tooltip.css'
export const NODE_SKIN_FILE = 'features/canvas/ui/styles/node-shared.css'
export const NODE_TOOL_SKIN_FILE = 'features/canvas/ui/styles/node-tool-buttons.css'
export const ADAPTER_SKIN_FILE = 'features/canvas/ui/styles/canvas-vue-flow-adapter-skin.css'
export const MINIMAP_FILE = 'features/canvas/ui/chrome/CanvasMiniMap.vue'
export const HANDLE_RAIL_FILE = 'features/canvas/ui/nodes/FlowCanvasNodeHandleRail.vue'
export const NODE_STAGE_FRAME_FILE = 'features/canvas/ui/nodes/primitives/NodeStageFrame.vue'
export const STAGE_PRESENTATION_FILE = 'features/canvas/tools/image-tools/ui/StagePresentation.vue'
export const RETOUCH_LAYER_ACTIONS_FILE = 'features/canvas/tools/retouch/ui/layers/LayerActions.vue'
export const RETOUCH_HISTOGRAM_FILE =
  'features/canvas/tools/retouch/ui/shared/styles/retouch-histogram-plot.css'
export const INPAINT_CHROME_FILE = 'features/canvas/tools/inpaint/ui/ImageInpaintChrome.vue'
export const GROUP_FRAMES_OVERLAY_FILE = 'features/canvas/ui/overlays/CanvasGroupFramesOverlay.vue'

export const CONSUMER_FILES = [
  CHROME_SKIN_FILE,
  NODE_SKIN_FILE,
  NODE_TOOL_SKIN_FILE,
  NODE_STAGE_FRAME_FILE,
  ADAPTER_SKIN_FILE,
  MINIMAP_FILE,
  RETOUCH_LAYER_ACTIONS_FILE,
  RETOUCH_HISTOGRAM_FILE,
  INPAINT_CHROME_FILE,
] as const

type TemplateNode = {
  type?: number
  tag?: string
  props?: TemplateProp[]
  children?: TemplateNode[]
  loc?: { source?: string }
}

type TemplateProp = {
  type?: number
  name?: string
  value?: { content?: string }
  exp?: { content?: string }
  arg?: { content?: string; isStatic?: boolean }
  loc?: { source?: string }
}

function normalize(value: string): string {
  return value.replace(/\s+/g, '').toLowerCase()
}

function styleContents(source: string): string[] {
  try {
    const parsed = parseSfc(source, { filename: 'canvas-ui-contract-fixture.vue' })
    if (parsed.errors.length > 0) return []
    return parsed.descriptor.styles.map((block) => block.content)
  } catch {
    return []
  }
}

function templateNodes(source: string): TemplateNode[] | undefined {
  try {
    const parsed = parseSfc(source, { filename: 'canvas-ui-contract-fixture.vue' })
    if (parsed.errors.length > 0 || !parsed.descriptor.template) return undefined
    const root = parseTemplate(parsed.descriptor.template.content) as unknown as TemplateNode
    const nodes: TemplateNode[] = []
    const visit = (node: TemplateNode) => {
      if (node.type === NodeTypes.ELEMENT) nodes.push(node)
      for (const child of node.children ?? []) visit(child)
    }
    visit(root)
    return nodes
  } catch {
    return undefined
  }
}

function staticClassTokens(node: TemplateNode): string[] {
  const prop = (node.props ?? []).find(
    (item) => item.type === NodeTypes.ATTRIBUTE && item.name === 'class',
  )
  return (prop?.value?.content ?? '').split(/\s+/).filter((token) => token.length > 0)
}

function boundClassExpressions(node: TemplateNode): string[] {
  return (node.props ?? [])
    .filter(
      (item) =>
        item.type === NodeTypes.DIRECTIVE &&
        item.name === 'bind' &&
        item.arg?.isStatic === true &&
        item.arg.content === 'class' &&
        typeof item.exp?.content === 'string',
    )
    .map((item) => item.exp?.content ?? '')
}

function allBindingExpressions(nodes: TemplateNode[]): string[] {
  return nodes.flatMap((node) =>
    (node.props ?? []).flatMap((prop) =>
      prop.type === NodeTypes.DIRECTIVE && typeof prop.exp?.content === 'string'
        ? [prop.exp.content]
        : [],
    ),
  )
}

/** Comment-proof token scan over parsed CSS: selectors plus declaration props/values. */
function parsedCssText(css: string): string {
  try {
    const root = postcss.parse(css, { from: 'canvas-ui-contract-fixture.css' })
    const parts: string[] = []
    root.walkRules((rule) => {
      parts.push(rule.selector)
    })
    root.walkDecls((decl) => {
      parts.push(`${decl.prop}:${decl.value}`)
    })
    root.walkAtRules((rule) => {
      parts.push(`@${rule.name} ${rule.params}`)
    })
    return parts.join('\n')
  } catch {
    return ''
  }
}

/** Comment-proof token scan over an SFC: parsed styles plus template bindings. */
function parsedSfcText(source: string): string {
  const styles = styleContents(source).map(parsedCssText).join('\n')
  const nodes = templateNodes(source) ?? []
  const template = [
    ...nodes.flatMap(staticClassTokens),
    ...allBindingExpressions(nodes),
    ...nodes.flatMap((node) =>
      (node.props ?? []).flatMap((prop) =>
        prop.type === NodeTypes.ATTRIBUTE ? [`${prop.name}=${prop.value?.content ?? ''}`] : [],
      ),
    ),
  ].join('\n')
  return `${styles}\n${template}`
}

function declarationsOf(css: string, selector: string): Map<string, string> {
  const result = new Map<string, string>()
  try {
    const root = postcss.parse(css, { from: 'canvas-ui-contract-fixture.css' })
    root.walkRules((rule) => {
      if (rule.selector === selector) {
        rule.walkDecls((decl) => {
          if (decl.parent === rule && !result.has(decl.prop)) result.set(decl.prop, decl.value)
        })
      }
    })
  } catch {
    // Parse failure surfaces as a missing declaration below (fail-closed).
  }
  return result
}

function allDeclarations(css: string): Map<string, string> {
  const result = new Map<string, string>()
  try {
    const root = postcss.parse(css, { from: 'canvas-ui-contract-fixture.css' })
    root.walkDecls((decl) => {
      if (!result.has(decl.prop)) result.set(decl.prop, decl.value)
    })
  } catch {
    // Parse failure surfaces as a missing declaration below (fail-closed).
  }
  return result
}

function requireDeclarations(
  issues: string[],
  css: string,
  selector: string,
  expected: Record<string, string>,
  label: string,
): void {
  const actual = declarationsOf(css, selector)
  for (const [prop, value] of Object.entries(expected)) {
    if (normalize(actual.get(prop) ?? '') !== normalize(value)) {
      issues.push(`${label} must declare ${prop}: ${value}.`)
    }
  }
}

/** CanvasScreen mounts the feature-local surface owner; the stable token table lives in the skin. */
export function validateSurfaceOwner(screenSource: string, skinCss: string): string[] {
  const issues: string[] = []
  const nodes = templateNodes(screenSource)
  if (!nodes) return ['CanvasScreen source must be a valid SFC with a parseable template.']
  const mounted = nodes.some(
    (node) =>
      staticClassTokens(node).includes('canvas-surface-skin') ||
      boundClassExpressions(node).some((expression) => expression.includes('canvas-surface-skin')),
  )
  if (!mounted) issues.push('CanvasScreen must mount the canvas-surface-skin class.')
  for (const style of styleContents(screenSource)) {
    try {
      const root = postcss.parse(style, { from: CANVAS_SCREEN_FILE })
      root.walkDecls((decl) => {
        if (/^--canvas-[a-z0-9-]+\s*$/.test(decl.prop))
          issues.push(`CanvasScreen must not declare the stable token ${decl.prop}.`)
        if (/^--flow-[a-z0-9-]+\s*$/.test(decl.prop))
          issues.push(`CanvasScreen must not declare the stable token ${decl.prop}.`)
      })
    } catch {
      issues.push('CanvasScreen styles must parse as CSS.')
    }
  }
  if (!/\.canvas-surface-skin\b/.test(parsedCssText(skinCss)))
    issues.push('The surface skin must own the .canvas-surface-skin rule.')
  if (!/html\.canvas-overscroll-guard/.test(parsedCssText(skinCss)))
    issues.push('The surface skin must own the html.canvas-overscroll-guard rule.')
  return issues
}

/** The named Canvas contract resolves through app tokens with pinned values. */
export function validateSurfaceTokenMapping(
  skinCss: string,
  appCss: string,
  handleRailSource: string,
): string[] {
  const issues: string[] = []
  requireDeclarations(
    issues,
    skinCss,
    '.canvas-surface-skin',
    {
      '--canvas-surface-bg': 'var(--color-canvas-surface-bg)',
      '--canvas-control-border-color': 'var(--color-canvas-neutral-line)',
      '--flow-edge-color': 'var(--color-canvas-edge)',
      '--canvas-accent': 'var(--color-canvas-accent)',
      '--canvas-accent-hover': 'var(--color-canvas-accent-hover)',
      '--canvas-text-muted': 'var(--color-canvas-text-secondary)',
      '--flow-edge-running-duration': '0.5s',
    },
    'The surface skin',
  )
  if (!/--canvas-shadow-floating\s*:/.test(parsedCssText(skinCss)))
    issues.push('The surface skin must declare --canvas-shadow-floating.')
  const appDecls = allDeclarations(appCss)
  if (
    normalize(appDecls.get('--color-page-chrome-border') ?? '') !==
    normalize('var(--color-board-card-border)')
  )
    issues.push(
      'The app theme must resolve --color-page-chrome-border through var(--color-board-card-border).',
    )
  const neutralLine = appDecls.get('--color-canvas-neutral-line') ?? ''
  if (normalize(neutralLine) !== normalize('rgb(177 177 183 / 65%)'))
    issues.push('The app theme must pin --color-canvas-neutral-line to rgb(177 177 183 / 65%).')
  const railStyles = styleContents(handleRailSource).join('\n')
  const railDecls = new Map<string, string>()
  try {
    const root = postcss.parse(railStyles || handleRailSource, {
      from: HANDLE_RAIL_FILE,
    })
    root.walkDecls((decl) => {
      if (!railDecls.has(decl.prop)) railDecls.set(decl.prop, decl.value)
    })
  } catch {
    // Fail-closed below.
  }
  if (normalize(railDecls.get('--handle-ring-color') ?? '') !== normalize('var(--flow-edge-color)'))
    issues.push('The handle rail must resolve --handle-ring-color through var(--flow-edge-color).')
  return issues
}

/** The tonal-range wash resolves through the app wash token with its fallback. */
export function validateSurfaceWash(skinCss: string, appCss: string): string[] {
  const issues: string[] = []
  requireDeclarations(
    issues,
    skinCss,
    '.canvas-surface-skin',
    {
      '--canvas-accent-wash': 'var(--color-canvas-accent-wash, rgba(122, 107, 213, 0.08))',
    },
    'The surface skin',
  )
  if (!/--color-canvas-accent-wash\s*:/.test(parsedCssText(appCss)))
    issues.push('The app theme must declare --color-canvas-accent-wash.')
  return issues
}

/** Consumers stay on the named surface contract instead of app token aliases. */
export function validateConsumerSurfaceContract(source: string, file: string): string[] {
  const issues: string[] = []
  const text = /\.vue$/.test(file) ? parsedSfcText(source) : parsedCssText(source)
  if (!text) return [`${file} must parse before the consumer contract applies.`]
  // The named contract spans the --canvas-* token table and the --node-tool-*
  // customization points owned by node-tool-surface (which resolves through
  // --canvas-*); either proves the consumer rides the contract.
  if (!text.includes('--canvas-') && !text.includes('--node-tool-'))
    issues.push(`${file} must consume the named Canvas surface contract (--canvas-*).`)
  if (text.includes('--color-canvas-'))
    issues.push(`${file} must not alias app Canvas tokens directly (--color-canvas-*).`)
  try {
    const css = /\.vue$/.test(file) ? styleContents(source).join('\n') : source
    const root = postcss.parse(css, { from: file })
    root.walkDecls((decl) => {
      if (
        /var\(--canvas-(?:surface|accent|text)[^)]*,\s*(?:var\(--color-canvas|#[0-9a-fA-F]|rgba?\(|rgb\()/.test(
          decl.value,
        )
      )
        issues.push(
          `${file} must not smuggle a literal fallback into a surface/accent/text alias (${decl.prop}).`,
        )
    })
  } catch {
    issues.push(`${file} styles must parse as CSS.`)
  }
  return issues
}

/** Tooltips live at the Canvas boundary; legacy node-only contracts stay retired. */
export function validateTooltipBoundary(
  styleContractsCss: string,
  tooltipCss: string,
  nodeToolCss: string,
  nodeCss: string,
): string[] {
  const issues: string[] = []
  const imports: string[] = []
  try {
    postcss
      .parse(styleContractsCss, { from: STYLE_CONTRACTS_FILE })
      .walkAtRules('import', (rule) => {
        imports.push(rule.params.replace(/^['"]|['"]$/g, ''))
      })
  } catch {
    // Fail-closed below.
  }
  if (!imports.includes('./canvas-tool-tooltip.css'))
    issues.push('The style contracts must import ./canvas-tool-tooltip.css.')
  requireDeclarations(issues, tooltipCss, '.canvas-tool-tooltip', {}, 'The tooltip skin')
  const tooltipText = parsedCssText(tooltipCss)
  for (const token of ['.canvas-tool-tooltip', '.canvas-tool-tooltip-trigger']) {
    if (!tooltipText.includes(token)) issues.push(`The tooltip skin must own ${token}.`)
  }
  requireDeclarations(
    issues,
    tooltipCss,
    '.canvas-tool-tooltip-trigger',
    {
      '--canvas-tool-tooltip-delay': '200ms',
    },
    'The tooltip skin',
  )
  if (!/transition-delay\s*:\s*var\(--canvas-tool-tooltip-delay\)/.test(tooltipText))
    issues.push('The tooltip reveal must ride --canvas-tool-tooltip-delay.')
  if (parsedCssText(nodeToolCss).includes('node-tool-tooltip'))
    issues.push('The node-tool skin must not redeclare the retired node-tool-tooltip contract.')
  if (parsedCssText(nodeCss).includes('node-inline-range'))
    issues.push('The node skin must not redeclare the retired node-inline-range contract.')
  return issues
}

/** Rendered retouch previews stay inside the existing image stage geometry. */
export function validateRetouchPreviewGeometry(source: string): string[] {
  const issues: string[] = []
  const styles = styleContents(source)
  if (!styles.length) return ['Stage presentation styles must parse as CSS.']
  const css = styles.join('\n')
  let preview: Rule | undefined
  try {
    const root = postcss.parse(css, { from: STAGE_PRESENTATION_FILE })
    root.walkRules((rule) => {
      if (rule.selector === '.image-node-stage-image--preview' && !preview) preview = rule
    })
  } catch {
    return ['Stage presentation styles must parse as CSS.']
  }
  if (!preview) return ['Stage presentation must own the .image-node-stage-image--preview rule.']
  const actual = new Map<string, string>()
  preview.walkDecls((decl) => {
    if (!actual.has(decl.prop)) actual.set(decl.prop, decl.value)
  })
  for (const [prop, value] of Object.entries({
    display: 'block',
    width: '100%',
    height: '100%',
    'object-fit': 'contain',
    'pointer-events': 'none',
  })) {
    if (normalize(actual.get(prop) ?? '') !== normalize(value))
      issues.push(`The preview rule must declare ${prop}: ${value}.`)
  }
  const nodes = templateNodes(source)
  if (!nodes) return ['Stage presentation must be a valid SFC with a parseable template.']
  const classes = nodes.flatMap(staticClassTokens)
  if (!classes.includes('image-tool-stage-zero-copy'))
    issues.push('Stage presentation must mount the image-tool-stage-zero-copy canvas.')
  if (!nodes.some((node) => node.tag === 'slot'))
    issues.push('Stage presentation must keep the slot for the CPU preview beneath.')
  if (
    allBindingExpressions(nodes).some((expression) => expression.includes('has-rendered-preview'))
  )
    issues.push('Stage presentation must not gate on a has-rendered-preview flag.')
  try {
    const root = postcss.parse(css, { from: STAGE_PRESENTATION_FILE })
    root.walkDecls((decl) => {
      if (decl.prop === 'visibility' && normalize(decl.value) === 'hidden')
        issues.push('Stage presentation must not hide the stage with visibility: hidden.')
    })
  } catch {
    issues.push('Stage presentation styles must parse as CSS.')
  }
  return issues
}

/** The selected group frame ring rides the cursor selection stroke width. */
export function validateGroupFrameRing(adapterCss: string, overlaySource: string): string[] {
  const issues: string[] = []
  requireDeclarations(
    issues,
    adapterCss,
    '.canvas-flow-wrap',
    {
      '--canvas-selection-stroke-width': '1px',
      '--canvas-group-frame-selected-ring-width': 'var(--canvas-selection-stroke-width)',
      '--canvas-group-frame-selected-ring-color': 'var(--canvas-selection-group-border)',
    },
    'The adapter skin',
  )
  const adapterText = parsedCssText(adapterCss)
  if (
    !/border\s*:\s*var\(--canvas-selection-stroke-width\)\s*solid\s*var\(--canvas-selection-active-border\)/.test(
      adapterText,
    )
  )
    issues.push('The adapter skin must draw the active selection border from the stroke width.')
  if (
    !/border\s*:\s*var\(--canvas-selection-stroke-width\)\s*solid\s*var\(--canvas-selection-group-border\)/.test(
      adapterText,
    )
  )
    issues.push('The adapter skin must draw the group selection border from the stroke width.')
  const overlayStyles = styleContents(overlaySource).join('\n')
  const overlayText = parsedCssText(overlayStyles)
  if (!/box-shadow\s*:\s*0 0 0 var\(--canvas-group-frame-selected-ring-width\)/.test(overlayText))
    issues.push(
      'The overlay must paint the selected ring from --canvas-group-frame-selected-ring-width.',
    )
  if (!overlayText.includes('--canvas-group-frame-selected-ring-color'))
    issues.push(
      'The overlay must paint the selected ring from --canvas-group-frame-selected-ring-color.',
    )
  return issues
}

export function checkCanvasUiStyleSources(
  sources: Record<string, string>,
): CanvasUiStyleViolation[] {
  const violations: CanvasUiStyleViolation[] = []
  const push = (rule: string, file: string, messages: string[]) => {
    for (const message of messages) violations.push({ rule, file, message })
  }
  push(
    SURFACE_OWNER_RULE,
    CANVAS_SCREEN_FILE,
    validateSurfaceOwner(sources[CANVAS_SCREEN_FILE] ?? '', sources[SURFACE_SKIN_FILE] ?? ''),
  )
  push(
    SURFACE_TOKEN_MAPPING_RULE,
    SURFACE_SKIN_FILE,
    validateSurfaceTokenMapping(
      sources[SURFACE_SKIN_FILE] ?? '',
      sources[APP_STYLE_FILE] ?? '',
      sources[HANDLE_RAIL_FILE] ?? '',
    ),
  )
  push(
    SURFACE_WASH_RULE,
    SURFACE_SKIN_FILE,
    validateSurfaceWash(sources[SURFACE_SKIN_FILE] ?? '', sources[APP_STYLE_FILE] ?? ''),
  )
  for (const file of CONSUMER_FILES) {
    push(
      CONSUMER_SURFACE_CONTRACT_RULE,
      file,
      validateConsumerSurfaceContract(sources[file] ?? '', file),
    )
  }
  push(
    TOOLTIP_BOUNDARY_RULE,
    STYLE_CONTRACTS_FILE,
    validateTooltipBoundary(
      sources[STYLE_CONTRACTS_FILE] ?? '',
      sources[TOOLTIP_SKIN_FILE] ?? '',
      sources[NODE_TOOL_SKIN_FILE] ?? '',
      sources[NODE_SKIN_FILE] ?? '',
    ),
  )
  push(
    RETOUCH_PREVIEW_GEOMETRY_RULE,
    STAGE_PRESENTATION_FILE,
    validateRetouchPreviewGeometry(sources[STAGE_PRESENTATION_FILE] ?? ''),
  )
  push(
    GROUP_FRAME_RING_RULE,
    ADAPTER_SKIN_FILE,
    validateGroupFrameRing(
      sources[ADAPTER_SKIN_FILE] ?? '',
      sources[GROUP_FRAMES_OVERLAY_FILE] ?? '',
    ),
  )
  return violations
}

/* ------------------------------------------------------------------ */
/* Self-test: every rule above must reject the shape it exists to      */
/* reject. Runs before the production check so a green run proves the  */
/* rules are armed, not vacuous.                                       */
/* ------------------------------------------------------------------ */

const SCREEN_POSITIVE = `<template>
  <div class="canvas-layout canvas-surface-skin"><slot /></div>
</template>
<script setup lang="ts"></script>
<style scoped>
.canvas-layout { overflow: hidden; --color-page-dot: var(--color-canvas-dot); }
</style>`

const SKIN_POSITIVE = `html.canvas-overscroll-guard,
body.canvas-overscroll-guard {
  overscroll-behavior: none;
}
.canvas-surface-skin {
  --canvas-control-border-color: var(--color-canvas-neutral-line);
  --canvas-surface-bg: var(--color-canvas-surface-bg);
  --canvas-shadow-floating: var(--primitive-shadow-xl);
  --canvas-accent: var(--color-canvas-accent);
  --canvas-accent-hover: var(--color-canvas-accent-hover);
  --canvas-text-muted: var(--color-canvas-text-secondary);
  --canvas-accent-wash: var(--color-canvas-accent-wash, rgba(122, 107, 213, 0.08));
  --flow-edge-color: var(--color-canvas-edge);
  --flow-edge-running-duration: 0.5s;
}`

const APP_POSITIVE = `@theme {
  --color-canvas-neutral-line: rgb(177 177 183 / 65%);
  --color-page-chrome-border: var(--color-board-card-border);
  --color-canvas-accent-wash: color-mix(in srgb, #7a6bd5 12%, transparent);
}`

const RAIL_POSITIVE = `<template><div /></template>
<style scoped>
.handle { --handle-ring-color: var(--flow-edge-color); border: 1px solid var(--handle-ring-color); }
</style>`

const CONSUMER_POSITIVE = `.consumer {
  background: var(--canvas-surface-bg);
  border-color: var(--canvas-accent);
}`

const CONTRACTS_POSITIVE = `@import './canvas-chrome.css';
@import './canvas-tool-tooltip.css';`

const TOOLTIP_POSITIVE = `.canvas-tool-tooltip {
  opacity: 0;
  transition: opacity var(--motion-feedback-duration) var(--motion-feedback-easing);
}
.canvas-tool-tooltip-trigger {
  --canvas-tool-tooltip-delay: 200ms;
  position: relative;
}
.canvas-tool-tooltip-trigger:not(:disabled):hover .canvas-tool-tooltip {
  opacity: 1;
  transition-delay: var(--canvas-tool-tooltip-delay);
}`

const STAGE_POSITIVE = `<template>
  <div class="image-tool-stage-presentation">
    <slot />
    <canvas class="image-node-stage-image image-node-stage-image--preview image-tool-stage-zero-copy" />
  </div>
</template>
<style scoped>
.image-node-stage-image--preview {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
}
</style>`

const ADAPTER_POSITIVE = `.canvas-flow-wrap {
  --canvas-selection-stroke-width: 1px;
  --canvas-group-frame-selected-ring-width: var(--canvas-selection-stroke-width);
  --canvas-group-frame-selected-ring-color: var(--canvas-selection-group-border);
}
.canvas-flow-wrap .vue-flow__selection {
  border: var(--canvas-selection-stroke-width) solid var(--canvas-selection-active-border);
}
.canvas-flow-wrap .vue-flow__nodesselection-rect::before {
  border: var(--canvas-selection-stroke-width) solid var(--canvas-selection-group-border);
}`

const OVERLAY_POSITIVE = `<template><div class="canvas-group-frame-body" /></template>
<style scoped>
.canvas-group-frame-body.is-selected {
  box-shadow: 0 0 0 var(--canvas-group-frame-selected-ring-width)
    var(--canvas-group-frame-selected-ring-color);
}
</style>`

type Fixture = { name: string; run: () => string[]; expectPass: boolean }

function fixture(name: string, run: () => string[], expectPass: boolean): Fixture {
  return { name, run, expectPass }
}

const FIXTURES: Fixture[] = [
  fixture(
    'surface owner canonical passes',
    () => validateSurfaceOwner(SCREEN_POSITIVE, SKIN_POSITIVE),
    true,
  ),
  fixture(
    'surface owner without the mounted class fails',
    () => validateSurfaceOwner(SCREEN_POSITIVE.replace(' canvas-surface-skin', ''), SKIN_POSITIVE),
    false,
  ),
  fixture(
    'surface owner comment decoy without a real class fails',
    () =>
      validateSurfaceOwner(
        `<!-- <div class="canvas-surface-skin" /> -->\n${SCREEN_POSITIVE.replace(' canvas-surface-skin', '')}`,
        SKIN_POSITIVE,
      ),
    false,
  ),
  fixture(
    'surface owner screen token declaration fails',
    () =>
      validateSurfaceOwner(
        SCREEN_POSITIVE.replace(
          '--color-page-dot:',
          '--canvas-screen-bg: #fff;\n  --color-page-dot:',
        ),
        SKIN_POSITIVE,
      ),
    false,
  ),
  fixture(
    'surface owner screen flow token declaration fails',
    () =>
      validateSurfaceOwner(
        SCREEN_POSITIVE.replace('.canvas-layout {', '.canvas-layout {\n  --flow-edge-width: 3;'),
        SKIN_POSITIVE,
      ),
    false,
  ),
  fixture(
    'surface owner missing skin rule fails',
    () =>
      validateSurfaceOwner(
        SCREEN_POSITIVE,
        SKIN_POSITIVE.replace('.canvas-surface-skin {', '.canvas-surface {'),
      ),
    false,
  ),
  fixture(
    'surface owner missing overscroll guard fails',
    () =>
      validateSurfaceOwner(
        SCREEN_POSITIVE,
        SKIN_POSITIVE.replace('html.canvas-overscroll-guard,', 'html.canvas-scroll,'),
      ),
    false,
  ),
  fixture(
    'token mapping canonical passes',
    () => validateSurfaceTokenMapping(SKIN_POSITIVE, APP_POSITIVE, RAIL_POSITIVE),
    true,
  ),
  fixture(
    'token mapping hardcoded surface value fails',
    () =>
      validateSurfaceTokenMapping(
        SKIN_POSITIVE.replace(
          '--canvas-surface-bg: var(--color-canvas-surface-bg);',
          '--canvas-surface-bg: #1c1c21;',
        ),
        APP_POSITIVE,
        RAIL_POSITIVE,
      ),
    false,
  ),
  fixture(
    'token mapping wrong neutral line fails',
    () =>
      validateSurfaceTokenMapping(
        SKIN_POSITIVE,
        APP_POSITIVE.replace('rgb(177 177 183 / 65%)', 'rgb(0 0 0 / 50%)'),
        RAIL_POSITIVE,
      ),
    false,
  ),
  fixture(
    'token mapping rail bypassing the edge token fails',
    () =>
      validateSurfaceTokenMapping(
        SKIN_POSITIVE,
        APP_POSITIVE,
        RAIL_POSITIVE.replace('var(--flow-edge-color)', 'var(--color-canvas-edge)'),
      ),
    false,
  ),
  fixture(
    'surface wash canonical passes',
    () => validateSurfaceWash(SKIN_POSITIVE, APP_POSITIVE),
    true,
  ),
  fixture(
    'surface wash hardcoded fallback fails',
    () =>
      validateSurfaceWash(
        SKIN_POSITIVE.replace(
          'var(--color-canvas-accent-wash, rgba(122, 107, 213, 0.08))',
          'rgba(122, 107, 213, 0.08)',
        ),
        APP_POSITIVE,
      ),
    false,
  ),
  fixture(
    'surface wash missing app token fails',
    () =>
      validateSurfaceWash(
        SKIN_POSITIVE,
        APP_POSITIVE.replace('--color-canvas-accent-wash:', '--color-canvas-wash:'),
      ),
    false,
  ),
  fixture(
    'consumer canonical passes',
    () => validateConsumerSurfaceContract(CONSUMER_POSITIVE, 'consumer.css'),
    true,
  ),
  fixture(
    'consumer riding the node-tool customization points passes',
    () =>
      validateConsumerSurfaceContract(
        `<template><div class="retouch-layer-actions node-tool-surface" /></template>\n<style scoped>\n.actions { gap: var(--node-tool-button-gap); }\n</style>`,
        'LayerActions.vue',
      ),
    true,
  ),
  fixture(
    'consumer without surface usage fails',
    () => validateConsumerSurfaceContract('.consumer { color: red; }', 'consumer.css'),
    false,
  ),
  fixture(
    'consumer aliasing app tokens fails',
    () =>
      validateConsumerSurfaceContract(
        `.consumer { background: var(--color-canvas-surface-bg); color: var(--canvas-text-body); }`,
        'consumer.css',
      ),
    false,
  ),
  fixture(
    'consumer literal fallback fails',
    () =>
      validateConsumerSurfaceContract(
        `.consumer { background: var(--canvas-surface-bg, #1c1c21); }`,
        'consumer.css',
      ),
    false,
  ),
  fixture(
    'consumer comment decoy does not satisfy usage',
    () =>
      validateConsumerSurfaceContract(
        `/* uses var(--canvas-surface-bg) */\n.consumer { color: red; }`,
        'consumer.css',
      ),
    false,
  ),
  fixture(
    'consumer commented-out alias does not fail',
    () =>
      validateConsumerSurfaceContract(
        `/* retired: var(--color-canvas-surface-bg) */\n${CONSUMER_POSITIVE}`,
        'consumer.css',
      ),
    true,
  ),
  fixture(
    'tooltip boundary canonical passes',
    () =>
      validateTooltipBoundary(
        CONTRACTS_POSITIVE,
        TOOLTIP_POSITIVE,
        '.node-tool-surface {}',
        '.workflow-node {}',
      ),
    true,
  ),
  fixture(
    'tooltip boundary missing import fails',
    () =>
      validateTooltipBoundary(
        CONTRACTS_POSITIVE.replace("'./canvas-tool-tooltip.css'", "'./node-shared.css'"),
        TOOLTIP_POSITIVE,
        '.node-tool-surface {}',
        '.workflow-node {}',
      ),
    false,
  ),
  fixture(
    'tooltip boundary legacy node tooltip fails',
    () =>
      validateTooltipBoundary(
        CONTRACTS_POSITIVE,
        TOOLTIP_POSITIVE,
        '.node-tool-tooltip { opacity: 1; }',
        '.workflow-node {}',
      ),
    false,
  ),
  fixture(
    'tooltip boundary legacy inline range fails',
    () =>
      validateTooltipBoundary(
        CONTRACTS_POSITIVE,
        TOOLTIP_POSITIVE,
        '.node-tool-surface {}',
        '.node-inline-range { color: red; }',
      ),
    false,
  ),
  fixture(
    'retouch preview canonical passes',
    () => validateRetouchPreviewGeometry(STAGE_POSITIVE),
    true,
  ),
  fixture(
    'retouch preview wrong geometry fails',
    () =>
      validateRetouchPreviewGeometry(
        STAGE_POSITIVE.replace('object-fit: contain;', 'object-fit: cover;'),
      ),
    false,
  ),
  fixture(
    'retouch preview missing slot fails',
    () => validateRetouchPreviewGeometry(STAGE_POSITIVE.replace('<slot />', '<div />')),
    false,
  ),
  fixture(
    'retouch preview flag gate fails',
    () =>
      validateRetouchPreviewGeometry(
        STAGE_POSITIVE.replace(
          'class="image-node-stage-image image-node-stage-image--preview image-tool-stage-zero-copy"',
          ':class="{ \'has-rendered-preview\': hasPreview }" class="image-node-stage-image--preview image-tool-stage-zero-copy"',
        ),
      ),
    false,
  ),
  fixture(
    'retouch preview hidden stage fails',
    () =>
      validateRetouchPreviewGeometry(
        `${STAGE_POSITIVE}\n<style scoped>\n.hidden-stage { visibility: hidden; }\n</style>`,
      ),
    false,
  ),
  fixture(
    'group frame ring canonical passes',
    () => validateGroupFrameRing(ADAPTER_POSITIVE, OVERLAY_POSITIVE),
    true,
  ),
  fixture(
    'group frame ring hardcoded stroke fails',
    () =>
      validateGroupFrameRing(
        ADAPTER_POSITIVE.replace(
          '--canvas-selection-stroke-width: 1px;',
          '--canvas-selection-stroke-width: 2px;',
        ),
        OVERLAY_POSITIVE,
      ),
    false,
  ),
  fixture(
    'group frame ring hardcoded border fails',
    () =>
      validateGroupFrameRing(
        ADAPTER_POSITIVE.replace(
          'border: var(--canvas-selection-stroke-width) solid var(--canvas-selection-group-border);',
          'border: 1px solid var(--canvas-selection-group-border);',
        ),
        OVERLAY_POSITIVE,
      ),
    false,
  ),
  fixture(
    'group frame ring overlay bypassing the width token fails',
    () =>
      validateGroupFrameRing(
        ADAPTER_POSITIVE,
        OVERLAY_POSITIVE.replace(
          'box-shadow: 0 0 0 var(--canvas-group-frame-selected-ring-width)',
          'box-shadow: 0 0 0 2px',
        ),
      ),
    false,
  ),
]

function runSelfTest(): void {
  let failed = 0
  for (const current of FIXTURES) {
    let issues: string[]
    try {
      issues = current.run()
    } catch (error) {
      issues = [`threw: ${(error as Error).message}`]
    }
    const passed = issues.length === 0
    if (passed !== current.expectPass) {
      failed += 1
      console.error(
        `[canvas-ui-style-self-test] ${current.name}: expected ${current.expectPass ? 'pass' : 'fail'}, got ${passed ? 'pass' : `fail (${issues.join('; ')})`}.`,
      )
    }
  }
  if (failed > 0) {
    console.error(`\nCanvas UI style self-test failed: ${failed} case(s).`)
    process.exit(1)
  }
  console.log(`Canvas UI style self-test passed: ${FIXTURES.length} case(s).`)
}

const SOURCE_FILES = [
  CANVAS_SCREEN_FILE,
  SURFACE_SKIN_FILE,
  APP_STYLE_FILE,
  CHROME_SKIN_FILE,
  STYLE_CONTRACTS_FILE,
  TOOLTIP_SKIN_FILE,
  NODE_SKIN_FILE,
  NODE_TOOL_SKIN_FILE,
  ADAPTER_SKIN_FILE,
  MINIMAP_FILE,
  HANDLE_RAIL_FILE,
  NODE_STAGE_FRAME_FILE,
  STAGE_PRESENTATION_FILE,
  RETOUCH_LAYER_ACTIONS_FILE,
  RETOUCH_HISTOGRAM_FILE,
  INPAINT_CHROME_FILE,
  GROUP_FRAMES_OVERLAY_FILE,
]

function main(): void {
  runSelfTest()
  const root = join(import.meta.dirname, '..', 'src')
  const sources: Record<string, string> = Object.fromEntries(
    SOURCE_FILES.map((file) => [file, readFileSync(join(root, file), 'utf8')]),
  )
  const violations = checkCanvasUiStyleSources(sources)
  if (violations.length > 0) {
    console.error('Canvas UI style contracts failed.\n')
    for (const violation of violations)
      console.error(`- ${violation.file}: ${violation.rule}: ${violation.message}`)
    console.error('')
    process.exit(1)
  }
  console.log(
    'Canvas UI style contracts passed: surface owner, token mapping, wash, consumer boundary, tooltips, retouch geometry, group ring.',
  )
}

main()
