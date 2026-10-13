export type MotionPolicyViolation = {
  rule: string
  message: string
  details?: readonly string[]
}

type LocalMotionAllowance = {
  properties?: readonly string[]
  animationEasingCount?: number
  reason: string
}

const localMotionAllowances: Record<string, LocalMotionAllowance> = {
  'src/features/canvas/ui/styles/canvas-surface-skin.css': {
    properties: ['--flow-edge-running-duration'],
    reason:
      'Canvas execution edges use a continuous activity cycle; finite Motion Policy intents do not describe cycle speed.',
  },
  'src/features/canvas/ui/styles/canvas-tool-tooltip.css': {
    properties: ['--canvas-tool-tooltip-delay'],
    reason: 'Canvas tooltips need a dwell delay, which is not a transition duration.',
  },
  'src/features/canvas/ui/styles/node-shared.css': {
    properties: ['--node-running-border-cycle'],
    animationEasingCount: 1,
    reason:
      'The running border is continuous execution feedback with a reduced-motion static fallback.',
  },
  'src/features/canvas/ui/nodes/workflows/FusionStage.vue': {
    animationEasingCount: 1,
    reason:
      'Fusion execution flow reuses the Canvas activity cycle and stops under reduced motion.',
  },
  'src/features/canvas/ui/styles/canvas-vue-flow-adapter-skin.css': {
    animationEasingCount: 1,
    reason:
      'The Vue Flow adapter paints the execution edge activity cycle and disables it under reduced motion.',
  },
}

const legacyMotionTokenPattern =
  /--(?:canvas-motion-(?:fast|standard|panel|dock|ease)|duration-ib-[\w-]+|ease-ib-[\w-]+)\b/g
const broadTransitionPattern =
  /\btransition-all\b|\btransition-\[all\]|\btransition(?:-property)?\s*:\s*all(?:\s|;|!|\]|$)/g
