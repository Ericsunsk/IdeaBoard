import { computed, nextTick, reactive, ref, toValue } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent } from '@testing-library/vue'
import type { ImageToolStageBinding } from '../../../tools/image-tools'
import ImageNodeStage from './ImageNodeStage.vue'
import { render } from '../../../../../test/render'

const previewValues = vi.hoisted(() => ({
  displayUrl: null as string | null,
  contentFallbackUrl: '' as string,
  isVisible: false,
  retainDisplayUrl: vi.fn(() => vi.fn()),
}))
const previewState = reactive(previewValues)

afterEach(() => {
  previewState.displayUrl = null
  previewState.contentFallbackUrl = ''
  previewState.retainDisplayUrl.mockClear()
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

beforeEach(() => {
  // jsdom never intersects: stages start offscreen (lazy) unless a test
  // overrides the visibility mock.
  previewState.isVisible = false
})

vi.mock('@vueuse/core', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@vueuse/core')>()
  return {
    ...actual,
    useElementVisibility: vi.fn(() => ref(previewState.isVisible)),
  }
})

vi.mock('../../../session/flow-adapter/projection/useDisplayImagePreview', () => ({
  useDisplayImagePreview: (
    sourceUrl: Parameters<typeof toValue>[0],
    options?: { assetId?: Parameters<typeof toValue>[0] },
  ) => ({
    displayUrl: computed(() => {
      if (toValue(options?.assetId)) return previewState.displayUrl ?? ''
      return previewState.displayUrl ?? toValue(sourceUrl) ?? ''
    }),
    contentFallbackUrl: computed(() => previewState.contentFallbackUrl),
    retainDisplayUrl: previewState.retainDisplayUrl,
  }),
}))

function renderImageStage(isToolActive: boolean) {
  return render(ImageNodeStage, {
    props: {
      nodeId: 'image-1',
      stageWidth: 280,
      stageHeight: 280,
      displayImage: { url: 'https://provider.example/image.png' },
      imageToolStageBinding: { isActive: isToolActive } as ImageToolStageBinding,
      isActive: false,
      isDragging: false,
    },
    global: {
      stubs: {
        StagePresentation: { template: '<div><slot /></div>' },
        NodeStageFrame: {
          props: ['isTransparent'],
          template:
            '<div data-testid="node-stage-frame" :data-transparent="String(isTransparent)"><slot /></div>',
        },
      },
    },
  })
}

describe('ImageNodeStage disabled state', () => {
  it('disables file selection while an image action is generating', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: null,
        isDirectGenerating: true,
        isActive: false,
        isDragging: false,
      },
      global: {
        stubs: {
          NodeStageFrame: { template: '<div><slot /></div>' },
        },
      },
    })

    expect(view.getByLabelText('Image is being generated; upload unavailable')).toBeTruthy()
    expect(view.getByLabelText('Image is being generated; upload unavailable').tagName).not.toBe(
      'BUTTON',
    )
  })
})

