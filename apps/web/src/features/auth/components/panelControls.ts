import { computed, readonly } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUserStore } from '../session/session'
import { useAsyncAction } from '../../../shared/composables/useAsyncAction'
import { resolveInteractiveControlState } from '../../../shared/interaction/interactiveControlState'
import { useAuthFeedback } from '../useAuthFeedback'

/**
 * Shared chrome wiring for the auth modal panels (login/register): locale,
 * session store, feedback channel, async-action pending state, and the
 * submit/secondary control states derived from it.
 */
export function useAuthPanelControls() {
  const { t } = useI18n()
  const userStore = useUserStore()
  const authFeedback = useAuthFeedback()
  const { isPending: loading, runAsyncAction } = useAsyncAction()
  const submitControlState = computed(() =>
    resolveInteractiveControlState({ pending: loading.value }),
  )
  const secondaryControlState = computed(() =>
    resolveInteractiveControlState({ disabled: loading.value }),
  )
  return {
    t,
    userStore,
    authFeedback,
    loading: readonly(loading),
    runAsyncAction,
    submitControlState,
    secondaryControlState,
  }
}
