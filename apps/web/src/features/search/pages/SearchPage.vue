<template>
  <div
    class="app-scrollbar relative h-full w-full overflow-y-auto overflow-x-hidden bg-board-page px-[var(--spacing-ib-page-chrome-inset)] pt-[var(--spacing-ib-page-chrome-inset)] pb-16 text-board-page-text"
  >
    <div class="ib-board-page-background ib-page-dot-bg"></div>
    <header class="sticky top-0 z-40 flex items-center justify-between pointer-events-none pb-2">
      <StudioName class="pointer-events-auto" />
      <UserWorkspaceNavigation class="pointer-events-auto" />
      <UserAccountMenu
        class="pointer-events-auto"
        :avatar-url="accountControls.viewModel.avatarUrl.value"
        :balance="accountControls.viewModel.balance.value"
        :items="accountControls.viewModel.menuItems.value"
      />
    </header>

    <!-- 🌟 主视窗容器：当左侧素材库浮岛展开时，右侧紧贴浮岛右侧保留标准 24px 呼吸间距；全屏展开时 1:1 物理被顶出右侧屏幕 -->
    <main
      class="relative z-[1] mt-12 will-change-[transform,margin,padding] transition-[transform,margin,padding] duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)] [transform:translateZ(0)]"
      :class="[
        libraryIslandOpen && !libraryIslandExpanded
          ? 'ml-0 md:ml-[350px] mr-auto max-w-[1600px] pl-0 md:pl-6 lg:pl-8'
          : 'mx-auto max-w-[1600px] pl-0',
        libraryIslandOpen && libraryIslandExpanded ? 'translate-x-[100vw] pointer-events-none' : '',
      ]"
    >
      <!-- 🌟 核心吸顶搜索控制中心 (保持原始呼吸间距，向下滚动时悬浮吸顶) -->
      <div class="sticky top-16 z-30 flex flex-col items-center mb-6 pointer-events-none">
        <div class="flex w-full max-w-[720px] items-center gap-2 pointer-events-auto">
          <form
            class="min-w-0 flex-1 rounded-full border border-board-card-border bg-board-control-bg/80 p-1.5 shadow-2xs backdrop-blur-md"
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
              <button
                type="button"
                class="grid size-8 shrink-0 place-items-center rounded-full border border-board-card-border text-board-control-icon transition-colors hover:border-board-primary hover:bg-board-primary/10 hover:text-board-primary"
                aria-label="Add search reference image"
                title="Add search reference image"
                :disabled="references.length >= 4"
                @click="openFilePicker"
              >
                <Camera class="size-4" aria-hidden="true" />
              </button>
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
            class="grid size-11 shrink-0 place-items-center rounded-full border border-board-card-border bg-board-control-bg/80 backdrop-blur-md text-board-page-text-muted transition-[transform,background-color,border-color,color,box-shadow] duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:scale-105 active:scale-90 hover:border-board-primary hover:bg-ib-surface-hover hover:text-board-primary focus-visible:outline-none focus-visible:shadow-[var(--shadow-board-focus)] cursor-pointer"
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
            @click="toggleLibraryIsland"
            @dragover.prevent="handleTriggerBtnDragOver"
            @dragleave="handleTriggerBtnDragLeave"
            @drop.prevent="handleTriggerBtnDrop"
          >
            <Bookmark
              class="size-4 transition-transform duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)]"
              :class="{ 'scale-110 rotate-6': libraryIslandOpen }"
              aria-hidden="true"
            />
          </button>
        </div>
        <div
          v-if="references.length > 0"
          class="mt-2 flex w-full max-w-[720px] flex-wrap items-center gap-2 pointer-events-auto"
        >
          <div
            v-for="reference in references"
            :key="reference.key"
            class="group relative size-12 overflow-hidden rounded-xl border border-board-card-border bg-board-card-bg"
          >
            <img
              :src="reference.previewUrl"
              alt="Search reference"
              class="h-full w-full object-cover"
            />
            <button
              type="button"
              class="absolute right-0.5 top-0.5 grid size-5 place-items-center rounded-full bg-black/65 text-white opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
              :aria-label="`Remove reference ${reference.key}`"
              @click="removeReference(reference.key)"
            >
              <Xmark class="size-3" aria-hidden="true" />
            </button>
          </div>
          <button
            v-if="references.length < 4"
            type="button"
            class="grid size-12 place-items-center rounded-xl border border-dashed border-board-card-border text-board-page-text-muted transition-colors hover:border-board-primary hover:text-board-primary"
            aria-label="Add search reference image"
            @click="openFilePicker"
          >
            <Plus class="size-4" aria-hidden="true" />
          </button>
          <input
            ref="fileInput"
            type="file"
            accept="image/*"
            multiple
            class="hidden"
            @change="handleFileInput"
          />
        </div>
        <input
          v-if="references.length === 0"
          ref="fileInput"
          type="file"
          accept="image/*"
          multiple
          class="hidden"
          @change="handleFileInput"
        />
        <div
          v-if="
            visionState === 'extracting' ||
            visionState === 'error' ||
            palette.length ||
            atmosphereTags.length
          "
          class="mt-2 flex min-h-10 w-full max-w-[720px] flex-wrap items-center gap-2 rounded-2xl border border-board-card-border bg-board-control-bg/80 px-3 py-2 text-xs pointer-events-auto"
          aria-live="polite"
        >
          <template v-if="visionState === 'extracting'">
            <span class="h-3 w-24 animate-pulse rounded bg-board-card-border/70"></span>
            <span class="h-3 w-16 animate-pulse rounded bg-board-card-border/70"></span>
          </template>
          <template v-else-if="visionState === 'error'">
            <span class="font-semibold text-board-danger">{{ visionError }}</span>
            <button
              type="button"
              class="ml-auto font-semibold text-board-primary"
              @click="runVisionSearch"
            >
              Retry
            </button>
          </template>
          <template v-else>
            <button
              v-for="color in palette"
              :key="color"
              type="button"
              class="size-5 rounded-full border-2 transition-transform"
              :class="
                selectedPalette.has(color) ? 'scale-110 border-board-primary' : 'border-white/80'
              "
              :style="{ backgroundColor: color }"
              :aria-label="`Toggle palette ${color}`"
              @click="togglePalette(color)"
            ></button>
            <button
              v-for="tag in atmosphereTags"
              :key="tag"
              type="button"
              class="rounded-full border px-2.5 py-1 transition-colors"
              :class="
                selectedTags.has(tag)
                  ? 'border-board-primary bg-board-primary/10 text-board-primary'
                  : 'border-board-card-border text-board-page-text-muted'
              "
              @click="toggleTag(tag)"
            >
              {{ tag }}
            </button>
          </template>
        </div>
        <p
          v-if="queryError"
          class="mt-2 mb-0 text-sm font-semibold text-board-danger pointer-events-auto"
        >
          {{ queryError }}
        </p>
      </div>

      <div class="mt-6 flex items-center justify-between gap-3">
        <div>
          <h2 class="m-0 text-[22px] font-semibold">
            {{ submittedQuery ? `Results for “${submittedQuery}”` : 'Inspiration' }}
          </h2>
          <p v-if="!hasError && statusText" class="mt-1 mb-0 text-sm text-board-page-text-muted">
            {{ statusText }}
          </p>
          <div
            v-if="hasError && results.length > 0"
            class="mt-1 flex items-center gap-2 text-sm text-board-danger"
          >
            <span>Unable to load results</span>
            <button type="button" class="font-semibold text-board-primary" @click="reload">
              Retry
            </button>
          </div>
        </div>
      </div>

      <section
        v-if="loading && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg/80 backdrop-blur-sm"
      >
        <ProgressSpinner
          class="!h-9 !w-9 text-board-primary"
          stroke-width="4"
          aria-label="加载灵感"
        />
      </section>
      <section
        v-else-if="hasError && results.length === 0"
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg/80 backdrop-blur-sm text-center"
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
        class="mt-6 grid min-h-[280px] place-items-center rounded-2xl border border-dashed border-board-card-border bg-board-loading-bg/80 backdrop-blur-sm"
      >
        <p class="m-0 text-sm text-board-page-text-muted">没有找到匹配的灵感</p>
      </section>

      <!-- 🌟 瀑布流容器 (纯 Tailwind CSS 响应式流式列排布：呼出浮岛自适应 4 列 / 全屏 5 列) -->
      <div
        v-else
        class="mt-4 transition-[column-gap] duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)]"
        :class="[
          libraryIslandOpen && !libraryIslandExpanded
            ? 'columns-1 sm:columns-2 md:columns-3 lg:columns-4 xl:columns-4 gap-3.5'
            : 'columns-1 sm:columns-2 md:columns-3 lg:columns-4 xl:columns-5 gap-4',
        ]"
      >
        <article
          v-for="(result, index) in results"
          :key="`${result.provider}:${result.id}`"
          draggable="true"
          class="group relative break-inside-avoid overflow-hidden rounded-2xl bg-board-card-bg shadow-sm transition-shadow duration-[var(--motion-control-duration)] ease-[var(--motion-control-easing)] hover:shadow-[var(--shadow-ib-panel)] cursor-grab active:cursor-grabbing select-none"
          :class="libraryIslandOpen && !libraryIslandExpanded ? 'mb-3.5' : 'mb-4'"
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
                class="block h-full w-full object-cover select-none pointer-events-none"
                @error="markImageError(result.id)"
              />
            </div>
          </button>

          <!-- 🌟 Demo 同款 Hover 渐变半透操作浮层 (顶栏右上角搜图 + 底栏标题/作者 + 存入个人素材库极简 + 号按钮) -->
          <div
            class="pointer-events-none absolute inset-0 z-20 flex flex-col justify-between p-3.5 opacity-0 transition-opacity duration-[var(--motion-feedback-duration)] ease-[var(--motion-feedback-easing)] bg-gradient-to-t from-black/80 via-transparent to-black/20 group-hover:opacity-100"
          >
            <div class="pointer-events-auto flex justify-end">
              <button
                type="button"
                class="grid size-7 place-items-center rounded-full bg-black/40 text-white backdrop-blur-md transition-colors hover:bg-board-primary focus-visible:outline-none cursor-pointer"
                title="查看大图"
                aria-label="查看大图"
                @click.stop="openLightbox(result)"
              >
                <Search class="size-3.5" aria-hidden="true" />
              </button>
            </div>

            <!-- 底部信息与存入素材库按钮 -->
            <div class="pointer-events-auto flex items-end justify-between gap-3">
              <div class="min-w-0 flex-1">
                <p class="m-0 truncate text-xs font-medium text-white leading-tight">
                  {{ result.title || 'Untitled reference' }}
                </p>
                <p class="mt-1 mb-0 truncate text-[11px] text-white/70 leading-tight">
                  by {{ result.author }} · {{ result.provider }}
                </p>
              </div>

              <!-- 存入个人素材库触发器 (SearchSavePopover) -->
              <SearchSavePopover
                :result="result"
                :saved="savedKeys.has(libraryAssetKey(result.provider, result.id))"
                @saved="markSaved(result)"
              />
            </div>
          </div>

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
      v-model:expanded="libraryIslandExpanded"
      :open="libraryIslandOpen"
      :refresh-key="libraryRefreshKey"
      :reference-keys="references.map((reference) => reference.key)"
      @close="closeLibraryIsland"
      @asset-saved="markSavedKey"
      @reference-selected="handleLibraryReference"
    />

    <!-- 🌟 Demo 同款零遮挡全景高清 Lightbox 弹窗 (bg-black/65 backdrop-blur-2xl) -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition-opacity duration-[var(--motion-enter-duration)] ease-[var(--motion-enter-easing)]"
        leave-active-class="transition-opacity duration-[var(--motion-exit-duration)] ease-[var(--motion-exit-easing)]"
        enter-from-class="opacity-0"
        leave-to-class="opacity-0"
      >
        <div
          v-if="lightboxVisible && activeResult"
          class="fixed inset-0 z-[2000] flex flex-col items-center justify-between p-4 sm:p-6 md:p-8 bg-black/65 backdrop-blur-2xl select-none"
          @click="closeLightbox"
        >
          <!-- 顶部极简悬浮信息与操作栏 -->
          <div
            class="flex w-full max-w-6xl items-center justify-between z-20 shrink-0 mb-3 px-2"
            @click.stop
          >
            <!-- 左上角：纯净悬浮文字 (Title + Author) -->
            <div class="min-w-0 pr-4">
              <h3
                class="m-0 text-white text-base sm:text-lg font-semibold tracking-tight drop-shadow-md truncate max-w-[240px] sm:max-w-md"
              >
                {{ activeResult.title || 'Inspiration' }}
              </h3>
              <p class="mt-1 mb-0 text-white/70 text-xs drop-shadow-md">
                {{ activeResult.author }} · {{ activeResult.provider }}
              </p>
            </div>

            <!-- 右上角：来源链接 + 极简圆形 ✕ 关闭按钮 -->
            <div class="flex items-center gap-2.5">
              <a
                v-if="activeResult.source_url"
                :href="activeResult.source_url"
                target="_blank"
                rel="noreferrer"
                class="h-9 px-4 rounded-full bg-white/10 hover:bg-white/20 text-white text-xs font-semibold backdrop-blur-md border border-white/15 flex items-center gap-1.5 transition no-underline shadow-sm"
              >
                <span>View Source</span>
              </a>

              <button
                type="button"
                class="size-9 rounded-full bg-black/40 hover:bg-white/20 text-white/80 hover:text-white backdrop-blur-xl border border-white/10 flex items-center justify-center transition shadow-md shrink-0 cursor-pointer focus-visible:outline-none"
                aria-label="Close preview"
                title="Close (Esc)"
                @click="closeLightbox"
              >
                <Xmark class="size-4" aria-hidden="true" />
              </button>
            </div>
          </div>

          <!-- 中间大图展示区 (彻底去除边框与遮挡，纯粹沉浸大图鉴赏) -->
          <div
            class="flex-1 w-full max-w-6xl flex items-center justify-center overflow-hidden min-h-0"
            @click.stop
          >
            <img
              :src="activeResult.original_url || activeResult.preview_url"
              :alt="activeResult.title || '灵感图片'"
              class="max-h-[82vh] max-w-full object-contain rounded-2xl shadow-2xl border-0 ring-0 outline-none select-none"
            />
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import ProgressSpinner from 'primevue/progressspinner'
import { Bookmark, Camera, Plus, Search, Xmark } from '@iconoir/vue'
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
  saveSearchReference,
  type LibrarySearchReference,
} from '../../library/libraryResource'
import { useUserAccountControls } from '../../user-shell/useUserAccountControls'
import { useAppToast } from '../../../shared/composables/useAppToast'
import {
  isSearchQueryWithinLimit,
  normalizeSearchQuery,
  type SearchResult,
} from '../searchResource'
import { useSearchResults } from '../useSearchResults'
import { useSearchLightbox } from '../useSearchLightbox'
import { useVisionSearchFlow } from '../useVisionSearchFlow'

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
const libraryIslandOpen = ref(false)
const libraryIslandExpanded = ref(false)
const libraryRefreshKey = ref(0)
const isTriggerHoveredDuringDrag = ref(false)
const selectedPalette = ref(new Set<string>())
const selectedTags = ref(new Set<string>())
const keepResultsOnNextLoad = ref(false)
let autoOpenTimer: ReturnType<typeof setTimeout> | null = null

