<template>
  <div class="admin-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">工作流系统提示词</h1>
        <p class="page-desc">为当前画布节点和右键快捷动作配置系统预设提示词，提交任务时将自动拼接到用户输入之前（用户不可见）</p>
      </div>
    </div>

    <div class="tip-bar">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
      <span>支持模板变量：在提示词中插入 <code v-pre>{{变量名}}</code>，系统会自动替换为用户实际选择的值。若不使用变量，系统会自动将提示词拼接在用户输入前面。</span>
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
        <div v-else-if="isDirectApiFeature(item.slug)" class="var-note">
          当前功能走直连接口，不经过任务模板变量替换。
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
  { slug: 'fabric-on-body', title: 'TryOn · 面料上身' },
  { slug: 'pattern-on-body', title: 'TryOn · 图案上身' },
  { slug: 'multi-angle', title: 'Model · 多角度' },
  { slug: 'virtual-3d', title: 'Viewpoint · 虚拟 3D' },
  { slug: 'mannequin-to-model', title: 'Mannequin · 人台变模特' },
  { slug: 'face-swap', title: 'FaceSwap · 定向换脸' },
  { slug: 'bg-swap', title: 'Edit · 换背景' },
  { slug: 'pattern-gen', title: 'Generate · 图案生成' },
  { slug: 'style-clone', title: 'Generate · 画风复刻' },
  { slug: 'seamless-tile-classic', title: 'Generate · 四方连续 Classic' },
  { slug: 'seamless-tile-cartoon', title: 'Generate · 四方连续 Cartoon' },
  { slug: 'pattern-inpaint', title: 'Edit · 图案涂改' },
  { slug: 'pattern-upscale', title: 'Enhance · 图案变清晰' },
  { slug: 'to-vector', title: 'Quick · 转矢量图' },
  { slug: 'remove-bg', title: 'Quick · 抠图褪底' },
  { slug: 'hd-upscale', title: 'Enhance · 高清放大' },
  { slug: 'deblur', title: 'Enhance · 模糊变清晰' },
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
    { key: '{{tile_mode}}', desc: '替换方式（seamless=四方连续 / single=非四方连续）' },
    { key: '{{fabric_rotation}}', desc: '面料旋转角度（0-360）' },
    { key: '{{fabric_scale}}', desc: '面料大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: '面料透明度百分比（0-100）' },
  ],
  'pattern-on-body': [
    { key: '{{tile_mode}}', desc: '替换方式（seamless=四方连续 / single=非四方连续）' },
    { key: '{{fabric_rotation}}', desc: '图案旋转角度（0-360）' },
    { key: '{{fabric_scale}}', desc: '图案大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: '图案透明度百分比（0-100）' },
  ],
  'multi-angle': [
    { key: '{{angles}}', desc: 'Model 节点勾选角度（逗号分隔）' },
    { key: '{{pose}}', desc: '系统默认姿势' },
    { key: '{{expression}}', desc: '系统默认表情' },
    { key: '{{background}}', desc: 'Model 节点背景（original / studio）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点或底部兜底提示词' },
  ],
  'virtual-3d': [
    { key: '{{viewpoints}}', desc: 'Viewpoint 节点勾选视角（逗号分隔）' },
    { key: '{{lighting}}', desc: 'Viewpoint 节点光影（warm_white / afternoon / hard_light）' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点或底部兜底提示词' },
  ],
  'mannequin-to-model': [
    { key: '{{user_prompt}}', desc: 'Prompt 节点或底部兜底提示词' },
  ],
  'face-swap': noVars,
  'bg-swap': [
    { key: '{{user_prompt}}', desc: 'Prompt 节点或底部兜底提示词（可选）' },
  ],
  'pattern-gen': [
    { key: '{{style_model}}', desc: 'Generate 节点选择的 Style' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点提示词' },
  ],
  'style-clone': [
    { key: '{{style_model}}', desc: 'Generate 节点选择的 Style' },
    { key: '{{user_prompt}}', desc: 'Prompt 节点提示词' },
  ],
  'seamless-tile-classic': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'seamless-tile-cartoon': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'style-fusion': [
    { key: '{{user_prompt}}', desc: '用于弱修饰/辅助修饰融合方式、质感、细节、氛围；Image A / Image B 主次建议通过滑块控制' },
    { key: '{{blend_ratio}}', desc: 'Image A 占比（0-100）' },
    { key: '{{image_a_weight}}', desc: 'Image A 实际融合百分比' },
    { key: '{{image_b_weight}}', desc: 'Image B 实际融合百分比' },
  ],
  'pattern-inpaint': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-upscale': noVars,
  'to-vector': noVars,
  'remove-bg': noVars,
  'hd-upscale': noVars,
  'deblur': promptVars,
  'canvas-extend': promptVars,
}

function getVarsForSlug(slug: FeatureSlug): VarDef[] {
  return featureVarMap[slug] || noVars
}

const directApiFeatureSlugs = new Set<FeatureSlug>(['remove-bg', 'to-vector', 'pattern-upscale', 'hd-upscale'])

function isDirectApiFeature(slug: FeatureSlug) {
  return directApiFeatureSlugs.has(slug)
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
