import { describe, expect, it } from 'vitest'
import {
  IMAGE_TOOL_CHROME_LAYOUT,
  ImageToolChromeFrame,
  ImageToolStagePresentation,
  useCanvasImageTools,
} from './index'
import type { RetouchChromeAnchorRect } from '../retouch'
import { IMAGE_TOOL_CHROME_LAYOUT as directLayout } from './core/imageToolChrome'
import { useCanvasImageTools as directUseCanvasImageTools } from './session/useCanvasImageTools'
import type { ImageToolChromeModel } from './session/bindings'
import DirectChromeFrame from './ui/ChromeFrame.vue'
import DirectStagePresentation from './ui/StagePresentation.vue'

// Compile-time assignability pin: the chrome-model anchor rect uses the
// Retouch barrel's declared port (RetouchChromeAnchorRect).
const _anchorSeam: RetouchChromeAnchorRect | null =
  null as unknown as ImageToolChromeModel['anchorRect']

/**
 * image-tools/seam-identity-leaks: ChromeFrame, anchor rect/layout and
 * bindings cross only approved seams (tool index / declared zone entries).
 * Barrel identity fails on drift instead of silently forking behavior.
 *
 * Cycle note: retouch/inpaint chrome and the coordinate adapter consume the
 * zone leaf entries (ui/ChromeFrame.vue, core/imageToolChrome.ts)
 * rather than this barrel — routing those four edges through the barrel
 * creates mutual-barrel no-circular violations (isMeasured 27, lint exit 1).
 * The barrel entries below stay as the declared seam for future
 * consolidation (e.g. a shared chrome seam) without changing call sites.
 */
describe('image-tools public seam', () => {
  it('declares ChromeFrame through the index', () => {
    expect(ImageToolChromeFrame).toBe(DirectChromeFrame)
  })

  it('exposes the stage presentation and session hook through the index', () => {
    expect(ImageToolStagePresentation).toBe(DirectStagePresentation)
    expect(useCanvasImageTools).toBe(directUseCanvasImageTools)
  })

  it('declares chrome layout through the index', () => {
    expect(IMAGE_TOOL_CHROME_LAYOUT).toBe(directLayout)
    expect(_anchorSeam).toBeNull()
  })
})
