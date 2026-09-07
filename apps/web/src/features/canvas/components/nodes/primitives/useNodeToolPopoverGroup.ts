import { computed, ref, type Ref } from 'vue'
import { useCanvasCustomLayerOverlay } from '../../../overlay-lifecycle'

type PopoverRootRef = Ref<HTMLElement | null | undefined>

interface NodeToolPopoverGroupOptions<T extends string> {
  keys: readonly T[]
  rootRefs: PopoverRootRef[]
  closeOnEscape?: boolean | undefined
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

  useCanvasCustomLayerOverlay({
    visible: computed(() => activeKey.value !== null),
    roots: options.rootRefs,
    dismissibleBy: closeOnEscape ? ['outside-pointer', 'escape'] : ['outside-pointer'],
    onDismiss: () => close(),
  })

  return {
    activeKey,
    isOpen,
    open,
    close,
    toggle,
  }
}
