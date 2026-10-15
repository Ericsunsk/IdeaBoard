export const motionIntents = ['control', 'enter', 'exit', 'feedback', 'image'] as const

export type MotionIntent = (typeof motionIntents)[number]
export type MotionPreference = 'no-preference' | 'reduce'

type MotionCssVariableName =
  | '--motion-control-duration'
  | '--motion-control-easing'
  | '--motion-enter-duration'
  | '--motion-enter-easing'
  | '--motion-exit-duration'
  | '--motion-exit-easing'
  | '--motion-feedback-duration'
  | '--motion-feedback-easing'
  | '--motion-image-duration'
  | '--motion-image-easing'

type MotionCss = Readonly<{
  duration: `var(${MotionCssVariableName})`
  easing: `var(${MotionCssVariableName})`
}>

type MotionTiming = Readonly<{
  durationMs: number
  easing: string
  allowsTransform: boolean
  allowsContinuousMotion: boolean
}>

type MotionCssVariableReader = (name: MotionCssVariableName) => string | undefined

const motionCssByIntent: Record<MotionIntent, MotionCss> = {
  control: {
    duration: 'var(--motion-control-duration)',
    easing: 'var(--motion-control-easing)',
  },
  enter: {
    duration: 'var(--motion-enter-duration)',
    easing: 'var(--motion-enter-easing)',
  },
  exit: {
    duration: 'var(--motion-exit-duration)',
    easing: 'var(--motion-exit-easing)',
  },
  image: {
    duration: 'var(--motion-image-duration)',
    easing: 'var(--motion-image-easing)',
  },
  feedback: {
    duration: 'var(--motion-feedback-duration)',
    easing: 'var(--motion-feedback-easing)',
  },
}

function cssVariableName(value: string): MotionCssVariableName {
  return value.slice(4, -1) as MotionCssVariableName
}

function parseCssDuration(value: string | undefined): number {
  const normalized = value?.trim() ?? ''
  const match = /^(\d+(?:\.\d+)?|\.\d+)(ms|s)$/.exec(normalized)
  if (!match) return 0

  const amountText = match[1]
  const unitText = match[2]
  if (amountText === undefined || unitText === undefined) return 0
  const durationMs = Number.parseFloat(amountText) * (unitText === 's' ? 1000 : 1)
  return Number.isFinite(durationMs) && durationMs >= 0 ? durationMs : 0
}

function readDocumentMotionVariable(name: MotionCssVariableName): string | undefined {
  if (typeof document === 'undefined') return undefined
  return getComputedStyle(document.documentElement).getPropertyValue(name)
}

export function getMotionCss(intent: MotionIntent): MotionCss {
  return motionCssByIntent[intent]
}

export function getMotionPreference(): MotionPreference {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'no-preference'
  }
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'reduce' : 'no-preference'
}

export function resolveMotionTiming(
  intent: MotionIntent,
  options: {
    preference?: MotionPreference
    readCssVariable?: MotionCssVariableReader
  } = {},
): MotionTiming {
  const preference = options.preference ?? getMotionPreference()
  if (preference === 'reduce') {
    return {
      durationMs: 0,
      easing: 'linear',
      allowsTransform: false,
      allowsContinuousMotion: false,
    }
  }

  const css = getMotionCss(intent)
  const readCssVariable = options.readCssVariable ?? readDocumentMotionVariable
  return {
    durationMs: parseCssDuration(readCssVariable(cssVariableName(css.duration))),
    easing: readCssVariable(cssVariableName(css.easing))?.trim() || 'linear',
    allowsTransform: true,
    allowsContinuousMotion: true,
  }
}
