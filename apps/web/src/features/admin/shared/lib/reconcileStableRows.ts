// 行引用稳定对账：内容一致即复用旧引用，只有真正变化的行才返回新
// 对象。调用方先把乐观覆盖投影进 nextRows，再调本 helper，保证回填
// 整体替换对象时 DOM/cell 引用不抖。顺带裁掉已消失的 key，防分页残留。
export function reconcileStableRows<Row, Key>(
  cache: Map<Key, Row>,
  nextRows: readonly Row[],
  getKey: (row: Row) => Key,
  isSame: (cached: Row, incoming: Row) => boolean,
): Row[] {
  const seen = new Set<Key>()
  const stable = nextRows.map((incoming) => {
    const key = getKey(incoming)
    seen.add(key)
    const cached = cache.get(key)
    if (cached !== undefined && isSame(cached, incoming)) return cached
    cache.set(key, incoming)
    return incoming
  })
  for (const key of [...cache.keys()]) {
    if (!seen.has(key)) cache.delete(key)
  }
  return stable
}
