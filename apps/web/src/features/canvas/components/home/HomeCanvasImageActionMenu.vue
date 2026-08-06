<template>
  <div
    class="image-action-menu node-tool-surface"
    :class="{ 'strength-mode': showStrengthControl }"
    :style="showStrengthControl ? strengthMenuStyle : undefined"
    @mousedown.stop
    @click.stop
  >
    <Transition v-bind="actionMenuMotion" appear>
      <div class="image-action-menu-inner">
        <div
          v-for="item in IMAGE_ACTION_MENU_ITEMS"
          :key="item.title"
          class="image-action-control"
          :class="{
            'has-strength-range': !!item.strengthAction,
            'is-strength-active': isStrengthActive(item),
            'is-strength-hidden': isStrengthHidden(item),
          }"
        >
          <NodeToolIconButton
            :ref="(el) => setItemStrengthTriggerRef(item, el)"
            class="image-action-icon-btn"
            :aria-label-value="getItemTooltip(item)"
            :disabled="isItemDisabled(item)"
            :tooltip="getItemTooltip(item)"
            @click="onActionClick(item)"
          >
            <ImageActionMenuIcon :icon="item.icon" />
          </NodeToolIconButton>
          <span v-if="getItemTouchPrice(item)" class="image-action-price" data-touch-quote>
            {{ getItemTouchPrice(item) }}
          </span>
          <input
            v-if="item.strengthAction"
            :ref="(el) => setItemStrengthRangeRef(item, el)"
            :name="`image-action-strength-${item.label}`"
            class="image-action-strength-range canvas-range-input"
            type="range"
            min="0"
            max="100"
            step="1"
            :value="getItemStrengthValue(item)"
            :style="getItemStrengthRangeStyle(item)"
            :disabled="isItemDisabled(item)"
            tabindex="-1"
            :aria-hidden="isStrengthActive(item) ? undefined : 'true'"
            :aria-label="`${item.label}强度`"
            @pointerdown.stop="onItemStrengthRangePointerDown(item, $event)"
            @input="onItemStrengthInput(item, $event)"
            @change="onItemStrengthChange(item, $event)"
            @pointerup="onItemStrengthPointerUp(item, $event)"
            @click="onStrengthRangeClick(item, $event)"
          />
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, type ComponentPublicInstance } from 'vue'
import {
  WORKFLOW_FEATURE_SLUGS,
  type WorkflowFeature,
} from '../../../../shared/domain/workflowFeatures'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'
import {
  IMAGE_ACTION_MENU_ITEMS,
  type ImageActionMenuItem,
  type ImageNodeAction,
} from '../../config/imageActionRegistry'
import type { ImageStrengthAction } from '../../execution'
import type { ImageActionStrengthControl } from '../../image-tools'
import ImageActionMenuIcon from './ImageActionMenuIcon.vue'
import NodeToolIconButton from '../nodes/NodeToolIconButton.vue'
import { useCanvasBillingReadinessStore } from '../../execution'
import { createPopoverMotionTransition } from '../../../../shared/motion/popoverMotion'

const props = defineProps<{
  strengthMenuStyle?: string | Record<string, string>
  directGenerating: boolean
  strengthControl: ImageActionStrengthControl
}>()

const emit = defineEmits<{
  (event: 'action', action: ImageNodeAction): void
}>()

const showStrengthControl = computed(() => props.strengthControl.visible)
const billingStore = useCanvasBillingReadinessStore()
const actionMenuMotion = createPopoverMotionTransition()

onMounted(() => {
  if (!billingStore) return
  for (const item of IMAGE_ACTION_MENU_ITEMS) {
    const feature = resolveItemBillingFeature(item)
    if (feature) void billingStore.ensureFeature(feature)
  }
})

function resolveItemBillingFeature(item: ImageActionMenuItem): WorkflowFeature | null {
  return WORKFLOW_FEATURE_SLUGS.includes(item.action as WorkflowFeature)
    ? (item.action as WorkflowFeature)
    : null
}

function getItemBillingState(item: ImageActionMenuItem) {
  const feature = resolveItemBillingFeature(item)
  return feature && billingStore ? billingStore.getFeatureState(feature) : null
}

function isItemDisabled(_item: ImageActionMenuItem) {
  return props.directGenerating
}

function getItemTooltip(item: ImageActionMenuItem) {
  const billing = getItemBillingState(item)
  if (!billing) return item.label
  if (billing.status === 'ready') {
    return `${item.label} · ${formatQuote(billing.unitCredits ?? 0)}`
  }
  return item.label
}

function formatQuote(totalCredits: number) {
  return totalCredits === 0 ? '免费' : formatIdeaAmount(totalCredits)
}

function getItemTouchPrice(item: ImageActionMenuItem) {
  const billing = getItemBillingState(item)
  return billing?.status === 'ready' ? formatQuote(billing.unitCredits ?? 0) : ''
}

function onActionClick(item: ImageActionMenuItem) {
  if (isItemDisabled(item)) return
  emit('action', item.action)
}

function getStrengthAction(item: ImageActionMenuItem): ImageStrengthAction | null {
  return item.strengthAction ?? null
}

function isStrengthActive(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return !!action && showStrengthControl.value && props.strengthControl.activeAction === action
}

function isStrengthHidden(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return showStrengthControl.value && props.strengthControl.activeAction !== action
}

function getItemStrengthValue(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return action ? props.strengthControl.getValue(action) : 0
}

function getItemStrengthRangeStyle(item: ImageActionMenuItem) {
  const strength = getItemStrengthValue(item)
  const progress = Math.max(0, Math.min(100, strength))
  return {
    '--image-action-strength-progress': `${progress}%`,
  }
}