describe('ImageNodeStage Library import recovery', () => {
  it('keeps an optimistic preview visible while the Library import materializes', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'https://provider.example/preview.png' },
        libraryImport: { state: 'pending', libraryAssetId: 'lasset_asset0007' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'https://provider.example/preview.png',
    )
  })

  it('offers Retry only for a failed persisted Library import', async () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: null,
        libraryImport: {
          state: 'failed',
          libraryAssetId: 'lasset_asset0007',
          failureMessage: '素材导入失败',
        },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    await fireEvent.click(view.getByLabelText('Retry image import'))
    expect(view.emitted('retry-image-import')).toEqual([[]])
  })

  it('offers the same compact Retry action for a failed local upload', async () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'blob:local-image' },
        localUpload: { previewUrl: 'blob:local-image', failureMessage: '上传失败' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:local-image',
    )
    await fireEvent.click(view.getByLabelText('Retry image import'))
    expect(view.emitted('retry-image-import')).toEqual([[]])
  })

  it('shows the local blob preview before the managed Media asset is ready', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: null,
        localUpload: { previewUrl: 'blob:local-image' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:local-image',
    )
    expect(view.queryByLabelText('Preparing image preview')).toBeNull()
  })

  it('prefers a local blob preview over an empty persisted image URL', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: '' },
        localUpload: { previewUrl: 'blob:local-image' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:local-image',
    )
  })

  it('keeps the original source visible while a derived preview is still decoding', () => {
    previewState.displayUrl = ''
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'blob:local-image' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:local-image',
    )
    expect(view.queryByLabelText('Preparing image preview')).toBeNull()
    previewState.displayUrl = null
  })

  it('does not bind the authenticated API URL while a media delivery URL is loading', () => {
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: {
          url: '/api/v1/workspaces/ws_test123/media-assets/asset_5/content',
          assetId: 'asset_5',
        },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })

    expect(view.queryByRole('button', { name: /Image node preview/ })).toBeNull()
    expect(view.getByLabelText('Preparing image preview')).toBeTruthy()
  })
})

describe('ImageNodeStage tier delivery', () => {
  function renderAssetStage() {
    return render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: {
          url: '/api/v1/workspaces/ws_test123/media-assets/asset_5/content',
          assetId: 'asset_5',
        },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })
  }

  it.each([
    { isVisible: false, loading: 'lazy' },
    { isVisible: true, loading: 'eager' },
  ])('uses $loading loading when visibility is $isVisible', ({ isVisible, loading }) => {
    previewState.displayUrl = 'https://provider.example/image.png'
    previewState.isVisible = isVisible
    const view = renderAssetStage()
    const image = view.getByRole('button', { name: /Image node preview/ })
    expect(image).toHaveAttribute('loading', loading)
    expect(image).toHaveAttribute('decoding', 'async')
  })

  it('unblurs the tier image on load', async () => {
    previewState.displayUrl = 'https://provider.example/image.png'
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'https://provider.example/image.png' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })
    const image = view.getByRole('button', { name: /Image node preview/ })
    expect(image.classList.contains('is-tier-loading')).toBe(true)
    await fireEvent.load(image)
    expect(image.classList.contains('is-tier-loading')).toBe(false)
  })

  it('falls back to the content URL once when a tier 409s', async () => {
    previewState.displayUrl =
      '/api/v1/workspaces/ws_test123/media-assets/asset_5/derivatives/preview'
    previewState.contentFallbackUrl = '/api/v1/workspaces/ws_test123/media-assets/asset_5/content'
    const view = renderAssetStage()
    const image = view.getByRole('button', { name: /Image node preview/ })
    expect(image).toHaveAttribute(
      'src',
      '/api/v1/workspaces/ws_test123/media-assets/asset_5/derivatives/preview',
    )

    await fireEvent.error(image)
    const fallback = view.getByRole('button', { name: /Image node preview/ })
    expect(fallback).toHaveAttribute(
      'src',
      '/api/v1/workspaces/ws_test123/media-assets/asset_5/content',
    )

    expect(fallback).not.toHaveAttribute('srcset')

    // A failing fallback must not error-loop back onto itself.
    await fireEvent.error(fallback)
    expect(fallback).toHaveAttribute(
      'src',
      '/api/v1/workspaces/ws_test123/media-assets/asset_5/content',
    )
  })

  it('leaves non-asset sources untouched when the image errors', async () => {
    previewState.displayUrl = 'https://provider.example/image.png'
    const view = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'https://provider.example/image.png' },
        isActive: false,
        isDragging: false,
      },
      global: { stubs: { NodeStageFrame: { template: '<div><slot /></div>' } } },
    })
    const image = view.getByRole('button', { name: /Image node preview/ })
    await fireEvent.error(image)
    expect(image).toHaveAttribute('src', 'https://provider.example/image.png')
  })
})

