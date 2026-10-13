<template>
  <PopoverRoot :open="isOpen" @update:open="handleOpenUpdate">
    <PopoverAnchor v-if="anchorEl" :reference="anchorEl" />
    <PopoverPortal>
      <PopoverContent
        class="z-admin-dialog w-[288px] max-w-[calc(100vw_-_48px)] rounded-panel border border-border bg-card p-3 text-foreground shadow-admin-popover outline-none data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2"
        :side-offset="6"
        align="start"
      >
        <div class="flex flex-col gap-2">
          <!-- 头部：标题与智能识别复位 -->
          <div class="flex items-center justify-between">
            <span class="text-admin-caption font-bold text-foreground"> 选择品牌 Logo </span>
            <button
              type="button"
              class="cursor-pointer border-0 bg-transparent p-0 text-admin-caption font-semibold text-primary hover:underline"
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
              class="shrink-0 cursor-pointer rounded-pill px-2 py-0.5 text-admin-caption font-medium transition-colors"
              :class="
                currentCategory === cat.key
                  ? 'bg-primary text-primary-foreground font-semibold shadow-xs'
                  : 'bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground'
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
              type="button"
              class="control-state flex size-9 items-center justify-center rounded-control border border-border bg-card p-1.5 cursor-pointer transition-colors hover:border-primary hover:bg-card data-[control-state=selected]:border-primary data-[control-state=selected]:bg-card"
              :data-control-state="selectedKey === item.key ? 'selected' : 'idle'"
              :aria-label="item.label"
              :title="item.label"
              :disabled="isSaving"
              @click="selectLogo(item.key)"
            >
              <img
                v-if="!failedLogoKeys.has(item.key)"
                :src="item.logoSrc"
                :alt="item.label"
                class="size-full object-contain"
                loading="lazy"
                @error="markLogoFailed(item.key)"
              />
              <Cube v-else class="size-5 text-muted-foreground" />
            </button>
          </div>
        </div>
      </PopoverContent>
    </PopoverPortal>
  </PopoverRoot>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { PopoverAnchor, PopoverContent, PopoverPortal, PopoverRoot } from 'reka-ui'
import { Cube } from '@iconoir/vue'
import {
  ADMIN_BRAND_CATEGORIES,
  ALL_ADMIN_BRAND_LOGOS,
  detectAdminBrandLogoKey,
  getAdminBrandLogoOverride,
  type AdminBrandLogoKey,
  type AdminBrandLogoScope,
  type AdminBrandCategoryKey,
} from '../../session/brand/brandLogos'
// Bridge retired early (P4→NOW convergence): canonical session id.
import { useAdminBrandLogoResource } from '../../session/brand/brandLogoResource'
import { useAdminToast } from '../../session/feedback/toast'

const props = withDefaults(
  defineProps<{
    scope: AdminBrandLogoScope
    targetId?: number | string | undefined
    targetName?: string | undefined
    targetBaseUrl?: string | undefined
    targetApiType?: string | undefined
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

const toast = useAdminToast()
const logoResource = useAdminBrandLogoResource()
const isOpen = ref(false)
const anchorEl = ref<HTMLElement | null>(null)

const currentCategory = ref<AdminBrandCategoryKey>(props.initialCategory)
const isSaving = ref(false)

// 品牌缩略图经后端透传 models.dev SVG：加载失败回落 Cube 占位，避免破图。
const failedLogoKeys = ref<Set<AdminBrandLogoKey>>(new Set())

function markLogoFailed(key: AdminBrandLogoKey): void {
  failedLogoKeys.value.add(key)
}

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
  if (isOpen.value) {
    hide()
    return
  }
  const target = (event.currentTarget || event.target) as HTMLElement | null
  anchorEl.value = target
  isOpen.value = true
}

function hide() {
  isOpen.value = false
  emit('hide')
}

function handleOpenUpdate(value: boolean) {
  isOpen.value = value
  if (!value) {
    emit('hide')
  }
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
    hide()
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存品牌 Logo 配置失败')
  } finally {
    isSaving.value = false
  }
}

defineExpose({
  toggle,
  hide,
})
</script>
