<template>
  <div
    class="app-scrollbar relative h-full w-full overflow-y-auto bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text"
  >
    <div class="ib-board-page-background ib-page-dot-bg"></div>
    <header class="relative z-[1] flex flex-wrap items-start justify-between gap-x-4">
      <StudioName />
      <UserWorkspaceNavigation />
      <UserAccountMenu
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      />
    </header>

    <!-- 🌟 主视窗容器：当左侧素材库浮岛展开时，右侧自适应推挤并保持几何重居中 -->
    <main
      class="relative z-[1] mx-auto mt-12 max-w-[1400px] ib-main-content-flow"
      :class="{ 'library-open-layout': libraryIslandOpen }"
    >
      <div class="mx-auto flex max-w-[720px] flex-col items-center text-center">
        <div class="mb-7 flex w-full items-center gap-2">
          <form
            class="min-w-0 flex-1 rounded-full border border-board-card-border bg-board-control-bg p-1.5 shadow-2xs"
            @submit.prevent="submitQuery"
          >
            <div class="flex items-center gap-2">
              <Search class="ml-3 size-4 shrink-0 text-board-control-icon" aria-hidden="true" />
              <InputText
                v-model="draftQuery"
                unstyled
                class="min-w-0 flex-1 bg-transparent px-2 text-[14px] text-board-control-input-text outline-none"
                placeholder="Search inspiration..."
                aria-label="搜索灵感"
                @input="queryError = ''"
              />
              <Button
                type="submit"
                label="Search"
                class="!h-9 !rounded-full !border-0 !bg-board-primary !px-5 !text-sm !font-bold !text-ib-text-inverse !shadow-none hover:!bg-board-primary-hover cursor-pointer"
              />
            </div>
          </form>

          <!-- 🌟 核心入口：搜索栏右侧专属【My Library 个人素材库】极简纯净正圆按钮 (支持点击展开与拖拽落盘) -->
          <button
            type="button"
            class="grid size-11 shrink-0 place-items-center rounded-full border border-board-card-border bg-board-control-bg text-board-page-text-muted transition-all duration-200 hover:border-board-primary hover:bg-ib-surface-hover hover:text-board-primary focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] cursor-pointer"
            :class="[
              libraryIslandOpen
                ? '!border-board-primary !bg-board-primary/10 !text-board-primary shadow-xs'
                : '',
              isTriggerHoveredDuringDrag
                ? '!scale-110 !border-board-primary !text-board-primary'
                : '',
            ]"
            aria-label="Toggle My Library"
            title="Toggle My Library / Drop here to Quick Save"
            :aria-expanded="libraryIslandOpen"
            @click="libraryIslandOpen = !libraryIslandOpen"
            @dragover.prevent="handleTriggerBtnDragOver"
            @dragleave="handleTriggerBtnDragLeave"
            @drop.prevent="handleTriggerBtnDrop"
          >
            <Bookmark class="size-4" aria-hidden="true" />
          </button>
        </div>
        <p v-if="queryError" class="mt-2 mb-0 text-sm font-semibold text-board-danger">
          {{ queryError }}
        </p>
      </div>

      <div class="mt-12 flex items-center justify-between gap-3">
        <div>
          <h2 class="m-0 text-[22px] font-semibold">
            {{ submittedQuery ? `Results for “${submittedQuery}”` : 'Inspiration' }}
          </h2>
          <p v-if="!hasError" class="mt-1 mb-0 text-sm text-board-page-text-muted">
            {{ statusText }}
          </p>
        </div>
      </div>

      <section
        v-if="loading && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg"
      >
        <ProgressSpinner
          class="!h-9 !w-9 text-board-primary"
          stroke-width="4"
          aria-label="加载灵感"
        />
      </section>
      <section
        v-else-if="hasError && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg text-center"
      >
        <div>
          <p class="m-0 font-semibold">Unable to load inspiration</p>
          <Button
            label="Retry"
            class="!mt-4 !rounded-full !border-0 !bg-board-primary !text-ib-text-inverse"
            @click="reload"
          />
        </div>
      </section>
      <section
        v-else-if="results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg"
      >
        <p class="m-0 text-sm text-board-page-text-muted">没有找到匹配的灵感</p>
      </section>

      <!-- 🌟 瀑布流容器 (支持自适应 14px 列间距与原生拖拽入库) -->
      <div
        v-else
        class="mt-6 columns-1 gap-4 sm:columns-2 lg:columns-3 xl:columns-4 masonry-grid-flow"
      >
        <article
          v-for="(result, index) in results"
          :key="`${result.provider}:${result.id}`"
          draggable="true"
          class="masonry-card-item group mb-4 break-inside-avoid overflow-hidden rounded-2xl border border-board-card-border bg-board-card-bg shadow-sm transition-all duration-150 hover:shadow-[var(--shadow-ib-panel)] cursor-grab active:cursor-grabbing"
          @dragstart="handleCardDragStart($event, result)"
          @dragend="handleCardDragEnd"
        >
          <button
            type="button"
            class="block w-full cursor-zoom-in text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-board-primary"
            :aria-label="`查看 ${result.title || '灵感图片'}`"
            @click="openLightbox(result)"
          >
            <div
              class="relative overflow-hidden bg-board-loading-bg"
              :style="{ aspectRatio: `${result.width} / ${result.height}` }"
            >
              <img
                :key="`${result.provider}:${result.id}:${imageRetryCount(result.id)}`"
                :src="result.preview_url"
                :alt="result.title || '灵感图片'"
                :loading="index < 8 ? 'eager' : 'lazy'"
                class="h-full w-full object-cover transition-transform duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] group-hover:scale-[1.025] select-none pointer-events-none"
                @error="markImageError(result.id)"
              />
            </div>
          </button>
          <SearchSavePopover
            :result="result"
            :saved="savedKeys.has(libraryAssetKey(result.provider, result.id))"
            @saved="markSaved(result)"
          />
          <div
            v-if="failedImages.has(result.id)"
            class="grid place-items-center gap-2 bg-board-loading-bg px-4 py-5 text-center"
          >
            <span class="text-xs font-semibold text-board-page-text-muted">图片暂时无法加载</span>
            <Button
              label="重试"
              size="small"
              severity="secondary"
              outlined
              class="!rounded-full"
              @click="retryImage(result.id)"
            />
          </div>
          <div class="p-3">
            <div class="truncate text-sm font-bold text-board-page-text">
              {{ result.title || 'Untitled reference' }}
            </div>
            <div class="mt-1 truncate text-xs text-board-page-text-muted">
              {{ result.author }} · {{ result.provider }}
            </div>
          </div>
        </article>
      </div>

      <div ref="sentinel" class="h-10" aria-hidden="true"></div>
      <div v-if="loadingMore" class="flex justify-center py-5">
        <ProgressSpinner
          class="!h-7 !w-7 text-board-primary"
          stroke-width="4"
          aria-label="加载更多灵感"
        />
      </div>
      <p
        v-else-if="results.length > 0 && !hasMore"
        class="py-4 text-center text-xs text-board-page-text-muted"
      >
        已加载全部可用灵感
      </p>
    </main>

    <!-- 🌟 左侧悬浮圆角素材岛组件 -->
    <LibraryFloatingIsland
      :open="libraryIslandOpen"
      :refresh-key="libraryRefreshKey"
      @close="libraryIslandOpen = false"
      @asset-saved="markSavedKey"
    />

    <!-- Lightbox 预览弹窗 -->
    <Dialog
      v-model:visible="lightboxVisible"
      modal
      dismissable-mask
      :show-header="false"
      :style="{ width: 'min(960px, calc(100vw - 32px))' }"
      :pt="{
        root: { class: '!overflow-hidden !rounded-2xl !border-board-card-border' },
        content: { class: '!p-0' },
      }"
    >
      <div v-if="activeResult" class="bg-board-surface-strong">
        <div
          class="flex items-center justify-between gap-3 border-b border-board-card-border px-5 py-4"
        >
          <div class="min-w-0">
            <div class="truncate text-base font-bold">
              {{ activeResult.title || 'Inspiration' }}
            </div>
            <div class="mt-1 text-xs text-board-page-text-muted">
              {{ activeResult.author }} · {{ activeResult.provider }}
            </div>
          </div>
          <Button
            text
            rounded
            aria-label="关闭预览"
            class="!size-9 !p-0"
            @click="lightboxVisible = false"
            ><template #icon><Xmark class="size-4" /></template
          ></Button>
        </div>
        <div class="grid max-h-[76vh] place-items-center bg-black/5 p-4">
          <img
            :src="activeResult.original_url || activeResult.preview_url"
            :alt="activeResult.title || '灵感图片'"
            class="max-h-[68vh] max-w-full object-contain"
          />
        </div>
        <div class="flex justify-end px-5 py-4">
          <a
            :href="activeResult.source_url"
            target="_blank"
            rel="noreferrer"
            class="text-sm font-semibold text-board-primary hover:underline"
            >查看来源</a
          >
        </div>
      </div>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import { Bookmark, Search, Xmark } from '@iconoir/vue'
