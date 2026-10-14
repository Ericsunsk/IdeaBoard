import { computed, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'

/** Use the ready original once when a displayed derivative fails to load. */
export function useTierFallback(
  displayUrl: MaybeRefOrGetter<string | null | undefined>,
  contentFallbackUrl: MaybeRefOrGetter<string | null | undefined>,
  sourceIdentity: () => readonly [string | null | undefined, string | undefined],
) {
  const tierErrorFallbackUrl = ref('')
  const src = computed(() => tierErrorFallbackUrl.value || toValue(displayUrl) || undefined)
  const isLoaded = ref(false)

  watch([() => sourceIdentity()[0], () => sourceIdentity()[1]], () => {
    tierErrorFallbackUrl.value = ''
    isLoaded.value = false
  })

  function onLoad() {
    isLoaded.value = true
  }

  function onError() {
    if (tierErrorFallbackUrl.value) return
    const fallback = toValue(contentFallbackUrl)
    if (!fallback || fallback === toValue(displayUrl)) return
    tierErrorFallbackUrl.value = fallback
  }

  return { src, isLoaded, onLoad, onError }
}
