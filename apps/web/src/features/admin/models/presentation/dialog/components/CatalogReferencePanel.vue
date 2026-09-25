<template>
  <div class="rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs">
    <div class="mb-3 flex items-center justify-between border-b border-admin-border-soft pb-2.5">
      <h4 class="text-xs font-semibold text-admin-text-1">models.dev 参照信息</h4>
      <button
        type="button"
        class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium text-admin-text-2 hover:text-admin-accent cursor-pointer disabled:opacity-50"
        :disabled="loading || !hasIdentity || isFal"
        @click="refreshReference"
      >
        <Refresh class="size-3.5" :class="{ 'animate-spin': loading }" />
        刷新
      </button>
    </div>

    <!-- 无身份：等用户输入模型标识 -->
    <p v-if="!hasIdentity" class="text-[11px] leading-relaxed text-admin-text-3">
      输入模型标识后自动查询参照价（经后端透传，不直连外部）。
    </p>

    <!-- fal 优雅缺省：models.dev 未收录 fal provider，恒为 miss；短路展示，不发参照请求 -->
    <p v-else-if="isFal" class="text-[11px] leading-relaxed text-admin-text-2">
      fal 模型暂无 models.dev 数据（上游未收录），以网关侧配置为准。
    </p>

    <p v-else-if="loading" class="text-[11px] text-admin-text-2">正在查询 models.dev 参照价...</p>

    <div v-else-if="loadError" class="flex items-center justify-between gap-2">
      <span class="text-[11px] text-admin-danger">{{ loadError }}</span>
      <button
        type="button"
        class="text-[11px] font-semibold underline hover:no-underline cursor-pointer"
        @click="refreshReference"
      >
        重试
      </button>
    </div>

    <div v-else-if="snapshot" class="flex flex-col gap-2.5">
      <!-- provider logo：本地品牌资产解析（经后端 brand-logo-overrides 生效覆盖），不请求 models.dev 任何域 -->
      <div class="flex items-center gap-2.5">
        <div
          class="flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-lg"
          :class="
            brandLogo
              ? 'border border-admin-border-soft/60 bg-admin-surface-muted/60'
              : 'bg-admin-accent/10 text-admin-accent'
          "
        >
          <img
            v-if="brandLogo && !logoLoadFailed"
            :src="brandLogo"
            :alt="snapshot.provider || 'provider'"
            class="size-4.5 object-contain"
            loading="lazy"
            @error="onLogoError"
          />
          <Cube v-else class="size-4" />
        </div>
        <div class="flex min-w-0 flex-col">
          <span class="truncate text-xs font-semibold text-admin-text-1">
            {{ snapshot.name || snapshot.modelId }}
          </span>
          <span class="truncate font-mono text-[11px] text-admin-text-3">
            {{ snapshot.modelId }}
          </span>
        </div>
        <span
          class="ml-auto inline-flex shrink-0 items-center rounded bg-admin-surface-muted px-1.5 py-0.5 text-[10px] font-medium text-admin-text-2"
        >
          source: {{ snapshot.source }}
        </span>
      </div>

      <!-- miss / 上游降级：found=false，HTTP 仍 200，按 error_code 解释 -->
      <p v-if="!snapshot.found" class="text-[11px] leading-relaxed text-admin-text-2">
        {{ missHint }}
      </p>

      <div v-else class="flex flex-col gap-2 text-xs">
        <div
          class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2"
        >
          <span class="text-admin-text-3">输入 / 输出 (USD/MTok):</span>
          <span class="font-medium text-admin-text-1">
            {{ pricingText || '未知' }}
          </span>
        </div>

        <div
          class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2"
        >
          <span class="text-admin-text-3">上下文上限:</span>
          <span class="font-medium text-admin-text-1">
            {{ contextText || '未知' }}
          </span>
        </div>

        <div
          class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2"
        >
          <span class="text-admin-text-3">上游更新时间:</span>
          <span class="font-mono text-[11px] text-admin-text-1">
            {{ snapshot.lastUpdated || '未知' }}
            <span v-if="freshnessText" class="text-admin-text-3">({{ freshnessText }})</span>
          </span>
        </div>

        <!-- 能力 enrichment：上游 6 booleans 原样透传，undefined 即未知不展示；
             只读展示，参照 name 仅作标题，永不回写表单 description
            （网关侧运营文案优先）。 -->
        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-admin-surface-muted/50 px-3 py-2"
          title="models.dev 能力原样透传（缺失即未知）"
        >
          <span class="shrink-0 text-admin-text-3">能力:</span>
          <span v-if="!hasCapabilityData" class="font-medium text-admin-text-3">暂无数据</span>
          <span v-else class="flex flex-wrap justify-end gap-1">
            <span
              v-for="capability in capabilities"
              :key="capability.key"
              class="inline-flex items-center rounded px-1.5 py-0.5 text-[11px] font-medium"
              :class="
                capability.supported === true
                  ? 'bg-emerald-50 text-emerald-700'
                  : capability.supported === false
                    ? 'bg-admin-surface-muted text-admin-text-3 line-through'
                    : 'bg-admin-surface-muted text-admin-text-3'
              "
            >
              {{ capability.label
              }}{{ capability.supported === true ? '✓' : capability.supported === false ? '✗' : '?' }}
            </span>
          </span>
        </div>

        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-admin-surface-muted/50 px-3 py-2"
          title="models.dev modalities 原样透传"
        >
          <span class="shrink-0 text-admin-text-3">输入 / 输出模态:</span>
          <span class="text-right font-medium text-admin-text-1">
            {{ modalitiesText || '暂无数据' }}
          </span>
        </div>

        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-admin-surface-muted/50 px-3 py-2"
          title="models.dev release_date / knowledge 透传"
        >
          <span class="shrink-0 text-admin-text-3">发布 / 知识截止:</span>
          <span class="font-mono text-[11px] text-admin-text-1">
            {{ releaseKnowledgeText || '暂无数据' }}
          </span>
        </div>

        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-admin-surface-muted/50 px-3 py-2"
          title="models.dev family 透传"
        >
          <span class="shrink-0 text-admin-text-3">模型家族:</span>
          <span class="font-medium text-admin-text-1">
            {{ snapshot.family || '暂无数据' }}
          </span>
        </div>

        <!-- 上游状态：deprecated 红色预警，beta 普通徽标；缺失即无预警，绝不误报。 -->
        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-admin-surface-muted/50 px-3 py-2"
          title="models.dev status 透传（deprecated 红色预警）"
        >
          <span class="shrink-0 text-admin-text-3">上游状态:</span>
          <span v-if="statusKind === 'unknown'" class="font-medium text-admin-text-3">暂无数据</span>
          <span
            v-else-if="statusKind === 'deprecated'"
            class="inline-flex items-center rounded border border-red-200 bg-red-50 px-1.5 py-0.5 text-[11px] font-semibold text-red-700"
          >
            {{ snapshot.status }} · 已下线预警
          </span>
          <span
            v-else-if="statusKind === 'beta'"
            class="inline-flex items-center rounded border border-admin-border-soft bg-white px-1.5 py-0.5 text-[11px] font-medium text-admin-text-2"
          >
            {{ snapshot.status }}
          </span>
          <span v-else class="font-medium text-admin-text-1">{{ snapshot.status }}</span>
        </div>
      </div>

      <!-- 非结算声明：后端 note 恒为“参照价，非结算依据” -->
      <p class="text-[11px] leading-relaxed text-admin-text-3">
        ※ {{ snapshot.note || '参照价，非结算依据' }}；结算唯一依据 billing SKU UnitCredits。
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { Cube, Refresh } from '@iconoir/vue'
import { createAdminGatewayResource } from '../../../infrastructure/adminGatewayResource'
import { resolveModelBrandLogo } from '../../../../shared/logos/brandLogos'
import {
  formatReferenceContext,
  formatReferenceFreshness,
  formatReferenceModalities,
  formatReferencePricing,
  formatReferenceReleaseKnowledge,
  hasReferenceCapabilityData,
  isFalReferenceModel,
  referenceCapabilities,
  referenceStatusKind,
  type ModelReferenceSnapshot,
} from '../../../domain/catalogData'

