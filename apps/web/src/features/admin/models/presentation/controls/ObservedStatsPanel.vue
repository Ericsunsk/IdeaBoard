<template>
  <section
    aria-label="模型观测数据"
    class="mt-3 rounded-lg border border-admin-border-soft bg-admin-surface-muted/40 px-3 py-2.5"
  >
    <div class="mb-2 flex items-center justify-between">
      <h4 class="text-[11px] font-semibold text-admin-text-1">近 7 天观测</h4>
      <span v-if="hasData" class="font-mono text-[10px] text-admin-text-3">
        {{ observed!.windowDays }} 天窗口
      </span>
    </div>

    <p v-if="loading" class="text-[11px] text-admin-text-2">正在加载观测数据...</p>

    <!-- 无观测优雅缺省：静默模型 / 接口缺席一律暂无数据，不误报 -->
    <p v-else-if="!hasData" class="text-[11px] leading-relaxed text-admin-text-3">
      暂无 7 天观测数据（该模型近期无终结调用或尚未接入观测）。
    </p>

    <div v-else class="flex flex-col gap-1.5 text-xs">
      <div class="grid grid-cols-2 gap-1.5">
        <div class="rounded-md bg-white/70 px-2.5 py-1.5">
          <div class="text-[10px] text-admin-text-3">成功率</div>
          <div class="font-semibold text-admin-text-1">
            {{ successText || '暂无数据' }}
          </div>
        </div>
        <div class="rounded-md bg-white/70 px-2.5 py-1.5">
          <div class="text-[10px] text-admin-text-3">Cache 命中率</div>
          <div class="font-semibold text-admin-text-1">
            {{ cacheHitText || '暂无数据' }}
          </div>
        </div>
      </div>

      <div
        class="flex items-center justify-between gap-2 rounded-md bg-white/70 px-2.5 py-1.5"
      >
        <span class="shrink-0 text-[10px] text-admin-text-3">头号失败</span>
        <span class="truncate text-right font-medium text-admin-text-1">
          {{ failureText || '暂无数据' }}
        </span>
      </div>

      <div class="rounded-md bg-white/70 px-2.5 py-1.5">
        <div class="mb-1 text-[10px] text-admin-text-3">SKU 状态</div>
        <div v-if="observed!.skuMargins.length === 0" class="font-medium text-admin-text-3">
          暂无数据
        </div>
        <div v-else class="flex flex-wrap gap-1">
          <span
            v-for="margin in observed!.skuMargins"
            :key="margin.sku"
            class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium"
            :class="
              margin.onSale
                ? 'bg-emerald-50 text-emerald-700'
                : 'bg-admin-surface-muted text-admin-text-3'
            "
            :title="margin.feature"
          >
            {{ margin.displayName || margin.sku }}
            {{ margin.onSale ? '· 在售' : '· 非在售' }}
          </span>
        </div>
      </div>

      <!-- 成本/毛利恒 null：网关侧暂无成本来源，后端 cost_source=unavailable；
           只展示暂无数据，绝不编造数字。cost 有来源后负值标红可直接扩展。 -->
      <div
        class="flex items-center justify-between gap-2 rounded-md bg-white/70 px-2.5 py-1.5"
        title="网关实际成本暂无来源（cost_source=unavailable），毛利未知"
      >
        <span class="shrink-0 text-[10px] text-admin-text-3">成本 / 毛利</span>
        <span class="font-medium text-admin-text-3">暂无数据</span>
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