import StudioName from '../../../shared/components/StudioName.vue'
import UserAccountMenu from '../../../shared/components/UserAccountMenu.vue'
import { HOME_PATH, USER_SEARCH_PATH } from '../../../shared/constants/routePaths'
import { useUserStore } from '../../auth/session/authSession'
import { useLogoutRedirect } from '../../../shared/composables/useLogoutRedirect'
import UserWorkspaceNavigation from '../../user-shell/components/UserWorkspaceNavigation.vue'
import SearchSavePopover from '../components/SearchSavePopover.vue'
import LibraryFloatingIsland from '../../library/components/LibraryFloatingIsland.vue'
import {
  libraryAssetKey,
  lookupSavedAssetKeys,
  saveSearchReference,
  type LibrarySearchReference,
} from '../../library/libraryResource'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import { useAppToast } from '../../../shared/composables/useAppToast'
import {
  isSearchQueryWithinLimit,
  normalizeSearchQuery,
  reportSearchImageFailure,
  searchInspiration,
  type SearchResult,
} from '../searchResource'

const route = useRoute()
const router = useRouter()
const toast = useAppToast()
const userStore = useUserStore()
const { logoutAndRedirect } = useLogoutRedirect({ logout: () => userStore.logout() })
const accountControls = useUserAccountControls({
  navigation: { navigate: (path) => router.push(path), logout: () => logoutAndRedirect(HOME_PATH) },
})
const draftQuery = ref('')
const submittedQuery = ref('')
const queryError = ref('')
const results = ref<SearchResult[]>([])
const nextCursor = ref('')
const hasMore = ref(false)
const loading = ref(false)
const loadingMore = ref(false)
const hasError = ref(false)
const lightboxVisible = ref(false)
const activeResult = ref<SearchResult | null>(null)
const sentinel = ref<HTMLElement>()
const failedImages = ref(new Set<string>())
const imageRetryCounts = ref(new Map<string, number>())
const savedKeys = ref(new Set<string>())
const libraryIslandOpen = ref(false)
const libraryRefreshKey = ref(0)
const isTriggerHoveredDuringDrag = ref(false)