const props = withDefaults(
  defineProps<{
    modelId: string
    provider?: string
    modelName?: string
    enabled?: boolean
  }>(),
  {
    provider: '',
    modelName: '',
    enabled: true,
  },
)

const catalogResource = createAdminGatewayResource().catalog

const snapshot = ref<ModelReferenceSnapshot | null>(null)
const loading = ref(false)
const loadError = ref('')
let requestVersion = 0

const hasIdentity = computed(() => identityOf() !== null)

// fal 短路：models.dev 未收录 fal，命中即优雅缺省，不发参照请求。
const isFal = computed(() =>
  isFalReferenceModel({ provider: props.provider, modelId: props.modelId }),
)

function identityOf(): { modelId: string } | { provider: string; model: string } | null {
  const modelId = props.modelId.trim()
  if (modelId) return { modelId }
  const provider = props.provider.trim()
  const model = props.modelName.trim()
  if (provider && model) return { provider, model }
  return null
}

async function fetchReference(refresh: boolean) {
  if (!props.enabled) {
    requestVersion += 1
    snapshot.value = null
    loading.value = false
    loadError.value = ''
    return
  }
  const version = ++requestVersion
  if (isFal.value) {
    if (version !== requestVersion) return
    snapshot.value = null
    loadError.value = ''
    return
  }
  const identity = identityOf()
  if (!identity) {
    if (version !== requestVersion) return
    snapshot.value = null
    loadError.value = ''
    return
  }
  loading.value = true
  loadError.value = ''
  try {
    const nextSnapshot = await catalogResource.getReference({ ...identity, refresh })
    if (version !== requestVersion) return
    snapshot.value = nextSnapshot
  } catch (err) {
    if (version !== requestVersion) return
    snapshot.value = null
    loadError.value = err instanceof Error ? err.message : '加载参照价失败'
  } finally {
    if (version === requestVersion) loading.value = false
  }
}

