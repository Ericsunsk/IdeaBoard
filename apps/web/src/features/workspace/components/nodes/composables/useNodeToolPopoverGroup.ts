import { computed, onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

type PopoverRootRef = Ref<HTMLElement | null | undefined>

interface NodeToolPopoverGroupOptions<T extends string> {
  keys: readonly T[]
  rootRefs: PopoverRootRef[]
  closeOnEscape?: boolean
}

export function useNodeToolPopoverGroup<T extends string>(options: NodeToolPopoverGroupOptions<T>) {
  const activeKey = ref<T | null>(null)
  const keySet = new Set(options.keys)
  const closeOnEscape = options.closeOnEscape ?? true

  function isOpen(key: T) {
    return computed(() => activeKey.value === key)
  }

  function open(key: T) {
    if (!keySet.has(key)) return
    activeKey.value = key
  }

  function close(key?: T) {
    if (!key) {
      activeKey.value = null
      return
    }
    if (activeKey.value === key) activeKey.value = null
  }

  function toggle(key: T) {
    if (!keySet.has(key)) return
    activeKey.value = activeKey.value === key ? null : key
  }

  function containsTarget(target: Node) {
    for (const rootRef of options.rootRefs) {
      const root = rootRef.value
      if (root?.contains(target)) return true
    }
    return false
  }

  function onGlobalPointerDown(event: PointerEvent) {
    const target = event.target
    if (!(target instanceof Node)) return
    if (containsTarget(target)) return
    close()
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (!closeOnEscape) return
    if (event.key !== 'Escape') return
    close()
  }

  onMounted(() => {
    window.addEventListener('pointerdown', onGlobalPointerDown, true)
    window.addEventListener('keydown', onGlobalKeydown, true)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('pointerdown', onGlobalPointerDown, true)
    window.removeEventListener('keydown', onGlobalKeydown, true)
  })

  return {
    activeKey,
    isOpen,
    open,
    close,
    toggle,
  }
}
