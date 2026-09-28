<template>
  <section
    :aria-label="MODELS_MESSAGES.observedAria"
    class="mt-3 rounded-lg border border-admin-border-soft bg-admin-surface-muted/40 px-3 py-2.5"
  >
    <div class="mb-2 flex items-center justify-between">
      <h4 class="text-admin-caption font-semibold text-admin-text-1">{{ MODELS_MESSAGES.observedTitle }}</h4>
      <span v-if="hasData" class="font-mono text-admin-micro text-admin-text-3">
        {{ MODELS_MESSAGES.observedWindow(observed!.windowDays) }}
      </span>
    </div>

    <p v-if="loading" class="text-admin-caption text-admin-text-2">{{ MODELS_MESSAGES.observedLoading }}</p>

    <!-- 无观测优雅缺省：静默模型 / 接口缺席一律暂无数据，不误报 -->
    <p v-else-if="!hasData" class="text-admin-caption leading-relaxed text-admin-text-3">
      {{ MODELS_MESSAGES.observedEmpty }}
    </p>

    <div v-else class="flex flex-col gap-1.5 text-xs">
      <div class="grid grid-cols-2 gap-1.5">
        <div class="rounded-md bg-white/70 px-2.5 py-1.5">
          <div class="text-admin-micro text-admin-text-3">{{ MODELS_MESSAGES.observedSuccessLabel }}</div>
          <div class="font-semibold text-admin-text-1">
            {{ successText || MODELS_MESSAGES.referenceNoData }}
          </div>
        </div>
        <div class="rounded-md bg-white/70 px-2.5 py-1.5">
          <div class="text-admin-micro text-admin-text-3">{{ MODELS_MESSAGES.observedCacheLabel }}</div>
          <div class="font-semibold text-admin-text-1">
            {{ cacheHitText || MODELS_MESSAGES.referenceNoData }}
          </div>
        </div>
      </div>

      <div
        class="flex items-center justify-between gap-2 rounded-md bg-white/70 px-2.5 py-1.5"
      >
        <span class="shrink-0 text-admin-micro text-admin-text-3">{{ MODELS_MESSAGES.observedTopFailureLabel }}</span>
        <span class="truncate text-right font-medium text-admin-text-1">
          {{ failureText || MODELS_MESSAGES.referenceNoData }}
        </span>
      </div>

      <div class="rounded-md bg-white/70 px-2.5 py-1.5">
        <div class="mb-1 text-admin-micro text-admin-text-3">{{ MODELS_MESSAGES.observedSkuLabel }}</div>
        <div v-if="observed!.skuMargins.length === 0" class="font-medium text-admin-text-3">
          {{ MODELS_MESSAGES.referenceNoData }}
        </div>
        <div v-else class="flex flex-wrap gap-1">
          <span
            v-for="margin in observed!.skuMargins"
            :key="margin.sku"
            class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-admin-caption font-medium"
            :class="
              margin.onSale
                ? 'bg-emerald-50 text-emerald-700'
                : 'bg-admin-surface-muted text-admin-text-3'
            "
            :title="margin.feature"
          >
            {{ margin.displayName || margin.sku }}
            {{ MODELS_MESSAGES.observedSaleState(margin.onSale) }}
          </span>
        </div>
      </div>

      <!-- 成本/毛利恒 null：网关侧暂无成本来源，后端 cost_source=unavailable；
           只展示暂无数据，绝不编造数字。cost 有来源后负值标红可直接扩展。 -->
      <div
        class="flex items-center justify-between gap-2 rounded-md bg-white/70 px-2.5 py-1.5"
        :title="MODELS_MESSAGES.observedCostTitle"
      >
        <span class="shrink-0 text-admin-micro text-admin-text-3">{{ MODELS_MESSAGES.observedCostLabel }}</span>
        <span class="font-medium text-admin-text-3">{{ MODELS_MESSAGES.referenceNoData }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  formatObservedFailure,
  formatObservedRate,
  type ModelObservedSnapshot,
} from '../../domain/catalogData'
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
