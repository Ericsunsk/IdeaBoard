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
