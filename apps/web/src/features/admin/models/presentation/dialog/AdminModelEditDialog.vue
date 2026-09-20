<template>
  <AdminDialogShell
    :visible="visible"
    :header="isEditing ? '编辑模型' : '新增模型'"
    description="填写模型配置后保存。"
    size="wide"
    @update:visible="emit('update:visible', $event)"
    @close="emit('update:visible', false)"
  >
    <div class="grid grid-cols-1 gap-6 p-5 lg:grid-cols-12">
      <!-- 左侧：主要配置工作区 (7 列) -->
      <div class="flex flex-col gap-4.5 lg:col-span-7">
        <!-- 模块 1: 核心身份 (Identity) -->
        <div class="rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs">
          <div class="mb-3 flex items-center justify-between border-b border-admin-border-soft pb-2.5">
            <h4 class="text-xs font-semibold text-admin-text-1">模型身份与接入定义</h4>
            <span class="text-[11px] text-admin-text-3">Cloudflare AI Gateway 路由基础</span>
          </div>

          <div class="flex flex-col gap-3.5">
            <!-- 模型标识 (ID) -->
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center justify-between">
                <label class="text-admin-caption font-semibold text-admin-text-1" for="model-id-input">
                  模型标识 <span class="text-admin-danger">*</span>
                </label>
                <span class="text-[11px] text-admin-text-3">建议 provider/model 格式</span>
              </div>
              <AdminInput
                id="model-id-input"
                v-model="form.id"
                placeholder="例如: fal/birefnet 或 openai/gpt-image-1"
                :disabled="isEditing"
              />
              <!-- 快捷填充供应商前缀（Registry 驱动） -->
              <div v-if="!isEditing && idPrefixOptions.length > 0" class="flex flex-wrap items-center gap-1 pt-0.5">
                <span class="text-[11px] text-admin-text-3">常用前缀:</span>
                <button
                  v-for="prefix in idPrefixOptions"
                  :key="prefix"
                  type="button"
                  class="rounded-md border border-admin-border-soft bg-admin-surface-muted/50 px-1.5 py-0.5 text-[11px] font-medium text-admin-text-2 transition-colors hover:border-admin-accent/40 hover:bg-admin-accent/5 hover:text-admin-accent cursor-pointer"
                  @click="applyPrefix(prefix)"
                >
                  {{ prefix }}
                </button>
              </div>
            </div>

            <!-- 模型显示名称 -->
            <div class="flex flex-col gap-1.5">
              <label class="text-admin-caption font-semibold text-admin-text-1" for="model-name-input">
                显示名称 <span class="text-admin-danger">*</span>
              </label>
              <AdminInput
                id="model-name-input"
                v-model="form.name"
                placeholder="例如: FLUX.1 [dev] 旗舰版"
              />
            </div>

            <!-- 提供商 -->
            <div class="flex flex-col gap-1.5">
              <label class="text-admin-caption font-semibold text-admin-text-1" for="model-provider-input">
                提供商 (Provider) <span class="text-admin-danger">*</span>
              </label>
              <AdminInput
                id="model-provider-input"
                v-model="form.provider"
                placeholder="例如: Fal AI, OpenAI"
              />
              <!-- 预设提供商快速选择（Registry 驱动，无数据时仅手输） -->
              <div v-if="providerOptions.length > 0" class="flex flex-wrap gap-1 pt-0.5">
                <button
                  v-for="p in providerOptions"
                  :key="p.slug"
                  type="button"
                  class="rounded px-1.5 py-0.5 text-[10px] font-medium transition-colors cursor-pointer"
                  :class="
                    form.provider === p.display_name
                      ? 'bg-admin-accent text-white font-semibold'
                      : 'bg-admin-surface-muted text-admin-text-2 hover:bg-admin-surface-muted/80'
                  "
                  @click="selectProvider(p)"
                >
                  {{ p.display_name }}
                </button>
              </div>
            </div>

            <!-- 功能描述 -->
            <div class="flex flex-col gap-1.5">
              <label class="text-admin-caption font-semibold text-admin-text-1" for="model-desc-input">
                功能说明与业务描述
              </label>
              <AdminInput
                id="model-desc-input"
                v-model="form.description"
                placeholder="例如: 针对电商白底图与高质感工业设计柔光渲染深度优化..."
              />
            </div>
          </div>
        </div>

        <!-- 模块 2: 规格参数与视觉设计 (Specs & Theme) -->
        <div class="rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs">
          <div class="mb-3 flex items-center justify-between border-b border-admin-border-soft pb-2.5">
            <h4 class="text-xs font-semibold text-admin-text-1">规格参数与卡片视觉</h4>
            <span class="text-[11px] text-admin-text-3">前台市场呈现配置</span>
          </div>

          <div class="flex flex-col gap-3.5">
            <!-- 分辨率与托管类型 -->
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <!-- 分辨率 / 最大上下文 -->
              <div class="flex flex-col gap-1.5">
                <label class="text-admin-caption font-semibold text-admin-text-1" for="model-resolution-input">
                  分辨率 / 最大上下文
                </label>
                <AdminInput
                  id="model-resolution-input"
                  v-model="form.contextOrResolution"
                  placeholder="例如: 2048×2048 或 128K"
                />
                <div class="flex gap-1 pt-0.5">
                  <button
                    v-for="res in ['1024×1024', '2048×2048', '4096×4096', '128K']"
                    :key="res"
                    type="button"
                    class="rounded bg-admin-surface-muted px-1.5 py-0.5 text-[10px] text-admin-text-2 hover:bg-admin-surface-muted/80 cursor-pointer"
                    @click="form.contextOrResolution = res"
                  >
                    {{ res }}
                  </button>
                </div>
              </div>

              <!-- 托管类型 (Hosting) -->
              <div class="flex flex-col gap-1.5">
                <label class="text-admin-caption font-semibold text-admin-text-1">
                  托管类型
                </label>
                <div class="flex h-9 items-center rounded-lg border border-admin-border-soft bg-admin-surface-muted/40 p-1">
                  <button
                    type="button"
                    class="flex-1 rounded-md py-1 text-xs font-medium transition-colors cursor-pointer text-center"
                    :class="
                      form.hosting === 'Cloudflare 托管'
                        ? 'bg-white text-emerald-700 shadow-xs font-semibold'
                        : 'text-admin-text-2 hover:text-admin-text-1'
                    "
                    @click="form.hosting = 'Cloudflare 托管'"
                  >
                    Cloudflare 托管
                  </button>
                  <button
                    type="button"
                    class="flex-1 rounded-md py-1 text-xs font-medium transition-colors cursor-pointer text-center"
                    :class="
                      form.hosting === '第三方'
                        ? 'bg-white text-admin-text-1 shadow-xs font-semibold'
                        : 'text-admin-text-2 hover:text-admin-text-1'
                    "
                    @click="form.hosting = '第三方'"
                  >
                    第三方托管
                  </button>
                </div>
              </div>
            </div>

            <!-- 能力标签 -->
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center justify-between">
                <label class="text-admin-caption font-semibold text-admin-text-1" for="model-caps-input">
                  能力标签
                </label>
                <span class="text-[11px] text-admin-text-3">支持英文逗号分隔或点击预设</span>
              </div>
              <AdminInput
                id="model-caps-input"
                v-model="capabilitiesDraft"
                placeholder="例如: 垫图生图, 透明通道, 提示词微调"
              />
              <div class="flex flex-wrap items-center gap-1 pt-0.5">
                <span class="text-[11px] text-admin-text-3">快速添加:</span>
                <button
                  v-for="tag in popularCapabilities"
                  :key="tag"
                  type="button"
                  class="rounded-md border border-dashed border-admin-border-soft px-1.5 py-0.5 text-[11px] font-medium text-admin-text-2 hover:border-admin-accent/40 hover:text-admin-accent cursor-pointer"
                  @click="addCapabilityTag(tag)"
                >
                  + {{ tag }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- 模块 3: 高级网关配置 (Advanced Gateway Tuning) -->
        <div class="rounded-xl border border-admin-border-soft bg-admin-surface-muted/20 p-4">
          <button
            type="button"
            class="flex w-full items-center justify-between text-xs font-semibold text-admin-text-2 hover:text-admin-text-1 cursor-pointer"
            @click="showAdvanced = !showAdvanced"
          >
            <div class="flex items-center gap-2">
              <Settings class="size-4 text-admin-text-3" />
              <span>高级网关适配与参数微调 (可选)</span>
            </div>
            <NavArrowDown
              class="size-4 transition-transform"
              :class="showAdvanced ? 'rotate-180' : ''"
            />
          </button>

          <div v-if="showAdvanced" class="mt-3 flex flex-col gap-3.5 border-t border-admin-border-soft pt-3">
            <!-- 自定义网关路径 (Path Override) -->
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center justify-between">
                <label class="text-admin-caption font-semibold text-admin-text-1" for="model-custom-path-input">
                  自定义网关端点路径 (Path Override)
                </label>
                <span class="text-[11px] text-admin-text-3">留空则由系统智能推导</span>
              </div>
              <AdminInput
                id="model-custom-path-input"
                v-model="form.customPath"
                placeholder="例如: api/v1/chat/completions 或 fal-ai/birefnet"
              />
              <div class="flex flex-wrap gap-1">
                <button
                  v-for="pathOpt in commonGatewayPaths"
                  :key="pathOpt"
                  type="button"
                  class="rounded bg-white border border-admin-border-soft px-1.5 py-0.5 text-[10px] text-admin-text-2 hover:text-admin-accent cursor-pointer"
                  @click="form.customPath = pathOpt"
                >
                  {{ pathOpt }}
                </button>
              </div>
            </div>

            <!-- 额外自定义参数 (JSON) -->
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center justify-between">
                <label class="text-admin-caption font-semibold text-admin-text-1" for="model-custom-params-input">
                  额外自定义请求参数 (Custom JSON)
                </label>
                <button
                  v-if="form.customParams"
                  type="button"
                  class="text-[11px] text-admin-accent hover:underline cursor-pointer"
                  @click="formatCustomParamsJSON"
                >
                  格式化 JSON
                </button>
              </div>
              <AdminInput
                id="model-custom-params-input"
                v-model="form.customParams"
                placeholder='例如: {"guidance_scale": 7.5, "output_format": "png"}'
              />
              <span class="text-[11px] text-admin-text-3">
                必须为合法 JSON 键值对对象。调用网关时将自动与基础 Payload 进行深度合并注入。
              </span>
            </div>
          </div>
        </div>

        <!-- 错误提示 -->
        <div v-if="validationError" class="text-xs text-admin-danger" role="alert">
          {{ validationError }}
        </div>
      </div>

      <!-- 右侧：实时卡片预览与网关透视区 (5 列) -->
      <div class="flex flex-col gap-4 lg:col-span-5">
        <!-- 实时卡片预览 -->
        <div class="flex flex-col gap-2 rounded-xl border border-admin-border-soft bg-admin-surface-muted/30 p-4">
          <div class="flex items-center justify-between">
            <span class="text-xs font-semibold text-admin-text-2">卡片效果实时预览</span>
            <span class="inline-flex items-center rounded bg-admin-accent/10 px-1.5 py-0.5 text-[10px] font-semibold text-admin-accent">
              Live Preview
            </span>
          </div>

          <!-- 模型卡片 (主体与列表页共用 AdminModelCardBody) -->
          <div class="flex flex-col overflow-hidden rounded-xl border border-admin-border-soft bg-white shadow-xs">
            <AdminModelCardBody :model="previewModel" />
            <!-- 底栏 -->
            <div class="flex items-center justify-between border-t border-admin-border-soft bg-admin-bg/50 px-4 py-2 text-xs">
              <span class="text-[11px] text-admin-text-3">ID: {{ form.id || '未配置' }}</span>
              <span class="text-[11px] font-medium text-admin-accent">预览视图</span>
            </div>
          </div>
        </div>

        <!-- 网关路由透视卡 (Cloudflare AI Gateway Route Inspector) -->
        <div class="flex flex-col gap-2.5 rounded-xl border border-admin-border-soft bg-white p-4 shadow-xs">
          <div class="flex items-center gap-2">
            <Globe class="size-4 text-admin-accent" />
            <span class="text-xs font-semibold text-admin-text-1">Cloudflare AI Gateway 路由透视</span>
          </div>

          <div class="flex flex-col gap-2 text-xs">
            <div class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2">
              <span class="text-admin-text-3">提供商空间:</span>
              <span class="font-mono font-medium text-admin-text-1">
                {{ gatewayRoutePreview.providerSlug }}
                <span
                  v-if="gatewayRoutePreview.isFirstParty"
                  class="ml-1 inline-flex rounded bg-emerald-100 px-1 text-[10px] font-medium text-emerald-700"
                >
                  原生第一方
                </span>
                <span
                  v-else
                  class="ml-1 inline-flex rounded bg-slate-200 px-1 text-[10px] font-medium text-slate-700"
                >
                  自定义命名空间
                </span>
              </span>
            </div>

            <div class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2">
              <span class="text-admin-text-3">预期端点路径:</span>
              <span class="font-mono text-admin-text-1 text-[11px]">
                {{ gatewayRoutePreview.targetPath }}
              </span>
            </div>

            <div class="flex items-center justify-between rounded-lg bg-admin-surface-muted/50 px-3 py-2">
              <span class="text-admin-text-3">调用调度协议:</span>
              <span class="font-medium text-admin-text-1">
                {{ gatewayRoutePreview.protocol }}
              </span>
            </div>

            <div class="mt-1 rounded-lg border border-dashed border-admin-border-soft p-2.5 text-[11px] text-admin-text-3">
              💡 系统后端采用统一 Cloudflare AI Gateway 驱动，无需配置第三方独立 SDK 或外部密钥池。
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 弹窗底栏 -->
    <template #footer>
      <AdminButton
        variant="outline"
        :class="adminActionButtonClass"
        :disabled="pending"
        @click="emit('update:visible', false)"
      >
        取消
      </AdminButton>
      <AdminButton
        :class="adminActionButtonClass"
        :loading="pending"
        @click="handleSubmit"
      >
        {{ isEditing ? '保存修改' : '确认新增' }}
      </AdminButton>
    </template>
  </AdminDialogShell>
</template>


<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { watchDebounced } from '@vueuse/core'
import { Globe, NavArrowDown, Settings } from '@iconoir/vue'
import AdminButton from '../../../shared/components/AdminButton.vue'
import AdminDialogShell from '../../../shared/components/AdminDialogShell.vue'
import AdminInput from '../../../shared/components/AdminInput.vue'
import AdminModelCardBody from '../controls/AdminModelCardBody.vue'
import { adminActionButtonClass } from '../../../shared/lib/uiClasses'
import { createAdminGatewayResource, type GatewayProviderOption } from '../../infrastructure/adminGatewayResource'
import { type AdminModelItem } from '../../domain/catalogData'
import {
  blankModelForm,
  finalizeModelSubmit,
  splitCapabilities,
  validateModelForm,
} from '../../domain/modelForm'

const props = defineProps<{
  visible: boolean
  model: AdminModelItem | null
  pending?: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  submit: [item: AdminModelItem]
}>()

const catalogResource = createAdminGatewayResource().catalog

const isEditing = computed(() => Boolean(props.model))
const capabilitiesDraft = ref('')
const validationError = ref('')
const showAdvanced = ref(false)

// 供应商 quick-pick 与 ID 前缀全部来自 Registry 接口（后端单源），
// 失败即空列表、手输不受影响。
const providerOptions = ref<GatewayProviderOption[]>([])
const idPrefixOptions = computed(() => {
  const seen = new Set<string>()
  for (const option of providerOptions.value) {
    if (option.id_prefix) seen.add(option.id_prefix)
  }
  return [...seen]
})

async function loadProviderOptions() {
  if (providerOptions.value.length > 0) return
  try {
    providerOptions.value = await catalogResource.listProviders()
  } catch {
    providerOptions.value = []
  }
}

// 常见能力标签库
const popularCapabilities = [
  '高精细节',
  '垫图生图',
  '种子控制',
  '透明通道',
  '发丝羽化',
  '多尺寸延展',
  '商用合规',
  '提示词微调',
]

// 常见网关端点路径
const commonGatewayPaths = [
  'api/v1/chat/completions',
  'v1/chat/completions',
  'v1/images/generations',
  'v1/images/cutout',
  'v1/images/upscale',
]

const defaultForm: AdminModelItem = blankModelForm()

const form = ref<AdminModelItem>({ ...defaultForm })

// 预览用合成模型：将表单草稿与占位文案拼成完整 AdminModelItem，
// 与列表页共用 AdminModelCardBody，同一样式单源。
const previewModel = computed<AdminModelItem>(() => ({
  id: form.value.id,
  name: form.value.name || '模型名称',
  provider: form.value.provider || '供应商',
  description: form.value.description || '暂无描述。保存后将在前台模型库中完整展示。',
  descriptionEnriched: form.value.descriptionEnriched ?? false,
  contextOrResolution: form.value.contextOrResolution || '2048×2048',
  capabilities: previewCapabilities.value,
  hosting: form.value.hosting,
  termsUrl: '',
  docsUrl: '',
  customPath: form.value.customPath ?? '',
  customParams: form.value.customParams ?? '',
  inputCostPerToken: form.value.inputCostPerToken,
  outputCostPerToken: form.value.outputCostPerToken,
  cacheReadCostPerToken: form.value.cacheReadCostPerToken,
  cacheCreateCostPerToken: form.value.cacheCreateCostPerToken,
  supportsPromptCaching: form.value.supportsPromptCaching,
}))

// 实时预览标签列表（展示占位；提交归一见 modelForm，不伪造入库能力）
const previewCapabilities = computed(() => {
  const list = splitCapabilities(capabilitiesDraft.value)
  return list.length > 0 ? list : ['标准推理', '云端加速']
})

// 路由透视：来源 label 由后端出（workflow 网关路径推导），前端不再手抄枚举。
const gatewayRoutePreview = ref({
  providerSlug: '未指定',
  isFirstParty: false,
  targetPath: '-',
  protocol: '-',
})

async function refreshGatewayRoutePreview() {
  const modelId = form.value.id.trim()
  if (!modelId) {
    gatewayRoutePreview.value = {
      providerSlug: '未指定',
      isFirstParty: false,
      targetPath: '-',
      protocol: '-',
    }
    return
  }
  try {
    const preview = await catalogResource.previewRoute({
      modelId,
      customPath: form.value.customPath?.trim() || undefined,
      customParams: form.value.customParams?.trim() || undefined,
    })
    gatewayRoutePreview.value = {
      providerSlug: preview.provider_slug || '未指定',
      isFirstParty: preview.is_first_party,
      targetPath: preview.target_path,
      protocol: preview.path_source_label || preview.path_source,
    }
  } catch {
    // 预览失败保留上次结果，不干扰表单
  }
}

watchDebounced(
  [() => form.value.id, () => form.value.customPath, () => form.value.customParams],
  () => {
    void refreshGatewayRoutePreview()
  },
  { debounce: 300, immediate: true },
)

watch(
  () => props.visible,
  (val) => {
    if (val) {
      validationError.value = ''
      void loadProviderOptions()
      if (props.model) {
        form.value = { ...props.model }
        capabilitiesDraft.value = props.model.capabilities.join(', ')
        showAdvanced.value = Boolean(props.model.customPath || props.model.customParams)
      } else {
        form.value = { ...defaultForm }
        capabilitiesDraft.value = ''
        showAdvanced.value = false
      }
    }
  },
  { immediate: true },
)

function applyPrefix(prefix: string) {
  if (!form.value.id) {
    form.value.id = prefix
  } else if (!form.value.id.includes('/')) {
    form.value.id = prefix + form.value.id
  }
}

function selectProvider(p: GatewayProviderOption) {
  form.value.provider = p.display_name
  if (p.id_prefix && !form.value.id) {
    form.value.id = p.id_prefix
  }
}

function addCapabilityTag(tag: string) {
  const current = splitCapabilities(capabilitiesDraft.value)
  if (!current.includes(tag)) {
    current.push(tag)
    capabilitiesDraft.value = current.join(', ')
  }
}

function formatCustomParamsJSON() {
  try {
    const parsed = JSON.parse(form.value.customParams || '{}')
    form.value.customParams = JSON.stringify(parsed, null, 2)
  } catch {
    // ignore format errors
  }
}

function handleSubmit() {
  validationError.value = ''
  const error = validateModelForm({
    id: form.value.id,
    name: form.value.name,
    provider: form.value.provider,
    customParams: form.value.customParams,
  })
  if (error) {
    validationError.value = error
    return
  }

  // 手工保存即运营接管：富化位清 false，后续刷新富化不再覆盖简介。
  emit('submit', finalizeModelSubmit({ ...form.value, descriptionEnriched: false }, splitCapabilities(capabilitiesDraft.value)))
}
</script>