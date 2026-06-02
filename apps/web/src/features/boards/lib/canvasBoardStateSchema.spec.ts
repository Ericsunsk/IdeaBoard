import * as v from 'valibot'
import { canvasBoardStateSchema } from './canvasBoardStateSchema'

function expect(condition: unknown, message: string) {
  if (!condition) {
    throw new Error(message)
  }
}

function createValidState() {
  return {
    imageNodes: [
      {
        id: 2,
        type: 'image-node' as const,
        x: 0,
        y: 0,
        width: 320,
        sourceImageUrl: 'https://example.com/b.png',
        sourceImageRatio: 1,
        outputStatus: 'ready' as const,
        outputImages: [{ url: 'https://example.com/o1.png', ratio: 1 }],
      },
    ],
    promptNodes: [
      {
        id: 3,
        type: 'prompt' as const,
        x: 0,
        y: 0,
        width: 200,
        title: 'Custom Prompt',
        text: 'prompt',
      },
    ],
    viewpointNodes: [
      {
        id: 5,
        type: 'viewpoint' as const,
        x: 0,
        y: 0,
        width: 240,
        activeCategory: 'top' as const,
        selectedViewpoint: 'front',
        lighting: 'warm_white' as const,
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    fusionNodes: [
      {
        id: 8,
        type: 'fusion' as const,
        x: 0,
        y: 0,
        width: 240,
        prompt: 'fusion prompt',
        outputCount: 1,
        aspectRatio: 'auto' as const,
        blendPoint: { x: 0.5, y: 0.5 },
        inputImages: [],
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    connections: [
      {
        id: 12,
        fromNodeType: 'image' as const,
        fromNodeId: 2,
        fromPort: 'image' as const,
        toNodeType: 'viewpoint' as const,
        toNodeId: 5,
        toPort: 'image' as const,
      },
    ],
    viewport: {
      canvasX: 0,
      canvasY: 0,
      zoomLevel: 1,
    },
    previewSnapshotDataUrl: 'data:image/webp;base64,aaa',
    previewSnapshotUpdatedAt: Date.now(),
    previewSnapshotVersion: 2,
    updatedAt: Date.now(),
  }
}

function testValidEmptySnapshot() {
  const parsed = v.safeParse(canvasBoardStateSchema, {
    imageNodes: [],
    promptNodes: [],
    viewpointNodes: [],
    fusionNodes: [],
    connections: [],
    viewport: { canvasX: 0, canvasY: 0, zoomLevel: 1 },
    updatedAt: 1,
  })
  expect(parsed.success === true, 'expected empty snapshot to be valid')
}

function testValidAllNodeMinimumShape() {
  const parsed = v.safeParse(canvasBoardStateSchema, createValidState())
  expect(parsed.success === true, 'expected minimum node shapes to be valid')
  if (parsed.success) {
    expect(
      parsed.output.promptNodes[0]?.title === 'Custom Prompt',
      'expected known title field to be preserved',
    )
    expect(parsed.output.previewSnapshotVersion === 2, 'expected preview version to be preserved')
  }
}

function testInvalidStateRejected() {
  const invalid = createValidState()
  invalid.connections[0]!.toNodeType = 'unknown' as any
  const parsed = v.safeParse(canvasBoardStateSchema, invalid)
  expect(parsed.success === false, 'expected invalid state to be rejected')
}

function testFusionBlendPointDefaultApplied() {
  const state = createValidState() as Record<string, any>
  state.fusionNodes = [{ ...state.fusionNodes[0] }]
  delete state.fusionNodes[0].blendPoint
  delete state.fusionNodes[0].aspectRatio
  const parsed = v.safeParse(canvasBoardStateSchema, state)
  expect(parsed.success === true, 'expected fusion node without defaulted fields to stay valid')
  if (parsed.success) {
    expect(
      parsed.output.fusionNodes[0]?.blendPoint.x === 0.5,
      'expected blendPoint x default to 0.5',
    )
    expect(
      parsed.output.fusionNodes[0]?.blendPoint.y === 0.5,
      'expected blendPoint y default to 0.5',
    )
    expect(parsed.output.fusionNodes[0]?.aspectRatio === 'auto', 'expected aspectRatio default to auto')
  }
}

function testUnknownFieldsAreRejected() {
  const parsed = v.safeParse(canvasBoardStateSchema, {
    ...createValidState(),
    futureTopLevelField: 'unexpected',
  })
  expect(parsed.success === false, 'expected unknown top-level field to be rejected')

  const nestedParsed = v.safeParse(canvasBoardStateSchema, {
    ...createValidState(),
    fusionNodes: [
      {
        ...createValidState().fusionNodes[0],
        futureNestedField: 'unexpected',
      },
    ],
  })
  expect(nestedParsed.success === false, 'expected unknown nested field to be rejected')
}

testValidEmptySnapshot()
testValidAllNodeMinimumShape()
testInvalidStateRejected()
testFusionBlendPointDefaultApplied()
testUnknownFieldsAreRejected()

console.log('canvasBoardStateSchema checks passed')
