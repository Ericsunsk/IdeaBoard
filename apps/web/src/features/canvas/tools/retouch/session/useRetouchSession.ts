import { computed, ref } from 'vue'
import type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchEffectParamKey,
  RetouchEffectParamMap,
  RetouchNonHslWholeParams,
  RetouchLayer,
} from '../core/model.ts'
import {
  applyLayerCommand,
  createDefaultImageEffectLayer,
  createInitialLayerStack,
  getSelectedEffect,
  SOURCE_LAYER_ID,
  updateEffectLayerPatch,
  type RetouchLayerCommand,
  type RetouchParamPatch,
} from '../core/layers/layerStack.ts'
import {
  getImageRetouchDefaultParams,
  isNeutralImageRetouch,
  normalizeImageEffectLayer,
} from '../core/definitions.ts'

export type RetouchPendingStatus = 'idle-pending' | 'pending-kind' | 'committed-selected'

/*
 * Single editing identity. `selectedLayerId` in the stack stays the tree
 * cursor only (Chrome remove-layer and the layer list read it); what is being
 * edited resolves through this target: a transient `pending` kind preview or
 * a `committed` layer referenced by id.
 */
export type RetouchEditingTarget = {
  kind: ImageRetouchKind
  origin: 'pending' | 'committed'
  layerId?: string
}