let controller: AbortController | null = null
let observer: IntersectionObserver | null = null
let loadGeneration = 0
let autoOpenTimer: ReturnType<typeof setTimeout> | null = null

const statusText = computed(() =>
  loadingMore.value
    ? '正在加载更多...'
    : submittedQuery.value
      ? `${results.value.length} 个结果`
      : '精选图片参考',
)

function clearAutoOpenTimer() {
  if (!autoOpenTimer) return
  clearTimeout(autoOpenTimer)
  autoOpenTimer = null
}

// 🌟 跨视窗拖拽入库事件处理
function toSearchReference(result: SearchResult): LibrarySearchReference {
  return {
    provider: result.provider,
    id: result.id,
    source_url: result.source_url,
    preview_url: result.preview_url,
    original_url: result.original_url,
    title: result.title,
    author: result.author,
    width: result.width,
    height: result.height,
  }
}

function handleCardDragStart(e: DragEvent, result: SearchResult) {
  const reference = toSearchReference(result)
  e.dataTransfer!.setData('application/json', JSON.stringify(reference))
  e.dataTransfer!.effectAllowed = 'copy'

  // 创建 58×44px 微缩圆角影子 (Micro Drag Ghost)
  const ghost = document.createElement('div')
  ghost.style.position = 'fixed'
  ghost.style.top = '-9999px'
  ghost.style.left = '-9999px'
  ghost.style.width = '58px'
  ghost.style.height = '44px'
  ghost.style.borderRadius = '8px'
  ghost.style.overflow = 'hidden'
  ghost.style.boxShadow = '0 12px 28px rgba(0,0,0,0.3)'
  ghost.style.transform = 'rotate(-3deg)'
  ghost.style.zIndex = '9999'

  const img = document.createElement('img')
  img.src = result.preview_url
  img.style.width = '100%'
  img.style.height = '100%'
  img.style.objectFit = 'cover'

  ghost.appendChild(img)
  document.body.appendChild(ghost)
  e.dataTransfer!.setDragImage(ghost, 29, 22)

  setTimeout(() => {
    document.body.removeChild(ghost)
  }, 0)
}

