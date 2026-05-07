import { onMounted, onUnmounted, type Ref } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'

type UseFlowLeaveGuardOptions = {
  isDirty: Ref<boolean>
  isSaving: Ref<boolean>
  confirmLeave?: () => Promise<boolean> | boolean
}

export function useFlowLeaveGuard(options: UseFlowLeaveGuardOptions) {
  function onWindowBeforeUnload(event: BeforeUnloadEvent) {
    if (!options.isDirty.value || options.isSaving.value) return
    event.preventDefault()
    event.returnValue = ''
  }

  onBeforeRouteLeave(async () => {
    if (!options.isDirty.value || options.isSaving.value) return true
    return options.confirmLeave ? await options.confirmLeave() : false
  })

  onMounted(() => {
    window.addEventListener('beforeunload', onWindowBeforeUnload)
  })

  onUnmounted(() => {
    window.removeEventListener('beforeunload', onWindowBeforeUnload)
  })
}