export function useRetouchSession() {
  const stack = ref(createInitialLayerStack())
  const pendingRetouch = ref<RetouchEffectLayer | null>(createDefaultImageEffectLayer('hsl'))

  /*
   * Explicit pending machine. Three states, one transition each:
   * - 'idle-pending': no transient preview and no committed selection (the
   *   tree cursor sits on source or a paint layer); edits have nowhere to go
   *   until an effect kind is picked.
   * - 'pending-kind': a transient kind preview exists; the first
   *   non-neutral edit commits it via commitPendingOnFirstNonNeutralEdit,
   *   structural commands drop it via discardPendingOnStructuralCommand.
   * - 'committed-selected': a committed effect layer is selected; edits apply
   *   to it directly through layerStack commands.
   */
  function resolvePendingStatus(): RetouchPendingStatus {
    if (pendingRetouch.value) return 'pending-kind'
    return getSelectedEffect(stack.value) ? 'committed-selected' : 'idle-pending'
  }

  const layers = computed<RetouchLayer[]>({
    get: () => stack.value.layers,
    set: (nextLayers) => {
      stack.value = { ...stack.value, layers: nextLayers }
    },
  })
  const selectedLayerId = computed({
    get: () => stack.value.selectedLayerId,
    set: (nextSelectedLayerId: string) => {
      stack.value = { ...stack.value, selectedLayerId: nextSelectedLayerId }
    },
  })
  const editingTarget = computed<RetouchEditingTarget | null>(() => {
    const status = resolvePendingStatus()
    if (status === 'pending-kind') {
      const pending = pendingRetouch.value
      return pending ? { kind: pending.kind, origin: 'pending' } : null
    }
    if (status === 'committed-selected') {
      const committed = getSelectedEffect(stack.value)
      return committed ? { kind: committed.kind, origin: 'committed', layerId: committed.id } : null
    }
    return null
  })
  const selectedEffectLayer = computed<RetouchEffectLayer | null>(() => {
    // Derived view over the single editing identity: pending previews resolve
    // through the transient slot, committed targets re-resolve through the
    // tree by id so cursor moves never leak a stale layer object.
    const target = editingTarget.value
    if (!target) return null
    if (target.origin === 'pending') return pendingRetouch.value
    const layer = stack.value.layers.find((item) => item.id === target.layerId)
    return layer && layer.type === 'effect' ? layer : null
  })
  // Explicit pending slot for the histogram resolver (B-road compromise):
  // selectedEffectLayer already resolves through pending, but the resolver
  // needs the staged layer separately to distinguish a pending target from
  // an id-miss. Optional at the chrome seam so existing callers keep working.
  const pendingEffectLayer = computed<RetouchEffectLayer | null>(() => pendingRetouch.value)
  function applyCommand(command: RetouchLayerCommand) {
    stack.value = applyLayerCommand(stack.value, command)
  }

  function resetRetouchSession() {
    stack.value = createInitialLayerStack()
    pendingRetouch.value = createDefaultImageEffectLayer('hsl')
  }

  function discardPendingOnStructuralCommand(command: RetouchLayerCommand) {
    pendingRetouch.value = null
    applyCommand(command)
  }

  function selectLayer(layerId: string) {
    discardPendingOnStructuralCommand({ type: 'select-layer', layerId })
  }

  function addLayer() {
    discardPendingOnStructuralCommand({ type: 'add-paint' })
  }

  function copyLayer() {
    discardPendingOnStructuralCommand({ type: 'copy-selected' })
  }

  function mergeLayer() {
    discardPendingOnStructuralCommand({ type: 'merge-selected-down' })
  }

  // Pure routing: an existing kind layer is selected through the structural
  // path (pending discarded); otherwise a transient pending of that kind is
  // staged and the tree cursor parks on source. No param writes here.
  function selectEffectLayer(kind: ImageRetouchKind) {
    const hasExistingLayer = stack.value.layers.some(
      (layer) => layer.type === 'effect' && layer.kind === kind,
    )
    if (hasExistingLayer) {
      // ensure-retouch selects the existing kind layer: same outcome, no duplicated lookup.
      discardPendingOnStructuralCommand({ type: 'ensure-retouch', kind })
      return
    }

    pendingRetouch.value = createDefaultImageEffectLayer(kind)
    stack.value = { ...stack.value, selectedLayerId: SOURCE_LAYER_ID }
  }

  function setLayerVisible(layerId: string, visible: boolean) {
    applyCommand({ type: 'set-visible', layerId, visible })
  }

  function removeLayer(layerId: string) {
    discardPendingOnStructuralCommand({ type: 'remove', layerId })
  }

  // Single fan-out: the dual APIs stay as thin adapters over one patch
  // core. Every kind travels single-key with per-key value correlation (a
  // wrong value type is a compile error); whole-params packets keep their
  // shape. No local kind/`in` checks here; layerStack owns the shape guards
  // through updateEffectLayerPatch.
  function updateSelectedLayerParam<TKey extends RetouchEffectParamKey>(
    key: TKey,
    value: RetouchEffectParamMap[TKey],
  ) {
    updateSelectedRetouchPatch({ kind: 'single-param', key, value } as RetouchParamPatch)
  }

  function updateSelectedRetouchParams(params: RetouchNonHslWholeParams) {
    updateSelectedRetouchPatch({ kind: 'whole-params', params })
  }

  function updateSelectedRetouchPatch(patch: RetouchParamPatch) {
    updateSelectedEffect(
      (pending) => {
        const next = updateEffectLayerPatch(pending, patch)
        return next !== pending ? next : null
      },
      patch.kind === 'single-param'
        ? ({
            type: 'update-selected-param',
            key: patch.key,
            value: patch.value,
          } as RetouchLayerCommand)
        : { type: 'update-selected-retouch-params', params: patch.params },
    )
  }

  // Dry-run reuses layerStack kind guards: mismatched packets return the same
  // reference, so they stay pending instead of committing by mistake.
  function updateSelectedEffect(
    buildNextPending: (pending: RetouchEffectLayer) => RetouchEffectLayer | null,
    commitCommand: RetouchLayerCommand,
  ) {
    const pending = pendingRetouch.value
    if (pending) {
      const next = buildNextPending(pending)
      if (next) commitPendingOnFirstNonNeutralEdit(next)
      return
    }
    applyCommand(commitCommand)
  }

  function commitPendingOnFirstNonNeutralEdit(next: RetouchEffectLayer) {
    // Neutral gate converges with the render pipeline: the engine plan
    // compiler skips neutral layers through the same
    // isNeutralImageRetouch(normalize(...)) predicate, so a neutral first edit
    // stays pending instead of committing a no-op layer. Threshold is never
    // neutral (any threshold posterizes the image), so even its defaults
    // commit on the first edit.
    if (isNeutralImageRetouch(normalizeImageEffectLayer(next))) {
      // Sharpen is neutral at amount 0 even with custom radius, threshold,
      // or color protection. Keep the sanitized draft for the first commit.
      if (next.kind === 'sharpen') {
        pendingRetouch.value = next
        return
      }
      const defaults = getImageRetouchDefaultParams(next.kind)
      pendingRetouch.value = { ...next, params: structuredClone(defaults) } as RetouchEffectLayer
      return
    }

    pendingRetouch.value = null
    applyCommand({ type: 'ensure-retouch', kind: next.kind })
    if (next.kind === 'hsl') {
      const target = getSelectedEffect(stack.value)
      if (target?.kind !== 'hsl') return
      const params = { ...next.params }
      stack.value = {
        ...stack.value,
        layers: stack.value.layers.map((layer) =>
          layer.id === target.id ? ({ ...layer, params } as RetouchLayer) : layer,
        ),
      }
      return
    }
    applyCommand({ type: 'update-selected-retouch-params', params: next.params })
  }

  return {
    retouchLayers: layers,
    selectedRetouchLayerId: selectedLayerId,
    selectedRetouchEffectLayer: selectedEffectLayer,
    pendingEffectLayer,
    resetRetouchSession,
    selectRetouchLayer: selectLayer,
    setRetouchLayerVisible: setLayerVisible,
    addRetouchLayer: addLayer,
    copyRetouchLayer: copyLayer,
    mergeRetouchLayer: mergeLayer,
    removeRetouchLayer: removeLayer,
    updateSelectedRetouchLayerParam: updateSelectedLayerParam,
    updateSelectedRetouchEffectParams: updateSelectedRetouchParams,
    updateSelectedRetouchPatch,
    selectImageEffectLayer: selectEffectLayer,
  }
}
