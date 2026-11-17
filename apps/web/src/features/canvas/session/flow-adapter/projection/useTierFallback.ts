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
  let lastLoadedUrl: string | undefined

  watch([() => sourceIdentity()[0], () => sourceIdentity()[1]], () => {
    const hasErrorFallback = !!tierErrorFallbackUrl.value
    tierErrorFallbackUrl.value = ''
    isLoaded.value = !hasErrorFallback && !!src.value && src.value === lastLoadedUrl
  })

  function onLoad() {
    lastLoadedUrl = src.value
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
