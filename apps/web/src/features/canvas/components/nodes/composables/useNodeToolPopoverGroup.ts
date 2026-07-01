import { computed, ref, watchEffect, type Ref } from 'vue'
import { useDismissableLayer } from '../../../../../shared/composables/useDismissableLayer'

type PopoverRootRef = Ref<HTMLElement | null | undefined>

interface NodeToolPopoverGroupOptions<T extends string> {
  keys: readonly T[]
  rootRefs: PopoverRootRef[]
  closeOnEscape?: boolean
}

const POPOVER_GROUP_ATTR = 'data-node-tool-popover-group'
let popoverGroupId = 0

export function useNodeToolPopoverGroup<T extends string>(options: NodeToolPopoverGroupOptions<T>) {
  const activeKey = ref<T | null>(null)
  const keySet = new Set(options.keys)
  const closeOnEscape = options.closeOnEscape ?? true
  const groupToken = `node-tool-popover-group-${++popoverGroupId}`

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

  watchEffect((onCleanup) => {
    const roots = options.rootRefs
      .map((rootRef) => rootRef.value)
      .filter((root): root is HTMLElement => !!root)

    for (const root of roots) {
      root.setAttribute(POPOVER_GROUP_ATTR, groupToken)
    }

    onCleanup(() => {
      for (const root of roots) {
        if (root.getAttribute(POPOVER_GROUP_ATTR) === groupToken) {
          root.removeAttribute(POPOVER_GROUP_ATTR)
        }
      }
    })
  })

  useDismissableLayer({
    enabled: computed(() => activeKey.value !== null),
    ignore: [`[${POPOVER_GROUP_ATTR}="${groupToken}"]`],
    onDismiss: (event) => {
      if (event instanceof KeyboardEvent && !closeOnEscape) return
      close()
    },
  })

  return {
    activeKey,
    isOpen,
    open,
    close,
    toggle,
  }
}
