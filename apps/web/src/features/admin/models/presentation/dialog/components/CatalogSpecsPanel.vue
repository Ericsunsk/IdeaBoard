<template>
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
            :model-value="contextOrResolution"
            placeholder="例如: 2048×2048 或 128K"
            @update:model-value="emit('update:contextOrResolution', String($event))"
          />
          <div class="flex gap-1 pt-0.5">
            <button
              v-for="res in ['1024×1024', '2048×2048', '4096×4096', '128K']"
              :key="res"
              type="button"
              class="rounded bg-admin-surface-muted px-1.5 py-0.5 text-[10px] text-admin-text-2 hover:bg-admin-surface-muted/80 cursor-pointer"
              @click="emit('update:contextOrResolution', res)"
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
                hosting === 'Cloudflare 托管'
                  ? 'bg-white text-emerald-700 shadow-xs font-semibold'
                  : 'text-admin-text-2 hover:text-admin-text-1'
              "
              @click="emit('update:hosting', 'Cloudflare 托管')"
            >
              Cloudflare 托管
            </button>
            <button
              type="button"
              class="flex-1 rounded-md py-1 text-xs font-medium transition-colors cursor-pointer text-center"
              :class="
                hosting === '第三方'
                  ? 'bg-white text-admin-text-1 shadow-xs font-semibold'
                  : 'text-admin-text-2 hover:text-admin-text-1'
              "
              @click="emit('update:hosting', '第三方')"
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
          :model-value="capabilitiesDraft"
          placeholder="例如: 垫图生图, 透明通道, 提示词微调"
          @update:model-value="emit('update:capabilitiesDraft', String($event))"
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
</template>

<script setup lang="ts">
import AdminInput from '../../../../shared/components/AdminInput.vue'
import { splitCapabilities } from '../../../domain/modelForm'

const props = defineProps<{
  contextOrResolution: string
  hosting: string
  capabilitiesDraft: string
}>()

const emit = defineEmits<{
  'update:contextOrResolution': [value: string]
  'update:hosting': [value: string]
  'update:capabilitiesDraft': [value: string]
}>()

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

function addCapabilityTag(tag: string) {
  const current = splitCapabilities(props.capabilitiesDraft)
  if (!current.includes(tag)) {
    current.push(tag)
    emit('update:capabilitiesDraft', current.join(', '))
  }
}
</script>
