import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'

type StyleBlock = {
  content: string
  scoped: boolean
}

type RiskHit = {
  label: string
  count: number
  weight: number
}

type FileAudit = {
  file: string
  kind: 'vue' | 'css'
  styleLines: number
  classCount: number
  unusedClassCount: number
  unusedClasses: string[]
  riskHits: RiskHit[]
  riskScore: number
  riskLevel: 'low' | 'medium' | 'high'
}

const webRoot = process.cwd()
const srcRoot = path.join(webRoot, 'src')
const targetRoots = [path.join(srcRoot, 'features'), path.join(srcRoot, 'pages')]
const sourceExtensions = /\.(vue|ts|tsx|js|jsx)$/
const maxUnusedClassesToPrint = 12
const runtimeClassPatterns = [
  /^n-/,
  /^vue-flow/,
  /^flow-canvas-node__/,
  /^canvas-flow-edge$/,
  /^canvas-connection-line$/,
  /^connectionindicator$/,
  /^enter-(active|from|to)$/,
  /^leave-(active|from|to)$/,
  /-(enter|leave)-(active|from|to)$/,
]

const riskPatterns: Array<{ label: string; pattern: RegExp; weight: number }> = [
  { label: ':deep third-party override', pattern: /:deep\(/g, weight: 7 },
  { label: 'pseudo selector', pattern: /::?[a-zA-Z-]+/g, weight: 3 },
  { label: '@keyframes', pattern: /@keyframes\b/g, weight: 6 },
  { label: '@media', pattern: /@media\b/g, weight: 3 },
  { label: 'absolute/fixed positioning', pattern: /position:\s*(absolute|fixed)/g, weight: 6 },
  { label: 'transform', pattern: /\btransform\s*:/g, weight: 5 },
  { label: 'z-index', pattern: /\bz-index\s*:/g, weight: 4 },
  {
    label: 'multi-layer background',
    pattern: /background(?:-image)?:[^;]*,\s*(?:radial|linear|conic)-gradient/g,
    weight: 7,
  },
  { label: '!important', pattern: /!important/g, weight: 3 },
  { label: 'browser private selector', pattern: /::-webkit-|::-moz-|:-ms-/g, weight: 6 },
  { label: 'descendant selector', pattern: /\.[A-Za-z_-][\w-]+\s+[.:#[A-Za-z_-]/g, weight: 2 },
  {
    label: 'stateful class selector',
    pattern:
      /\.[A-Za-z_-][\w-]+\.(active|selected|current|disabled|loading|open|dragging|danger|ghost|primary)\b/g,
    weight: 3,
  },
]

function walkFiles(dir: string): string[] {
  if (!existsSync(dir)) return []
  return readdirSync(dir)
    .flatMap((entry) => {
      const fullPath = path.join(dir, entry)
      const stat = statSync(fullPath)
      if (stat.isDirectory()) return walkFiles(fullPath)
      if (/\.(vue|css)$/.test(entry)) return [fullPath]
      return []
    })
    .sort()
}

function walkSourceFiles(dir: string): string[] {
  if (!existsSync(dir)) return []
  return readdirSync(dir)
    .flatMap((entry) => {
      const fullPath = path.join(dir, entry)
      const stat = statSync(fullPath)
      if (stat.isDirectory()) return walkSourceFiles(fullPath)
      if (sourceExtensions.test(entry)) return [fullPath]
      return []
    })
    .sort()
}

function relativeFile(file: string): string {
  return path.relative(webRoot, file)
}

function countLines(source: string): number {
  if (!source.trim()) return 0
  return source.split(/\r?\n/).length
}

function extractStyleBlocks(source: string): StyleBlock[] {
  return Array.from(source.matchAll(/<style\b([^>]*)>([\s\S]*?)<\/style>/g)).map((match) => ({
    scoped: /\bscoped\b/.test(match[1] ?? ''),
    content: match[2] ?? '',
  }))
}

function extractTemplate(source: string): string {
  return source.match(/<template\b[^>]*>([\s\S]*?)<\/template>/)?.[1] ?? ''
}

function stripStyleBlocks(source: string): string {
  return source.replace(/<style\b[^>]*>[\s\S]*?<\/style>/g, '')
}

function extractCssClasses(css: string): string[] {
  const classes = new Set<string>()
  const normalizedCss = stripCssComments(css)
  const selectorPattern = /(^|[,{]\s*)([^{}@]+)\{/g
  for (const match of normalizedCss.matchAll(selectorPattern)) {
    const selector = match[2] ?? ''
    for (const classMatch of selector.matchAll(/\.([A-Za-z_-][\w-]*)/g)) {
      classes.add(classMatch[1])
    }
  }
  return [...classes].sort()
}

function stripCssComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, '')
}

function isClassReferenced(className: string, source: string, template: string): boolean {
  if (runtimeClassPatterns.some((pattern) => pattern.test(className))) return true
  const escaped = escapeRegExp(className)
  const classAttrPattern = new RegExp(`\\bclass=["'][^"']*\\b${escaped}\\b`, 'm')
  const dynamicClassPattern = new RegExp(`['"\`]${escaped}['"\`]`, 'm')
  const objectClassPattern = new RegExp(`\\b${escaped}\\b\\s*:`, 'm')
  const domQueryPattern = new RegExp(
    `\\b(?:closest|querySelector|querySelectorAll)\\(["']\\.${escaped}\\b`,
    'm',
  )
  return (
    classAttrPattern.test(template) ||
    dynamicClassPattern.test(source) ||
    objectClassPattern.test(source) ||
    domQueryPattern.test(source) ||
    source.includes(`.${className}`)
  )
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function scoreRisk(style: string, styleLines: number, classCount: number): RiskHit[] {
  const hits: RiskHit[] = []
  for (const risk of riskPatterns) {
    const count = Array.from(style.matchAll(risk.pattern)).length
    if (count > 0) {
      hits.push({ label: risk.label, count, weight: risk.weight })
    }
  }
  if (styleLines >= 200)
    hits.push({ label: 'large style block >= 200 lines', count: 1, weight: 12 })
  if (styleLines >= 500)
    hits.push({ label: 'large style block >= 500 lines', count: 1, weight: 18 })
  if (classCount >= 40) hits.push({ label: 'many CSS classes >= 40', count: 1, weight: 8 })
  return hits
}

function riskLevel(score: number): FileAudit['riskLevel'] {
  if (score >= 45) return 'high'
  if (score >= 18) return 'medium'
  return 'low'
}

function auditVue(file: string): FileAudit | null {
  const source = readFileSync(file, 'utf8')
  const styleBlocks = extractStyleBlocks(source)
  if (styleBlocks.length === 0) return null
  const styleLines = styleBlocks.reduce((sum, block) => sum + countLines(block.content), 0)
  if (styleLines === 0) return null
  const template = extractTemplate(source)
  const nonStyleSource = stripStyleBlocks(source)
  const style = styleBlocks.map((block) => block.content).join('\n')
  const classes = extractCssClasses(style)
  const unusedClasses = classes.filter(
    (className) => !isClassReferenced(className, nonStyleSource, template),
  )
  const riskHits = scoreRisk(style, styleLines, classes.length)
  const riskScore = riskHits.reduce((sum, hit) => sum + hit.count * hit.weight, 0)
  return {
    file: relativeFile(file),
    kind: 'vue',
    styleLines,
    classCount: classes.length,
    unusedClassCount: unusedClasses.length,
    unusedClasses,
    riskHits,
    riskScore,
    riskLevel: riskLevel(riskScore),
  }
}

function auditCss(file: string, allSource: string): FileAudit {
  const source = readFileSync(file, 'utf8')
  const classes = extractCssClasses(source)
  const unusedClasses = classes.filter(
    (className) => !isClassReferenced(className, allSource, allSource),
  )
  const styleLines = countLines(source)
  const riskHits = scoreRisk(source, styleLines, classes.length)
  const riskScore = riskHits.reduce((sum, hit) => sum + hit.count * hit.weight, 0)
  return {
    file: relativeFile(file),
    kind: 'css',
    styleLines,
    classCount: classes.length,
    unusedClassCount: unusedClasses.length,
    unusedClasses,
    riskHits,
    riskScore,
    riskLevel: riskLevel(riskScore),
  }
}

function printAudit(audits: FileAudit[]) {
  const sorted = [...audits].sort(
    (a, b) => b.riskScore - a.riskScore || b.styleLines - a.styleLines,
  )
  const totals = sorted.reduce(
    (acc, audit) => {
      acc.files += 1
      acc.lines += audit.styleLines
      acc.classes += audit.classCount
      acc.unused += audit.unusedClassCount
      acc[audit.riskLevel] += 1
      return acc
    },
    { files: 0, lines: 0, classes: 0, unused: 0, low: 0, medium: 0, high: 0 },
  )

  console.log('CSS migration audit')
  console.log(
    `Files: ${totals.files}, style lines: ${totals.lines}, CSS classes: ${totals.classes}`,
  )
  console.log(`Risk: high ${totals.high}, medium ${totals.medium}, low ${totals.low}`)
  console.log(`Potential unused classes: ${totals.unused}`)
  console.log('')

  console.log('High and medium risk files')
  sorted.filter((audit) => audit.riskLevel !== 'low').forEach((audit) => printFileAudit(audit))

  console.log('')
  console.log('Low-risk migration candidates')
  sorted
    .filter((audit) => audit.riskLevel === 'low')
    .sort((a, b) => a.styleLines - b.styleLines)
    .slice(0, 20)
    .forEach((audit) => {
      console.log(`- ${audit.file} (${audit.styleLines} lines, ${audit.classCount} classes)`)
    })
}

function printFileAudit(audit: FileAudit) {
  console.log(`- [${audit.riskLevel}] ${audit.file}`)
  console.log(
    `  lines=${audit.styleLines}, classes=${audit.classCount}, unused=${audit.unusedClassCount}, score=${audit.riskScore}`,
  )
  const topHits = audit.riskHits
    .sort((a, b) => b.count * b.weight - a.count * a.weight)
    .slice(0, 5)
    .map((hit) => `${hit.label} x${hit.count}`)
    .join('; ')
  if (topHits) console.log(`  risks: ${topHits}`)
  if (audit.unusedClasses.length > 0) {
    const sample = audit.unusedClasses.slice(0, maxUnusedClassesToPrint).join(', ')
    const more = audit.unusedClasses.length > maxUnusedClassesToPrint ? ', ...' : ''
    console.log(`  possible unused: ${sample}${more}`)
  }
}

const files = targetRoots.flatMap(walkFiles)
const vueFiles = files.filter((file) => file.endsWith('.vue'))
const cssFiles = files.filter((file) => file.endsWith('.css'))
const allSource = walkSourceFiles(srcRoot)
  .map((file) => readFileSync(file, 'utf8'))
  .join('\n')
const vueAudits = vueFiles.map(auditVue).filter((audit): audit is FileAudit => Boolean(audit))
const cssAudits = cssFiles.map((file) => auditCss(file, allSource))

printAudit([...vueAudits, ...cssAudits])
