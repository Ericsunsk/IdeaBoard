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
    promptNodes: [{ id: 3, type: 'prompt' as const, x: 0, y: 0, width: 200, text: 'prompt' }],
    modelNodes: [
      {
        id: 4,
        type: 'model' as const,
        x: 0,
        y: 0,
        width: 240,
        angleSpec: { yaw: 0, framing: 'full' as const },
        modelStrength: 80,
        poseStrength: 70,
        background: 'white',
        outputStatus: 'idle' as const,
        outputImages: [],
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
    maskNodes: [
      {
        id: 6,
        type: 'mask' as const,
        x: 0,
        y: 0,
        width: 300,
        sourceImageUrl: 'https://example.com/c.png',
        sourceImageRatio: 1,
        executionMaskDataUrl: 'data:image/png;base64,aaa',
        maskDataUrl: 'data:image/png;base64,bbb',
        brushSize: 8,
        drawMode: 'paint' as const,
        refineAmount: 0,
        invertMask: false,
      },
    ],
    colorNodes: [
      {
        id: 7,
        type: 'color' as const,
        x: 0,
        y: 0,
        width: 240,
        colors: ['#000000'],
        strength: 50,
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    editNodes: [
      {
        id: 8,
        type: 'edit' as const,
        x: 0,
        y: 0,
        width: 240,
        mode: 'partial-edit' as const,
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    fusionNodes: [
      {
        id: 9,
        type: 'fusion' as const,
        x: 0,
        y: 0,
        width: 240,
        blendRatio: 50,
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    enhanceNodes: [
      {
        id: 10,
        type: 'enhance' as const,
        x: 0,
        y: 0,
        width: 240,
        mode: 'hd-upscale' as const,
        upscaleMode: 'base' as const,
        upscaleFactor: 2 as const,
        outputFormat: 'jpg' as const,
        outputQuality: 80,
        extendTop: 0,
        extendRight: 0,
        extendBottom: 0,
        extendLeft: 0,
        outputStatus: 'idle' as const,
        outputImages: [],
      },
    ],
    wearNodes: [
      {
        id: 11,
        type: 'wear' as const,
        x: 0,
        y: 0,
        width: 240,
        tileMode: 'single' as const,
        rotation: 0,
        scale: 88,
        opacity: 82,
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
        toNodeType: 'model' as const,
        toNodeId: 4,
        toPort: 'image' as const,
      },
    ],
    viewport: {
      canvasX: 0,
      canvasY: 0,
      zoomLevel: 1,
    },
    updatedAt: Date.now(),
  }
}

function testValidEmptySnapshot() {
  const parsed = canvasBoardStateSchema.safeParse({
    imageNodes: [],
    promptNodes: [],
    modelNodes: [],
    viewpointNodes: [],
    maskNodes: [],
    colorNodes: [],
    editNodes: [],
    fusionNodes: [],
    enhanceNodes: [],
    wearNodes: [],
    connections: [],
    viewport: { canvasX: 0, canvasY: 0, zoomLevel: 1 },
    updatedAt: 1,
  })
  expect(parsed.success === true, 'expected empty snapshot to be valid')
}

function testValidAllNodeMinimumShape() {
  const parsed = canvasBoardStateSchema.safeParse(createValidState())
  expect(parsed.success === true, 'expected minimum node shapes to be valid')
}

function testInvalidStateRejected() {
  const invalid = createValidState()
  invalid.modelNodes[0]!.angleSpec.yaw = 999
  const parsed = canvasBoardStateSchema.safeParse(invalid)
  expect(parsed.success === false, 'expected invalid state to be rejected')
}

function testUnknownFieldsAreStripped() {
  const parsed = canvasBoardStateSchema.parse({
    ...createValidState(),
    futureTopLevelField: 'unexpected',
    modelNodes: [
      {
        ...createValidState().modelNodes[0],
        futureNestedField: 'unexpected',
      },
    ],
  }) as Record<string, unknown>

  expect(!('futureTopLevelField' in parsed), 'expected unknown top-level field to be stripped')
  const firstModelNode = (parsed.modelNodes as Array<Record<string, unknown>>)[0]
  expect(!('futureNestedField' in firstModelNode), 'expected unknown nested field to be stripped')
}

testValidEmptySnapshot()
testValidAllNodeMinimumShape()
testInvalidStateRejected()
testUnknownFieldsAreStripped()

console.log('canvasBoardStateSchema checks passed')