const refinements = computed(() => ({
  palette: [...selectedPalette.value],
  atmosphereTags: [...selectedTags.value],
}))
const searchResults = useSearchResults({
  submittedQuery,
  refinements,
  keepResultsOnNextLoad: keepResultsOnNextLoad,
})
const visionFlow = useVisionSearchFlow({
  draftQuery,
  submittedQuery,
  queryError,
  keepResultsOnNextLoad,
  loadCatalog: () => searchResults.load(true),
  selectedPalette,
  selectedTags,
})
const lightbox = useSearchLightbox()
const {
  results,
  hasMore,
  loading,
  loadingMore,
  hasError,
  sentinel,
  failedImages,
  savedKeys,
  load,
  reload,
  markImageError,
  retryImage,
  imageRetryCount,
  markSaved,
  markSavedKey,
} = searchResults
const {
  references,
  fileInput,
  visionState,
  visionError,
  palette,
  atmosphereTags,
  runVisionSearch,
  togglePalette,
  toggleTag,
  removeReference,
  handleFileInput,
  openFilePicker,
  handleLibraryReference,
  clearVisionExtraction,
} = visionFlow
const {
  visible: lightboxVisible,
  activeResult,
  open: openLightbox,
  close: closeLightbox,
} = lightbox

const statusText = computed(() =>
  loadingMore.value
    ? '正在加载更多...'
    : submittedQuery.value
      ? `${results.value.length} 个结果`
      : '',
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

async function submitQuery() {
  const query = normalizeSearchQuery(draftQuery.value)
  if (!isSearchQueryWithinLimit(query)) {
    queryError.value = '搜索关键词不能超过 200 个字符'
    return
  }
  queryError.value = ''
  if (references.value.length > 0 && visionState.value !== 'ready') {
    await runVisionSearch()
    return
  }
  if (references.value.length === 0) clearVisionExtraction()
  else keepResultsOnNextLoad.value = results.value.length > 0
  const routeQuery: Record<string, string | string[]> = query ? { q: query } : {}
  if (references.value.length > 0) {
    if (selectedPalette.value.size > 0) routeQuery.palette = [...selectedPalette.value]
    if (selectedTags.value.size > 0) routeQuery.atmosphere_tag = [...selectedTags.value]
  }
  await router.replace({ path: USER_SEARCH_PATH, query: routeQuery })
}

function closeLibraryIsland() {
  libraryIslandOpen.value = false
  libraryIslandExpanded.value = false
}

function toggleLibraryIsland() {
  if (libraryIslandOpen.value) {
    closeLibraryIsland()
    return
  }
  libraryIslandOpen.value = true
}

function queryValue() {
  const value = route.query.q
  return Array.isArray(value) ? String(value[0] || '') : String(value || '')
}

watch(
  () => [route.query.q, route.query.palette, route.query.atmosphere_tag],
  async () => {
    submittedQuery.value = normalizeSearchQuery(queryValue())
    draftQuery.value = submittedQuery.value
    visionFlow.syncFromRoute()
    queryError.value = ''
    await load(true)
  },
  { immediate: true },
)

</script>