function handleCardDragEnd() {
  clearAutoOpenTimer()
  isTriggerHoveredDuringDrag.value = false
}

function handleTriggerBtnDragOver(e: DragEvent) {
  e.dataTransfer!.dropEffect = 'copy'
  isTriggerHoveredDuringDrag.value = true

  // 悬停 280ms 自动展开左侧素材库浮岛
  if (!libraryIslandOpen.value && !autoOpenTimer) {
    autoOpenTimer = setTimeout(() => {
      libraryIslandOpen.value = true
      autoOpenTimer = null
    }, 280)
  }
}

function handleTriggerBtnDragLeave() {
  isTriggerHoveredDuringDrag.value = false
  clearAutoOpenTimer()
}

async function handleTriggerBtnDrop(e: DragEvent) {
  isTriggerHoveredDuringDrag.value = false
  clearAutoOpenTimer()

  try {
    const raw = e.dataTransfer?.getData('application/json')
    if (!raw) return
    const reference = JSON.parse(raw) as LibrarySearchReference
    await saveSearchReference(reference)
    markSavedKey(libraryAssetKey(reference.provider, reference.id))
    libraryRefreshKey.value += 1
    toast.success(`Saved "${reference.title || 'Image'}" to Quick Saves!`)
  } catch (error) {
    toast.error(error instanceof Error ? error.message : '保存素材失败')
  }
}

async function load(reset: boolean) {
  const generation = reset ? ++loadGeneration : loadGeneration
  if (reset) {
    controller?.abort()
    controller = new AbortController()
    results.value = []
    nextCursor.value = ''
    hasMore.value = false
    hasError.value = false
    failedImages.value = new Set()
    imageRetryCounts.value = new Map()
    savedKeys.value = new Set()
    loading.value = true
  } else {
    if (loadingMore.value || !hasMore.value) return
    loadingMore.value = true
  }
  try {
    const page = await searchInspiration({
      query: submittedQuery.value,
      cursor: nextCursor.value || undefined,
      limit: 24,
      signal: controller?.signal,
    })
    if (generation !== loadGeneration) return
    results.value = reset ? page.results : [...results.value, ...page.results]
    try {
      const saved = await lookupSavedAssetKeys(
        page.results.map((result) => ({ provider: result.provider, source_id: result.id })),
      )
      if (generation === loadGeneration) {
        savedKeys.value = new Set([...savedKeys.value, ...saved])
      }
    } catch (error) {
      if (import.meta.env.DEV) console.debug('library lookup unavailable', error)
    }
    if (generation === loadGeneration) {
      nextCursor.value = page.next_cursor || ''
      hasMore.value = page.has_more
    }
  } catch (error) {
    if (
      generation === loadGeneration &&
      !(error instanceof DOMException && error.name === 'AbortError')
    )
      hasError.value = true
  } finally {
    if (generation === loadGeneration) {
      loading.value = false
      loadingMore.value = false
    }
  }
}