describe('ImageNodeStage image tool presentation', () => {
  it('keeps the stage transparent and hides stage actions only while the tool binding is active', () => {
    const activeTool = renderImageStage(true)
    const noTool = renderImageStage(false)
    expect(activeTool.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'true',
    )
    expect(activeTool.container.querySelector('.image-node-stage-actions')).toBeTruthy()
    expect(noTool.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'false',
    )
    expect(noTool.container.querySelector('.image-node-stage-actions')).toBeTruthy()
  })

  it('renders source media neutrally until a selected tool owns the stage', () => {
    const noBinding = render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'https://provider.example/image.png' },
        imageToolStageBinding: null,
        isActive: false,
        isDragging: false,
      },
      global: {
        stubs: {
          StagePresentation: { template: '<div><slot /></div>' },
          NodeStageFrame: {
            props: ['isTransparent'],
            template:
              '<div data-testid="node-stage-frame" :data-transparent="String(isTransparent)"><slot /></div>',
          },
        },
      },
    })
    const inactive = renderImageStage(false)
    const active = renderImageStage(true)

    for (const view of [noBinding, inactive, active]) {
      const media = view.container.querySelector('.image-node-stage-image')
      expect(media?.getAttribute('src')).toBe('https://provider.example/image.png')
    }
    expect(noBinding.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'false',
    )
    expect(inactive.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'false',
    )
    expect(active.container.querySelector('[data-testid="node-stage-frame"]')).toHaveAttribute(
      'data-transparent',
      'true',
    )
  })
})

