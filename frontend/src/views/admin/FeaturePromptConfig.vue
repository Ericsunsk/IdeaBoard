<template>
  <div class="admin-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">工作流系统提示词</h1>
        <p class="page-desc">为当前走任务链路的画布工作流配置系统预设提示词，提交任务时将自动拼接到用户输入之前（用户不可见）</p>
      </div>
    </div>

    <div class="tip-bar">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      <span>支持模板变量：在提示词中插入 <code v-pre>{{变量名}}</code>，系统会自动替换为用户实际选择的值。仅对当前工作流任务链生效。</span>
    </div>

    <div class="prompt-list">
      <div v-for="item in featureList" :key="item.slug" class="prompt-card">
        <div class="prompt-card-header">
          <span class="prompt-card-title">{{ item.title }}</span>
          <span class="prompt-card-slug">{{ item.slug }}</span>
        </div>
        <div v-if="getVarsForSlug(item.slug).length > 0" class="var-buttons">
          <span class="var-buttons-label">插入变量：</span>
          <button
            v-for="v in getVarsForSlug(item.slug)"
            :key="v.key"
            class="var-btn"
            @click="insertVar(item.slug, v.key)"
          ><span class="var-btn-key">{{ v.key }}</span><span class="var-btn-desc">{{ v.desc }}</span></button>
        </div>
        <div v-if="getFeatureNote(item.slug)" class="var-note">
          {{ getFeatureNote(item.slug) }}
        </div>
        <n-input
          :ref="(el: any) => { if (el) textareaRefs[item.slug] = el }"
          v-model:value="promptMap[item.slug]"
          type="textarea"
          :placeholder="`为「${item.title}」设置系统提示词，可使用上方变量按钮插入模板变量...`"
          :autosize="{ minRows: 3, maxRows: 8 }"
          :loading="saving[item.slug]"
        />
        <div class="prompt-card-actions">
          <span class="prompt-card-count">{{ (promptMap[item.slug] || '').length }} 字</span>
          <n-button size="small" type="primary" :loading="saving[item.slug]" @click="savePrompt(item.slug)">
            保存
          </n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { NInput, NButton, useMessage } from 'naive-ui'
import { featurePromptApi } from '../../api'
import { WEAR_TILE_MODE_TEMPLATE_DESC } from '../../components/canvas/nodes/nodeOptionConfig'

const message = useMessage()

// 模板变量定义
interface VarDef { key: string; desc: string }

const promptVars: VarDef[] = [
  { key: '{{user_prompt}}', desc: '当前请求拼装后的提示词文本' },
]

const noVars: VarDef[] = []

const featureList = [
  { slug: 'sketch-to-design', title: 'Quick · 线稿成款' },
  { slug: 'partial-edit', title: 'Edit · 局部改款' },
  { slug: 'style-innovate', title: 'Quick · 款式创新' },
  { slug: 'style-fusion', title: 'Fusion · 双图融合' },
  { slug: 'series-color', title: 'Color · 多色配色' },
  { slug: 'directed-recolor', title: 'Color · 单色换色' },
  { slug: 'fabric-on-body', title: 'Wear · 上身贴合' },
  { slug: 'multi-angle', title: 'Model · 单视角控制' },
  { slug: 'virtual-3d', title: 'Display · 商品展示' },
  { slug: 'face-swap', title: 'Edit · 定向换脸' },
  { slug: 'bg-swap', title: 'Edit · 换背景' },
  { slug: 'pattern-gen', title: 'Pattern · 图案生成' },
  { slug: 'hd-upscale', title: 'Enhance · 高清放大' },
  { slug: 'canvas-extend', title: 'Enhance · 画布延伸' },
] as const

type FeatureSlug = (typeof featureList)[number]['slug']

