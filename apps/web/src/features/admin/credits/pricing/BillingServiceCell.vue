<template>
  <div class="flex min-w-0 items-center gap-1.5 font-mono text-xs text-admin-text-1">
    <component :is="icon" v-if="icon" class="size-4 shrink-0 text-admin-text-2" />
    <div class="flex min-w-0 flex-col">
      <span class="truncate text-slate-700">{{ serviceSlug }}</span>
      <span class="truncate text-admin-caption text-admin-text-3">{{ nodeTitle }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Search } from '@iconoir/vue'
import { getWorkflowFeatureIcon } from '../../../../shared/domain/workflowFeatureIcons'
import {
  getAdminWorkflowFeatureGroupMeta,
  getAdminWorkflowFeatureMeta,
} from '../../shared/lib/workflowFeatureAdminMeta'
import { featureOfSku } from './skuFeature'
import { pricingMappingFeature } from './groups'

// 对齐任务监控页 Service 列：图标 + slug；次标题为对应节点名（不带“节点”后缀）。
// 无 workflow 元信息的行：主文案去 namespace 前缀（如 vision），图标按
// namespace 补（ai-search 用搜索图标），次标题回退 namespace。
const props = defineProps<{
  sku: string
  namespace: string
}>()

const mappingFeature = computed(() => pricingMappingFeature(props.sku))
const serviceSlug = computed(() => featureOfSku(props.sku) ?? mappingFeature.value ?? props.sku)
const icon = computed(
  () =>
    getWorkflowFeatureIcon(serviceSlug.value) ??
    (props.namespace === 'ai-search' ? Search : undefined),
)
const nodeTitle = computed(() => {
  const featureSlug = featureOfSku(props.sku)
  const feature = featureSlug ? getAdminWorkflowFeatureMeta(featureSlug) : null
  if (feature) return getAdminWorkflowFeatureGroupMeta(feature.groupKey).title
  return props.namespace || props.sku
})
</script>
