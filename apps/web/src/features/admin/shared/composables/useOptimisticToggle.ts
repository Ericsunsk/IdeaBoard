import { ref } from 'vue'

export type OptimisticToggleFeedback<Key> = {
  onSuccess?: (key: Key, next: boolean) => void
  onFailure?: (key: Key, next: boolean, error: unknown) => void
}

export type OptimisticToggleMutate<Key> = (key: Key, next: boolean) => Promise<void>

// 行开关乐观收口：intent（调用 toggle 即翻转）→ pending（独立开关态，
// 不碰同行保存/输入）→ reconcile（服务端追上后清覆盖）/ rollback（失败
// 删覆盖，toast 由调用方 feedback 传入，本模块不碰 toast）。
export function useOptimisticToggle<Key extends string | number>() {
  const optimistic = ref(new Map<Key, boolean>())
  const pending = ref(new Map<Key, boolean>())

  function isPending(key: Key): boolean {
    return pending.value.get(key) === true
  }

  function effective(key: Key, serverValue: boolean): boolean {
    return optimistic.value.get(key) ?? serverValue
  }

  function reconcile(key: Key, serverValue: boolean): void {
    if (optimistic.value.get(key) === serverValue) optimistic.value.delete(key)
  }

  function reconcileAll(entries: ReadonlyArray<{ key: Key; enabled: boolean }>): void {
    for (const entry of entries) reconcile(entry.key, entry.enabled)
  }

  async function toggle(
    key: Key,
    next: boolean,
    mutate: OptimisticToggleMutate<Key>,
    feedback: OptimisticToggleFeedback<Key> = {},
  ): Promise<boolean> {
    if (isPending(key)) return false
    optimistic.value.set(key, next)
    pending.value.set(key, true)
    try {
      await mutate(key, next)
      feedback.onSuccess?.(key, next)
      return true
    } catch (error) {
      optimistic.value.delete(key)
      feedback.onFailure?.(key, next, error)
      return false
    } finally {
      pending.value.delete(key)
    }
  }

  return { optimistic, pending, isPending, effective, reconcile, reconcileAll, toggle }
}