function setItemStrengthRangeRef(
  item: ImageActionMenuItem,
  el: Element | ComponentPublicInstance | null,
) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.setRangeRef(action, el)
}

function setItemStrengthTriggerRef(
  item: ImageActionMenuItem,
  el: Element | ComponentPublicInstance | null,
) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.setTriggerRef(action, el)
}

function onItemStrengthRangePointerDown(item: ImageActionMenuItem, event: PointerEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.onRangePointerDown(action, event)
}

function onItemStrengthInput(item: ImageActionMenuItem, event: Event) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.onInput(action, event)
}

function onItemStrengthChange(item: ImageActionMenuItem, event: Event) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.onChange(action, event)
}

function onItemStrengthPointerUp(item: ImageActionMenuItem, event: PointerEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.onPointerUp(action, event)
}

function onStrengthRangeClick(item: ImageActionMenuItem, event: MouseEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  if (isItemDisabled(item)) return
  props.strengthControl.onClick(action, event)
  emit('action', item.action)
}
</script>

<style scoped>
.image-action-menu {
  --canvas-image-action-price-z-index: 3;
  position: absolute;
  top: calc(100% + 10px);
  left: 50%;
  width: max-content;
  max-width: min(100vw - 32px, 320px);
  transform: translateX(-50%);
  padding: 0;
  border-radius: 0;
  background: transparent;
  border: none;
  backdrop-filter: none;
  box-shadow: none;
  z-index: 30;
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: center;
  gap: var(--node-tool-button-gap);
  box-sizing: border-box;
}

.image-action-menu.strength-mode {
  top: calc(100% + 10px);
  left: 0;
  transform: none;
  width: 100%;
  max-width: none;
  gap: 0;
}

.image-action-menu-inner {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  justify-content: inherit;
  gap: inherit;
  width: 100%;
}

.image-action-control {
  position: relative;
  flex: 0 0 var(--node-tool-button-size);
  width: var(--node-tool-button-size);
  height: var(--node-tool-button-size);
}

.image-action-price {
  position: absolute;
  top: calc(100% + 3px);
  left: 50%;
  z-index: var(--canvas-image-action-price-z-index);
  max-width: 82px;
  transform: translateX(-50%);
  display: none;
  color: var(--canvas-text-muted);
  font-size: 10px;
  font-weight: 600;
  line-height: 14px;
  white-space: nowrap;
}

@media (hover: none), (pointer: coarse) {
  .image-action-price {
    display: inline-flex;
  }
}

.image-action-menu:not(.strength-mode) .image-action-menu-inner {
  padding-bottom: 18px;
}

.image-action-control.is-strength-active {
  flex: 1 1 100%;
  width: 100%;
}

.image-action-control.is-strength-hidden {
  display: none;
}

.image-action-icon-btn {
  flex: 0 0 var(--node-tool-button-size);
  --node-tool-icon-button-size: var(--node-tool-button-size);
  --node-tool-icon-button-radius: var(--node-tool-button-radius);
  --node-tool-icon-button-border: var(--node-tool-button-border);
  --node-tool-icon-button-color: #6f61c6;
  --node-tool-icon-button-active-border: #7a6bd5;
  --node-tool-icon-button-active-color: #7a6bd5;
  --node-tool-icon-button-active-bg: #ffffff;
  position: relative;
  overflow: visible;
}

.image-action-icon-btn:hover:not(:disabled) {
  border-color: #7a6bd5;
  background: #ffffff;
  color: #7a6bd5;
  transform: scale(1.06);
}

.image-action-icon-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.image-action-strength-range {
  --canvas-range-track-height: 6px;
  --canvas-range-thumb-size: 14px;
  --canvas-range-thumb-offset: -4px;
  --canvas-range-fill-color: var(--image-action-strength-fill);
  --image-action-strength-fill: #7a6bd5;
  --canvas-range-track-background: rgba(122, 107, 213, 0.2);
  position: absolute;
  inset: 0;
  z-index: 2;
  opacity: 0;
  pointer-events: none;
}

.image-action-strength-range::-webkit-slider-runnable-track {
  height: var(--canvas-range-track-height);
  border: 0;
  border-radius: 999px;
  background: linear-gradient(
    90deg,
    var(--image-action-strength-fill) 0%,
    var(--image-action-strength-fill) var(--image-action-strength-progress, 0%),
    var(--canvas-range-track-background) var(--image-action-strength-progress, 0%),
    var(--canvas-range-track-background) 100%
  );
}

.image-action-strength-range::-webkit-slider-thumb {
  appearance: none;
  -webkit-appearance: none;
  width: 0;
  height: 0;
  margin: 0;
  border: 0;
  opacity: 0;
  background: transparent;
  box-shadow: none;
}

.image-action-strength-range::-moz-range-track {
  height: var(--canvas-range-track-height);
  border: 0;
  border-radius: 999px;
  background: var(--canvas-range-track-background);
}

.image-action-strength-range::-moz-range-progress {
  height: var(--canvas-range-track-height);
  border: 0;
  border-radius: 999px;
  background: var(--image-action-strength-fill);
}

.image-action-strength-range::-moz-range-thumb {
  width: 0;
  height: 0;
  border: 0;
  opacity: 0;
  background: transparent;
  box-shadow: none;
}

.image-action-strength-range:focus-visible {
  outline: none;
}

.image-action-control.is-strength-active .image-action-icon-btn {
  display: none;
}

.image-action-control.is-strength-active .image-action-strength-range {
  position: relative;
  opacity: 1;
  pointer-events: auto;
}
</style>
