import type { ShallowRef } from 'vue'

type TriggerReference = HTMLElement | { $el?: HTMLElement; el?: HTMLElement } | null

export function useMenuTriggerFocusRestore(triggerRef: Readonly<ShallowRef<TriggerReference>>) {
  let closedByOutsidePointer = false

  function handlePointerDownOutside() {
    closedByOutsidePointer = true
  }

  function handleMenuOpenUpdate(open: boolean) {
    if (open || !closedByOutsidePointer) return
    closedByOutsidePointer = false

    const activeElement = document.activeElement
    if (activeElement !== null && activeElement !== document.body) return

    const trigger = triggerRef.value
    const element = trigger instanceof HTMLElement ? trigger : (trigger?.$el ?? trigger?.el)
    element?.focus()
  }

  return { handleMenuOpenUpdate, handlePointerDownOutside }
}