const featureVarMap: Partial<Record<FeatureSlug, VarDef[]>> = {
  'sketch-to-design': promptVars,
  'partial-edit': promptVars,
  'style-innovate': [
    { key: '{{strength}}', desc: '改变幅度分数（1-10）' },
    { key: '{{user_prompt}}', desc: '当前提交的描述文本' },
  ],
  'series-color': [
    { key: '{{color_type}}', desc: '配色类型，当前 Color 节点多色模式固定为 reference（自定义多色配色）' },
    { key: '{{colors}}', desc: 'Color 节点多色模式下的配色列表（如 #8b3a3a, #6b2d3e）' },
    { key: '{{strength}}', desc: 'Color 节点多色模式的配色强度百分比（0-100）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点拼装后的提示词，可用来描述颜色主次、比例倾向和整体换色方向' },
  ],
  'directed-recolor': [
    { key: '{{target_color}}', desc: 'Color 节点单色模式下的目标颜色（如 #ff5500）' },
    { key: '{{strength}}', desc: 'Color 节点单色模式的换色强度百分比（0-100）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点拼装后的提示词（单色模式可选）' },
  ],
  'fabric-on-body': [
    { key: '{{tile_mode}}', desc: WEAR_TILE_MODE_TEMPLATE_DESC },
    { key: '{{fabric_rotation}}', desc: 'Texture 方向角度（0-360）' },
    { key: '{{fabric_scale}}', desc: 'Texture 大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: 'Texture 融合强度百分比（0-100）' },
    { key: '{{original_image_url}}', desc: 'Body 原图 URL（由 Wear 节点 Image 输入自动传入，无需手填）' },
    { key: '{{mask_image_url}}', desc: 'Mask 节点输出上传后的 URL（连接 Mask 时自动传入；未连接时为空）' },
    { key: '{{use_mask}}', desc: '是否启用蒙版（连接 Mask 时自动为 true，未连接时自动为 false）' },
  ],
  'multi-angle': [
    { key: '{{angle_spec}}', desc: '视角控制参数（JSON，系统按它决定 yaw 与景别）' },
    { key: '{{model_url}}', desc: '画布里接到 Model 输入口的参考图（系统自动传值）' },
    { key: '{{model_strength}}', desc: 'Model 输入口权重（0-100，越高越贴近参考图主体）' },
    { key: '{{pose_url}}', desc: '画布里接到 Pose 输入口的姿态参考图（系统自动传值）' },
    { key: '{{pose_strength}}', desc: 'Pose 输入口权重（0-100，越高越跟随姿态）' },
    { key: '{{background}}', desc: 'Model 节点背景参数（original / studio）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点输入口文本（弱修饰，用于氛围和细节）' },
  ],
  'virtual-3d': [
    { key: '{{viewpoint}}', desc: 'Display 节点所选展示视角，系统会自动转换成中文描述（如：45° 侧视）' },
    { key: '{{lighting}}', desc: 'Display 节点光影，系统会自动转换成中文描述（如：高级暖白）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点输入口文本；未连接时系统会用当前视角/光影摘要兜底' },
  ],
  'face-swap': noVars,
  'bg-swap': [
    { key: '{{user_prompt}}', desc: 'Prompt 节点输入口文本（可选，未连接时系统会用默认背景切换描述）' },
  ],
  'pattern-gen': [
    { key: '{{user_prompt}}', desc: '底部输入栏提交的图案生成提示词' },
  ],
  'style-fusion': [
    { key: '{{user_prompt}}', desc: '用于弱修饰/辅助修饰融合方式、质感、细节、氛围；Image A / Image B 主次建议通过滑块控制' },
    { key: '{{blend_ratio}}', desc: 'Image A 占比（0-100）' },
    { key: '{{image_a_weight}}', desc: 'Image A 实际融合百分比' },
    { key: '{{image_b_weight}}', desc: 'Image B 实际融合百分比' },
  ],
  'hd-upscale': [
    { key: '{{use_mask}}', desc: '是否启用局部增强（连接 Mask 时自动为 true，否则为 false）' },
    { key: '{{original_image_url}}', desc: 'Enhance 节点输入的原图 URL（自动传入）' },
    { key: '{{mask_image_url}}', desc: 'Mask 节点输出上传后的 URL（仅连接 Mask 时自动传入）' },
    { key: '{{upscale_mode}}', desc: 'HD 模式（base / enhancement）' },
    { key: '{{upscale_factor}}', desc: '放大倍数（2 / 3 / 4）' },
    { key: '{{output_format}}', desc: '输出格式（jpg / png）' },
    { key: '{{output_quality}}', desc: '输出质量（30-100）' },
  ],
  'canvas-extend': promptVars,
}

function getVarsForSlug(slug: FeatureSlug): VarDef[] {
  return featureVarMap[slug] || noVars
}

const featureNotes: Partial<Record<FeatureSlug, string>> = {
  'hd-upscale': 'Enhance · HD 在未连接 Mask 时走直连超分，不使用这里的系统提示词；连接 Mask 进入局部增强任务链后，才会使用下方模板变量与系统提示词。',
}

function getFeatureNote(slug: FeatureSlug) {
  return featureNotes[slug] || ''
}

// textarea 引用 + 插入逻辑
const textareaRefs = reactive<Record<string, any>>({})

function insertVar(slug: FeatureSlug, varKey: string) {
  const current = promptMap[slug] || ''
  const ref = textareaRefs[slug]
  let pos = current.length
  const textareaEl = ref?.textareaElRef ?? ref?.$el?.querySelector?.('textarea')
  if (textareaEl instanceof HTMLTextAreaElement) {
    pos = textareaEl.selectionStart ?? current.length
  }
  promptMap[slug] = current.slice(0, pos) + varKey + current.slice(pos)
}

const promptMap = reactive<Record<string, string>>({})
const saving = reactive<Record<string, boolean>>({})
const loading = ref(false)

async function loadPrompts() {
  loading.value = true
  try {
    const res: any = await featurePromptApi.adminList()
    if (res.code === 0 && res.data?.prompts) {
      for (const [slug, prompt] of Object.entries(res.data.prompts)) {
        promptMap[slug] = prompt as string
      }
    }
  } catch (error) {
    console.error('[FeaturePromptConfig] Failed to load prompts', error)
    message.error('加载提示词失败')
  }
  loading.value = false
}

async function savePrompt(slug: FeatureSlug) {
  saving[slug] = true
  try {
    const res: any = await featurePromptApi.adminSet(slug, promptMap[slug] || '')
    if (res.code === 0) {
      message.success(`「${featureList.find(f => f.slug === slug)?.title}」提示词已保存`)
    } else {
      message.error(res.msg || '保存失败')
    }
  } catch {
    message.error('保存失败')
  }
  saving[slug] = false
}

onMounted(loadPrompts)
</script>

<style scoped>
@import './admin-common.css';

.prompt-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.prompt-card {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.prompt-card-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.prompt-card-title {
  font-size: 14px;
  font-weight: 600;
  color: #e4e4e7;
}

.prompt-card-slug {
  font-size: 11px;
  color: #52525b;
  background: #27272a;
  padding: 2px 8px;
  border-radius: 4px;
  font-family: monospace;
}

.prompt-card-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.prompt-card-count {
  font-size: 11px;
  color: #52525b;
}

.var-buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.var-buttons-label {
  font-size: 11px;
  color: #71717a;
  flex-shrink: 0;
}

.var-note {
  font-size: 11px;
  color: #71717a;
}

.var-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid #3f3f46;
  background: #27272a;
  cursor: pointer;
  transition: all 0.15s;
}

.var-btn:hover {
  background: rgba(163,230,53,0.1);
  border-color: #a3e635;
}

.var-btn-key {
  color: #a3e635;
  font-size: 11px;
  font-family: monospace;
}

.var-btn-desc {
  color: #71717a;
  font-size: 10px;
}
</style>
