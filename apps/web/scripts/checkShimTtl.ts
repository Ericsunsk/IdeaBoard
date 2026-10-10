import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const srcRoot = process.env['SHIM_TTL_ROOT'] ?? path.join(webRoot, 'src')
const markerPath =
  process.env['SHIM_TTL_PHASE_MARKER'] ??
  path.join(webRoot, '..', '..', 'docs', 'architecture', 'phase-marker.json')

/** Phase order shared with the migration ledger (plus P0/P1); later index wins. */
const PHASE_ORDER = ['P0', 'P1a', 'P1b', 'P1', 'P2', 'P2.5', 'P3', 'P4', 'P5', 'P6', 'P7']

function phaseIndex(phase: string): number {
  return PHASE_ORDER.indexOf(phase)
}

type HeaderCheck = { ok: true; expiryPhase: string } | { ok: false; reason: string }

/**
 * Format gate: every `*.shim.ts` must carry TTL + DEPRECATED + deletion-owner
 * in its header, and the TTL line must name an explicit expiry phase
 * (`delete after P<n>`) so the expiry gate below stays machine-readable.
 */
function checkShimHeader(source: string): HeaderCheck {
  const head = source.slice(0, 1200)
  if (!head.includes('@deprecated')) return { ok: false, reason: 'missing @deprecated' }
  if (!head.includes('TTL:')) return { ok: false, reason: 'missing TTL:' }
  if (!/deletion owner:/i.test(head)) return { ok: false, reason: 'missing Deletion owner:' }
  const match = head.match(/delete after (P\d+(?:\.\d+)?)/)
  if (!match?.[1])
    return { ok: false, reason: 'TTL line must name an expiry phase ("delete after P<n>")' }
  if (phaseIndex(match[1]) < 0) return { ok: false, reason: `unknown expiry phase "${match[1]}"` }
  return { ok: true, expiryPhase: match[1] }
}

function listShims(root: string): string[] {
  const found: string[] = []
  const visit = (dir: string) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      if (entry.name === 'node_modules' || entry.name === 'generated-query') continue
      const absolute = path.join(dir, entry.name)
      if (entry.isDirectory()) {
        visit(absolute)
        continue
      }
      if (entry.name.endsWith('.shim.ts')) found.push(absolute)
    }
  }
  visit(root)
  return found.sort()
}

function loadCurrentPhase(markerFile: string): string {
  const raw = JSON.parse(fs.readFileSync(markerFile, 'utf8')) as { currentPhase?: unknown }
  if (typeof raw.currentPhase !== 'string' || phaseIndex(raw.currentPhase) < 0)
    throw new Error(
      `Phase marker ${markerFile} must set currentPhase to one of ${PHASE_ORDER.join(', ')}.`,
    )
  return raw.currentPhase
}

/** Expiry gate: a shim fails once the marker has moved past its TTL phase. */
function scanShims(root: string, markerFile: string): string[] {
  const currentPhase = loadCurrentPhase(markerFile)
  const current = phaseIndex(currentPhase)
  const errors: string[] = []
  for (const absolute of listShims(root)) {
    const relative = path.relative(path.dirname(root), absolute).split(path.sep).join('/')
    const checked = checkShimHeader(fs.readFileSync(absolute, 'utf8'))
    if (!checked.ok) {
      errors.push(`${relative}: ${checked.reason}`)
      continue
    }
    if (current > phaseIndex(checked.expiryPhase))
      errors.push(
        `${relative}: TTL expired (delete after ${checked.expiryPhase}, current phase ${currentPhase})`,
      )
  }
  return errors
}

function fail(label: string): never {
  console.error(`✗ shim-ttl self-test: ${label}`)
  process.exit(1)
}

/**
 * Red-proof fixtures: they run on every invocation in a disposable temp dir,
 * so a broken gate fails here before it can wave through the real tree.
 */
function runSelfTest(): void {
  const valid = checkShimHeader(
    '/**\n * @deprecated P1b compatibility shim.\n * TTL: 1 phase — delete after P1.\n * Deletion owner: p1-lane.\n */\nexport * from "./x"\n',
  )
  if (!valid.ok || valid.expiryPhase !== 'P1') fail('valid header must parse expiry P1')
  console.log('✓ shim-ttl self-test: valid header parses expiry P1.')
  if (checkShimHeader('export const x = 1\n').ok) fail('headerless shim must fail format')
  console.log('✓ shim-ttl self-test: headerless shim fails format.')
  if (checkShimHeader('/** @deprecated */\nexport const x = 1\n').ok)
    fail('shim without TTL/owner must fail format')
  console.log('✓ shim-ttl self-test: shim without TTL/owner fails format.')

  const workspace = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'ideaboard-shim-ttl-')))
  try {
    const src = path.join(workspace, 'src')
    fs.mkdirSync(src, { recursive: true })
    fs.writeFileSync(
      path.join(src, 'late.shim.ts'),
      '/**\n * @deprecated P1b compatibility shim.\n * TTL: 1 phase — delete after P1.\n * Deletion owner: p1-lane.\n */\nexport * from "./x"\n',
    )
    const marker = path.join(workspace, 'phase-marker.json')
    fs.writeFileSync(marker, JSON.stringify({ currentPhase: 'P1' }))
    if (scanShims(src, marker).length !== 0) fail('P1 marker must keep delete-after-P1 shims green')
    console.log('✓ shim-ttl self-test: P1 marker keeps delete-after-P1 shims green.')
    fs.writeFileSync(marker, JSON.stringify({ currentPhase: 'P2' }))
    const expired = scanShims(src, marker)
    if (expired.length !== 1 || !expired[0]?.includes('TTL expired'))
      fail('P2 marker must fail delete-after-P1 shims with TTL expired')
    console.log('✓ shim-ttl self-test: P2 marker fails delete-after-P1 shims with TTL expired.')
  } finally {
    fs.rmSync(workspace, { recursive: true, force: true })
  }
}

runSelfTest()

const errors = scanShims(srcRoot, markerPath)
if (errors.length > 0) {
  console.error('Shim TTL check failed!')
  for (const error of errors) console.error(`  ${error}`)
  process.exit(1)
}
const count = listShims(srcRoot).length
console.log(
  `✓ shim TTL: ${count} shim(s) carry TTL + DEPRECATED + deletion-owner headers; ` +
    `none expired at ${loadCurrentPhase(markerPath)}.`,
)