const nonZeroCssTimeSource = String.raw`(?:[1-9]\d*(?:\.\d+)?|0?\.\d*[1-9]\d*)(?:ms|s)\b`
const rawTransitionTimingPattern = new RegExp(
  String.raw`\btransition(?:-(?:duration|delay|timing-function))?\s*:[^;{}]*(?:${nonZeroCssTimeSource}|cubic-bezier\(|\bease(?:-in|-out|-in-out)?\b)[^;{}]*;?`,
  'g',
)
const rawAnimationDurationPattern = new RegExp(
  String.raw`\banimation(?:-duration)?\s*:[^;{}]*${nonZeroCssTimeSource}[^;{}]*;?`,
  'g',
)
const rawTailwindDurationPattern =
  /\bduration-(?:[1-9]\d*|\[(?!var\()[^\]]*(?:[1-9]\d*(?:\.\d+)?|0?\.\d*[1-9]\d*)(?:ms|s)[^\]]*\])(?=\s|["'`])/g
const rawTailwindEasingPattern =
  /\bease-(?:linear|in-out|in|out|\[(?!var\()[^\]]*(?:ease|cubic-bezier)[^\]]*\])(?=\s|["'`])/g
const rawArbitraryTransitionPattern = new RegExp(
  String.raw`\[transition:[^\]]*(?:${nonZeroCssTimeSource}|cubic-bezier\(|\blinear\b|\bease(?:-in|-out|-in-out)?\b)[^\]]*\]`,
  'g',
)
const rawCustomTimingPattern = new RegExp(
  String.raw`(--[\w-]+)\s*:\s*(?:${nonZeroCssTimeSource}|cubic-bezier\([^;]+\)|ease(?:-in|-out|-in-out)?)\s*;`,
  'g',
)
const rawAnimationEasingPattern =
  /\banimation\s*:[^;{}]*(?:\blinear\b|\bease(?:-in|-out|-in-out)?\b|cubic-bezier\()[^;{}]*;?/g
const activeCssAnimationPattern = /\banimation\s*:\s*(?!none\b)[^;{}]+;?/g
const reducedMotionMediaPattern = /@media\s*\(prefers-reduced-motion:\s*reduce\)/
const disabledAnimationPattern = /\banimation\s*:\s*none\b/

export function findMotionPolicyViolations(
  sourcePath: string,
  content: string,
): MotionPolicyViolation[] {
  if (!isProductionSource(sourcePath)) return []

  const allowance = localMotionAllowances[sourcePath]
  const violations: MotionPolicyViolation[] = []
  const utilitySource = getUtilitySource(sourcePath, content)
  const transitionSource = content.replace(/\[transition(?:-property)?:[^\]]+\]/g, (utility) =>
    utility.replaceAll('_', ' '),
  )

  pushPatternViolation(
    violations,
    content,
    legacyMotionTokenPattern,
    'legacy-motion-token',
    'Use semantic --motion-* policy tokens; legacy Motion aliases cannot be restored.',
  )
  pushPatternViolation(
    violations,
    transitionSource,
    broadTransitionPattern,
    'broad-transition',
    'List intentional transition properties instead of using transition-all.',
  )

  const rawTimingMatches = [
    ...findMatches(content, rawTransitionTimingPattern),
    ...findMatches(content, rawAnimationDurationPattern),
    ...findMatches(utilitySource, rawTailwindDurationPattern),
    ...findMatches(utilitySource, rawTailwindEasingPattern),
    ...findMatches(transitionSource, rawArbitraryTransitionPattern),
  ]
  if (rawTimingMatches.length > 0) {
    violations.push({
      rule: 'raw-motion-timing',
      message: 'Use Motion Policy tokens or an explicitly registered local Canvas timing.',
      details: rawTimingMatches,
    })
  }

  const allowedProperties = new Set(allowance?.properties ?? [])
  const unregisteredProperties = [...content.matchAll(clonePattern(rawCustomTimingPattern))]
    .map((match) => match[1])
    .filter((property): property is string => property !== undefined)
    .filter((property) => !isAppMotionPolicyProperty(sourcePath, property))
    .filter((property) => !allowedProperties.has(property))
  if (unregisteredProperties.length > 0) {
    violations.push({
      rule: 'unregistered-local-motion-timing',
      message: 'Local raw Motion timing requires a narrow semantic allowance with a reason.',
      details: [...new Set(unregisteredProperties)],
    })
  }

  const rawAnimationEasingCount = countMatches(content, rawAnimationEasingPattern)
  const allowedAnimationEasingCount = allowance?.animationEasingCount ?? 0
  if (rawAnimationEasingCount > allowedAnimationEasingCount) {
    violations.push({
      rule: 'raw-animation-easing',
      message:
        'Raw animation easing is reserved for registered Canvas continuous motion with a reason.',
      details: findMatches(content, rawAnimationEasingPattern),
    })
  }

  if (
    isStyleBearingFile(sourcePath) &&
    countMatches(content, activeCssAnimationPattern) > 0 &&
    (!reducedMotionMediaPattern.test(content) || !disabledAnimationPattern.test(content))
  ) {
    violations.push({
      rule: 'missing-reduced-motion',
      message: 'CSS animations must provide a same-file reduced-motion animation: none fallback.',
    })
  }

  if (allowance && !allowance.reason.trim()) {
    violations.push({
      rule: 'motion-allowance-reason',
      message: 'Local Motion allowances must explain why app-level policy is insufficient.',
    })
  }

  return violations
}

export function findStaleMotionAllowances(
  contents: ReadonlyMap<string, string>,
): MotionPolicyViolation[] {
  return Object.entries(localMotionAllowances).flatMap(([sourcePath, allowance]) => {
    const content = contents.get(sourcePath) ?? ''
    const violations: MotionPolicyViolation[] = []

    for (const property of allowance.properties ?? []) {
      if (!content.includes(`${property}:`)) {
        violations.push({
          rule: 'stale-motion-allowance',
          message: `${sourcePath} no longer declares ${property}; remove its allowance.`,
        })
      }
    }

    const expectedEasingCount = allowance.animationEasingCount ?? 0
    const actualEasingCount = countMatches(content, rawAnimationEasingPattern)
    if (actualEasingCount !== expectedEasingCount) {
      violations.push({
        rule: 'stale-motion-allowance',
        message: `${sourcePath} has ${actualEasingCount} raw animation easing declaration(s), expected ${expectedEasingCount}.`,
      })
    }

    return violations
  })
}

function isProductionSource(sourcePath: string) {
  return !sourcePath.includes('.spec.') && !sourcePath.includes('.test.')
}

function isStyleBearingFile(sourcePath: string) {
  return sourcePath.endsWith('.css') || sourcePath.endsWith('.vue')
}

function getUtilitySource(sourcePath: string, content: string) {
  if (sourcePath.endsWith('.css')) return ''
  if (!sourcePath.endsWith('.vue')) return content

  return content.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '')
}

function isAppMotionPolicyProperty(sourcePath: string, property: string) {
  if (sourcePath === 'src/theme/tokens/primitives.css') {
    return /^--primitive-(?:duration-(?:0|160|220)|easing-standard)$/.test(property)
  }
  return (
    (sourcePath === 'src/app/style.css' ||
      sourcePath === 'src/theme/theme.css' ||
      sourcePath === 'src/theme/tokens/motion.css') &&
    /^--motion-(?:control|enter|exit|feedback)-(?:duration|easing)$/.test(property)
  )
}

function pushPatternViolation(
  violations: MotionPolicyViolation[],
  content: string,
  pattern: RegExp,
  rule: string,
  message: string,
) {
  const details = findMatches(content, pattern)
  if (details.length > 0) violations.push({ rule, message, details })
}

function findMatches(content: string, pattern: RegExp) {
  return [...content.matchAll(clonePattern(pattern))].map((match) => match[0].trim())
}

function countMatches(content: string, pattern: RegExp) {
  return findMatches(content, pattern).length
}

function clonePattern(pattern: RegExp) {
  return new RegExp(pattern.source, pattern.flags)
}
