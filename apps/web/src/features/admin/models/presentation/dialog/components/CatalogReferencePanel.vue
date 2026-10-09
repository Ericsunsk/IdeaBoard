<template>
  <div class="rounded-xl border border-border bg-card p-4 shadow-xs">
    <div class="mb-3 flex items-center justify-between border-b border-border pb-2.5">
      <h4 class="text-xs font-semibold text-foreground">{{ MODELS_MESSAGES.referenceTitle }}</h4>
      <button
        type="button"
        class="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-admin-caption font-medium text-muted-foreground hover:text-primary cursor-pointer disabled:opacity-50"
        :disabled="loading || !hasIdentity || isFal"
        @click="refreshReference"
      >
        <Refresh class="size-3.5" :class="{ 'animate-spin': loading }" />
        {{ MODELS_MESSAGES.referenceRefresh }}
      </button>
    </div>

    <!-- 无身份：等用户输入模型标识 -->
    <p v-if="!hasIdentity" class="text-admin-caption leading-relaxed text-muted-foreground">
      {{ MODELS_MESSAGES.referenceNoIdentity }}
    </p>

    <!-- fal 优雅缺省：models.dev 未收录 fal provider，恒为 miss；短路展示，不发参照请求 -->
    <p v-else-if="isFal" class="text-admin-caption leading-relaxed text-muted-foreground">
      {{ MODELS_MESSAGES.referenceFalNotice }}
    </p>

    <p v-else-if="loading" class="text-admin-caption text-muted-foreground">
      {{ MODELS_MESSAGES.referenceLoading }}
    </p>

    <div v-else-if="loadError" class="flex items-center justify-between gap-2">
      <span class="text-admin-caption text-destructive">{{ loadError }}</span>
      <button
        type="button"
        class="text-admin-caption font-semibold underline hover:no-underline cursor-pointer"
        @click="refreshReference"
      >
        {{ MODELS_MESSAGES.retry }}
      </button>
    </div>

    <div v-else-if="snapshot" class="flex flex-col gap-2.5">
      <!-- provider logo：本地品牌资产解析（经后端 brand-logo-overrides 生效覆盖），不请求 models.dev 任何域 -->
      <div class="flex items-center gap-2.5">
        <div
          class="flex size-7 shrink-0 items-center justify-center overflow-hidden rounded-lg"
          :class="
            brandLogo ? 'border border-border/60 bg-muted/60' : 'bg-primary-faint text-primary'
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
          <span class="truncate text-xs font-semibold text-foreground">
            {{ snapshot.name || snapshot.modelId }}
          </span>
          <span class="truncate font-mono text-admin-caption text-muted-foreground">
            {{ snapshot.modelId }}
          </span>
        </div>
        <span
          class="ml-auto inline-flex shrink-0 items-center rounded bg-muted px-1.5 py-0.5 text-admin-micro font-medium text-muted-foreground"
        >
          source: {{ snapshot.source }}
        </span>
      </div>

      <!-- miss / 上游降级：found=false，HTTP 仍 200，按 error_code 解释 -->
      <p v-if="!snapshot.found" class="text-admin-caption leading-relaxed text-muted-foreground">
        {{ missHint }}
      </p>

      <div v-else class="flex flex-col gap-2 text-xs">
        <div class="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2">
          <span class="text-muted-foreground">{{ MODELS_MESSAGES.referencePricingLabel }}</span>
          <span class="font-medium text-foreground">
            {{ pricingText || MODELS_MESSAGES.referenceUnknown }}
          </span>
        </div>

        <div class="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2">
          <span class="text-muted-foreground">{{ MODELS_MESSAGES.referenceContextLabel }}</span>
          <span class="font-medium text-foreground">
            {{ contextText || MODELS_MESSAGES.referenceUnknown }}
          </span>
        </div>

        <div class="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2">
          <span class="text-muted-foreground">{{ MODELS_MESSAGES.referenceUpdatedLabel }}</span>
          <span class="font-mono text-admin-caption text-foreground">
            {{ snapshot.lastUpdated || MODELS_MESSAGES.referenceUnknown }}
            <span v-if="freshnessText" class="text-muted-foreground">({{ freshnessText }})</span>
          </span>
        </div>

        <!-- 能力 enrichment：上游 6 booleans 原样透传，undefined 即未知不展示；
             只读展示，参照 name 仅作标题，永不回写表单 description
            （网关侧运营文案优先）。 -->
        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-muted/50 px-3 py-2"
          :title="MODELS_MESSAGES.referenceCapabilitiesTitle"
        >
          <span class="shrink-0 text-muted-foreground">{{
            MODELS_MESSAGES.referenceCapabilitiesLabel
          }}</span>
          <span v-if="!hasCapabilityData" class="font-medium text-muted-foreground">{{
            MODELS_MESSAGES.referenceNoData
          }}</span>
          <span v-else class="flex flex-wrap justify-end gap-1">
            <!-- 能力徽标收口 AdminStatusPill：supported→success，未知/不支持→neutral；
                 静态属性不呼吸；✓/✗/? 后缀保留 false 与未知区分（不再用 line-through）。 -->
            <AdminStatusPill
              v-for="capability in capabilities"
              :key="capability.key"
              :label="`${capability.label}${capability.supported === true ? '✓' : capability.supported === false ? '✗' : '?'}`"
              :variant="capability.supported === true ? 'success' : 'neutral'"
              :pulse="false"
              size="small"
            />
          </span>
        </div>

        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-muted/50 px-3 py-2"
          :title="MODELS_MESSAGES.referenceModalitiesTitle"
        >
          <span class="shrink-0 text-muted-foreground">{{
            MODELS_MESSAGES.referenceModalitiesLabel
          }}</span>
          <span class="text-right font-medium text-foreground">
            {{ modalitiesText || MODELS_MESSAGES.referenceNoData }}
          </span>
        </div>

        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-muted/50 px-3 py-2"
          :title="MODELS_MESSAGES.referenceReleaseTitle"
        >
          <span class="shrink-0 text-muted-foreground">{{
            MODELS_MESSAGES.referenceReleaseLabel
          }}</span>
          <span class="font-mono text-admin-caption text-foreground">
            {{ releaseKnowledgeText || MODELS_MESSAGES.referenceNoData }}
          </span>
        </div>

        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-muted/50 px-3 py-2"
          :title="MODELS_MESSAGES.referenceFamilyTitle"
        >
          <span class="shrink-0 text-muted-foreground">{{
            MODELS_MESSAGES.referenceFamilyLabel
          }}</span>
          <span class="font-medium text-foreground">
            {{ snapshot.family || MODELS_MESSAGES.referenceNoData }}
          </span>
        </div>

        <!-- 上游状态：deprecated 红色预警，beta 普通徽标；缺失即无预警，绝不误报。 -->
        <div
          class="flex items-center justify-between gap-2 rounded-lg bg-muted/50 px-3 py-2"
          :title="MODELS_MESSAGES.referenceStatusTitle"
        >
          <span class="shrink-0 text-muted-foreground">{{
            MODELS_MESSAGES.referenceStatusLabel
          }}</span>
          <span v-if="statusKind === 'unknown'" class="font-medium text-muted-foreground">{{
            MODELS_MESSAGES.referenceNoData
          }}</span>
          <!-- 上游状态收口 AdminStatusPill：deprecated→danger 红色预警，beta→neutral；
               缺失即无预警，绝不误报；静态属性不呼吸。 -->
          <AdminStatusPill
            v-else-if="statusKind === 'deprecated'"
            :label="MODELS_MESSAGES.referenceDeprecated(snapshot.status)"
            variant="danger"
            :pulse="false"
            size="small"
          />
          <AdminStatusPill
            v-else-if="statusKind === 'beta'"
            :label="snapshot.status ?? ''"
            variant="neutral"
            :pulse="false"
            size="small"
          />
          <span v-else class="font-medium text-foreground">{{ snapshot.status }}</span>
        </div>
      </div>

      <!-- 非结算声明：后端 note 恒为“参照价，非结算依据” -->
      <p class="text-admin-caption leading-relaxed text-muted-foreground">
        {{ MODELS_MESSAGES.referenceDisclaimer(snapshot.note) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { Cube, Refresh } from '@iconoir/vue'
import AdminStatusPill from '../../../../shared/components/AdminStatusPill.vue'
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
import { MODELS_MESSAGES } from '../../pages/messages'

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
    loadError.value = err instanceof Error ? err.message : MODELS_MESSAGES.referenceLoadFailed
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
const capabilities = computed(() => (snapshot.value ? referenceCapabilities(snapshot.value) : []))

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
      return MODELS_MESSAGES.referenceMissNotFound
    case 'upstream_error':
      return MODELS_MESSAGES.referenceMissUpstream
    case 'allowlist_missing':
      return MODELS_MESSAGES.referenceMissAllowlist
    default:
      return MODELS_MESSAGES.referenceMissDefault
  }
})
</script>