describe('ImageNodeStage decoded image transitions', () => {
  function renderTransitionStage() {
    vi.useFakeTimers()
    const readStyle = window.getComputedStyle.bind(window)
    vi.spyOn(window, 'getComputedStyle').mockImplementation((element) => {
      const style = readStyle(element)
      return new Proxy(style, {
        get(target, property) {
          if (property === 'transitionDuration') return '0.2s'
          if (property === 'transitionDelay') return '0s'
          if (property === 'transitionProperty') return 'opacity'
          const value = Reflect.get(target, property)
          return typeof value === 'function' ? value.bind(target) : value
        },
      })
    })
    previewState.displayUrl = 'blob:thumb'
    return render(ImageNodeStage, {
      props: {
        nodeId: 'image-1',
        stageWidth: 280,
        stageHeight: 280,
        displayImage: { url: 'https://provider.example/image.png' },
        isActive: false,
        isDragging: false,
      },
      global: {
        stubs: {
          transition: false,
          StagePresentation: { template: '<div><slot /></div>' },
          NodeStageFrame: { template: '<div><slot /></div>' },
        },
      },
    })
  }

  it('keeps and pins the existing decoded DOM image until the crossfade completes', async () => {
    const view = renderTransitionStage()
    const outgoing = view.getByRole('button', { name: /Image node preview/ })
    await fireEvent.load(outgoing)
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    const incoming = view.getByRole('button', { name: /Image node preview/ })
    expect(incoming).toHaveAttribute('src', 'blob:preview')
    expect(outgoing).toHaveAttribute('src', 'blob:thumb')
    expect(outgoing).toHaveAttribute('aria-hidden', 'true')
    expect(outgoing).toHaveAttribute('inert')
    expect(outgoing).toHaveClass('canvas-image-leave-active')
    expect(incoming).not.toHaveClass('canvas-image-leave-active')
    expect(previewState.retainDisplayUrl).toHaveBeenCalledWith('blob:thumb')
    const release = previewState.retainDisplayUrl.mock.results[0]?.value
    expect(release).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(40)
    await fireEvent.transitionEnd(outgoing)
    expect(outgoing).not.toBeInTheDocument()
    expect(release).toHaveBeenCalledOnce()
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
  })

  it('releases rapid tier replacements and all pending leaves on disposal', async () => {
    const view = renderTransitionStage()
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    previewState.displayUrl = 'blob:large-preview'
    await nextTick()
    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:large-preview',
    )
    const releases = previewState.retainDisplayUrl.mock.results.map((result) => result.value)
    cleanup()
    for (const release of releases) expect(release).toHaveBeenCalledOnce()
    expect(view.container.querySelectorAll('img')).toHaveLength(0)
  })

  it('pins only the rendered frame when tier updates are batched before rendering', async () => {
    const view = renderTransitionStage()
    const outgoing = view.getByRole('button', { name: /Image node preview/ })
    previewState.displayUrl = 'blob:preview'
    previewState.displayUrl = 'blob:large-preview'
    await nextTick()
    expect(previewState.retainDisplayUrl.mock.calls).toEqual([['blob:thumb']])
    await vi.advanceTimersByTimeAsync(40)
    await fireEvent.transitionEnd(outgoing)
    expect(previewState.retainDisplayUrl.mock.results[0]?.value).toHaveBeenCalledOnce()
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
  })

  it('releases a pending pin when batched updates return to the rendered frame', async () => {
    const view = renderTransitionStage()
    const image = view.getByRole('button', { name: /Image node preview/ })
    previewState.displayUrl = 'blob:preview'
    previewState.displayUrl = 'blob:thumb'
    await nextTick()
    expect(view.getByRole('button', { name: /Image node preview/ })).toBe(image)
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
    expect(previewState.retainDisplayUrl.mock.calls).toEqual([['blob:thumb']])
    expect(previewState.retainDisplayUrl.mock.results[0]?.value).toHaveBeenCalledOnce()
  })

  it('releases each DOM frame independently when a tier is revisited during a leave', async () => {
    const view = renderTransitionStage()
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    previewState.displayUrl = 'blob:thumb'
    await nextTick()
    previewState.displayUrl = 'blob:large-preview'
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)
    for (const result of previewState.retainDisplayUrl.mock.results) {
      expect(result.value).toHaveBeenCalledOnce()
    }
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
  })

  it('does not pin frames owned by the image tool presentation branch', async () => {
    const view = renderTransitionStage()
    await view.rerender({ imageToolStageBinding: { isActive: false } as ImageToolStageBinding })
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    expect(previewState.retainDisplayUrl).not.toHaveBeenCalled()
  })

  it('releases a pending plain-stage pin when an inactive binding takes ownership', async () => {
    const view = renderTransitionStage()
    previewState.displayUrl = 'blob:preview'
    await view.rerender({ imageToolStageBinding: { isActive: false } as ImageToolStageBinding })
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
    expect(previewState.retainDisplayUrl.mock.results[0]?.value).toHaveBeenCalledOnce()
  })

  it('releases pending leaves when tool presentation replaces the stage', async () => {
    const view = renderTransitionStage()
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    const release = previewState.retainDisplayUrl.mock.results[0]?.value
    await view.rerender({ imageToolStageBinding: { isActive: true } as ImageToolStageBinding })
    expect(release).toHaveBeenCalledOnce()
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
    previewState.displayUrl = 'blob:original'
    await nextTick()
    expect(previewState.retainDisplayUrl).toHaveBeenCalledTimes(1)
  })

  it('discards outgoing presentation when the source changes', async () => {
    const view = renderTransitionStage()
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    previewState.displayUrl = 'blob:other-image'
    await view.rerender({ displayImage: { url: 'https://provider.example/other.png' } })
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:other-image',
    )
    expect(previewState.retainDisplayUrl.mock.results[0]?.value).toHaveBeenCalledOnce()
  })

  it('replaces the image immediately without a retained layer under reduced motion', async () => {
    vi.stubGlobal(
      'matchMedia',
      vi.fn((): Partial<MediaQueryList> => ({ matches: true })),
    )
    const view = renderTransitionStage()
    previewState.displayUrl = 'blob:preview'
    await nextTick()
    expect(view.container.querySelectorAll('img')).toHaveLength(1)
    expect(view.getByRole('button', { name: /Image node preview/ })).toHaveAttribute(
      'src',
      'blob:preview',
    )
    expect(previewState.retainDisplayUrl).not.toHaveBeenCalled()
  })
})
