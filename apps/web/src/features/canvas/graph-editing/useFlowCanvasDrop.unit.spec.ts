import { describe, expect, it, vi } from 'vitest'
import {
  CANVAS_LIBRARY_IMPORT_MIME,
  writeCanvasLibraryImportTransfer,
  type CanvasLibraryImportDescriptor,
} from '../../library/libraryTransfer'
import { useFlowCanvasDrop } from './useFlowCanvasDrop'

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
  assetId: 17,
  title: 'Drop asset',
  previewUrl: 'https://images.unsplash.com/drop-17-preview',
  width: 1200,
  height: 800,
}

function createDropHarness() {
  const markDropAsCopy = vi.fn()
  const readDroppedFile = vi.fn(() => new File(['file'], 'upload.png', { type: 'image/png' }))
  const createImageNodeFromFile = vi.fn(async () => 'image-1' as `image-${number}`)
  const addLibraryAsset = vi.fn()
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
    onWarning: vi.fn(),
    createImageNodeFromFile,
    addLibraryAsset,
  })
  return { drop, markDropAsCopy, readDroppedFile, createImageNodeFromFile, addLibraryAsset, screenToFlowCoordinate }
}

describe('useFlowCanvasDrop', () => {
  it('maps Library drag screen coordinates into Canvas coordinates without file upload', async () => {
    const harness = createDropHarness()
    const dataTransfer = transfer()
    writeCanvasLibraryImportTransfer(dataTransfer, asset)

    await harness.drop.onCanvasDrop({ clientX: 80, clientY: 90, dataTransfer } as unknown as DragEvent)

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
      JSON.stringify({ type: 'canvas-library-import', asset: { ...asset, assetId: 0 } }),
    )

    await harness.drop.onCanvasDrop({ clientX: 80, clientY: 90, dataTransfer } as unknown as DragEvent)

    expect(harness.addLibraryAsset).not.toHaveBeenCalled()
    expect(harness.readDroppedFile).not.toHaveBeenCalled()
    expect(harness.createImageNodeFromFile).not.toHaveBeenCalled()
  })

  it('advertises copy to Canvas drop targets', () => {
    const harness = createDropHarness()
    harness.drop.onCanvasDragOver({ dataTransfer: transfer() } as unknown as DragEvent)
    expect(harness.markDropAsCopy).toHaveBeenCalled()
  })
})
