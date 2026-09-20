import { onMounted, onUnmounted, shallowRef } from 'vue'

export function useAdminSubHeaderTarget() {
  const target = shallowRef<HTMLElement | null>(null)
  let observer: MutationObserver | null = null

  function resolveTarget() {
    if (target.value || typeof document === 'undefined') return
    target.value = document.getElementById('admin-sub-header')
    if (target.value) observer?.disconnect()
  }

  onMounted(() => {
    resolveTarget()
    if (target.value || typeof document === 'undefined') return

    observer = new MutationObserver(resolveTarget)
    observer.observe(document.body, { childList: true, subtree: true })
  })

  onUnmounted(() => observer?.disconnect())

  return target
}
