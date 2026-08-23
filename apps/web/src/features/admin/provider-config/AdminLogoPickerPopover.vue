<template>
  <Popover ref="popoverRef" :pt="popoverPt" @hide="emit('hide')">
    <div class="flex w-[288px] flex-col gap-2 p-3">
      <!-- 头部：标题与智能识别复位 -->
      <div class="flex items-center justify-between">
        <span class="text-admin-caption font-bold text-admin-title-color"> 选择品牌 Logo </span>
        <button
          type="button"
          class="cursor-pointer border-0 bg-transparent p-0 text-[11px] font-semibold text-admin-accent hover:underline"
          :disabled="isSaving"
          @click="selectLogo('auto')"
        >
          智能识别
        </button>
      </div>

      <!-- 分类筛选药丸栏 (Category Pills) -->
      <div
        class="flex items-center gap-1 overflow-x-auto pb-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        <button
          v-for="cat in ADMIN_BRAND_CATEGORIES"
          :key="cat.key"
          type="button"
          class="shrink-0 cursor-pointer rounded-full px-2 py-0.5 text-[11px] font-medium transition-colors"
          :class="
            currentCategory === cat.key
              ? 'bg-admin-accent text-admin-text-inverse font-semibold shadow-xs'
              : 'bg-admin-surface-muted text-admin-text-2 hover:bg-admin-border-soft hover:text-admin-title-color'
          "
          @click="currentCategory = cat.key"
        >
          {{ cat.label }}
        </button>
      </div>

      <!-- 品牌 Logo 6 列自适应网格 -->
      <div
        class="grid max-h-[250px] grid-cols-6 content-start gap-1.5 overflow-y-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        <button
          v-for="item in filteredLogos"
          :key="item.key"
          v-tooltip.top="item.label"
          type="button"
          class="ib-control-state flex size-9 items-center justify-center rounded-lg border border-admin-border-soft bg-white p-1.5 cursor-pointer transition-colors hover:border-admin-accent hover:bg-white data-[control-state=selected]:border-admin-accent data-[control-state=selected]:bg-white"
          :data-control-state="selectedKey === item.key ? 'selected' : 'idle'"
          :aria-label="item.label"
          :disabled="isSaving"
          @click="selectLogo(item.key)"
        >
          <img :src="item.logoSrc" :alt="item.label" class="size-full object-contain" />
        </button>
      </div>
    </div>
  </Popover>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Popover from 'primevue/popover'
import Tooltip from 'primevue/tooltip'
import { createAdminPopoverPt } from '../shared/lib/adminOverlayPt'
import {
  ADMIN_BRAND_CATEGORIES,
  ALL_ADMIN_BRAND_LOGOS,
  detectAdminBrandLogoKey,
  getAdminBrandLogoOverride,
  type AdminBrandLogoKey,
  type AdminBrandLogoScope,
  type AdminBrandCategoryKey,
} from '../shared/logos/adminBrandLogos'
import { useAdminBrandLogoResource } from '../shared/logos/adminBrandLogoResource'
import { useAdminToast } from '../shared/lib/adminToast'

const props = withDefaults(
  defineProps<{
    scope: AdminBrandLogoScope
    targetId?: number | string
    targetName?: string
    targetBaseUrl?: string
    targetApiType?: string
    initialCategory?: AdminBrandCategoryKey
  }>(),
  {
    targetId: undefined,
    targetName: undefined,
    targetBaseUrl: undefined,
    targetApiType: undefined,
    initialCategory: 'all',
  },
)

const emit = defineEmits<{
  hide: []
}>()

const vTooltip = Tooltip
const toast = useAdminToast()
const logoResource = useAdminBrandLogoResource()
const popoverRef = ref<InstanceType<typeof Popover> | null>(null)
const popoverPt = createAdminPopoverPt({ contentClass: '!p-0' })

const currentCategory = ref<AdminBrandCategoryKey>(props.initialCategory)
const isSaving = ref(false)

watch(
  () => props.initialCategory,
  (val) => {
    if (val) currentCategory.value = val
  },
)

const filteredLogos = computed(() => {
  if (currentCategory.value === 'all') return ALL_ADMIN_BRAND_LOGOS
  return ALL_ADMIN_BRAND_LOGOS.filter((item) => item.category === currentCategory.value)
})

const selectedKey = computed(() => {
  if (props.targetId !== undefined) {
    const override = getAdminBrandLogoOverride(props.scope, props.targetId)
    if (override) return override
  }
  return detectAdminBrandLogoKey({
    name: props.targetName,
    baseUrl: props.targetBaseUrl,
    apiType: props.targetApiType,
  })
})

function toggle(event: Event) {
  popoverRef.value?.toggle(event)
}

async function selectLogo(key: AdminBrandLogoKey | 'auto') {
  if (props.targetId === undefined || isSaving.value) return
  isSaving.value = true
  try {
    if (key === 'auto') {
      await logoResource.remove(props.scope, props.targetId)
    } else {
      await logoResource.save(props.scope, props.targetId, key)
    }
    popoverRef.value?.hide()
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存品牌 Logo 配置失败')
  } finally {
    isSaving.value = false
  }
}

defineExpose({
  toggle,
  hide: () => popoverRef.value?.hide(),
})
</script>
