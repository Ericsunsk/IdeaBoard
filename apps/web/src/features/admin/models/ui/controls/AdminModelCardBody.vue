<template>
  <div class="flex flex-1 flex-col p-4">
    <!-- 头部：品牌图标 + 模型名称 -->
    <div class="flex items-center min-w-0">
      <div
        class="flex size-7 shrink-0 items-center justify-center rounded-control mr-2.5 overflow-hidden transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
        :class="brandLogo ? 'bg-muted/60 border border-border/60' : 'bg-primary-faint text-primary'"
      >
        <img
          v-if="brandLogo && !logoLoadFailed"
          :src="brandLogo"
          :alt="model.provider"
          class="size-4.5 object-contain"
          loading="lazy"
          @error="onLogoError"
        />
        <Cube v-else class="size-4" />
      </div>
      <div
        class="text-base font-semibold text-foreground line-clamp-1 flex-1 transition-colors duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
        :class="hoverable ? 'group-hover:text-primary' : ''"
      >
        {{ model.name }}
      </div>
    </div>

    <!-- 第二行：提供商名 + mode 胶囊（社区原样，无站内分类） -->
    <div class="mt-2.5 flex items-center gap-2 text-xs">
      <span class="font-semibold text-foreground">{{ model.provider }}</span>
      <span
        v-if="model.mode"
        class="inline-flex items-center rounded-pill border border-border bg-muted/60 px-2.5 py-0.5 text-admin-caption font-medium text-muted-foreground"
      >
        {{ model.mode }}
      </span>
    </div>

    <!-- 第三行：模型功能描述 (两行截断) -->
    <p class="mt-2 text-xs leading-relaxed text-muted-foreground line-clamp-2">
      {{ model.description }}
    </p>

    <!-- 第四行：元数据与能力标签 (上下文/分辨率 + 特性 + 托管形式) -->
    <div class="mt-3.5 flex flex-wrap items-center gap-1.5 pt-1">
      <!-- 上下文 / 分辨率胶囊 (有值才展示，避免空灰色长条) -->
      <span
        v-if="model.contextOrResolution"
        class="inline-flex items-center rounded-control bg-muted/80 px-2 py-0.5 text-admin-caption font-medium text-muted-foreground"
      >
        {{ model.contextOrResolution }}
      </span>

      <!-- 特性标签 -->
      <span
        v-for="cap in model.capabilities"
        :key="cap"
        class="inline-flex items-center rounded-control bg-muted/80 px-2 py-0.5 text-admin-caption font-medium text-muted-foreground"
      >
        {{ cap }}
      </span>

      <!-- prompt cache 徽标 (支持才展示；目录仅保留能力位，价格走参照接口) -->
      <span
        v-if="model.supportsPromptCaching"
        class="inline-flex items-center rounded-control bg-info-soft px-2 py-0.5 text-admin-caption font-medium text-info"
        :title="MODELS_MESSAGES.cardCacheTitle"
      >
        cache √
      </span>

      <!-- 托管类型标签 (Cloudflare 托管使用翡翠绿，第三方使用浅灰) -->
      <span
        class="inline-flex items-center rounded-control px-2 py-0.5 text-admin-caption font-medium"
        :class="
          model.hosting === MODEL_HOSTING_CLOUDFLARE
            ? 'bg-success-soft text-success'
            : 'bg-muted/80 text-muted-foreground'
        "
      >
        {{ model.hosting }}
      </span>
    </div>

    <!-- 观测区：近 7 天成功率/失败分类/cache 命中率/在售 SKU；无观测优雅缺省 -->
    <ObservedStatsPanel :observed="observed" :loading="observedLoading" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Cube } from '@iconoir/vue'
import { resolveModelBrandLogo } from '../../../shared'
import { MODEL_HOSTING_CLOUDFLARE } from '../../core/catalogData'
import type { AdminModelItem, ModelObservedSnapshot } from '../../core/catalogData'
import { MODELS_MESSAGES } from '../pages/messages'
import ObservedStatsPanel from './ObservedStatsPanel.vue'

const props = withDefaults(
  defineProps<{
    model: AdminModelItem
    hoverable?: boolean | undefined
    observed?: ModelObservedSnapshot | null | undefined
    observedLoading?: boolean | undefined
  }>(),
  {
    hoverable: false,
    observed: null,
    observedLoading: false,
  },
)

// 模板多处使用：一次解析，多处复用。
const brandLogo = computed(() => resolveModelBrandLogo(props.model))

// 品牌 logo：站内现成资产直展，AI provider 无外部图时回落 Cube 占位；
// 模型切换后重置失败态。
const logoLoadFailed = ref(false)
watch(brandLogo, () => {
  logoLoadFailed.value = false
})

function onLogoError(): void {
  logoLoadFailed.value = true
}
</script>
