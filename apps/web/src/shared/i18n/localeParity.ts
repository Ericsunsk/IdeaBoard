/**
 * Locale bundle structural comparison, shared by the lint parity guard and unit tests
 * so both verify the same invariant: every User locale tree matches `en` exactly in keys and shape.
 */

export function leafPaths(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) {
    return prefix ? [prefix] : []
  }
  const out: string[] = []
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const full = prefix ? `${prefix}.${k}` : k
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
      out.push(...leafPaths(v, full))
    } else {
      out.push(full)
    }
  }
  return out
}

export function bundleShapeMap(
  obj: unknown,
  prefix = '',
  map = new Map<string, string>(),
): Map<string, string> {
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) {
    if (prefix) map.set(prefix, typeof obj)
    return map
  }
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const full = prefix ? `${prefix}.${k}` : k
    if (v !== null && typeof v === 'object' && !Array.isArray(v)) {
      bundleShapeMap(v, full, map)
    } else {
      map.set(full, typeof v)
    }
  }
  return map
}

export interface LocaleBundleDiff {
  equal: boolean
  /** Keys present in the reference but missing in the target. */
  missing: string[]
  /** Keys present in the target but absent in the reference. */
  extra: string[]
  /** Leaf-vs-object / type mismatches at shared key paths. */
  shapeMismatches: string[]
}

/**
 * P0 i18n structure limits (plan §3.7, warn-only, enforced by
 * `scripts/checkLocaleParity.ts` without failing the lane).
 *
 * - Nesting: at most 3 levels below the top namespace, i.e. leaf paths carry
 *   at most 4 segments (`canvas.retouch.brush.size`). Deeper trees split into
 *   a new namespace instead of growing god-keys.
 * - No-god-keys: a single top-level namespace holds at most 300 leaves.
 */
export const I18N_MAX_PATH_SEGMENTS = 4
export const I18N_MAX_LEAVES_PER_NAMESPACE = 300

/** Deepest leaf-path segment count in the bundle (0 for an empty bundle). */
export function maxLeafSegments(obj: unknown): number {
  return leafPaths(obj).reduce((max, key) => Math.max(max, key.split('.').length), 0)
}

/** Leaf counts grouped by top-level namespace, e.g. `{ canvas: 270 }`. */
export function countLeavesByNamespace(obj: Record<string, unknown>): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const key of leafPaths(obj)) {
    const top = key.split('.')[0] ?? ''
    counts[top] = (counts[top] ?? 0) + 1
  }
  return counts
}

/** Structure limits produce advisory messages, independent of parity errors. */
export function localeStructureWarnings(bundle: Record<string, unknown>): string[] {
  const warnings: string[] = []
  const depth = maxLeafSegments(bundle)
  if (depth > I18N_MAX_PATH_SEGMENTS) {
    warnings.push(
      `[locale-structure:warn] en nesting depth ${depth} exceeds ${I18N_MAX_PATH_SEGMENTS} path segments; split deep trees into a new namespace.`,
    )
  }
  const counts = countLeavesByNamespace(bundle)
  for (const [namespace, count] of Object.entries(counts).sort((a, b) => b[1] - a[1])) {
    if (count > I18N_MAX_LEAVES_PER_NAMESPACE) {
      warnings.push(
        `[locale-structure:warn] en.${namespace} holds ${count} leaves (limit ${I18N_MAX_LEAVES_PER_NAMESPACE}); split the god-key namespace.`,
      )
    }
  }
  return warnings
}

export function diffLocaleBundles(
  reference: Record<string, unknown>,
  target: Record<string, unknown>,
): LocaleBundleDiff {
  const refKeys = leafPaths(reference).sort()
  const tgtKeys = leafPaths(target).sort()
  const refShape = bundleShapeMap(reference)
  const tgtShape = bundleShapeMap(target)

  const missing = refKeys.filter((k) => !tgtKeys.includes(k))
  const extra = tgtKeys.filter((k) => !refKeys.includes(k))
  const shapeMismatches: string[] = []

  for (const key of refKeys) {
    if (tgtKeys.includes(key)) {
      const refType = refShape.get(key)
      const tgtType = tgtShape.get(key)
      if (refType !== tgtType) {
        shapeMismatches.push(`${key}: en is ${refType}, target is ${tgtType}`)
      }
    } else {
      const prefixMatch = refKeys.find((ek) => key.startsWith(`${ek}.`))
      if (prefixMatch) {
        shapeMismatches.push(`${prefixMatch} is leaf in en but object in target`)
      }
    }
  }

  return {
    equal: missing.length === 0 && extra.length === 0 && shapeMismatches.length === 0,
    missing,
    extra,
    shapeMismatches,
  }
}
