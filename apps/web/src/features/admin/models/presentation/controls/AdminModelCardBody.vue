<template>
  <div class="flex flex-1 flex-col p-4">
    <!-- 头部：品牌图标 + 模型名称 -->
    <div class="flex items-center min-w-0">
      <div
        class="flex size-7 shrink-0 items-center justify-center rounded-lg mr-2.5 overflow-hidden transition-colors"
        :class="
          brandLogo
            ? 'bg-admin-surface-muted/60 border border-admin-border-soft/60'
            : 'bg-admin-accent/10 text-admin-accent'
        "
      >
        <img
          v-if="brandLogo"
          :src="brandLogo"
          :alt="model.provider"
          class="size-4.5 object-contain"
          loading="lazy"
        />
        <Cube v-else class="size-4" />
      </div>
      <div
        class="text-base font-semibold text-admin-title-color line-clamp-1 flex-1 transition-colors"
        :class="hoverable ? 'group-hover:text-admin-accent' : ''"
      >
        {{ model.name }}
      </div>
    </div>

    <!-- 第二行：提供商名 + mode 胶囊（社区原样，无站内分类） -->
    <div class="mt-2.5 flex items-center gap-2 text-xs">
      <span class="font-semibold text-admin-text-1">{{ model.provider }}</span>
      <span
        v-if="model.mode"
        class="inline-flex items-center rounded-full border border-admin-border-soft bg-admin-surface-muted/60 px-2.5 py-0.5 text-[11px] font-medium text-admin-text-2"
      >
        {{ model.mode }}
      </span>
    </div>

    <!-- 第三行：模型功能描述 (两行截断) -->
    <p class="mt-2 text-xs leading-relaxed text-admin-text-3 line-clamp-2">
      {{ model.description }}
    </p>

    <!-- 第四行：元数据与能力标签 (上下文/分辨率 + 特性 + 托管形式) -->
    <div class="mt-3.5 flex flex-wrap items-center gap-1.5 pt-1">
      <!-- 上下文 / 分辨率胶囊 (有值才展示，避免空灰色长条) -->
      <span
        v-if="model.contextOrResolution"
        class="inline-flex items-center rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-700"
      >
        {{ model.contextOrResolution }}
      </span>

      <!-- 特性标签 -->
      <span
        v-for="cap in model.capabilities"
        :key="cap"
        class="inline-flex items-center rounded-md bg-admin-surface-muted/80 px-2 py-0.5 text-[11px] font-medium text-admin-text-2"
      >
        {{ cap }}
      </span>

      <!-- 社区参考价胶囊 (USD/1M tokens，有价才展示，不进计费) -->
      <span
        v-if="pricingText"
        class="inline-flex items-center rounded-md bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700"
        :title="pricingTitle"
      >
        {{ pricingText }}
      </span>

      <!-- prompt cache 徽标 (支持才展示) -->
      <span
        v-if="model.supportsPromptCaching"
        class="inline-flex items-center rounded-md bg-sky-50 px-2 py-0.5 text-[11px] font-medium text-sky-700"
        :title="cacheTitle || '支持 prompt caching'"
      >
        cache √
      </span>

      <!-- 托管类型标签 (Cloudflare 托管使用翡翠绿，第三方使用浅灰) -->
      <span
        class="inline-flex items-center rounded-md px-2 py-0.5 text-[11px] font-medium"
        :class="
          model.hosting === MODEL_HOSTING_CLOUDFLARE
            ? 'bg-emerald-50 text-emerald-700'
            : 'bg-slate-100 text-slate-600'
        "
      >
        {{ model.hosting }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Cube } from '@iconoir/vue'
import { resolveModelBrandLogo } from '../../../shared/logos/brandLogos'
import {
  formatCachePricing,
  formatModelPricing,
  MODEL_HOSTING_CLOUDFLARE,
} from '../../domain/catalogData'
import type { AdminModelItem } from '../../domain/catalogData'

const props = withDefaults(
  defineProps<{
    model: AdminModelItem
    hoverable?: boolean | undefined
  }>(),
  {
    hoverable: false,
  },
)

// 模板多处使用：一次解析，多处复用。
const brandLogo = computed(() => resolveModelBrandLogo(props.model))
const pricingText = computed(() => formatModelPricing(props.model))
const cacheText = computed(() => formatCachePricing(props.model))
// hover 展示完整分解（含缓存读写价），胶囊只放 in/out 主价。
const pricingTitle = computed(() => {
  const base = `社区参考价（USD/1M tokens，不计费）：${pricingText.value}`
  return cacheText.value ? `${base}；${cacheText.value}` : base
})
const cacheTitle = computed(() => (cacheText.value ? `${cacheText.value} /1M` : ''))
</script>
