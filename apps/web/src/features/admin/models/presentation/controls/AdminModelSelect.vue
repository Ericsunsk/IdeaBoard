<template>
  <AdminPopover v-model:open="isOpen">
    <AdminPopoverTrigger as-child>
      <button
        type="button"
        :disabled="disabled"
        class="flex h-9 w-full min-w-0 items-center justify-between gap-2 rounded-lg border border-admin-border-soft bg-white px-2.5 text-xs text-admin-text-1 transition-colors hover:bg-admin-surface-muted focus-visible:border-admin-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20 disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer"
        :aria-label="ariaLabel || '选择模型'"
      >
        <div class="flex min-w-0 items-center gap-2">
          <!-- 当前选中的模型图标 -->
          <div
            v-if="currentModel"
            class="flex size-5 shrink-0 items-center justify-center rounded-md bg-admin-accent/10 text-admin-accent"
          >
            <Cube class="size-3" />
          </div>
          <Cube v-else class="size-4 shrink-0 text-admin-text-3" />

          <!-- 当前选中的模型名称 / 标识 -->
          <span v-if="currentModel" class="truncate font-semibold text-admin-text-1">
            {{ currentModel.name }}
            <span class="font-normal text-admin-text-3 font-mono text-[11px]"
              >({{ currentModel.id }})</span
            >
          </span>
          <span v-else-if="modelValue" class="truncate font-mono text-[11px] text-admin-text-1">
            {{ modelValue }}
          </span>
          <span v-else class="truncate text-admin-text-3">
            {{ placeholder || '选择配置的模型...' }}
          </span>
        </div>

        <NavArrowDown
          class="size-3.5 shrink-0 text-admin-text-3 transition-transform"
          :class="isOpen ? 'rotate-180' : ''"
        />
      </button>
    </AdminPopoverTrigger>

    <AdminPopoverContent align="start" :side-offset="4" class="w-[320px] max-w-[90vw] p-2">
      <!-- 搜索过滤栏 -->
      <div class="relative mb-2 flex items-center">
        <Search class="pointer-events-none absolute left-2.5 size-3.5 text-admin-text-3" />
        <input
          v-model="searchKeyword"
          type="text"
          :aria-label="allowCustom ? '搜索模型或输入自定义标识' : '搜索已入库模型'"
          :placeholder="allowCustom ? '搜索已入库模型或输入自定义标识...' : '搜索已入库模型...'"
          class="h-8 w-full rounded-md border border-admin-border-soft bg-white pr-2.5 pl-8 text-xs text-admin-text-1 placeholder:text-admin-text-3 focus-visible:border-admin-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-admin-accent/20"
        />
      </div>

      <!-- 可选模型列表 -->
      <div class="max-h-56 overflow-y-auto flex flex-col gap-0.5 app-scrollbar" role="listbox">
        <button
          v-for="model in filteredModels"
          :key="model.id"
          type="button"
          role="option"
          :aria-selected="model.id === modelValue"
          :class="[
            'flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition-colors cursor-pointer',
            model.id === modelValue
              ? 'bg-admin-accent/10 text-admin-accent font-semibold'
              : 'text-admin-text-2 hover:bg-admin-surface-muted hover:text-admin-text-1',
          ]"
          @click="selectModel(model.id)"
        >
          <div class="flex min-w-0 items-center gap-2">
            <div
              class="flex size-5 shrink-0 items-center justify-center rounded-md bg-admin-accent/10 text-admin-accent"
            >
              <Cube class="size-3" />
            </div>
            <div class="flex min-w-0 flex-col">
              <span class="truncate text-admin-title-color font-medium">{{ model.name }}</span>
              <span class="truncate font-mono text-[10px] text-admin-text-3"
                >{{ model.id }} · {{ model.provider }}</span
              >
            </div>
          </div>

          <div class="flex shrink-0 items-center gap-1.5">
            <span
              v-if="model.mode"
              class="rounded bg-admin-surface-muted px-1.5 py-0.5 text-[10px] text-admin-text-3"
            >
              {{ model.mode }}
            </span>
            <Check v-if="model.id === modelValue" class="size-3.5 text-admin-accent shrink-0" />
          </div>
        </button>

        <!-- 自定义模型选项 (如果输入了不在库中的自定义标识) -->
        <button
          v-if="showCustomOption"
          type="button"
          class="flex w-full items-center justify-between gap-2 rounded-lg border border-dashed border-admin-accent/40 bg-admin-accent/5 px-2 py-1.5 text-left text-xs text-admin-accent hover:bg-admin-accent/10 cursor-pointer"
          @click="selectModel(searchKeyword.trim())"
        >
          <div class="flex min-w-0 items-center gap-2">
            <Plus class="size-3.5 text-admin-accent shrink-0" />
            <span class="truncate"
              >使用自定义模型:
              <strong class="font-mono font-semibold">{{ searchKeyword.trim() }}</strong></span
            >
          </div>
        </button>

        <div
          v-if="filteredModels.length === 0 && !showCustomOption"
          class="py-4 text-center text-xs text-admin-text-3"
        >
          {{ allowCustom ? '未找到匹配的模型' : '未找到匹配的模型，请前往「模型管理」先入库' }}
        </div>
      </div>

      <!-- 底部跳转链接 -->
      <div class="mt-1.5 border-t border-admin-border-soft pt-1.5">
        <router-link
          :to="ADMIN_MODELS_PATH"
          class="flex w-full items-center justify-center gap-1 rounded-md py-1 text-[11px] font-medium text-admin-accent hover:bg-admin-accent/5 transition-colors"
          @click="isOpen = false"
        >
          <span>前往「模型管理」添加或刷新模型</span>
          <OpenNewWindow class="size-3" />
        </router-link>
      </div>
    </AdminPopoverContent>
  </AdminPopover>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Check, Cube, NavArrowDown, OpenNewWindow, Plus, Search } from '@iconoir/vue'
import AdminPopover from '../../../shared/components/AdminPopover.vue'
import AdminPopoverContent from '../../../shared/components/AdminPopoverContent.vue'
import AdminPopoverTrigger from '../../../shared/components/AdminPopoverTrigger.vue'
import { ADMIN_MODELS_PATH } from '../../../../../shared/constants/routePaths'
import type { AdminModelItem } from '../../domain/catalogData'
import { matchesModelKeyword } from '../../domain/catalogData'

const props = withDefaults(
  defineProps<{
    modelValue?: string | undefined
    models: AdminModelItem[]
    disabled?: boolean | undefined
    placeholder?: string | undefined
    ariaLabel?: string | undefined
    allowCustom?: boolean | undefined
  }>(),
  {
    modelValue: undefined,
    disabled: false,
    placeholder: undefined,
    ariaLabel: undefined,
    allowCustom: true,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  select: [modelId: string]
}>()

const isOpen = ref(false)
const searchKeyword = ref('')

const currentModel = computed(() => {
  if (!props.modelValue) return null
  return props.models.find((m) => m.id === props.modelValue) || null
})

const filteredModels = computed(() => {
  return props.models.filter((m) => matchesModelKeyword(m, searchKeyword.value))
})

const showCustomOption = computed(() => {
  if (!props.allowCustom) return false
  const kw = searchKeyword.value.trim()
  if (!kw) return false
  const exists = props.models.some((m) => m.id.toLowerCase() === kw.toLowerCase())
  return !exists
})

function selectModel(modelId: string) {
  emit('update:modelValue', modelId)
  emit('select', modelId)
  isOpen.value = false
  searchKeyword.value = ''
}
</script>
