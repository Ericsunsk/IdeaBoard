<template>
  <div
    class="image-action-menu node-tool-surface"
    :class="{ 'strength-mode': shouldShowStrengthControl }"
    :style="shouldShowStrengthControl ? strengthMenuStyle : undefined"
    @mousedown.stop
    @click.stop
  >
    <Transition v-bind="actionMenuMotion" appear>
      <div class="image-action-menu-inner">
        <div
          v-for="item in actionMenuItems"
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
            :aria-label="getItemTooltip(item)"
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
            :min="getItemStrengthConfig(item)?.min"
            :max="getItemStrengthConfig(item)?.max"
            :step="getItemStrengthConfig(item)?.step"
            :value="getItemStrengthValue(item)"
            :style="getItemStrengthRangeStyle(item)"
            tabindex="-1"
            :aria-hidden="isStrengthActive(item) ? undefined : 'true'"
            :aria-label="t('canvas.controls.strengthLabel', { label: item.label })"
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
import { computed, watch, type ComponentPublicInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import { SERVICE_SLUGS, SERVICE, type Service } from '../../../../../core/contracts/imageServices'
import { formatIdeaAmount } from '../../../../../core/i18n/userIdeaCurrency'
import {
  getImageActionMenuItems,
  IMAGE_NODE_ACTION,
  type ImageActionMenuItem,
  type ImageNodeAction,
} from '../../../core/nodes/imageActionRegistry.ts'
import {
  isImageTaskAction,
  getImageActionStrengthConfig,
  useCanvasBillingReadinessStore,
  type ImageStrengthAction,
  buildImageActionReadinessRequest,
  buildInpaintReadinessRequest,
} from '../../../session/execution'
import type { ImageActionStrengthControl } from '../../../tools/image-tools'
import type { OutputResolution } from '../../../../../core/contracts/generationContract'
import ImageActionMenuIcon from './ImageActionMenuIcon.vue'
import { NodeToolIconButton } from '../../nodes/primitives'
import { createPopoverMotionTransition } from '../../../../../core/motion/popoverMotion'

const props = defineProps<{
  strengthMenuStyle?: string | Record<string, string>
  outputCount: number
  outputResolution?: OutputResolution | undefined
  strengthControl: ImageActionStrengthControl
}>()

const emit = defineEmits<{
  (event: 'action', action: ImageNodeAction, strength?: number): void
}>()

const { t } = useI18n()

const actionMenuItems = computed(() => getImageActionMenuItems((key) => t(key)))
const shouldShowStrengthControl = computed(() => props.strengthControl.isVisible)
const billingStore = useCanvasBillingReadinessStore()
const actionMenuMotion = createPopoverMotionTransition()
const outputResolution = computed(() => props.outputResolution ?? '1k')

function resolveItemBillingService(item: ImageActionMenuItem): Service | null {
  if (item.action === IMAGE_NODE_ACTION.inpaint) return SERVICE.inpaint
  return SERVICE_SLUGS.includes(item.action as Service) ? (item.action as Service) : null
}

function buildItemReadinessRequest(item: ImageActionMenuItem) {
  const options = {
    outputCount: props.outputCount,
    outputResolution: outputResolution.value,
  }
  if (item.action === IMAGE_NODE_ACTION.inpaint) {
    return buildInpaintReadinessRequest()
  }
  if (item.action === IMAGE_NODE_ACTION.outpaint || isImageTaskAction(item.action)) {
    return buildImageActionReadinessRequest(item.action, options)
  }
  return undefined
}

function refreshBillingQuotes() {
  if (!billingStore) return
  const entries: Array<{
    service: Service
    request: NonNullable<ReturnType<typeof buildItemReadinessRequest>>
  }> = actionMenuItems.value.flatMap((item) => {
    const service = resolveItemBillingService(item)
    const request = buildItemReadinessRequest(item)
    return service && request ? [{ service, request }] : []
  })
  if (entries.length === 0) return
  void billingStore.ensureMatrix(entries)
}

watch([() => props.outputCount, outputResolution], refreshBillingQuotes, { immediate: true })

function getItemBillingState(item: ImageActionMenuItem) {
  const service = resolveItemBillingService(item)
  return service && billingStore
    ? billingStore.getServiceState(service, buildItemReadinessRequest(item))
    : null
}

function getItemTooltip(item: ImageActionMenuItem) {
  const billing = getItemBillingState(item)
  if (!billing) return item.label
  if (billing.status === 'ready') {
    return `${item.label} · ${formatQuote(billing.totalCredits ?? (billing.unitCredits ?? 0) * props.outputCount)}`
  }
  return item.label
}

function formatQuote(totalCredits: number) {
  return totalCredits === 0 ? t('canvas.controls.free') : formatIdeaAmount(totalCredits)
}

function getItemTouchPrice(item: ImageActionMenuItem) {
  const billing = getItemBillingState(item)
  return billing?.status === 'ready'
    ? formatQuote(billing.totalCredits ?? (billing.unitCredits ?? 0) * props.outputCount)
    : ''
}

function onActionClick(item: ImageActionMenuItem) {
  emit('action', item.action)
}

function getStrengthAction(item: ImageActionMenuItem): ImageStrengthAction | null {
  return item.strengthAction ?? null
}

function isStrengthActive(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return (
    !!action && shouldShowStrengthControl.value && props.strengthControl.activeAction === action
  )
}

function isStrengthHidden(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return shouldShowStrengthControl.value && props.strengthControl.activeAction !== action
}

function getItemStrengthValue(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return action ? props.strengthControl.getValue(action) : 0
}

function getItemStrengthConfig(item: ImageActionMenuItem) {
  const action = getStrengthAction(item)
  return action ? getImageActionStrengthConfig(action) : null
}

function getItemStrengthRangeStyle(item: ImageActionMenuItem) {
  const strength = getItemStrengthValue(item)
  const config = getItemStrengthConfig(item)
  const progress = config
    ? ((Math.max(config.min, Math.min(config.max, strength)) - config.min) /
        (config.max - config.min)) *
      100
    : 0
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
  const strength = Number((event.currentTarget as HTMLInputElement).value)
  props.strengthControl.onChange(action, event)
  emit('action', item.action, strength)
}

function onItemStrengthPointerUp(item: ImageActionMenuItem, event: PointerEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.onPointerUp(action, event)
}

function onStrengthRangeClick(item: ImageActionMenuItem, event: MouseEvent) {
  const action = getStrengthAction(item)
  if (!action) return
  props.strengthControl.onClick(action, event)
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
  font-size: var(--canvas-font-size-micro);
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
  position: relative;
}

.image-action-strength-range {
  --canvas-range-track-height: 6px;
  --canvas-range-thumb-size: 14px;
  --canvas-range-thumb-offset: -4px;
  --canvas-range-fill-color: var(--image-action-strength-fill);
  --image-action-strength-fill: var(--canvas-accent-hover);
  --canvas-range-track-background: var(--canvas-accent-outline);
  position: absolute;
  inset: 0;
  z-index: 2;
  opacity: 0;
  pointer-events: none;
}

.image-action-strength-range::-webkit-slider-runnable-track {
  height: var(--canvas-range-track-height);
  border: 0;
  border-radius: var(--radius-pill);
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
  border-radius: var(--radius-pill);
  background: var(--canvas-range-track-background);
}

.image-action-strength-range::-moz-range-progress {
  height: var(--canvas-range-track-height);
  border: 0;
  border-radius: var(--radius-pill);
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
