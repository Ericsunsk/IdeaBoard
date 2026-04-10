<template>
  <div class="admin-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">功能页系统提示词</h1>
        <p class="page-desc">为各功能页配置系统预设提示词，提交任务时将自动拼接到用户输入之前（用户不可见）</p>
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

const commonVars: VarDef[] = [
  { key: '{{user_prompt}}', desc: '用户输入的提示词' },
  { key: '{{category}}', desc: '用户选择的品类（如连衣裙、T恤）' },
]

const featureVarMap: Record<string, VarDef[]> = {
  'style-innovate': [
    { key: '{{strength}}', desc: '改变幅度分数（1-10）' },
    { key: '{{user_prompt}}', desc: '用户输入的描述' },
  ],
  'style-fusion': [
    { key: '{{category}}', desc: '用户选择的品类' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'series-color': [
    { key: '{{color_type}}', desc: '配色类型（women/men/kids/reference）' },
    { key: '{{colors}}', desc: '用户选择的配色列表（如 #8b3a3a, #6b2d3e）' },
    { key: '{{category}}', desc: '用户选择的品类' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'directed-recolor': [
    { key: '{{target_color}}', desc: '用户选择的目标颜色（如 #ff5500）' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'fabric-on-body': [
    { key: '{{tile_mode}}', desc: '替换方式（seamless=四方连续 / single=非四方连续）' },
    { key: '{{fabric_rotation}}', desc: '面料旋转角度（0-360）' },
    { key: '{{fabric_scale}}', desc: '面料大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: '面料透明度百分比（0-100）' },
    { key: '{{category}}', desc: '用户选择的品类（如连衣裙、T恤）' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-on-body': [
    { key: '{{pattern_mode}}', desc: '上身模式（full=全身 / partial=局部）' },
    { key: '{{tile_mode}}', desc: '替换方式（seamless=四方连续 / single=非四方连续）' },
    { key: '{{fabric_rotation}}', desc: '图案旋转角度（0-360）' },
    { key: '{{fabric_scale}}', desc: '图案大小百分比（10-300）' },
    { key: '{{fabric_opacity}}', desc: '图案透明度百分比（0-100）' },
    { key: '{{category}}', desc: '用户选择的品类（全身模式）' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'multi-angle': [
    { key: '{{angles}}', desc: '模特角度（逗号分隔，如 front_full,left_90）' },
    { key: '{{pose}}', desc: '模特动作（如 runway_stand, hands_on_waist）' },
    { key: '{{expression}}', desc: '面部表情（cool/smile/fierce/soft）' },
    { key: '{{background}}', desc: '环境背景（original=原图背景 / studio=影棚白底）' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'virtual-3d': [
    { key: '{{viewpoints}}', desc: '视角（逗号分隔，如 top_front,top_45,btm_front）' },
    { key: '{{lighting}}', desc: '光影氛围（warm_white=高级暖白 / afternoon=午后暖阳 / hard_light=质感硬光）' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'mannequin-to-model': [
    { key: '{{user_prompt}}', desc: '用户自定义提示词（描述模特效果）' },
  ],
  'face-swap': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'bg-swap': [
    { key: '{{user_prompt}}', desc: '用户提示词（本功能默认不需要用户输入）' },
  ],
  'garment-on-model': [
    { key: '{{model_url}}', desc: '模特图片 URL' },
    { key: '{{pose_url}}', desc: '姿势图片 URL' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-gen': [
    { key: '{{style_model}}', desc: '风格模型名称' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'style-clone': [
    { key: '{{style_model}}', desc: '风格模型名称' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'seamless-tile-classic': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'seamless-tile-cartoon': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-fusion-creative': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-fusion-style': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'craft-fusion-creative': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'craft-fusion-style': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-inpaint': [
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
  'pattern-upscale': [
    { key: '{{style_model}}', desc: '风格模型名称' },
    { key: '{{user_prompt}}', desc: '用户提示词' },
  ],
}

function getVarsForSlug(slug: string): VarDef[] {
  return featureVarMap[slug] || commonVars
}

// textarea 引用 + 插入逻辑
const textareaRefs = reactive<Record<string, any>>({})

function insertVar(slug: string, varKey: string) {
  const current = promptMap[slug] || ''
  const ref = textareaRefs[slug]
  let pos = current.length
  if (ref) {
    try {
      const textareaEl = ref.textareaElRef || ref.$el?.querySelector('textarea')
      if (textareaEl) pos = textareaEl.selectionStart ?? current.length
    } catch { /* fallback to end */ }
  }
  promptMap[slug] = current.slice(0, pos) + varKey + current.slice(pos)
}

const featureList = [
  { slug: 'sketch-to-design', title: '线稿成款' },
  { slug: 'partial-edit', title: '局部改款' },
  { slug: 'style-innovate', title: '款式创新' },
  { slug: 'style-fusion', title: '风格融合' },
  { slug: 'series-color', title: '系列配色' },
  { slug: 'directed-recolor', title: '定向换色' },
  { slug: 'fabric-on-body', title: '面料上身' },
  { slug: 'pattern-on-body', title: '图案上身' },
  { slug: 'multi-angle', title: '模特多角度' },
  { slug: 'virtual-3d', title: '虚拟 3D' },
  { slug: 'mannequin-to-model', title: '人台变模特' },
  { slug: 'face-swap', title: '定向换脸' },
  { slug: 'bg-swap', title: '换背景' },
  { slug: 'garment-on-model', title: '款式上身' },
  { slug: 'pattern-gen', title: '图案生成' },
  { slug: 'style-clone', title: '画风复刻' },
  { slug: 'seamless-tile-classic', title: '四方连续 · 经典风格' },
  { slug: 'seamless-tile-cartoon', title: '四方连续 · 卡通风格' },
  { slug: 'pattern-fusion-creative', title: '图案融合 · 创意融合' },
  { slug: 'pattern-fusion-style', title: '图案融合 · 画风融合' },
  { slug: 'craft-fusion-creative', title: '工艺融合 · 创意融合' },
  { slug: 'craft-fusion-style', title: '工艺融合 · 画风融合' },
  { slug: 'pattern-inpaint', title: '图案涂改' },
  { slug: 'pattern-upscale', title: '图案变清晰' },
  { slug: 'to-vector', title: '转矢量图' },
  { slug: 'remove-bg', title: '抠图褪底' },
  { slug: 'hd-upscale', title: '高清放大' },
  { slug: 'deblur', title: '模糊变清晰' },
  { slug: 'canvas-extend', title: '画布延伸' },
]

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
  } catch {}
  loading.value = false
}

async function savePrompt(slug: string) {
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