function refreshReference() {
  void fetchReference(true)
}

// 表单输入抖动时才查后端，避免每个 keystroke 打一次参照接口。
watchDebounced(
  [() => props.enabled, () => props.modelId, () => props.provider, () => props.modelName],
  () => {
    void fetchReference(false)
  },
  { debounce: 400, immediate: true },
)

watch(
  () => [props.enabled, props.modelId] as const,
  () => {
    // 标识清空即丢弃旧快照，避免把上一个模型的价挂在空表单下。
    if (!identityOf()) {
      snapshot.value = null
      loadError.value = ''
    }
  },
)

const brandLogo = computed(() =>
  resolveModelBrandLogo({
    id: props.modelId,
    name: props.modelName,
    provider: props.provider,
  }),
)

// provider logo 经后端透传 models.dev SVG：加载失败时回落 Cube 占位，避免破图。
const logoLoadFailed = ref(false)
watch(brandLogo, () => {
  logoLoadFailed.value = false
})

function onLogoError(): void {
  logoLoadFailed.value = true
}

const pricingText = computed(() => (snapshot.value ? formatReferencePricing(snapshot.value) : ''))

const contextText = computed(() =>
  snapshot.value ? formatReferenceContext(snapshot.value.contextLimit) : '',
)

const freshnessText = computed(() =>
  snapshot.value ? formatReferenceFreshness(snapshot.value.lastUpdated) : '',
)

// enrichment 接线：能力/modality/发布知识截止/家族/上游状态 5 行；
// 缺失一律暂无数据，deprecated 红 badge，beta 普通徽标，无 status 绝不误报。
const capabilities = computed(() =>
  snapshot.value ? referenceCapabilities(snapshot.value) : [],
)

const hasCapabilityData = computed(() =>
  snapshot.value ? hasReferenceCapabilityData(snapshot.value) : false,
)

const modalitiesText = computed(() =>
  snapshot.value ? formatReferenceModalities(snapshot.value.modalities) : '',
)

const releaseKnowledgeText = computed(() =>
  snapshot.value
    ? formatReferenceReleaseKnowledge(snapshot.value.releaseDate, snapshot.value.knowledge)
    : '',
)

const statusKind = computed(() =>
  snapshot.value ? referenceStatusKind(snapshot.value.status) : 'unknown',
)

// found=false 文案：后端 error_code 语义前台直译，message 作为补充。
const missHint = computed(() => {
  const current = snapshot.value
  if (!current) return ''
  if (current.message) return current.message
  switch (current.errorCode) {
    case 'not_found':
      return 'models.dev 未收录该模型，请确认标识或改用 provider/model 完整写法。'
    case 'upstream_error':
      return '参照上游暂不可用，稍后可点右上刷新重试。'
    case 'allowlist_missing':
      return '服务端未放行 models.dev，参照价不可用。'
    default:
      return '暂无该模型参照价。'
  }
})
</script>
