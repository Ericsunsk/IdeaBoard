import {
  clampModelAngleYaw,
  createDefaultModelAngleSpec,
  type ModelAngleFraming,
  type ModelNodeItem,
} from '../types/canvasNodes'

export function useModelNode() {
  function ensureModelAngleSpec(node: ModelNodeItem) {
    if (node.angleSpec) return node.angleSpec
    node.angleSpec = createDefaultModelAngleSpec()
    return node.angleSpec
  }

  function updateModelAngleYaw(node: ModelNodeItem, yaw: number) {
    const spec = ensureModelAngleSpec(node)
    spec.yaw = clampModelAngleYaw(yaw)
  }

  function updateModelAngleFraming(node: ModelNodeItem, framing: ModelAngleFraming) {
    const spec = ensureModelAngleSpec(node)
    spec.framing = framing
  }

  return {
    updateModelAngleYaw,
    updateModelAngleFraming,
    clampModelAngleYaw,
  }
}