async function submitQuery() {
  const query = normalizeSearchQuery(draftQuery.value)
  if (!isSearchQueryWithinLimit(query)) {
    queryError.value = '搜索关键词不能超过 200 个字符'
    return
  }
  queryError.value = ''
  await router.replace({ path: USER_SEARCH_PATH, query: query ? { q: query } : {} })
}

async function reload() {
  await load(true)
}

function openLightbox(result: SearchResult) {
  activeResult.value = result
  lightboxVisible.value = true
}

function markImageError(id: string) {
  if (failedImages.value.has(id)) return
  failedImages.value = new Set(failedImages.value).add(id)
  const result = results.value.find((item) => item.id === id)
  if (result) void reportSearchImageFailure(result.provider)
}

function retryImage(id: string) {
  const next = new Map(imageRetryCounts.value)
  next.set(id, (next.get(id) ?? 0) + 1)
  imageRetryCounts.value = next
  const failed = new Set(failedImages.value)
  failed.delete(id)
  failedImages.value = failed
}

function imageRetryCount(id: string) {
  return imageRetryCounts.value.get(id) ?? 0
}

function markSaved(result: SearchResult) {
  savedKeys.value = new Set(savedKeys.value).add(libraryAssetKey(result.provider, result.id))
}

function markSavedKey(key: string) {
  savedKeys.value = new Set(savedKeys.value).add(key)
}

function queryValue() {
  const value = route.query.q
  return Array.isArray(value) ? String(value[0] || '') : String(value || '')
}

watch(
  () => route.query.q,
  async () => {
    submittedQuery.value = normalizeSearchQuery(queryValue())
    draftQuery.value = submittedQuery.value
    queryError.value = ''
    await load(true)
  },
  { immediate: true },
)

onMounted(async () => {
  await nextTick()
  observer = new IntersectionObserver(
    (entries) => {
      if (entries.some((entry) => entry.isIntersecting)) void load(false)
    },
    { rootMargin: '640px' },
  )
  if (sentinel.value) observer.observe(sentinel.value)
})

onBeforeUnmount(() => {
  controller?.abort()
  observer?.disconnect()
  clearAutoOpenTimer()
})
</script>

<style scoped>
/* 🌟 核心自适应流式排版：当左侧素材库展开时，右侧主视窗平滑右移并保持重居中 */
.ib-main-content-flow {
  transition:
    padding-left 0.32s cubic-bezier(0.16, 1, 0.3, 1),
    padding-right 0.32s cubic-bezier(0.16, 1, 0.3, 1);
}

.library-open-layout {
  padding-left: 398px !important;
}

@media (max-width: 1280px) {
  .library-open-layout {
    padding-left: 370px !important;
  }
}

@media (max-width: 1024px) {
  .library-open-layout {
    padding-left: 350px !important;
  }
}

@media (max-width: 860px) {
  .library-open-layout {
    padding-left: 0 !important;
  }
}

/* 🌟 瀑布流间距自适应收敛 (全屏 20px -> 展开 14px) */
.masonry-grid-flow {
  transition: column-gap 0.32s cubic-bezier(0.16, 1, 0.3, 1);
}

.library-open-layout .masonry-grid-flow {
  column-gap: 0.875rem !important; /* 14px */
}

.masonry-card-item {
  transition: margin-bottom 0.32s cubic-bezier(0.16, 1, 0.3, 1);
}

.library-open-layout .masonry-card-item {
  margin-bottom: 0.875rem !important; /* 14px */
}
</style>
