import { describe, expect, it } from 'vitest'
import { nextTick, ref } from 'vue'
import { useTierFallback } from './useTierFallback'

describe('useTierFallback', () => {
  it('prefers the display URL until a tier error swaps once to the content fallback', () => {
    const displayUrl = ref('https://delivery.example/derivatives/preview')
    const contentFallbackUrl = ref('https://delivery.example/content')
    const { src, isLoaded, onLoad, onError } = useTierFallback(
      displayUrl,
      contentFallbackUrl,
      () => ['original.png', 'asset_1'],
    )

    expect(src.value).toBe('https://delivery.example/derivatives/preview')
    expect(isLoaded.value).toBe(false)
    onLoad()
    expect(isLoaded.value).toBe(true)

    onError()
    expect(src.value).toBe('https://delivery.example/content')

    // A failing fallback must not error-loop back onto itself.
    onError()
    expect(src.value).toBe('https://delivery.example/content')
  })

  it('leaves sources without a ready fallback untouched when the image errors', () => {
    const displayUrl = ref('https://provider.example/image.png')
    const contentFallbackUrl = ref('')
    const { src, onError } = useTierFallback(displayUrl, contentFallbackUrl, () => [
      'https://provider.example/image.png',
      undefined,
    ])

    onError()
    expect(src.value).toBe('https://provider.example/image.png')
  })

  it('ignores a fallback identical to the display URL', () => {
    const displayUrl = ref('https://delivery.example/content')
    const contentFallbackUrl = ref('https://delivery.example/content')
    const { src, onError } = useTierFallback(displayUrl, contentFallbackUrl, () => [
      'original.png',
      'asset_1',
    ])

    onError()
    expect(src.value).toBe('https://delivery.example/content')
  })

  it.each(['url', 'assetId'] as const)(
    'resets fallback and loading when source %s changes',
    async (field) => {
      const source = ref({ url: 'original.png', assetId: 'asset_1' })
      const { src, isLoaded, onLoad, onError } = useTierFallback(
        () => '/derivatives/preview',
        () => '/content',
        () => [source.value.url, source.value.assetId],
      )
      onLoad()
      onError()
      expect(src.value).toBe('/content')
      source.value[field] = 'replacement'
      await nextTick()
      expect(src.value).toBe('/derivatives/preview')
      expect(isLoaded.value).toBe(false)
    },
  )

  it('keeps the loaded frame and fallback when only the display tier changes', async () => {
    const displayUrl = ref('/derivatives/preview')
    const { src, isLoaded, onLoad, onError } = useTierFallback(
      displayUrl,
      () => '/content',
      () => ['original.png', 'asset_1'],
    )
    onLoad()
    displayUrl.value = '/derivatives/large-preview'
    await nextTick()
    expect(src.value).toBe('/derivatives/large-preview')
    expect(isLoaded.value).toBe(true)
    onError()
    displayUrl.value = '/derivatives/thumb'
    await nextTick()
    expect(src.value).toBe('/content')
    expect(isLoaded.value).toBe(true)
  })

  it('keeps the frame when the source object changes with the same identity', async () => {
    const source = ref({ url: 'original.png', assetId: 'asset_1' })
    const { src, isLoaded, onLoad, onError } = useTierFallback(
      () => '/derivatives/preview',
      () => '/content',
      () => [source.value.url, source.value.assetId],
    )
    onLoad()
    onError()
    source.value = { ...source.value }
    await nextTick()
    expect(src.value).toBe('/content')
    expect(isLoaded.value).toBe(true)
  })
})
