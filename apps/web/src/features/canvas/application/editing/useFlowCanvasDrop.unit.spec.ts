import { describe, expect, it, vi } from 'vitest'
import {
  CANVAS_LIBRARY_IMPORT_MIME,
  writeCanvasLibraryImportTransfer,
  type CanvasLibraryImportDescriptor,
} from '../../../library'
import { useFlowCanvasDrop } from './useFlowCanvasDrop'
import type { CanvasImageNodeCreationResult } from './useFlowImageUploadActions'

function transfer(): DataTransfer {
  const values = new Map<string, string>()
  const types: string[] = []
  return {
    types,
    effectAllowed: 'all',
    dropEffect: 'none',
    setData(type: string, value: string) {
      values.set(type, value)
      types.push(type)
    },
    getData(type: string) {
      return values.get(type) ?? ''
    },
  } as unknown as DataTransfer
}

const asset: CanvasLibraryImportDescriptor = {
  assetId: 'lasset_asset0017',
  title: 'Drop asset',
  previewUrl: 'https://images.unsplash.com/drop-17-preview',
  width: 1200,
  height: 800,
}

function createDropHarness() {
  const markDropAsCopy = vi.fn()
  const readDroppedFile = vi.fn(() => new File(['file'], 'upload.png', { type: 'image/png' }))
  const createImageNodeFromFile = vi.fn(
    async (): Promise<CanvasImageNodeCreationResult | null> => ({
      kind: 'ready',
      nodeId: 'image-1',
    }),
  )
  const addLibraryAsset = vi.fn()
  const onWarning = vi.fn()
  const screenToFlowCoordinate = vi.fn(({ x, y }: { x: number; y: number }) => ({
    x: x + 12,
    y: y + 18,
  }))
  const drop = useFlowCanvasDrop({
    browser: {
      markDropAsCopy,
      readDroppedFile,
      readDropScreenPoint: (event) => ({ x: event.clientX, y: event.clientY }),
    },
    screenToFlowCoordinate,
    onWarning,
    createImageNodeFromFile,
    addLibraryAsset,
  })
  return {
    addLibraryAsset,
    createImageNodeFromFile,
    drop,
    markDropAsCopy,
    onWarning,
    readDroppedFile,
    screenToFlowCoordinate,
  }
}

describe('useFlowCanvasDrop', () => {
  it('maps Library drag screen coordinates into Canvas coordinates without file upload', async () => {
    const harness = createDropHarness()
    const dataTransfer = transfer()
    writeCanvasLibraryImportTransfer(dataTransfer, asset)

    await harness.drop.onCanvasDrop({
      clientX: 80,
      clientY: 90,
      dataTransfer,
    } as unknown as DragEvent)

    expect(harness.screenToFlowCoordinate).toHaveBeenCalledWith({ x: 80, y: 90 })
    expect(harness.addLibraryAsset).toHaveBeenCalledWith(asset, { x: 92, y: 108 })
    expect(harness.readDroppedFile).not.toHaveBeenCalled()
    expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
  })

  it('keeps invalid Library payloads as no-ops rather than falling through to upload', async () => {
    const harness = createDropHarness()
    const dataTransfer = transfer()
    dataTransfer.setData(
      CANVAS_LIBRARY_IMPORT_MIME,
      JSON.stringify({
        type: 'canvas-library-import',
        asset: { ...asset, assetId: 'not-a-library-id' },
      }),
    )

    await harness.drop.onCanvasDrop({
      clientX: 80,
      clientY: 90,
      dataTransfer,
    } as unknown as DragEvent)

    expect(harness.addLibraryAsset).not.toHaveBeenCalled()
    expect(harness.readDroppedFile).not.toHaveBeenCalled()
    expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
  })

  it('advertises copy to Canvas drop targets', () => {
    const harness = createDropHarness()
    harness.drop.onCanvasDragOver({ dataTransfer: transfer() } as unknown as DragEvent)
    expect(harness.markDropAsCopy).toHaveBeenCalled()
  })

  it('reports one upload warning when a dropped image is retained for retry', async () => {
    const harness = createDropHarness()
    harness.createImageNodeFromFile.mockResolvedValueOnce({
      kind: 'awaiting-retry',
      nodeId: 'image-1',
      failureMessage: 'Upload failed.',
    })

    await harness.drop.onCanvasDrop({
      clientX: 80,
      clientY: 90,
      dataTransfer: transfer(),
    } as unknown as DragEvent)

    expect(harness.onWarning).toHaveBeenCalledOnce()
    expect(harness.onWarning).toHaveBeenCalledWith('Upload failed.')
  })
})
