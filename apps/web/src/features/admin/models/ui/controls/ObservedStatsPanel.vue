<template>
  <section
    :aria-label="MODELS_MESSAGES.observedAria"
    class="mt-3 rounded-panel border border-border bg-muted/40 px-3 py-2.5"
  >
    <div class="mb-2 flex items-center justify-between">
      <h4 class="text-admin-caption font-semibold text-foreground">
        {{ MODELS_MESSAGES.observedTitle }}
      </h4>
      <span v-if="hasData" class="font-mono text-admin-micro text-muted-foreground">
        {{ MODELS_MESSAGES.observedWindow(observed!.windowDays) }}
      </span>
    </div>

    <p v-if="loading" class="text-admin-caption text-muted-foreground">
      {{ MODELS_MESSAGES.observedLoading }}
    </p>

    <!-- 无观测优雅缺省：静默模型 / 接口缺席一律暂无数据，不误报 -->
    <p v-else-if="!hasData" class="text-admin-caption leading-relaxed text-muted-foreground">
      {{ MODELS_MESSAGES.observedEmpty }}
    </p>

    <div v-else class="flex flex-col gap-1.5 text-xs">
      <div class="grid grid-cols-2 gap-1.5">
        <div class="rounded-control bg-card/70 px-2.5 py-1.5">
          <div class="text-admin-micro text-muted-foreground">
            {{ MODELS_MESSAGES.observedSuccessLabel }}
          </div>
          <div class="font-semibold text-foreground">
            {{ successText || MODELS_MESSAGES.referenceNoData }}
          </div>
        </div>
        <div class="rounded-control bg-card/70 px-2.5 py-1.5">
          <div class="text-admin-micro text-muted-foreground">
            {{ MODELS_MESSAGES.observedCacheLabel }}
          </div>
          <div class="font-semibold text-foreground">
            {{ cacheHitText || MODELS_MESSAGES.referenceNoData }}
          </div>
        </div>
      </div>

      <div class="flex items-center justify-between gap-2 rounded-control bg-card/70 px-2.5 py-1.5">
        <span class="shrink-0 text-admin-micro text-muted-foreground">{{
          MODELS_MESSAGES.observedTopFailureLabel
        }}</span>
        <span class="truncate text-right font-medium text-foreground">
          {{ failureText || MODELS_MESSAGES.referenceNoData }}
        </span>
      </div>

      <div class="rounded-control bg-card/70 px-2.5 py-1.5">
        <div class="mb-1 text-admin-micro text-muted-foreground">
          {{ MODELS_MESSAGES.observedSkuLabel }}
        </div>
        <div v-if="observed!.skuMargins.length === 0" class="font-medium text-muted-foreground">
          {{ MODELS_MESSAGES.referenceNoData }}
        </div>
        <div v-else class="flex flex-wrap gap-1">
          <!-- 在售 SKU 状态收口 StatusBadge：onSale→success，非在售→neutral；
               静态属性不呼吸（pulse=false），文案保持“名称 · 在售/非在售”。 -->
          <StatusBadge
            v-for="margin in observed!.skuMargins"
            :key="margin.sku"
            :label="`${margin.displayName || margin.sku} ${MODELS_MESSAGES.observedSaleState(margin.onSale)}`"
            :tone="margin.onSale ? 'success' : 'neutral'"
            :pulse="false"
            size="small"
            :title="margin.service"
          />
        </div>
      </div>

      <!-- 成本/毛利恒 null：网关侧暂无成本来源，后端 cost_source=unavailable；
           只展示暂无数据，绝不编造数字。cost 有来源后负值标红可直接扩展。 -->
      <div
        class="flex items-center justify-between gap-2 rounded-control bg-card/70 px-2.5 py-1.5"
        :title="MODELS_MESSAGES.observedCostTitle"
      >
        <span class="shrink-0 text-admin-micro text-muted-foreground">{{
          MODELS_MESSAGES.observedCostLabel
        }}</span>
        <span class="font-medium text-muted-foreground">{{ MODELS_MESSAGES.referenceNoData }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { StatusBadge } from '../../../shared'
import {
  formatObservedFailure,
  formatObservedRate,
  type ModelObservedSnapshot,
} from '../../core/catalogData'
import { MODELS_MESSAGES } from '../pages/messages'

const props = withDefaults(
  defineProps<{
    observed?: ModelObservedSnapshot | null | undefined
    loading?: boolean | undefined
  }>(),
  {
    observed: null,
    loading: false,
  },
)

// 有观测才展数据区：映射但静默的模型（has_observations=false）走缺省文案。
const hasData = computed(() => Boolean(props.observed?.hasObservations))

const successText = computed(() =>
  props.observed ? formatObservedRate(props.observed.successRate) : '',
)

const cacheHitText = computed(() =>
  props.observed ? formatObservedRate(props.observed.cacheHitRate) : '',
)

const failureText = computed(() => (props.observed ? formatObservedFailure(props.observed) : ''))
</script>
